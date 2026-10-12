package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// smoke indexes real setup artifacts without creating a RUN, score or manifest.
// It reuses the declared source schemas and profile normalizer, and leaves
// benchmark-specific quality/score semantics to a real finalized RUN.
func (r runner) smoke(dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(filepath.Base(dir), "setup-smoke-") {
		return fmt.Errorf("expected a setup-smoke directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "run.json")); !os.IsNotExist(err) {
		return fmt.Errorf("smoke input must not have a RUN manifest")
	}
	active, err := r.activeSources([]string{dir})
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "isucon-smoke-analysis-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	profiles := filepath.Join(temp, "profiles")
	if err := os.Mkdir(profiles, 0700); err != nil {
		return err
	}
	if len(r.config.Profiles.Tables) > 0 {
		if err := r.prepareProfiles(profiles, []string{dir}); err != nil {
			return err
		}
	}
	schema, err := r.buildSchema(active)
	if err != nil {
		return err
	}
	text := string(schema)
	start := strings.Index(text, "create or replace view runs as")
	if start < 0 {
		return fmt.Errorf("base schema has no runs view")
	}
	terminator := strings.Index(text[start:], ";\n")
	if terminator < 0 {
		return fmt.Errorf("base schema has no runs view")
	}
	end := start + terminator + 2
	text = text[:start] + `create or replace view runs as select
 ''::varchar as run_id, null::bigint as score, null::timestamp as started_at,
 []::varchar[] as app_hosts, []::varchar[] as nginx_hosts,
 ''::varchar as mysql_host, []::varchar[] as app_traffic_hosts where false;
` + text[end:]
	text = strings.ReplaceAll(text, "'runs/([^/]+)/'", "'/(setup-smoke-[^/]+)/'")
	var sql strings.Builder
	sql.WriteString(text)
	seen := map[string]bool{}
	for _, source := range active {
		if seen[source.Table] {
			fmt.Fprintf(&sql, "insert into smoke_%s select * from %s;\n", source.Table, source.View)
		} else {
			fmt.Fprintf(&sql, "create table smoke_%s as select * from %s;\n", source.Table, source.View)
			// Name collisions occur when a raw schema view shares its table name.
			seen[source.Table] = true
		}
	}
	// Keep a separate namespace for all sources and their shared table groups.
	script := sql.String()
	for _, table := range r.config.Profiles.Tables {
		script += fmt.Sprintf("create table smoke_%s as select * from %s_raw;\n", table, table)
		script += fmt.Sprintf("drop view %s_raw;\n", table)
	}
	db := filepath.Join(dir, "analysis.duckdb")
	if err := os.Remove(db); err != nil && !os.IsNotExist(err) {
		return err
	}
	env := append(os.Environ(), "ISUCON_RUN_GLOB="+dir, "ISUCON_PROFILE_DIR="+profiles)
	if err := r.duckdbCommand(env, db, "-c", script); err != nil {
		return err
	}
	fmt.Fprintf(r.stdout, "Smoke analysis: %s (no RUN or score registered)\n", db)
	return nil
}
