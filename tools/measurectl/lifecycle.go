package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type lifecycleOptions struct {
	action              string
	runID               string
	resultsDir          string
	rawDir              string
	runStateFile        string
	snapshotPath        string
	scoresPath          string
	score               string
	collectorConfig     string
	digesterConfig      string
	collectorFlags      string
	digesterFlags       string
	compareRun          string
	compareAllowedCards string
	compareAllowedRoles string
	sshUser             string
	sshOpts             string
	roles               map[string][]string
	vars                map[string]string
}

type lifecycleRunner struct {
	collect          func([]string) error
	digest           func([]string) error
	manifestBegin    func([]string) error
	manifestFinalize func([]string) error
}

func runLifecycle(args []string) error {
	if len(args) == 0 || (args[0] != "begin" && args[0] != "finalize") {
		return errors.New("run requires begin or finalize")
	}
	action := args[0]
	fs := flag.NewFlagSet("run "+action, flag.ContinueOnError)
	opts := lifecycleOptions{action: action}
	fs.StringVar(&opts.runID, "run-id", "", "RUN ID (required for begin)")
	fs.StringVar(&opts.resultsDir, "results", "runs", "RUN result directory")
	fs.StringVar(&opts.rawDir, "raw-dir", "raw", "raw artifact directory")
	fs.StringVar(&opts.runStateFile, "run-state-file", "raw/current-run-id", "active RUN marker")
	fs.StringVar(&opts.snapshotPath, "applied-snapshot", "", "before-bench APPLIED snapshot (required for begin)")
	fs.StringVar(&opts.scoresPath, "scores", "runs/scores.tsv", "derived score history TSV")
	fs.StringVar(&opts.score, "score", "", "benchmark score for finalize")
	fs.StringVar(&opts.collectorConfig, "collectors", defaultMeasureConfigPath("collectors.yaml"), "collector declaration")
	fs.StringVar(&opts.digesterConfig, "digesters", defaultMeasureConfigPath("digesters.yaml"), "digester declaration")
	fs.StringVar(&opts.collectorFlags, "collector-flags", "", "additional collect flags")
	fs.StringVar(&opts.digesterFlags, "digester-flags", "", "additional digest flags")
	fs.StringVar(&opts.compareRun, "compare-run", "", "declared control RUN ID")
	fs.StringVar(&opts.compareAllowedCards, "compare-allow-cards", "", "card IDs allowed to differ from control")
	fs.StringVar(&opts.compareAllowedRoles, "compare-allow-roles", "", "role fields allowed to differ from control")
	fs.StringVar(&opts.sshUser, "ssh-user", "ubuntu", "SSH user")
	fs.StringVar(&opts.sshOpts, "ssh-opts", "", "additional SSH options")
	roles := keyValues{}
	vars := keyValue{}
	fs.Var(roles, "role", "role=host1,host2 (repeatable)")
	fs.Var(vars, "var", "name=value (repeatable)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected run arguments: %s", strings.Join(fs.Args(), " "))
	}
	opts.roles = roles
	opts.vars = vars
	runner := lifecycleRunner{
		collect: runCollect, digest: runDigest,
		manifestBegin: runManifestBegin, manifestFinalize: runManifestFinalize,
	}
	if action == "begin" {
		return runner.begin(opts)
	}
	return runner.finalize(opts)
}

