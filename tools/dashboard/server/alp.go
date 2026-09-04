package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

type alpResponse struct {
	RunID     string                   `json:"run_id"`
	Available bool                     `json:"available"`
	Rows      []map[string]interface{} `json:"rows"`
}

// parseAlpJSON reads alp.json produced by `alp --format json`, which is an
// array-of-arrays: the first row is the column header, the rest are values
// in the same column order. It converts that into a list of row objects
// sorted by "sum" (total response time) descending.
func parseAlpJSON(path string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var raw [][]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return []map[string]interface{}{}, nil
	}

	headers := make([]string, len(raw[0]))
	for i, h := range raw[0] {
		headers[i], _ = h.(string)
	}

	rows := make([]map[string]interface{}, 0, len(raw)-1)
	for _, r := range raw[1:] {
		row := make(map[string]interface{}, len(headers))
		for i, h := range headers {
			if i < len(r) {
				row[h] = r[i]
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		si, _ := rows[i]["sum"].(float64)
		sj, _ := rows[j]["sum"].(float64)
		return si > sj
	})
	return rows, nil
}

func (a *app) handleAlp(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}
	rows, err := parseAlpJSON(filepath.Join(dir, "alp.json"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	available := rows != nil
	if rows == nil {
		rows = make([]map[string]interface{}, 0)
	}
	writeJSON(w, alpResponse{RunID: runID, Available: available, Rows: rows})
}
