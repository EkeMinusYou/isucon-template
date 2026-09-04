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

func TestOnlyCanSelectDisabledCollector(t *testing.T) {
	collectors := []Collector{
		{Name: "proc"},
		{Name: "nginx-oncpu", EnabledByDefault: boolPointer(false)},
	}
	got := filterCollectors(collectors, map[string]bool{"nginx-oncpu": true})
	if want := []string{"nginx-oncpu"}; !reflect.DeepEqual(collectorNames(got), want) {
		t.Fatalf("only collectors = %v, want %v", collectorNames(got), want)
	}
}
