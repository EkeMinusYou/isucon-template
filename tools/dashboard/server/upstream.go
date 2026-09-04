package main

import (
	"net/http"
	"os"
	"path/filepath"
)

// upstreamRow mirrors one row of `upstream-breakdown.tsv` — a single
// snapshot (not a time series) of nginx access-log requests grouped by
// upstream address, upstream status, and cache status for the whole RUN.
type upstreamRow struct {
	UpstreamAddr      string  `json:"upstream_addr"`
	UpstreamStatus    string  `json:"upstream_status"`
	CacheStatus       string  `json:"cache_status"`
	Requests          int64   `json:"requests"`
	Status2xx         int64   `json:"status_2xx"`
	Status3xx         int64   `json:"status_3xx"`
	Status4xx         int64   `json:"status_4xx"`
	Status5xx         int64   `json:"status_5xx"`
	StatusOther       int64   `json:"status_other"`
	ResponseTimeSumMs float64 `json:"response_time_sum_ms"`
	ResponseTimeAvgMs float64 `json:"response_time_avg_ms"`
	UpstreamTimeSumMs float64 `json:"upstream_time_sum_ms"`
	UpstreamTimeAvgMs float64 `json:"upstream_time_avg_ms"`
}

type upstreamResponse struct {
	RunID     string        `json:"run_id"`
	Available bool          `json:"available"`
	Rows      []upstreamRow `json:"rows"`
}

func parseUpstreamRows(path string) ([]upstreamRow, error) {
	header, rows, err := readTSV(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if columnIndex(header, "upstream_addr") < 0 {
		return nil, nil
	}

	out := make([]upstreamRow, 0, len(rows))
	for _, row := range rows {
		c := &colIndexer{header: header, row: row}
		addrIdx := columnIndex(header, "upstream_addr")
		statusIdx := columnIndex(header, "upstream_status")
		cacheIdx := columnIndex(header, "cache_status")
		get := func(idx int) string {
			if idx < 0 || idx >= len(row) {
				return ""
			}
			return row[idx]
		}
		out = append(out, upstreamRow{
			UpstreamAddr:      get(addrIdx),
			UpstreamStatus:    get(statusIdx),
			CacheStatus:       get(cacheIdx),
			Requests:          c.int64("requests"),
			Status2xx:         c.int64("status_2xx"),
			Status3xx:         c.int64("status_3xx"),
			Status4xx:         c.int64("status_4xx"),
			Status5xx:         c.int64("status_5xx"),
			StatusOther:       c.int64("status_other"),
			ResponseTimeSumMs: c.float("response_time_sum_ms"),
			ResponseTimeAvgMs: c.float("response_time_avg_ms"),
			UpstreamTimeSumMs: c.float("upstream_time_sum_ms"),
			UpstreamTimeAvgMs: c.float("upstream_time_avg_ms"),
		})
	}
	return out, nil
}

func (a *app) handleUpstream(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}
	rows, err := parseUpstreamRows(filepath.Join(dir, "upstream-breakdown.tsv"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	available := rows != nil
	if rows == nil {
		rows = make([]upstreamRow, 0)
	}
	writeJSON(w, upstreamResponse{RunID: runID, Available: available, Rows: rows})
}
