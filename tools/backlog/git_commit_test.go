package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitBacklogDumpKeepsUnrelatedStaging(t *testing.T) {
	repo := t.TempDir()
	gitTestCommand(t, repo, "init", "-q")
	gitTestCommand(t, repo, "config", "user.name", "Test")
	gitTestCommand(t, repo, "config", "user.email", "test@example.com")
	gitTestCommand(t, repo, "config", "commit.gpgsign", "false")

	dumpPath := filepath.Join(repo, "tools", "backlog", "backlog.sql")
	writeTestFile(t, dumpPath, "initial\n")
	gitTestCommand(t, repo, "add", "--", "tools/backlog/backlog.sql")
	gitTestCommand(t, repo, "commit", "-qm", "Initial")

	writeTestFile(t, filepath.Join(repo, "notes.txt"), "keep staged\n")
	gitTestCommand(t, repo, "add", "--", "notes.txt")
	writeTestFile(t, dumpPath, "updated\n")

	if err := commitBacklogDump(repo, dumpPath); err != nil {
		t.Fatal(err)
	}

	if got := gitTestCommand(t, repo, "diff", "--cached", "--name-only"); got != "notes.txt\n" {
		t.Fatalf("staged files = %q, want notes.txt only", got)
	}
	if got := gitTestCommand(t, repo, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); got != "tools/backlog/backlog.sql\n" {
		t.Fatalf("committed files = %q, want backlog.sql only", got)
	}
}

func TestCommitBacklogDumpRejectsStagedDump(t *testing.T) {
	repo := t.TempDir()
	gitTestCommand(t, repo, "init", "-q")
	gitTestCommand(t, repo, "config", "user.name", "Test")
	gitTestCommand(t, repo, "config", "user.email", "test@example.com")
	gitTestCommand(t, repo, "config", "commit.gpgsign", "false")

	dumpPath := filepath.Join(repo, "tools", "backlog", "backlog.sql")
	writeTestFile(t, dumpPath, "initial\n")
	gitTestCommand(t, repo, "add", "--", "tools/backlog/backlog.sql")
	gitTestCommand(t, repo, "commit", "-qm", "Initial")

	writeTestFile(t, dumpPath, "staged\n")
	gitTestCommand(t, repo, "add", "--", "tools/backlog/backlog.sql")
	writeTestFile(t, dumpPath, "working tree\n")
	headBefore := gitTestCommand(t, repo, "rev-parse", "HEAD")

	err := commitBacklogDump(repo, dumpPath)
	if err == nil || !strings.Contains(err.Error(), "already staged") {
		t.Fatalf("error = %v, want already staged error", err)
	}
	if got := gitTestCommand(t, repo, "rev-parse", "HEAD"); got != headBefore {
		t.Fatalf("HEAD changed from %q to %q", headBefore, got)
	}
	if got := gitTestCommand(t, repo, "diff", "--cached", "--name-only"); got != "tools/backlog/backlog.sql\n" {
		t.Fatalf("staged files = %q, want backlog.sql only", got)
	}
}

func gitTestCommand(t *testing.T, repo string, args ...string) string {
	t.Helper()
	args = append([]string{"-C", repo}, args...)
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
