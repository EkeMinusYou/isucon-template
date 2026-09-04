package main

import (
	"bufio"
	"fmt"
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

const scoresHeader = "run_id\tscore\tapp\tnginx\tmysql\tapp_traffic"

// parseScores reads the current runs/scores.tsv format.
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
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("scores.tsv is missing its header")
	}
	if scanner.Text() != scoresHeader {
		return nil, fmt.Errorf("scores.tsv has an unsupported header %q", scanner.Text())
	}
	lineNumber := 1
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 6 {
			return nil, fmt.Errorf("scores.tsv line %d has %d columns, expected 6", lineNumber, len(cols))
		}
		var score *int64
		if rawScore := strings.TrimSpace(cols[1]); rawScore != "" {
			parsed, parseErr := strconv.ParseInt(rawScore, 10, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("scores.tsv line %d has invalid score %q: %w", lineNumber, cols[1], parseErr)
			}
			score = &parsed
		}
		if strings.TrimSpace(cols[0]) == "" {
			return nil, fmt.Errorf("scores.tsv line %d has an empty run_id", lineNumber)
		}
		entries = append(entries, scoreEntry{
			RunID: cols[0], Score: score, App: cols[2], Nginx: cols[3], Mysql: cols[4], AppTraffic: cols[5],
		})
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
