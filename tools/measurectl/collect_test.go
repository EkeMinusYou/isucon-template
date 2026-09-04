package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func boolPointer(value bool) *bool {
	return &value
}

func TestSweepScriptTerminatesVerifiedCollectorProcessGroup(t *testing.T) {
	script := sweepScript([]string{"'/tmp/collector-root'"})
	for _, want := range []string{
		`grep -qF "$d"`,
		`sudo kill -TERM -- "-$pgid"`,
		`sudo kill -KILL -- "-$pgid"`,
		`[ "$pgid" != "$shell_pgid" ]`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("sweep script does not contain %q:\n%s", want, script)
		}
	}
}

func TestCheckCleanScriptFindsProcessesAndWorkDirectories(t *testing.T) {
	script := checkCleanScript([]string{"'/tmp/collector-root'"})
	for _, want := range []string{
		`/proc/[0-9]*/cmdline`,
		`grep -qF "$root/"`,
		`"$root"/*/`,
		`collector residue detected`,
		`exit 1`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("clean check script does not contain %q:\n%s", want, script)
		}
	}
}

func TestCheckCleanScriptRejectsWorkDirectory(t *testing.T) {
	root := t.TempDir()
	script := checkCleanScript([]string{shq(root)})
	if out, err := exec.Command("sh", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("clean root rejected: %v: %s", err, out)
	}

	if err := os.Mkdir(filepath.Join(root, "old-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err == nil {
		t.Fatalf("residual work directory accepted: %s", out)
	}
	if !strings.Contains(string(out), "collector residue detected") {
		t.Fatalf("unexpected clean check failure: %v: %s", err, out)
	}
}

func TestAllHostsAreDeduplicatedAndIndependentOfCollectorRoles(t *testing.T) {
	r := &runner{
		roles: map[string][]string{
			"all":           {"isucon-3", "isucon-1", "isucon-2", "isucon-1"},
			"nginx_profile": {"isucon-2"},
		},
	}
	got, err := r.allHosts()
	if err != nil {
		t.Fatalf("allHosts: %v", err)
	}
	if want := []string{"isucon-1", "isucon-2", "isucon-3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("allHosts = %v, want %v", got, want)
	}
}

func TestAllHostsRequiresAllRole(t *testing.T) {
	r := &runner{roles: map[string][]string{"nginx_profile": {"isucon-2"}}}
	if _, err := r.allHosts(); err == nil {
		t.Fatal("allHosts accepted missing role all")
	}
}

func collectorNames(collectors []Collector) []string {
	names := make([]string, 0, len(collectors))
	for _, collector := range collectors {
		names = append(names, collector.Name)
	}
	return names
}

func TestEnabledCollectorsDefaultsAndInclude(t *testing.T) {
	collectors := []Collector{
		{Name: "proc"},
		{Name: "nginx-oncpu", EnabledByDefault: boolPointer(false)},
	}

	got, err := enabledCollectors(collectors, nil)
	if err != nil {
		t.Fatalf("enabledCollectors defaults: %v", err)
	}
	if want := []string{"proc"}; !reflect.DeepEqual(collectorNames(got), want) {
		t.Fatalf("default collectors = %v, want %v", collectorNames(got), want)
	}

	got, err = enabledCollectors(collectors, map[string]bool{"nginx-oncpu": true})
	if err != nil {
		t.Fatalf("enabledCollectors include: %v", err)
	}
	if want := []string{"proc", "nginx-oncpu"}; !reflect.DeepEqual(collectorNames(got), want) {
		t.Fatalf("included collectors = %v, want %v", collectorNames(got), want)
	}
}

func TestEnabledCollectorsRejectsUnknownInclude(t *testing.T) {
	_, err := enabledCollectors([]Collector{{Name: "proc"}}, map[string]bool{"typo": true})
	if err == nil {
		t.Fatal("enabledCollectors accepted an unknown include")
	}
}

func TestEnabledOneshotsDefaultsAndInclude(t *testing.T) {
	ones := []Oneshot{
		{Name: "default"},
		{Name: "go-cpu", EnabledByDefault: boolPointer(false)},
	}
	got, err := enabledOneshots(ones, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "default" {
		t.Fatalf("default oneshots = %#v", got)
	}
	got, err = enabledOneshots(ones, map[string]bool{"go-cpu": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Name != "go-cpu" {
		t.Fatalf("included oneshots = %#v", got)
	}
	if _, err := enabledOneshots(ones, map[string]bool{"typo": true}); err == nil {
		t.Fatal("enabledOneshots accepted an unknown include")
	}
}

func TestOnlyCanSelectDisabledCollector(t *testing.T) {
	collectors := []Collector{
		{Name: "proc"},
		{Name: "nginx-oncpu", EnabledByDefault: boolPointer(false)},
	}
	got, err := filterCollectors(collectors, map[string]bool{"nginx-oncpu": true})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"nginx-oncpu"}; !reflect.DeepEqual(collectorNames(got), want) {
		t.Fatalf("only collectors = %v, want %v", collectorNames(got), want)
	}
}

func TestOnlyRejectsUnknownCollector(t *testing.T) {
	if _, err := filterCollectors([]Collector{{Name: "proc"}}, map[string]bool{"none": true}); err == nil {
		t.Fatal("filterCollectors accepted an unknown collector")
	}
}

func TestCollectAcceptsKnownOneshotOnly(t *testing.T) {
	config := writeOneshotConfig(t)
	err := runCollect([]string{
		"oneshot",
		"-config", config,
		"-run-dir", t.TempDir(),
		"-only", "fgprof",
		"-role", "app=isucon-1",
		"-var", "profile_url=http://127.0.0.1:6060/debug/fgprof?seconds=60",
		"-dry-run",
	})
	if err != nil {
		t.Fatalf("known oneshot -only was rejected: %v", err)
	}
}

func TestCollectRejectsUnknownOneshotOnly(t *testing.T) {
	err := runCollect([]string{"oneshot", "-config", writeOneshotConfig(t), "-run-dir", t.TempDir(), "-only", "unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown oneshot -only error = %v", err)
	}
}

func TestCollectRejectsMissingOneshotVariable(t *testing.T) {
	err := runCollect([]string{
		"oneshot", "-config", writeOneshotConfig(t), "-run-dir", t.TempDir(),
		"-only", "fgprof", "-role", "app=isucon-1", "-dry-run",
	})
	if err == nil || !strings.Contains(err.Error(), "{var:profile_url}") {
		t.Fatalf("missing oneshot variable error = %v", err)
	}
}

func writeOneshotConfig(t *testing.T) string {
	t.Helper()
	config := filepath.Join(t.TempDir(), "collectors.yaml")
	body := `oneshots:
  - name: fgprof
    hosts: app
    run: curl -o {remote_out} '{var:profile_url}'
    remote_out: /tmp/fgprof.pprof
    output: '{host}-fgprof.pprof'
`
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestNoCollectorsStartsWithoutRolesOrSSH(t *testing.T) {
	config := filepath.Join(t.TempDir(), "collectors.yaml")
	body := `collectors:
  - name: proc
    hosts: all
    binary: proc-metrics
    remote_root: /tmp/proc
    outputs: {metrics.tsv: metrics.tsv}
    stderr: metrics.stderr
`
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCollect([]string{"start", "-config", config, "-run-id", "20260904-120000", "-no-collectors"}); err != nil {
		t.Fatalf("no-collector start failed: %v", err)
	}
}
