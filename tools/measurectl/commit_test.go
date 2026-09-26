package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGitWithRetry(t *testing.T) {
	t.Run("succeeds after transient failures", func(t *testing.T) {
		attempts := 0
		out, err := runGitWithRetry(func() ([]byte, error) {
			attempts++
			if attempts < 3 {
				return []byte("temporary failure"), errors.New("temporary failure")
			}
			return []byte("success"), nil
		}, "commit")
		if err != nil {
			t.Fatal(err)
		}
		if attempts != 3 {
			t.Fatalf("attempts = %d, want 3", attempts)
		}
		if string(out) != "success" {
			t.Fatalf("output = %q, want success", out)
		}
	})

	t.Run("returns the final failure after all retries", func(t *testing.T) {
		attempts := 0
		out, err := runGitWithRetry(func() ([]byte, error) {
			attempts++
			return []byte("failure"), errors.New("temporary failure")
		}, "commit")
		if err == nil {
			t.Fatal("expected error")
		}
		if attempts != gitCommandRetryCount+1 {
			t.Fatalf("attempts = %d, want %d", attempts, gitCommandRetryCount+1)
		}
		if !strings.Contains(err.Error(), "git commit") || !strings.Contains(err.Error(), "failure") {
			t.Fatalf("error = %v", err)
		}
		if string(out) != "failure" {
			t.Fatalf("output = %q, want failure", out)
		}
	})
}

func TestCommitWorkingTreeAndManifestSource(t *testing.T) {
	tmp := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	git("config", "commit.gpgsign", "false")
	git("checkout", "-qb", "benchmark-work")
	write(".gitignore", "ignored.txt\n")
	write("staged.txt", "original\n")
	write("mixed.txt", "original\n")
	git("add", ".")
	git("commit", "-qm", "Initial")
	write("staged.txt", "staged\n")
	git("add", "staged.txt")
	write("mixed.txt", "staged\n")
	git("add", "mixed.txt")
	write("mixed.txt", "final\n")
	write("new.txt", "untracked\n")
	write("ignored.txt", "ignored\n")
	if err := commitWorkingTree("20260901-120000"); err != nil {
		t.Fatal(err)
	}
	commit := git("rev-parse", "HEAD")
	if got := git("show", "HEAD:mixed.txt"); got != "final" {
		t.Fatalf("committed mixed.txt = %q", got)
	}
	if got := git("show", "--pretty=format:", "--name-only", "HEAD"); got != "mixed.txt\nnew.txt\nstaged.txt" {
		t.Fatalf("committed files = %q", got)
	}
	if got := git("status", "--porcelain=v1", "--untracked-files=all"); got != "" {
		t.Fatalf("working tree is dirty: %q", got)
	}
	if err := commitWorkingTree("20260901-120001"); err != nil {
		t.Fatal(err)
	}
	if got := git("rev-parse", "HEAD"); got != commit {
		t.Fatalf("clean tree created a commit: %s -> %s", commit, got)
	}
	runDir := "20260901-120000"
	if err := os.Mkdir(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runManifestBegin([]string{"-dir", runDir}); err != nil {
		t.Fatal(err)
	}
	manifest := readTestManifest(t, filepath.Join(runDir, "run.json"))
	if manifest.Source.Commit != commit || manifest.Source.Branch != "benchmark-work" {
		t.Fatalf("source = %#v", manifest.Source)
	}
}

func TestCommitRunArtifactsIsolation(t *testing.T) {
	for _, stagedArtifact := range []bool{false, true} {
		t.Run(map[bool]string{false: "isolated", true: "reject_staged_artifact"}[stagedArtifact], func(t *testing.T) {
			tmp := t.TempDir()
			old, _ := os.Getwd()
			if err := os.Chdir(tmp); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(old) })
			git := func(args ...string) string {
				t.Helper()
				out, err := exec.Command("git", args...).CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
				return string(out)
			}
			write := func(p, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0644); err != nil {
					t.Fatal(err)
				}
			}
			git("init", "-q")
			git("config", "user.name", "Test")
			git("config", "user.email", "test@example.com")
			git("config", "commit.gpgsign", "false")
			write("app.go", "original")
			write(".gitignore", "runs/*/raw/\n")
			git("add", ".")
			git("commit", "-qm", "Initial")
			write("app.go", "staged")
			git("add", "app.go")
			write("app.go", "unstaged")
			run := "runs/20260906-120000"
			write(run+"/run.json", `{"phase":"finalized","run_id":"20260906-120000"}`)
			write(run+"/bench.log", "BENCHMARK_FAIL")
			write(run+"/alp.tsv", "result")
			write(run+"/notes.txt", "unrelated")
			write(run+"/raw/access.log", "raw")
			write("runs/other/run.json", "other run")
			write("runs/scores.tsv", "score")
			if stagedArtifact {
				git("add", run+"/alp.tsv")
			}
			before := git("diff", "--cached")
			head := git("rev-parse", "HEAD")
			err := commitRunArtifacts(run, "runs/scores.tsv")
			if stagedArtifact {
				if err == nil || !strings.Contains(err.Error(), "already staged") {
					t.Fatalf("expected staged conflict, got %v", err)
				}
				if git("rev-parse", "HEAD") != head {
					t.Fatal("HEAD changed")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got := git("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
				want := run + "/alp.tsv\n" + run + "/bench.log\n" + run + "/notes.txt\n" + run + "/run.json\nruns/scores.tsv\n"
				if got != want {
					t.Fatalf("committed files:\n%s\nwant:\n%s", got, want)
				}
				if err := commitRunArtifacts(run, "runs/scores.tsv"); err != nil {
					t.Fatal(err)
				}
			}
			if git("diff", "--cached") != before {
				t.Fatal("existing staging changed")
			}
			body, _ := os.ReadFile("app.go")
			if string(body) != "unstaged" {
				t.Fatal("working tree changed")
			}
		})
	}
}
