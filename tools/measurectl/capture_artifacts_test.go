package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"gopkg.in/yaml.v3"
)

func TestStandardCaptureSelectionAndFrozenContract(t *testing.T) {
	cfg, err := loadConfig("collectors.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dcfg, err := loadDigestConfig("digesters.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Oneshots[0].EnabledByDefault = boolPointer(false)
	for i := range dcfg.Digesters {
		if dcfg.Digesters[i].Name == "user-transitions" {
			dcfg.Digesters[i].EnabledByDefault = boolPointer(false)
		}
	}
	dir := t.TempDir()
	writeConfig := func(name string, value any) string {
		body, err := yaml.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	collectors := writeConfig("collectors.yaml", cfg)
	digesters := writeConfig("digesters.yaml", dcfg)
	m := Manifest{ProfilesEnabled: true, Roles: Roles{App: []string{"host"}, Nginx: []string{"host"}}}
	m.RequiredArtifacts, err = captureRequirements(m, collectors, digesters)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.RequiredArtifacts) != 5 {
		t.Fatalf("requirements=%v", m.RequiredArtifacts)
	}
	selected, err := enabledOneshotsInGroup(cfg.Oneshots, "profiles")
	if err != nil || len(selected) != 4 {
		t.Fatalf("selected=%v err=%v", selected, err)
	}
	for _, o := range selected {
		if !strings.Contains(strings.Join(m.RequiredArtifacts, ","), "host-"+o.Name+".pprof") {
			t.Fatalf("capture missing for %s", o.Name)
		}
	}
	m.ArtifactContract, err = captureArtifactContract(m, collectors, digesters)
	if err != nil {
		t.Fatal(err)
	}
	// Re-enabling the declarations must not change a RUN already in progress.
	current, err := loadArtifactSpecs("collectors.yaml", "digesters.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range specsForRun(current, m) {
		if (spec.Producer == "oneshot:fgprof" || spec.Producer == "digester:user-transitions") && !spec.Optional {
			t.Fatalf("frozen default changed: %+v", spec)
		}
	}
	for _, spec := range specsForRun(current, Manifest{}) {
		if (spec.Producer == "oneshot:fgprof" || spec.Producer == "digester:user-transitions") && !spec.Optional {
			t.Fatalf("old RUN gained new requirement: %+v", spec)
		}
	}
	m.ProfilesEnabled = false
	m.RequiredArtifacts, err = captureRequirements(m, collectors, digesters)
	if err != nil || len(m.RequiredArtifacts) != 1 {
		t.Fatalf("override requirements=%v err=%v", m.RequiredArtifacts, err)
	}
	specs, err := captureArtifactContract(m, collectors, digesters)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range specs {
		if strings.HasPrefix(spec.Producer, "oneshot:") && !spec.Optional {
			t.Fatalf("disabled profile still required: %+v", spec)
		}
	}
}

func TestUserTransitionCaptureRequiresIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-transitions.json")
	for _, tc := range []struct {
		identity int
		valid    bool
	}{{0, false}, {2, true}} {
		body := fmt.Sprintf(`{"schema_version":3,"summary":{"input_files":1,"api_requests":3,"classified_requests":3,"requests_with_identity":%d}}`, tc.identity)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		q := inspectUserTransitions(path)
		if (q.Status == "valid") != tc.valid {
			t.Fatalf("quality=%+v", q)
		}
	}
}

