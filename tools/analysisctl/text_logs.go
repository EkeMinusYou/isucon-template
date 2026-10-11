package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// DuckDB read_text requires UTF-8. Stage only malformed benchmark logs and
// link other artifacts, retaining RUN IDs in paths and the original evidence.
// Zero-byte artifacts are left out of the stage: activeSources already treats
// them as absent, and read_csv would otherwise infer a column0 from them and
// change a view's columns.
func (r runner) prepareBenchmarkLogs(tempDir, runGlob string, dirs []string) (string, error) {
	invalid := make(map[string][]byte)
	empty := false
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if isEmptyArtifact(entry) {
				empty = true
			}
		}
		body, err := os.ReadFile(filepath.Join(dir, "bench.log"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !utf8.Valid(body) {
			invalid[dir] = bytes.ToValidUTF8(body, []byte("\ufffd"))
		}
	}
	if len(invalid) == 0 && !empty {
		return runGlob, nil
	}
	root := filepath.Join(tempDir, "runs")
	for _, dir := range dirs {
		stage := filepath.Join(root, filepath.Base(dir))
		if err := os.MkdirAll(stage, 0o700); err != nil {
			return "", err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if isEmptyArtifact(entry) {
				continue
			}
			target := filepath.Join(stage, entry.Name())
			if body, ok := invalid[dir]; ok && entry.Name() == "bench.log" {
				if err := os.WriteFile(target, body, 0o600); err != nil {
					return "", err
				}
				fmt.Fprintf(r.stderr, "warning: replaced invalid UTF-8 in %s for analysis; original unchanged\n", filepath.Join(dir, entry.Name()))
				continue
			}
			source, err := filepath.Abs(filepath.Join(dir, entry.Name()))
			if err != nil {
				return "", err
			}
			if err := os.Symlink(source, target); err != nil {
				return "", err
			}
		}
	}
	return filepath.Join(root, "*"), nil
}

func isEmptyArtifact(entry os.DirEntry) bool {
	if !entry.Type().IsRegular() {
		return false
	}
	info, err := entry.Info()
	return err == nil && info.Size() == 0
}
