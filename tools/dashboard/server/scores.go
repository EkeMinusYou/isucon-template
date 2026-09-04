package main

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type scoreEntry struct {
	RunID      string `json:"run_id"`
	Score      *int64 `json:"score"`
	App        string `json:"app"`
	Nginx      string `json:"nginx"`
	Mysql      string `json:"mysql"`
	AppTraffic string `json:"app_traffic"`
}

// parseScores reads runs/scores.tsv
// (header: run_id score app nginx mysql app_traffic).
// app_traffic was added later, so older rows may carry "unknown" or be absent.
func parseScores(path string) ([]scoreEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []scoreEntry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []scoreEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			continue // skip header
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 2 {
			continue
		}
		var score *int64
		if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(cols[1]), 10, 64); parseErr == nil {
			score = &parsed
		}
		e := scoreEntry{RunID: cols[0], Score: score}
		if len(cols) > 2 {
			e.App = cols[2]
		}
		if len(cols) > 3 {
			e.Nginx = cols[3]
		}
		if len(cols) > 4 {
			e.Mysql = cols[4]
		}
		if len(cols) > 5 {
			e.AppTraffic = cols[5]
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (a *app) handleScores(w http.ResponseWriter, r *http.Request) {
	entries, err := parseScores(filepath.Join(a.runsDir, "scores.tsv"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, entries)
}
