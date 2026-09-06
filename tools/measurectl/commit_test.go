package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
