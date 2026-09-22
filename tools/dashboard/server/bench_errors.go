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

type benchErrorCount struct {
	ElapsedS   float64 `json:"elapsed_s"`
	ErrorCount int64   `json:"error_count"`
}

// readBenchErrors consumes the shared semantic view without parsing bench.log.
func (a *app) readBenchErrors(ctx context.Context, runID string) ([]string, bool, []int64, []benchErrorCount, error) {
	errors := []string{}
	if a.analysisDB == "" {
		return errors, false, nil, []benchErrorCount{}, nil
	}
	if _, err := os.Stat(a.analysisDB); os.IsNotExist(err) {
		return errors, false, nil, []benchErrorCount{}, nil
	} else if err != nil {
		return nil, false, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	query := `select exists(select 1 from bench_error_logs where run_id = '` + strings.ReplaceAll(runID, "'", "''") + `') as available,
 coalesce((select list(message order by line_number) from bench_errors where run_id = '` + strings.ReplaceAll(runID, "'", "''") + `'), []::varchar[]) as errors,
 coalesce((select list(cast(floor(epoch(occurred_at)) as bigint)) from bench_warning_events where run_id = '` + strings.ReplaceAll(runID, "'", "''") + `'), []::bigint[]) as warning_events,
 coalesce((select list(struct_pack(elapsed_s := elapsed_s, error_count := error_count) order by line_number) from bench_error_counts where run_id = '` + strings.ReplaceAll(runID, "'", "''") + `'), []) as error_counts`
	cmd := exec.CommandContext(ctx, "duckdb", "-readonly", "-json", a.analysisDB, "-c", query)
	output, err := cmd.Output()
	if err != nil {
		return nil, false, nil, nil, fmt.Errorf("read bench_errors semantic view (run task q to refresh analysis DB): %w", err)
	}
	var rows []struct {
		ErrorCounts   []benchErrorCount `json:"error_counts"`
		Available     bool              `json:"available"`
		Errors        []string          `json:"errors"`
		WarningEvents []int64           `json:"warning_events"`
	}
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, false, nil, nil, err
	}
	if len(rows) != 1 {
		return nil, false, nil, nil, fmt.Errorf("unexpected bench_errors result: %s", output)
	}
	return rows[0].Errors, rows[0].Available, rows[0].WarningEvents, rows[0].ErrorCounts, nil
}