func TestCaptureRequirementsArePerHostAndFrozen(t *testing.T) {
	m := Manifest{ProfilesEnabled: true, Roles: Roles{App: []string{"host-1", "host-2", "host-3"}, Nginx: []string{"host-1", "host-2", "host-3"}}}
	required, err := captureRequirements(m, "collectors.yaml", "digesters.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(required) != 19 {
		t.Fatalf("required=%v", required)
	}
	m.RequiredArtifacts = required
	dir := t.TempDir()
	for _, name := range required {
		if name == "host-2-go-cpu.pprof" {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	missing, err := appendMissingArtifacts(dir, nil, appendCaptureSpecs(nil, m))
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0].Name != "host-2-go-cpu.pprof" {
		t.Fatalf("missing=%v", missing)
	}
	if err := checkRunDir(dir, appendCaptureSpecs(nil, m)); err == nil {
		t.Fatal("one host missing was accepted")
	}
	m.ProfilesEnabled = false
	required, err = captureRequirements(m, "collectors.yaml", "digesters.yaml")
	if err != nil || len(required) != 4 {
		t.Fatalf("disabled profiles requirements=%v err=%v", required, err)
	}
	if got := appendCaptureSpecs(nil, Manifest{}); len(got) != 0 {
		t.Fatal("old RUN gained new requirements")
	}
}

func TestProfileCoverageAndCorruption(t *testing.T) {
	start := time.Date(2026, 9, 6, 4, 0, 0, 0, time.UTC)
	window := LoadWindow{Status: "ok", StartedAt: start.Format(time.RFC3339Nano), EndedAt: start.Add(time.Minute).Format(time.RFC3339Nano), DurationMS: 60000}
	for _, tc := range []struct {
		name             string
		offset, duration time.Duration
		valid            bool
	}{
		{"host-go-cpu.pprof", -5 * time.Second, 120 * time.Second, true},
		{"host-fgprof.pprof", -5 * time.Second, 120 * time.Second, true},
		{"late-go-cpu.pprof", time.Second, 120 * time.Second, false},
		{"short-fgprof.pprof", -5 * time.Second, 60 * time.Second, false},
		{"host-go-heap.pprof", 40 * time.Second, 0, true},
		{"late-go-heap.pprof", 80 * time.Second, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sampleType, unit := "cpu", "nanoseconds"
			if strings.HasSuffix(tc.name, "-fgprof.pprof") {
				sampleType = "time"
			} else if strings.HasSuffix(tc.name, "-go-heap.pprof") {
				sampleType, unit = "inuse_space", "bytes"
			}
			p := &profile.Profile{SampleType: []*profile.ValueType{{Type: sampleType, Unit: unit}}, TimeNanos: start.Add(tc.offset).UnixNano(), DurationNanos: int64(tc.duration)}
			path := filepath.Join(t.TempDir(), tc.name)
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Write(f); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
			q := inspectProfile(path, window)
			if (q.Status == "valid") != tc.valid {
				t.Fatalf("quality=%+v", q)
			}
			if err := os.WriteFile(path, []byte("HTTP error"), 0o600); err != nil {
				t.Fatal(err)
			}
			if q := inspectProfile(path, window); q.Status == "valid" {
				t.Fatal("HTTP error accepted as profile")
			}
		})
	}
}

func TestEmptyAccessLogsAreCompressedAndManifestListsRawFiles(t *testing.T) {
	if _, err := exec.LookPath("zstd"); err != nil {
		t.Skip("zstd unavailable")
	}
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw")
	if err := os.MkdirAll(raw, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"msec":"1788667201.0","method":"GET","uri":"/api/tag","status":200,"response_time":0.01,"body_bytes":42,"upstream_time":"0.01","upstream_addr":"127.0.0.1:8080","upstream_status":"200","cache_status":""}` + "\n"
	for i, content := range []string{line, ""} {
		if err := os.WriteFile(filepath.Join(raw, fmt.Sprintf("access-host-%d.log", i+1)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r := digestRunner{runDir: dir, cfg: &DigestConfig{Sources: []Source{{Name: "access", Local: "{run_dir}/raw/access-{host}.log", Compress: true}}}}
	if err := r.compressAll(); err != nil {
		t.Fatal(err)
	}
	artifacts, size, err := scanArtifacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 || size == 0 {
		t.Fatalf("artifacts=%v size=%d", artifacts, size)
	}
	window := LoadWindow{Status: "ok", StartedAt: "2026-09-06T04:00:00Z", EndedAt: "2026-09-06T04:01:00Z"}
	for i, artifact := range artifacts {
		if !strings.HasSuffix(artifact.Name, ".zst") {
			t.Fatalf("uncompressed empty log: %s", artifact.Name)
		}
		q := inspectAccessLog(filepath.Join(dir, artifact.Name), window)
		if q.Status != "valid" {
			t.Fatalf("quality=%+v", q)
		}
		want := int64(1 - i)
		if q.Rows != want || q.InWindowSamples != want {
			t.Fatalf("quality=%+v", q)
		}
	}
	path := filepath.Join(raw, "access-broken.log.zst")
	if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if q := inspectAccessLog(path, window); q.Status == "valid" {
		t.Fatal("invalid compressed log accepted")
	}
	for _, content := range []string{"not JSON\n", "{}\n", strings.Replace(line, "1788667201.0", "NaN", 1)} {
		cmd := exec.Command("zstd", "-q", "-c")
		cmd.Stdin = strings.NewReader(content)
		compressed, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, compressed, 0o600); err != nil {
			t.Fatal(err)
		}
		if q := inspectAccessLog(path, window); q.Status == "valid" {
			t.Fatalf("invalid JSON record accepted: %q", content)
		}
	}
}
