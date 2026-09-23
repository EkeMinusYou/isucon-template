package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListRunsIncludesArchivedRunsAndSkipsContainerDirectories(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	activeID := "20260829-120000"
	archivedID := "20260828-120000"
	activeDir := filepath.Join(runsDir, activeID)
	archivedDir := filepath.Join(runsDir, "archive", archivedID)
	for _, dir := range []string{activeDir, archivedDir, filepath.Join(runsDir, "archive", activeID), filepath.Join(runsDir, "not-a-run")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(activeDir, "alp.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeDir, "isucon-1-go-cpu.pprof"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeDir, "run.json"), []byte(`{"score":0,"passed":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archivedDir, "slp.tsv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	a := &app{runsDir: runsDir}
	runs, err := a.listRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2: %#v", len(runs), runs)
	}
	if runs[0].RunID != activeID || !runs[0].HasAlp || !runs[0].HasPprof {
		t.Fatalf("active run = %#v, want %s with alp and pprof", runs[0], activeID)
	}
	if runs[0].Score == nil || *runs[0].Score != 0 || runs[0].Passed == nil || *runs[0].Passed {
		t.Fatalf("active benchmark result = %#v, want real zero score and fail", runs[0])
	}
	if runs[1].RunID != archivedID || !runs[1].HasSlow {
		t.Fatalf("archived run = %#v, want %s with slow query", runs[1], archivedID)
	}

	if gotDir, ok := a.resolveRunDir(activeID); !ok || gotDir != activeDir {
		t.Fatalf("active RUN not preferred: %q, %v", gotDir, ok)
	}
	gotDir, ok := a.resolveRunDir(archivedID)
	if !ok || gotDir != archivedDir {
		t.Fatalf("resolveRunDir(%q) = %q, %v; want %q, true", archivedID, gotDir, ok, archivedDir)
	}
	if _, ok := a.resolveRunDir("archive"); ok {
		t.Fatal("resolveRunDir(archive) succeeded, want false")
	}
}
