package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/EkeMinusYou/isucon-template/tools/analysisctl/internal/pprofimport"
	"gopkg.in/yaml.v3"
)

type sourceConfig struct {
	Table  string `yaml:"table"`
	Schema string `yaml:"schema"`
	Match  string `yaml:"match"`
	View   string `yaml:"view"`
}

type profileConfig struct {
	Match  string   `yaml:"match"`
	Schema string   `yaml:"schema"`
	Tables []string `yaml:"tables"`
}

type reportConfig struct {
	BottleneckEvidence string `yaml:"bottleneck_evidence"`
	BottleneckProfile  string `yaml:"bottleneck_profile"`
}

type config struct {
	Version         int            `yaml:"version"`
	BaseSchema      string         `yaml:"base_schema"`
	SemanticSchemas []string       `yaml:"semantic_schemas"`
	Reports         reportConfig   `yaml:"reports"`
	Profiles        profileConfig  `yaml:"profiles"`
	Sources         []sourceConfig `yaml:"sources"`
	baseDir         string
}

type options struct {
	db         string
	results    string
	configPath string
	duckdb     string
	stdout     io.Writer
	stderr     io.Writer
}

type runner struct {
	options
	config config
}

type runManifest struct {
	Phase string `json:"phase"`
}

var (
	runIDPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}$`)
	hostPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

const importSchemaVersion = "4"

func main() {
	if err := runCLI(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCLI(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: analysisctl <sync|rebuild|status|evidence|profile> [flags] [arguments]")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	opts := options{stdout: stdout, stderr: stderr}
	flags.StringVar(&opts.db, "db", "runs/analysis.duckdb", "DuckDB index path")
	flags.StringVar(&opts.results, "results", "runs", "RUN artifact directory")
	flags.StringVar(&opts.configPath, "config", "tools/analysis/sources.yaml", "source declaration path")
	flags.StringVar(&opts.duckdb, "duckdb", "duckdb", "DuckDB CLI path")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	arguments := flags.Args()
	cfg, err := loadConfig(opts.configPath)
	if err != nil {
		return err
	}
	r := runner{options: opts, config: cfg}
	switch command {
	case "sync":
		if len(arguments) != 0 {
			return fmt.Errorf("unexpected arguments: %s", strings.Join(arguments, " "))
		}
		return r.withLock(r.sync)
	case "rebuild":
		if len(arguments) != 0 {
			return fmt.Errorf("unexpected arguments: %s", strings.Join(arguments, " "))
		}
		return r.withLock(r.rebuild)
	case "status":
		if len(arguments) != 0 {
			return fmt.Errorf("unexpected arguments: %s", strings.Join(arguments, " "))
		}
		return r.status()
	case "evidence":
		if len(arguments) != 1 {
			return errors.New("usage: analysisctl evidence [flags] RUN_ID")
		}
		return r.evidence(arguments[0])
	case "profile":
		if len(arguments) != 3 {
			return errors.New("usage: analysisctl profile [flags] RUN_ID HOST SYMBOL_PATTERN")
		}
		return r.profile(arguments[0], arguments[1], arguments[2])
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func loadConfig(path string) (config, error) {
	f, err := os.Open(path)
	if err != nil {
		return config{}, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()
	var cfg config
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return config{}, fmt.Errorf("decode config: %w", err)
	}
	cfg.baseDir = filepath.Dir(path)
	if cfg.Version != 1 {
		return config{}, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if cfg.BaseSchema == "" || len(cfg.Sources) == 0 {
		return config{}, errors.New("base_schema and sources are required")
	}
	for i, source := range cfg.Sources {
		if source.Table == "" || source.Schema == "" || source.Match == "" || source.View == "" {
			return config{}, fmt.Errorf("sources[%d] requires table, schema, match, and view", i)
		}
	}
	if (cfg.Profiles.Schema == "") != (len(cfg.Profiles.Tables) == 0) {
		return config{}, errors.New("profiles.schema and profiles.tables must be specified together")
	}
	if cfg.Reports.BottleneckEvidence == "" || cfg.Reports.BottleneckProfile == "" {
		return config{}, errors.New("reports.bottleneck_evidence and reports.bottleneck_profile are required")
	}
	return cfg, nil
}

func (c config) path(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.baseDir, path)
}

func (r runner) withLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(r.db), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(r.db+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock index: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

func (r runner) sync() error {
	runs, err := r.discoverRuns()
	if err != nil {
		return err
	}
	if _, err := os.Stat(r.db); errors.Is(err, os.ErrNotExist) {
		return r.rebuildRuns(runs)
	} else if err != nil {
		return err
	}
	dirs := make([]string, 0, len(runs))
	for _, runID := range runs {
		dirs = append(dirs, filepath.Join(r.results, runID))
	}
	active, err := r.activeSources(dirs)
	if err != nil {
		return err
	}
	schemaCurrent, err := r.importSchemaCurrent()
	if err != nil {
		return err
	}
	if !schemaCurrent {
		fmt.Fprintf(r.stdout, "%s の取り込みschema versionが古いため再構築します\n", r.db)
		return r.rebuildRuns(runs)
	}
	missing, err := r.missingTables(active)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		fmt.Fprintf(r.stdout, "%s の取り込みschemaが古いため再構築します: %s\n", r.db, strings.Join(missing, " "))
		return r.rebuildRuns(runs)
	}
	indexed, err := r.indexedRuns()
	if err != nil {
		return err
	}
	added := 0
	for _, runID := range runs {
		if indexed[runID] {
			continue
		}
		dir := filepath.Join(r.results, runID)
		if err := r.syncSelection(dir, []string{dir}, []string{runID}); err != nil {
			return fmt.Errorf("import %s: %w", runID, err)
		}
		fmt.Fprintf(r.stdout, "%s を %s へ取り込みました\n", runID, r.db)
		added++
	}
	if err := r.applySemanticSchemas(); err != nil {
		return err
	}
	if added == 0 {
		fmt.Fprintf(r.stdout, "%s は最新です\n", r.db)
	}
	return nil
}

func (r runner) rebuild() error {
	runs, err := r.discoverRuns()
	if err != nil {
		return err
	}
	return r.rebuildRuns(runs)
}

func (r runner) rebuildRuns(runs []string) error {
	if err := os.MkdirAll(filepath.Dir(r.db), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(r.db), ".analysis-rebuild-*.duckdb")
	if err != nil {
		return fmt.Errorf("create rebuild database: %w", err)
	}
	tempDB := temp.Name()
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Remove(tempDB); err != nil {
		return err
	}
	defer os.Remove(tempDB)
	buildRunner := r
	buildRunner.db = tempDB

	fmt.Fprintf(r.stdout, "%s を全 RUN から構築します (履歴量に応じて数分かかります)\n", r.db)
	dirs := make([]string, 0, len(runs))
	for _, runID := range runs {
		dirs = append(dirs, filepath.Join(r.results, runID))
	}
	if err := buildRunner.syncSelection(filepath.Join(r.results, "*"), dirs, runs); err != nil {
		return err
	}
	if err := buildRunner.applySemanticSchemas(); err != nil {
		return err
	}
	if err := os.Rename(tempDB, r.db); err != nil {
		return fmt.Errorf("replace index: %w", err)
	}
	fmt.Fprintf(r.stdout, "%s 構築完了\n", r.db)
	return nil
}

func (r runner) status() error {
	runs, err := r.discoverRuns()
	if err != nil {
		return err
	}
	if _, err := os.Stat(r.db); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(r.stdout, "database: missing\nruns: %d\nindexed: 0\npending: %d\n", len(runs), len(runs))
		return nil
	} else if err != nil {
		return err
	}
	indexed, err := r.indexedRuns()
	if err != nil {
		return err
	}
	pending := 0
	for _, runID := range runs {
		if !indexed[runID] {
			pending++
		}
	}
	fmt.Fprintf(r.stdout, "database: %s\nruns: %d\nindexed: %d\npending: %d\n", r.db, len(runs), len(indexed), pending)
	return nil
}

func (r runner) discoverRuns() ([]string, error) {
	entries, err := os.ReadDir(r.results)
	if err != nil {
		return nil, fmt.Errorf("read results: %w", err)
	}
	var runs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(r.results, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "run.json"))
		if err == nil {
			var manifest runManifest
			if json.Unmarshal(data, &manifest) == nil && manifest.Phase == "started" {
				continue
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s/run.json: %w", entry.Name(), err)
		}
		runs = append(runs, entry.Name())
	}
	sort.Strings(runs)
	return runs, nil
}

func (r runner) activeSources(dirs []string) ([]sourceConfig, error) {
	var active []sourceConfig
	for _, source := range r.config.Sources {
		found := false
		for _, dir := range dirs {
			matches, err := filepath.Glob(filepath.Join(dir, source.Match))
			if err != nil {
				return nil, fmt.Errorf("invalid source match %q: %w", source.Match, err)
			}
			if len(matches) > 0 {
				found = true
				break
			}
		}
		if found {
			active = append(active, source)
		}
	}
	return active, nil
}

func (r runner) syncSelection(runGlob string, dirs, runIDs []string) error {
	active, err := r.activeSources(dirs)
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "isucon-analysis-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	profileDir := filepath.Join(tempDir, "profiles")
	if err := os.Mkdir(profileDir, 0o755); err != nil {
		return err
	}
	if len(r.config.Profiles.Tables) > 0 {
		if err := r.prepareProfiles(profileDir, dirs); err != nil {
			return err
		}
	}

	schema, err := r.buildSchema(active)
	if err != nil {
		return err
	}
	sql := r.buildImportSQL(active, runIDs)
	schemaPath := filepath.Join(tempDir, "schema.sql")
	sqlPath := filepath.Join(tempDir, "import.sql")
	if err := os.WriteFile(schemaPath, schema, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(sqlPath, []byte(sql), 0o600); err != nil {
		return err
	}
	env := append(os.Environ(), "ISUCON_RUN_GLOB="+runGlob, "ISUCON_PROFILE_DIR="+profileDir)
	return r.duckdbCommand(env, "-init", schemaPath, "-c", ".read "+sqlPath)
}

func (r runner) prepareProfiles(output string, dirs []string) error {
	var paths []string
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, r.config.Profiles.Match))
		if err != nil {
			return err
		}
		sort.Strings(matches)
		paths = append(paths, matches...)
	}
	if err := pprofimport.Run(output, paths, r.stderr); err != nil {
		return fmt.Errorf("normalize profiles: %w", err)
	}
	return nil
}

func (r runner) buildSchema(active []sourceConfig) ([]byte, error) {
	var out bytes.Buffer
	paths := []string{r.config.path(r.config.BaseSchema)}
	seen := map[string]bool{}
	for _, source := range active {
		path := r.config.path(source.Schema)
		if !seen[path] {
			paths = append(paths, path)
			seen[path] = true
		}
	}
	if r.config.Profiles.Schema != "" {
		paths = append(paths, r.config.path(r.config.Profiles.Schema))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", path, err)
		}
		out.Write(data)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func (r runner) buildImportSQL(active []sourceConfig, runIDs []string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "attach %s as db;\n", sqlString(r.db))
	out.WriteString("begin transaction;\n")
	out.WriteString("create or replace table db.runs as select * from runs;\n")
	out.WriteString("create table if not exists db.analysis_metadata (key varchar primary key, value varchar not null);\n")
	fmt.Fprintf(&out, "insert or replace into db.analysis_metadata values ('import_schema_version', %s);\n", sqlString(importSchemaVersion))
	out.WriteString("create table if not exists db.ingested (run_id varchar primary key);\n")
	for _, source := range active {
		fmt.Fprintf(&out, "create table if not exists db.%s as select * from %s limit 0;\n", source.Table, source.View)
		fmt.Fprintf(&out, "insert into db.%s select * from %s;\n", source.Table, source.View)
	}
	for _, table := range r.config.Profiles.Tables {
		fmt.Fprintf(&out, "create table if not exists db.%s as select * from %s_raw limit 0;\n", table, table)
		fmt.Fprintf(&out, "insert into db.%s select * from %s_raw;\n", table, table)
	}
	for _, runID := range runIDs {
		fmt.Fprintf(&out, "insert or ignore into db.ingested values (%s);\n", sqlString(runID))
	}
	out.WriteString("commit;\n")
	return out.String()
}

func (r runner) applySemanticSchemas() error {
	for _, path := range r.config.SemanticSchemas {
		data, err := os.ReadFile(r.config.path(path))
		if err != nil {
			return fmt.Errorf("read semantic schema: %w", err)
		}
		if err := r.duckdbCommand(nil, r.db, "-c", string(data)); err != nil {
			return fmt.Errorf("apply %s: %w", path, err)
		}
	}
	return nil
}

func (r runner) evidence(runID string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("invalid RUN ID %q; expected YYYYMMDD-HHMMSS", runID)
	}
	if err := r.requireDatabase(); err != nil {
		return err
	}
	found, err := r.queryCount("select count(*) from manifests where run_id = " + sqlString(runID))
	if err != nil {
		return err
	}
	if found != 1 {
		return fmt.Errorf("RUN not found in analysis DB: %s", runID)
	}
	return r.runReport(r.config.Reports.BottleneckEvidence, map[string]string{"run_id": runID})
}

func (r runner) profile(runID, host, pattern string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("invalid RUN ID %q; expected YYYYMMDD-HHMMSS", runID)
	}
	if !hostPattern.MatchString(host) {
		return fmt.Errorf("invalid host: %s", host)
	}
	if pattern == "" {
		return errors.New("symbol pattern is required")
	}
	if err := r.requireDatabase(); err != nil {
		return err
	}
	query := "select count(*) from profile_metadata where run_id = " + sqlString(runID) + " and host = " + sqlString(host)
	found, err := r.queryCount(query)
	if err != nil {
		return err
	}
	if found != 1 {
		return fmt.Errorf("fgprof profile not found in analysis DB: %s / %s", runID, host)
	}
	fmt.Fprintf(r.stdout, "== FOCUSED TOP: %s / %s ==\n", host, pattern)
	return r.runReport(r.config.Reports.BottleneckProfile, map[string]string{
		"host_name":      host,
		"run_id":         runID,
		"symbol_pattern": pattern,
	})
}

func (r runner) requireDatabase() error {
	if _, err := os.Stat(r.db); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("analysis DB not found: %s", r.db)
	} else if err != nil {
		return err
	}
	return nil
}

func (r runner) queryCount(query string) (int, error) {
	output, err := r.duckdbOutput(nil, "-noheader", "-list", r.db, "-c", query)
	if err != nil {
		return 0, err
	}
	var count int
	if _, err := fmt.Sscan(strings.TrimSpace(output), &count); err != nil {
		return 0, fmt.Errorf("parse DuckDB count %q: %w", strings.TrimSpace(output), err)
	}
	return count, nil
}

func (r runner) runReport(path string, variables map[string]string) error {
	data, err := os.ReadFile(r.config.path(path))
	if err != nil {
		return fmt.Errorf("read report %s: %w", path, err)
	}
	var script strings.Builder
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&script, "set variable %s = %s;\n", key, sqlString(variables[key]))
	}
	script.Write(data)
	cmd := exec.Command(r.duckdb, r.db)
	cmd.Stdin = strings.NewReader(script.String())
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run report %s: %w", path, err)
	}
	return nil
}

func (r runner) indexedRuns() (map[string]bool, error) {
	output, err := r.duckdbOutput(nil, "-noheader", "-list", r.db, "-c", "select run_id from ingested order by run_id")
	if err != nil {
		return nil, err
	}
	indexed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line != "" {
			indexed[line] = true
		}
	}
	return indexed, nil
}

func (r runner) missingTables(active []sourceConfig) ([]string, error) {
	wanted := map[string]bool{}
	for _, source := range active {
		wanted[source.Table] = true
	}
	for _, table := range r.config.Profiles.Tables {
		wanted[table] = true
	}
	var tables []string
	for table := range wanted {
		tables = append(tables, table)
	}
	if len(tables) == 0 {
		return nil, nil
	}
	sort.Strings(tables)
	query := "select table_name from information_schema.tables where table_name in ("
	quoted := make([]string, len(tables))
	for i, table := range tables {
		quoted[i] = sqlString(table)
	}
	query += strings.Join(quoted, ",") + ")"
	output, err := r.duckdbOutput(nil, "-noheader", "-list", r.db, "-c", query)
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		present[line] = true
	}
	var missing []string
	for _, table := range tables {
		if !present[table] {
			missing = append(missing, table)
		}
	}
	return missing, nil
}

func (r runner) importSchemaCurrent() (bool, error) {
	tableCount, err := r.queryCount("select count(*) from information_schema.tables where table_name = 'analysis_metadata'")
	if err != nil {
		return false, err
	}
	if tableCount == 0 {
		return false, nil
	}
	output, err := r.duckdbOutput(nil, "-noheader", "-list", r.db, "-c", "select value from analysis_metadata where key = 'import_schema_version'")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == importSchemaVersion, nil
}

func (r runner) duckdbCommand(env []string, args ...string) error {
	_, err := r.duckdbOutput(env, args...)
	return err
}

func (r runner) duckdbOutput(env []string, args ...string) (string, error) {
	cmd := exec.Command(r.duckdb, args...)
	if env != nil {
		cmd.Env = env
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("duckdb %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
