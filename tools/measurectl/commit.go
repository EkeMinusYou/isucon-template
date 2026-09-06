package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func commitRunArtifacts(runDir, scores string) error {
	git := func(args ...string) ([]byte, error) {
		args = append([]string{"--literal-pathspecs", "-c", "core.hooksPath=/dev/null"}, args...)
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			return out, fmt.Errorf("git %s: %w: %s", args[3], err, out)
		}
		return out, nil
	}
	rootOut, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	root := strings.TrimSpace(string(rootOut))
	// Restrict paths to this checkout and reject symlink traversal.
	validate := func(p string) (string, error) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("artifact outside repository: %s", p)
		}
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", err
		}
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", err
		}
		if resolved != filepath.Join(resolvedRoot, rel) {
			return "", fmt.Errorf("symlink artifact path: %s", p)
		}
		return abs, nil
	}
	runDir, err = validate(runDir)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(filepath.Join(runDir, "run.json"))
	if err != nil {
		return err
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return err
	}
	if m.Phase != "finalized" || m.RunID != filepath.Base(runDir) {
		return fmt.Errorf("RUN is not finalized or ID does not match: %s", runDir)
	}
	paths := []string{runDir}
	if scores != "" {
		p, err := validate(scores)
		if err != nil {
			return err
		}
		if p != filepath.Join(filepath.Dir(runDir), "scores.tsv") {
			return fmt.Errorf("scores must be adjacent to RUN directory")
		}
		paths = append(paths, p)
	}
	// Never overwrite the user's staging choices for these files.
	staged, err := git(append([]string{"diff", "--cached", "--name-only", "--"}, paths...)...)
	if err != nil {
		return err
	}
	if len(staged) != 0 {
		return fmt.Errorf("artifact paths already staged; commit manually: %s", staged)
	}
	// Scoped paths and --only exclude all unrelated staged changes.
	if _, err := git(append([]string{"add", "--"}, paths...)...); err != nil {
		return err
	}
	changed, err := git(append([]string{"diff", "--cached", "--name-only", "--"}, paths...)...)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		return nil
	}
	out, err := git(append([]string{"commit", "--only", "-m", "Record benchmark artifacts for " + m.RunID, "--"}, paths...)...)
	fmt.Print(string(out))
	return err
}
