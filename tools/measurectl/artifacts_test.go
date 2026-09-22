package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRunDirRejectsMissingRequiredArtifact(t *testing.T) {
	dir := t.TempDir()
	specs := []ArtifactSpec{
		{Pattern: "required.tsv", Producer: "collector:required"},
		{Pattern: "optional.tsv", Producer: "collector:optional", Optional: true},
	}
	if err := checkRunDir(dir, specs); err == nil {
		t.Fatal("checkRunDir accepted a missing required artifact")
	}
	if err := os.WriteFile(filepath.Join(dir, "required.tsv"), []byte("value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkRunDir(dir, specs); err != nil {
		t.Fatalf("checkRunDir rejected complete required artifacts: %v", err)
	}
}

func TestAppendMissingArtifactsRecordsRequiredAndSkipsOptional(t *testing.T) {
	dir := t.TempDir()
	got, err := appendMissingArtifacts(dir, nil, []ArtifactSpec{
		{Pattern: "required.tsv", Producer: "collector:required"},
		{Pattern: "optional.tsv", Producer: "oneshot:optional", Optional: true},
		{Pattern: "raw/access-*.log.zst", Producer: "source:access"},
		{Pattern: "run.json", Producer: "measurectl:manifest"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "raw/access-*.log.zst" || got[1].Name != "required.tsv" ||
		got[0].Status != "missing" || got[1].Status != "missing" || got[0].Reason == "" || got[1].Reason == "" {
		t.Fatalf("missing artifacts = %#v", got)
	}
}

func TestPerHostDigesterArtifactsMatchCollectedFiles(t *testing.T) {
	dir := t.TempDir()
	collectors := filepath.Join(dir, "collectors.yaml")
	digesters := filepath.Join(dir, "digesters.yaml")
	if err := os.WriteFile(collectors, []byte("prepare: [{name: noop, hosts: app, script: true}]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := `digesters:
  - name: journal
    remote:
      role: app
      per_host: true
      script: echo journal
    outputs:
      - file: '{host}-app-journal.log'
`
	if err := os.WriteFile(digesters, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	specs, err := loadArtifactSpecs(collectors, digesters)
	if err != nil {
		t.Fatal(err)
	}
	var journals []ArtifactSpec
	for _, spec := range specs {
		if spec.Producer == "digester:journal" && !spec.Optional {
			journals = append(journals, spec)
		}
	}
	if len(journals) != 1 {
		t.Fatalf("journal specs = %#v", journals)
	}
	if err := os.WriteFile(filepath.Join(dir, "isucon-1-app-journal.log"), []byte("journal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkRunDir(dir, journals); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "isucon-1-app-journal.log")); err != nil {
		t.Fatal(err)
	}
	if err := checkRunDir(dir, journals); err == nil {
		t.Fatal("a completely missing per-host artifact was accepted")
	}
}

func TestCaptureArtifactContractExpandsEveryRoleHost(t *testing.T) {
	dir := t.TempDir()
	collectors := filepath.Join(dir, "collectors.yaml")
	digesters := filepath.Join(dir, "digesters.yaml")
	if err := os.WriteFile(collectors, []byte(`collectors:
  - name: mysql
    hosts: mysql_all
    binary: mysql-metrics
    remote_root: /tmp/mysql-metrics
    outputs:
      metrics.tsv: '{host}-mysql-status.tsv'
    stderr: '{host}-mysql-status.stderr'
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(digesters, []byte(`sources:
  - name: slow
    role: mysql_all
    remote: /var/log/mysql/mysql-slow.log
    local: '{raw_dir}/mysql-slow-{host}.log'
digesters:
  - name: pt
    source: slow
    per_host: true
    stderr: '{host}-pt.stderr'
    outputs:
      - file: '{host}-pt.log'
        run: printf ok
`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Roles: Roles{Additional: map[string][]string{
		"mysql_all": {"isucon-2", "isucon-3"},
	}}}
	contract, err := captureArtifactContract(m, collectors, digesters)
	if err != nil {
		t.Fatal(err)
	}
	var required []ArtifactSpec
	for _, spec := range contract {
		if !spec.Optional && (spec.Producer == "collector:mysql" || spec.Producer == "digester:pt") {
			required = append(required, spec)
		}
	}
	if len(required) != 4 {
		t.Fatalf("required host artifacts = %#v", required)
	}
	for _, spec := range required {
		if strings.ContainsAny(spec.Pattern, "*?[") || spec.HostRole != "" {
			t.Fatalf("host artifact was not expanded: %#v", spec)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "isucon-2-mysql-status.tsv"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "isucon-2-pt.log"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkRunDir(dir, required); err == nil {
		t.Fatal("one role host was missing but the contract passed")
	}
	for _, name := range []string{"isucon-3-mysql-status.tsv", "isucon-3-pt.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ok"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkRunDir(dir, required); err != nil {
		t.Fatalf("complete host contract was rejected: %v", err)
	}
}

func TestCollectorDisabledModeStillRequiresLogsAndDigests(t *testing.T) {
	dir := t.TempDir()
	specs := []ArtifactSpec{
		{Pattern: "metrics.tsv", Producer: "collector:proc"},
		{Pattern: "access.log", Producer: "source:access"},
		{Pattern: "alp.json", Producer: "digester:alp"},
	}
	withoutCollectors := specsForCollectorMode(specs, true)
	for _, name := range []string{"access.log", "alp.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkRunDir(dir, withoutCollectors); err != nil {
		t.Fatalf("intentional collector absence rejected: %v", err)
	}
	if err := checkRunDir(dir, specs); err == nil {
		t.Fatal("enabled collector absence was accepted")
	}
	if err := os.Remove(filepath.Join(dir, "alp.json")); err != nil {
		t.Fatal(err)
	}
	if err := checkRunDir(dir, withoutCollectors); err == nil {
		t.Fatal("missing digester was hidden by collector mode")
	}
}