func (r lifecycleRunner) begin(opts lifecycleOptions) error {
	if opts.runID == "" || filepath.Base(opts.runID) != opts.runID {
		return fmt.Errorf("-run-id must be one basename: %q", opts.runID)
	}
	if opts.snapshotPath == "" {
		return errors.New("-applied-snapshot is required for run begin")
	}
	if _, err := os.Stat(opts.runStateFile); err == nil {
		return fmt.Errorf("unfinished RUN exists: %s", opts.runStateFile)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := r.collect(append([]string{"check-clean"}, opts.collectArgs(true)...)); err != nil {
		return err
	}
	runDir := filepath.Join(opts.resultsDir, opts.runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	manifestArgs, err := opts.manifestBeginArgs(runDir)
	if err != nil {
		return err
	}
	if err := r.manifestBegin(manifestArgs); err != nil {
		return err
	}
	if err := writeRunState(opts.runStateFile, opts.runID); err != nil {
		return err
	}
	cleanup := func(cause error) error {
		sweepErr := r.collect(append([]string{"sweep"}, opts.collectArgs(true)...))
		stateErr := archiveRunState(opts.runStateFile, "aborted-run-id")
		return errors.Join(cause, sweepErr, stateErr)
	}
	if err := r.collect(append([]string{"prepare"}, opts.collectArgs(true)...)); err != nil {
		return cleanup(err)
	}
	startArgs := []string{"start", "-run-id", opts.runID}
	startArgs = append(startArgs, opts.collectArgs(true)...)
	if err := r.collect(startArgs); err != nil {
		return cleanup(err)
	}
	return nil
}

func (r lifecycleRunner) finalize(opts lifecycleOptions) error {
	runID, err := readRunState(opts.runStateFile)
	if err != nil {
		return err
	}
	runDir := filepath.Join(opts.resultsDir, runID)
	stopArgs := []string{"stop", "-run-id", runID, "-run-dir", runDir}
	stopArgs = append(stopArgs, opts.collectArgs(true)...)
	if err := r.collect(stopArgs); err != nil {
		fmt.Fprintf(os.Stderr, "measurectl: collector stop failed; finalization continues: %v\n", err)
	}
	digestArgs := []string{"-config", opts.digesterConfig, "-run-dir", runDir, "-raw-dir", opts.rawDir}
	digestArgs = append(digestArgs, opts.remoteArgs()...)
	digestArgs = append(digestArgs, strings.Fields(opts.digesterFlags)...)
	if err := r.digest(digestArgs); err != nil {
		fmt.Fprintf(os.Stderr, "measurectl: digest failed; manifest will record missing artifacts: %v\n", err)
	}
	if err := r.manifestFinalize([]string{
		"-dir", runDir, "-scores", opts.scoresPath, "-score", opts.score,
		"-collectors", opts.collectorConfig, "-digesters", opts.digesterConfig,
	}); err != nil {
		return err
	}
	if err := archiveRunState(opts.runStateFile, "last-run-id"); err != nil {
		return err
	}
	fmt.Printf("RUN_ID=%s collection completed\n", runID)
	return nil
}

func (o lifecycleOptions) collectArgs(withConfiguredFlags bool) []string {
	args := []string{"-config", o.collectorConfig}
	args = append(args, o.remoteArgs()...)
	if withConfiguredFlags {
		args = append(args, strings.Fields(o.collectorFlags)...)
	}
	return args
}

func (o lifecycleOptions) remoteArgs() []string {
	args := []string{"-ssh-user", o.sshUser, "-ssh-opts", o.sshOpts}
	roleNames := make([]string, 0, len(o.roles))
	for name := range o.roles {
		roleNames = append(roleNames, name)
	}
	sort.Strings(roleNames)
	for _, name := range roleNames {
		args = append(args, "-role", name+"="+strings.Join(o.roles[name], ","))
	}
	varNames := make([]string, 0, len(o.vars))
	for name := range o.vars {
		varNames = append(varNames, name)
	}
	sort.Strings(varNames)
	for _, name := range varNames {
		args = append(args, "-var", name+"="+o.vars[name])
	}
	return args
}

func (o lifecycleOptions) manifestBeginArgs(runDir string) ([]string, error) {
	first := func(role string) (string, error) {
		hosts := o.roles[role]
		if len(hosts) == 0 {
			return "", fmt.Errorf("role %q has no hosts", role)
		}
		return hosts[0], nil
	}
	entry, err := first("entry")
	if err != nil {
		return nil, err
	}
	mysql, err := first("mysql")
	if err != nil {
		return nil, err
	}
	args := []string{
		"-dir", runDir, "-applied-snapshot", o.snapshotPath, "-collector-clean",
		"-compare-allow-cards", o.compareAllowedCards, "-compare-allow-roles", o.compareAllowedRoles,
		"-app", strings.Join(o.roles["app"], ","), "-app-traffic", strings.Join(o.roles["app_traffic"], ","),
		"-nginx", strings.Join(o.roles["nginx"], ","), "-entry", entry, "-mysql", mysql,
	}
	disabled := false
	for _, token := range strings.Fields(o.collectorFlags) {
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(token, "-"), "-"), "=")
		if name != "no-collectors" {
			continue
		}
		disabled = true
		if hasValue {
			var err error
			disabled, err = strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid no-collectors flag: %w", err)
			}
		}
	}
	if disabled {
		args = append(args, "-collectors-disabled")
	}
	if o.compareRun != "" {
		args = append(args, "-compare-run-dir", filepath.Join(o.resultsDir, o.compareRun))
	}
	var additionalNames []string
	for name := range o.roles {
		switch name {
		case "app", "app_traffic", "nginx", "entry", "mysql":
			continue
		}
		additionalNames = append(additionalNames, name)
	}
	sort.Strings(additionalNames)
	for _, name := range additionalNames {
		args = append(args, "-role", name+"="+strings.Join(o.roles[name], ","))
	}
	return args, nil
}

func writeRunState(path, runID string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".current-run-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := fmt.Fprintln(tmp, runID); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readRunState(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read active RUN state: %w", err)
	}
	runID := strings.TrimSpace(string(body))
	if runID == "" || filepath.Base(runID) != runID {
		return "", fmt.Errorf("invalid active RUN ID %q", runID)
	}
	return runID, nil
}

func archiveRunState(path, name string) error {
	if _, err := readRunState(path); err != nil {
		return err
	}
	destination := filepath.Join(filepath.Dir(path), name)
	return os.Rename(path, destination)
}
