package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDigestRemotePerHostExpandsLoadWindowAndKeepsHostOutputs(t *testing.T) {
	dir := t.TempDir()
	var (
		mu      sync.Mutex
		scripts = map[string]string{}
	)
	runner := &digestRunner{
		runDir: dir,
		roles:  map[string][]string{"app": {"isucon-1", "isucon-2"}},
		vars:   map[string]string{"app_service": "isucon-app"},
		loadWindow: LoadWindow{
			Status:    "ok",
			StartedAt: "2026-09-04T03:00:00Z",
			EndedAt:   "2026-09-04T03:01:00Z",
		},
		sshScriptFn: func(host, script string) (string, string, error) {
			mu.Lock()
			scripts[host] = script
			mu.Unlock()
			return host + " journal\n", "", nil
		},
	}
	digester := Digester{
		Name: "app-journal",
		Remote: &Remote{
			Role:    "app",
			PerHost: true,
			Script:  "unit={var:app_service} host={host} since={load_start} until={load_end}",
		},
		Outputs: []Output{{File: "{host}-app-journal.log"}},
	}
	if err := runner.digestRemote(digester); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"isucon-1", "isucon-2"} {
		body, err := os.ReadFile(filepath.Join(dir, host+"-app-journal.log"))
		if err != nil || string(body) != host+" journal\n" {
			t.Fatalf("%s output=%q err=%v", host, body, err)
		}
		script := scripts[host]
		for _, want := range []string{"unit=isucon-app", "host=" + host, "since=2026-09-04T03:00:00Z", "until=2026-09-04T03:01:00Z"} {
			if !strings.Contains(script, want) {
				t.Fatalf("%s script does not contain %q: %s", host, want, script)
			}
		}
	}
}

func TestDigestRemoteLoadWindowPlaceholderRequiresValidWindow(t *testing.T) {
	runner := &digestRunner{roles: map[string][]string{"all": {"isucon-1"}}}
	err := runner.digestRemote(Digester{
		Name:    "kernel",
		Remote:  &Remote{Role: "all", Script: "journalctl --since {load_start}"},
		Outputs: []Output{{File: "kernel.log"}},
	})
	if err == nil || !strings.Contains(err.Error(), "requires a valid benchmark load window") {
		t.Fatalf("invalid load window error = %v", err)
	}
}

func TestDigestRemoteRejectsMissingVariable(t *testing.T) {
	runner := &digestRunner{
		roles:      map[string][]string{"app": {"isucon-1"}},
		loadWindow: LoadWindow{Status: "ok", StartedAt: "2026-09-04T03:00:00Z", EndedAt: "2026-09-04T03:01:00Z"},
	}
	err := runner.digestRemote(Digester{
		Name:    "journal",
		Remote:  &Remote{Role: "app", Script: "journalctl -u {var:app_service}"},
		Outputs: []Output{{File: "journal.log"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved placeholders") {
		t.Fatalf("missing remote variable error = %v", err)
	}
}

func TestDigestRemoteFailureKeepsFallbackOutput(t *testing.T) {
	dir := t.TempDir()
	runner := &digestRunner{
		runDir: dir,
		roles:  map[string][]string{"mysql": {"isucon-1"}},
		sshScriptFn: func(_, _ string) (string, string, error) {
			return "", "performance_schema disabled", errors.New("exit status 1")
		},
	}
	digester := Digester{
		Name:    "mysql-digest",
		Stderr:  "mysql-digest.stderr",
		Remote:  &Remote{Role: "mysql", Script: "exit 1"},
		Outputs: []Output{{File: "mysql-digest.tsv", OnSkip: "column_a\tcolumn_b"}},
	}
	if err := runner.digestRemote(digester); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "mysql-digest.tsv"))
	if err != nil || string(body) != "column_a\tcolumn_b\n" {
		t.Fatalf("fallback output=%q err=%v", body, err)
	}
	stderr, err := os.ReadFile(filepath.Join(dir, "mysql-digest.stderr"))
	if err != nil || !strings.Contains(string(stderr), "performance_schema disabled") {
		t.Fatalf("stderr=%q err=%v", stderr, err)
	}
}

func TestExecCancelsBlockedInputProducerWhenConsumerFails(t *testing.T) {
	if _, err := exec.LookPath("yes"); err != nil {
		t.Skip("yes is not available")
	}
	if _, err := exec.LookPath("head"); err != nil {
		t.Skip("head is not available")
	}
	dir := t.TempDir()
	runner := &digestRunner{runDir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := time.Now()
	err := runner.exec(ctx, Digester{Stdin: "yes"}, []string{"head", "-c", "1"}, []string{"input"}, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("failing consumer returned nil")
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("blocked producer was not cancelled promptly: %v", elapsed)
	}
}

func TestExecWaitsForSuccessfulInputProducerBeforeCleanupCancel(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat is not available")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	runner := &digestRunner{runDir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Close stdout before the producer process exits. The consumer can finish
	// successfully during the delay, reproducing the cleanup cancellation race.
	err := runner.exec(ctx, Digester{Stdin: "sh"}, []string{"cat"}, []string{"-c", "printf complete; exec 1>&-; sleep 0.1"}, out)
	if err != nil {
		t.Fatalf("complete producer was reported as failed: %v", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "complete" {
		t.Fatalf("output = %q, want complete", body)
	}
}

func TestFetchAtomicRenameAndStaleRawRemoval(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "mysql-slow.log")
	if err := os.WriteFile(local, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &digestRunner{
		runDir: dir,
		rawDir: dir,
		fetchFn: func(_, _, target string) error {
			return os.WriteFile(target, []byte("fresh"), 0o600)
		},
	}
	if err := r.fetchSource(Source{Remote: "/slow.log", Local: local}, "mysql"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fresh" {
		t.Fatalf("fetched body = %q, want fresh", body)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".mysql-slow.log.fetch-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestFetchFailureDoesNotReuseStaleRawAndMarksDigestArtifacts(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "mysql-slow.log")
	if err := os.WriteFile(local, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &digestRunner{
		runDir: dir,
		rawDir: dir,
		cfg: &DigestConfig{Digesters: []Digester{{
			Name: "slp", Source: "slow", Stderr: "slp.stderr",
			Outputs: []Output{{File: "slp.tsv"}},
		}}},
		fetchFn: func(_, _, _ string) error { return errors.New("ENOSPC") },
	}
	err := r.fetchSource(Source{Name: "slow", Remote: "/slow.log", Local: local}, "mysql")
	if err == nil || !strings.Contains(err.Error(), "ENOSPC") {
		t.Fatalf("fetch error = %v, want ENOSPC", err)
	}
	r.markSourceFetchFailure(Source{Name: "slow"}, "mysql", err)
	if _, statErr := os.Stat(local); !os.IsNotExist(statErr) {
		t.Fatalf("stale raw still exists: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "slp.tsv")); statErr != nil {
		t.Fatalf("failed artifact missing: %v", statErr)
	}
	stderr, readErr := os.ReadFile(filepath.Join(dir, "slp.stderr"))
	if readErr != nil || !strings.Contains(string(stderr), "ENOSPC") {
		t.Fatalf("stderr = %q, err = %v", stderr, readErr)
	}
}
