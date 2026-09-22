package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	backlogCommitMessage = "chore(backlog): update backlog ledger"
	backlogGitRetryCount = 3
	backlogGitRetryDelay = 100 * time.Millisecond
)

// commitBacklogDump commits only the canonical tracked backlog dump. A custom
// dump path and a directory outside a Git worktree remain usable for tests and
// standalone CLI use without triggering an unrelated commit.
func commitBacklogDump(root, dumpPath string) error {
	if dumpPath == "" || dumpPath == ":memory:" {
		return nil
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve backlog repository root: %w", err)
	}
	dumpAbs, err := filepath.Abs(dumpPath)
	if err != nil {
		return fmt.Errorf("resolve backlog dump path: %w", err)
	}

	repoRootOutput, err := exec.Command("git", "-C", rootAbs, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		// The CLI is also used with temporary databases outside a repository.
		return nil
	}
	repoRoot, err := filepath.Abs(strings.TrimSpace(string(repoRootOutput)))
	if err != nil {
		return fmt.Errorf("resolve Git repository root: %w", err)
	}
	repoRoot, err = filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve Git repository root symlinks: %w", err)
	}
	dumpAbs, err = filepath.EvalSymlinks(dumpAbs)
	if err != nil {
		return fmt.Errorf("resolve backlog dump symlinks: %w", err)
	}
	relativeDump, err := filepath.Rel(repoRoot, dumpAbs)
	if err != nil || relativeDump == ".." || strings.HasPrefix(relativeDump, ".."+string(filepath.Separator)) {
		return nil
	}
	if filepath.Clean(relativeDump) != filepath.Join("tools", "backlog", "backlog.sql") {
		return nil
	}

	git := func(args ...string) ([]byte, error) {
		command := args[0]
		gitArgs := []string{"-C", repoRoot, "--literal-pathspecs", "-c", "core.hooksPath=/dev/null"}
		gitArgs = append(gitArgs, args...)
		var output []byte
		var commandErr error
		for attempt := 0; attempt <= backlogGitRetryCount; attempt++ {
			output, commandErr = exec.Command("git", gitArgs...).CombinedOutput()
			if commandErr == nil {
				return output, nil
			}
			if attempt < backlogGitRetryCount {
				time.Sleep(backlogGitRetryDelay)
			}
		}
		return output, fmt.Errorf("git %s: %w: %s", command, commandErr, strings.TrimSpace(string(output)))
	}

	staged, err := git("diff", "--cached", "--name-only", "--", relativeDump)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(staged)) != "" {
		return fmt.Errorf("backlog dump is already staged; commit it manually: %s", strings.TrimSpace(string(staged)))
	}
	if _, err := git("add", "--", relativeDump); err != nil {
		return err
	}
	changed, err := git("diff", "--cached", "--name-only", "--", relativeDump)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(changed)) == "" {
		return nil
	}
	output, err := git("commit", "--only", "-m", backlogCommitMessage, "--", relativeDump)
	if len(output) > 0 {
		fmt.Fprint(os.Stderr, string(output))
	}
	return err
}
