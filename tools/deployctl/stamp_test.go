package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalFingerprintTracksSentContentOnly(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string, mode os.FileMode) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main", 0o644)
	write("tls/key.pem", "secret", 0o600)
	write("cpu.pprof", "profile", 0o644)
	excludes := []string{"tls/", "*.pprof"}
	fingerprint := func() string {
		t.Helper()
		v, err := localFingerprint(dir+"/", excludes)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	base := fingerprint()
	write("tls/key.pem", "rotated", 0o600)
	write("cpu.pprof", "another profile", 0o644)
	if got := fingerprint(); got != base {
		t.Fatal("excluded files changed the fingerprint")
	}
	write("main.go", "package main // edited", 0o644)
	edited := fingerprint()
	if edited == base {
		t.Fatal("content change kept the fingerprint")
	}
	write("main.go", "package main // edited", 0o755)
	if fingerprint() == edited {
		t.Fatal("permission change kept the fingerprint")
	}
}

type stampExecutor struct {
	stamps string
	stdins []string
}

func (e *stampExecutor) Run(name string, _ []string, stdin string) ([]byte, error) {
	e.stdins = append(e.stdins, stdin)
	if strings.HasPrefix(stdin, "cat ") {
		return []byte(e.stamps), nil
	}
	return nil, nil
}

func TestUploadLeavesStampOfSentContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	exec := &stampExecutor{}
	r := &deployRunner{sshUser: "isucon", exec: exec}
	j := uploadJob{u: upload{Label: "app"}, host: "isucon-2", local: dir + "/", remote: "/home/isucon/app/"}
	if res := r.runUpload(j); res.err != nil {
		t.Fatal(res.err)
	}
	want, _ := localFingerprint(dir+"/", nil)
	last := exec.stdins[len(exec.stdins)-1]
	if !strings.Contains(last, stampDir) || !strings.Contains(last, want) {
		t.Fatalf("stamp was not written with the local fingerprint: %q", exec.stdins)
	}
}

func TestVerifyRejectsContentFromAnotherWorktree(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	local := dir + "/"
	cfg := &config{Deployments: map[string]deployment{
		"app": {Uploads: []upload{{Label: "app", Role: "app", Local: local, Remote: "/home/isucon/app/"}}},
	}}
	stamp := func(fingerprint, worktree string) string {
		b, _ := json.Marshal(deployStamp{Remote: "/home/isucon/app/", Local: local, Fingerprint: fingerprint, Worktree: worktree, Commit: "0123456789abcdef"})
		return string(b) + "\n"
	}
	current, _ := localFingerprint(local, nil)
	runner := func(stamps string) *deployRunner {
		return &deployRunner{sshUser: "isucon", parallel: 1, roles: map[string][]string{"app": {"isucon-2"}}, exec: &stampExecutor{stamps: stamps}}
	}

	if err := runner(stamp(current, "/elsewhere")).verify(cfg); err != nil {
		t.Fatalf("matching content must pass regardless of the sender: %v", err)
	}
	if err := runner("").verify(cfg); err != nil {
		t.Fatalf("unstamped destinations must be skipped: %v", err)
	}
	if err := runner(stamp("stale", "/other/worktree")).verify(cfg); err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("stale content must fail verification, got %v", err)
	}
}
