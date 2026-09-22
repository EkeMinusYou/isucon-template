package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// analysisWindow is the benchmark-run window persisted by analysisctl. It is
// kept as wall-clock time because collector-local elapsed_ms starts before the
// benchmark traffic and cannot be used to identify the window by itself.
type analysisWindow struct {
	StartedAt time.Time
	EndedAt   time.Time
}

func (w analysisWindow) contains(t time.Time) bool {
	return !t.Before(w.StartedAt) && t.Before(w.EndedAt)
}

// readAnalysisWindow reads the shared analysisctl result. A missing database,
// missing table, or run without a usable window is expected for historical
// runs created before traffic-window detection was introduced; nil then means
// that callers should preserve their previous all-samples behavior.
func (a *app) readAnalysisWindow(ctx context.Context, runID string) (*analysisWindow, error) {
	if a.analysisDB == "" {
		return nil, nil
	}
	if _, err := os.Stat(a.analysisDB); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tableQuery := `select count(*) > 0 as available
from information_schema.tables
where table_schema = 'main' and table_name = 'analysis_windows'`
	output, err := exec.CommandContext(ctx, "duckdb", "-readonly", "-json", a.analysisDB, "-c", tableQuery).Output()
	if err != nil {
		return nil, fmt.Errorf("check analysis_windows table: %w", err)
	}
	var tables []struct {
		Available bool `json:"available"`
	}
	if err := json.Unmarshal(output, &tables); err != nil {
		return nil, fmt.Errorf("decode analysis_windows table check: %w", err)
	}
	if len(tables) != 1 || !tables[0].Available {
		return nil, nil
	}

	query := `select epoch_us(started_at) as started_at_us,
 epoch_us(ended_at) as ended_at_us
from analysis_windows
where run_id = '` + strings.ReplaceAll(runID, "'", "''") + `'
  and status = 'ok'
  and started_at is not null
  and ended_at is not null`
	output, err = exec.CommandContext(ctx, "duckdb", "-readonly", "-json", a.analysisDB, "-c", query).Output()
	if err != nil {
		return nil, fmt.Errorf("read analysis window: %w", err)
	}
	var rows []struct {
		StartedAtUs int64 `json:"started_at_us"`
		EndedAtUs   int64 `json:"ended_at_us"`
	}
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, fmt.Errorf("decode analysis window: %w", err)
	}
	if len(rows) != 1 {
		return nil, nil
	}

	window := &analysisWindow{
		StartedAt: time.UnixMicro(rows[0].StartedAtUs),
		EndedAt:   time.UnixMicro(rows[0].EndedAtUs),
	}
	if !window.StartedAt.Before(window.EndedAt) {
		return nil, nil
	}
	return window, nil
}
