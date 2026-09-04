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
	for _, dir := range []string{activeDir, archivedDir, filepath.Join(runsDir, "not-a-run")} {
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
	if runs[1].RunID != archivedID || !runs[1].HasSlow {
		t.Fatalf("archived run = %#v, want %s with slow query", runs[1], archivedID)
	}

	gotDir, ok := a.resolveRunDir(archivedID)
	if !ok || gotDir != archivedDir {
		t.Fatalf("resolveRunDir(%q) = %q, %v; want %q, true", archivedID, gotDir, ok, archivedDir)
	}
	if _, ok := a.resolveRunDir("archive"); ok {
		t.Fatal("resolveRunDir(archive) succeeded, want false")
	}
}

func TestResolveRunDirPrefersActiveRun(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	runID := "20260829-120000"
	activeDir := filepath.Join(runsDir, runID)
	for _, dir := range []string{activeDir, filepath.Join(runsDir, "archive", runID)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	a := &app{runsDir: runsDir}
	gotDir, ok := a.resolveRunDir(runID)
	if !ok || gotDir != activeDir {
		t.Fatalf("resolveRunDir(%q) = %q, %v; want %q, true", runID, gotDir, ok, activeDir)
	}
}
