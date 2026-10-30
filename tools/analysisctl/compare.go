package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func (r runner) compareRuns(base, current string) error {
	if r.config.Reports.CompareRuns == "" {
		return fmt.Errorf("reports.compare_runs is not configured")
	}
	for _, id := range []string{base, current} {
		if !runIDPattern.MatchString(id) {
			return fmt.Errorf("invalid RUN ID %q; expected YYYYMMDD-HHMMSS", id)
		}
		body, err := os.ReadFile(filepath.Join(r.results, id, "run.json"))
		if err != nil {
			return fmt.Errorf("read RUN %s: %w", id, err)
		}
		var manifest runManifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			return fmt.Errorf("decode RUN %s: %w", id, err)
		}
		if manifest.Phase != "finalized" {
			return fmt.Errorf("RUN %s is not finalized", id)
		}
	}
	if err := r.requireDatabase(); err != nil {
		return err
	}
	for _, id := range []string{base, current} {
		count, err := r.queryCount("select count(*) from manifests m join ingested i using (run_id) where m.run_id = " + sqlString(id))
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("RUN %s is not indexed; run task q-sync", id)
		}
	}
	if r.checkOnly {
		return nil
	}
	fmt.Fprintf(r.stdout, "== RUN comparison: %s -> %s ==\n", base, current)
	variables := map[string]string{"base_run": base, "new_run": current}
	if err := r.runReport(r.config.Reports.CompareRuns, variables); err != nil {
		return err
	}
	if r.config.Reports.RouteQuality != "" {
		if err := r.routeQuality(base, current); err != nil {
			return err
		}
	}
	if path := r.config.Reports.CompareContest; path != "" {
		fmt.Fprintln(r.stdout, "== Contest aggregates ==")
		return r.runReport(path, variables)
	}
	return nil
}

func (r runner) routeQuality(base, current string) error {
	if r.config.Reports.RouteQuality == "" {
		return fmt.Errorf("reports.route_quality is not configured")
	}
	fmt.Fprintln(r.stdout, "== API classification and count reconciliation ==")
	count, err := r.queryCount("select count(*) from route_coverage where run_id in (" + sqlString(base) + "," + sqlString(current) + ")")
	if err != nil {
		return err
	}
	if count == 0 {
		fmt.Fprintln(r.stdout, "route coverage unavailable; no observed counts to assess")
		return nil
	}
	return r.runReport(r.config.Reports.RouteQuality, map[string]string{"base_run": base, "new_run": current})
}
