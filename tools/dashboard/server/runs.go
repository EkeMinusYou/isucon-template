package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type runInfo struct {
	RunID      string `json:"run_id"`
	HasAlp     bool   `json:"has_alp"`
	HasSlow    bool   `json:"has_slowquery"`
	HasMetrics bool   `json:"has_metrics"`
	HasFgprof  bool   `json:"has_fgprof"`
	HasPprof   bool   `json:"has_pprof"`
}

type runDir struct {
	ID   string
	Path string
}

const runIDLayout = "20060102-150405"

func isRunID(name string) bool {
	_, err := time.Parse(runIDLayout, name)
	return err == nil
}

// listRunDirs returns RUN directories from both the active and archived
// locations. Active directories take precedence if the same RUN_ID exists in
// both locations.
func listRunDirs(runsDir string) ([]runDir, error) {
	byID := make(map[string]runDir)
	if err := collectRunDirs(runsDir, byID, false); err != nil {
		return nil, err
	}
	if err := collectRunDirs(filepath.Join(runsDir, "archive"), byID, true); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	runs := make([]runDir, 0, len(byID))
	for _, run := range byID {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	return runs, nil
}

func collectRunDirs(parent string, byID map[string]runDir, onlyMissing bool) error {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || !isRunID(e.Name()) {
			continue
		}
		if _, exists := byID[e.Name()]; onlyMissing && exists {
			continue
		}
		byID[e.Name()] = runDir{ID: e.Name(), Path: filepath.Join(parent, e.Name())}
	}
	return nil
}

func (a *app) listRuns() ([]runInfo, error) {
	dirs, err := listRunDirs(a.runsDir)
	if err != nil {
		return nil, err
	}
	runs := make([]runInfo, 0, len(dirs))
	for _, run := range dirs {
		runs = append(runs, runInfo{
			RunID:      run.ID,
			HasAlp:     fileExists(filepath.Join(run.Path, "alp.json")),
			HasSlow:    fileExists(filepath.Join(run.Path, "slp.tsv")),
			HasMetrics: len(mustGlob(filepath.Join(run.Path, "*-proc-metrics.tsv"))) > 0,
			HasFgprof:  len(mustGlob(filepath.Join(run.Path, "*-fgprof.pprof"))) > 0,
			HasPprof:   hasGoPprofFiles(run.Path),
		})
	}
	// newest first for UI convenience (run selector defaults to latest)
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID > runs[j].RunID })
	return runs, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func mustGlob(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}

func (a *app) handleRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := a.listRuns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, runs)
}

// resolveRunDir validates run_id against the active and archived RUN directory
// listings (rejecting path traversal) and returns its directory.
func (a *app) resolveRunDir(runID string) (string, bool) {
	dirs, err := listRunDirs(a.runsDir)
	if err != nil {
		return "", false
	}
	for _, run := range dirs {
		if run.ID == runID {
			return run.Path, true
		}
	}
	return "", false
}
