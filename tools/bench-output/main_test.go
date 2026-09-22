package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The core carries no log format, so the generic tests declare their own.
const genericDeclaration = `{"score": "^score:\\s*(\\d+)$", "pass": "^result: ok$", "fail": "^result: ng$"}`

func writeDeclaration(t *testing.T, declaration string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "patterns.json")
	if err := os.WriteFile(path, []byte(declaration), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A different contest replaces only the declaration and keeps the same markers.
func TestMarkersFollowDeclaration(t *testing.T) {
	declared, err := loadPatterns(writeDeclaration(t, genericDeclaration))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	w := &resultWriter{out: &output, patterns: declared}
	if _, err := w.Write([]byte("score: 480\n[PASSED]: false\nresult: ok\n")); err != nil {
		t.Fatal(err)
	}
	if got := string(w.markers()); got != "SCORE: 480\nBENCHMARK_PASS\n" {
		t.Fatalf("markers = %q", got)
	}
}

func TestLoadPatternsRejectsUnusableDeclarations(t *testing.T) {
	for _, tt := range []struct{ name, declaration string }{
		{"score without capture", `{"score": "^score: \\d+$"}`},
		{"invalid expression", `{"pass": "^(unclosed$"}`},
		{"no pattern", `{}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := loadPatterns(writeDeclaration(t, tt.declaration)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCommandExitAndOutput(t *testing.T) {
	var output, errors bytes.Buffer
	patterns := writeDeclaration(t, genericDeclaration)
	code := run([]string{"-patterns", patterns, "sh", "-c", "printf 'result: ng\\n' >&2; exit 7"}, &output, &errors)
	if code != 7 || output.String() != "result: ng\n\nBENCHMARK_FAIL\n" {
		t.Fatalf("code=%d output=%q errors=%q", code, output.String(), errors.String())
	}
}

func TestCommandKilled(t *testing.T) {
	var output, errors bytes.Buffer
	patterns := writeDeclaration(t, genericDeclaration)
	code := run([]string{"-patterns", patterns, "sh", "-c", "printf partial; kill -TERM $$"}, &output, &errors)
	if code != 143 || output.String() != "partial" {
		t.Fatalf("code=%d output=%q", code, output.String())
	}
}

func TestMissingDeclarationFailsBeforeRunning(t *testing.T) {
	var output, errors bytes.Buffer
	code := run([]string{"-patterns", filepath.Join(t.TempDir(), "absent.json"), "sh", "-c", "echo ran"}, &output, &errors)
	if code != 2 || output.String() != "" {
		t.Fatalf("code=%d output=%q", code, output.String())
	}
}

func TestMarkerAcrossWritesStreamsImmediately(t *testing.T) {
	declared, err := loadPatterns(writeDeclaration(t, genericDeclaration))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	w := &resultWriter{out: &output, patterns: declared}
	var raw string
	for _, chunk := range []string{"resu", "lt: o", "k\n"} {
		raw += chunk
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
		if output.String() != raw {
			t.Fatal("raw output was buffered or changed")
		}
	}
	if got := string(w.markers()); got != "BENCHMARK_PASS\n" {
		t.Fatalf("markers = %q", got)
	}
}
