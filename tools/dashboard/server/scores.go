package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type routePoints struct {
	Method string `json:"method"`
	Route  string `json:"route"`
	Points int64  `json:"points"`
}

type benchResult struct {
	RunID     string        `json:"run_id"`
	Score     *int64        `json:"score"`
	Passed    *bool         `json:"passed"`
	Addition  *int64        `json:"addition"`
	Deduction *int64        `json:"deduction"`
	Routes    []routePoints `json:"routes"`
}

type scoreEntry struct {
	RunID      string        `json:"run_id"`
	Score      *int64        `json:"score"`
	Passed     *bool         `json:"passed"`
	Addition   *int64        `json:"addition"`
	Deduction  *int64        `json:"deduction"`
	Routes     []routePoints `json:"routes"`
	App        string        `json:"app"`
	Nginx      string        `json:"nginx"`
	Mysql      string        `json:"mysql"`
	AppTraffic string        `json:"app_traffic"`
}

// Saved manifests are immutable. The semantic view can recover results that
// predate the benchmark output pattern declaration.
func (a *app) readBenchResults() (map[string]benchResult, error) {
	results := make(map[string]benchResult)
	if a.analysisDB == "" {
		return results, nil
	}
	if _, err := os.Stat(a.analysisDB); os.IsNotExist(err) {
		return results, nil
	} else if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	query := `select r.run_id, r.score, r.passed, r.addition, r.deduction,
coalesce((select list(struct_pack(method := p.method, route := p.route, points := p.points)
    order by p.method, p.route) from bench_score_routes p where p.run_id = r.run_id), []) as routes
from bench_results r`
	output, err := exec.CommandContext(ctx, "duckdb", "-readonly", "-json", a.analysisDB, "-c", query).Output()
	if err != nil {
		return nil, fmt.Errorf("read benchmark result view (run task q-sync to refresh analysis DB): %w", err)
	}
	var rows []benchResult
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		results[row.RunID] = row
	}
	return results, nil
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
	results, err := a.readBenchResults()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range entries {
		if result, ok := results[entries[i].RunID]; ok {
			if entries[i].Score == nil {
				entries[i].Score = result.Score
			}
			entries[i].Passed = result.Passed
			entries[i].Addition = result.Addition
			entries[i].Deduction = result.Deduction
			entries[i].Routes = result.Routes
		}
	}
	writeJSON(w, entries)
}
