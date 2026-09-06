package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/pprof/profile"
)

// Historical RUNs predate the frozen artifact contract and had optional profiles.
var legacyProfiles = map[string]bool{"fgprof": true, "go-cpu": true, "go-heap": true, "go-allocs": true, "go-goroutine": true}

func captureArtifactContract(m Manifest, collectors, digesters string) ([]ArtifactSpec, error) {
	specs, err := loadArtifactSpecs(collectors, digesters)
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(collectors)
	if err != nil {
		return nil, err
	}
	if !m.ProfilesEnabled {
		for _, o := range cfg.Oneshots {
			if o.Group != "profiles" {
				continue
			}
			for i := range specs {
				if specs[i].Producer == "oneshot:"+o.Name {
					specs[i].Optional = true
				}
			}
		}
	}
	return appendCaptureSpecs(specsForCollectorMode(specs, m.CollectorsDisabled), m), nil
}

func specsForRun(current []ArtifactSpec, m Manifest) []ArtifactSpec {
	if m.ArtifactContract != nil {
		return append([]ArtifactSpec{}, m.ArtifactContract...)
	}
	// Preserve the old optional defaults; RequiredArtifacts still enforces any
	// per-host captures explicitly recorded by the previous manifest version.
	specs := specsForCollectorMode(current, m.CollectorsDisabled)
	for i := range specs {
		if legacyProfiles[strings.TrimPrefix(specs[i].Producer, "oneshot:")] || specs[i].Producer == "digester:user-transitions" {
			specs[i].Optional = true
		}
	}
	return appendCaptureSpecs(specs, m)
}

func captureRequirements(m Manifest, collectors, digesters string) ([]string, error) {
	cfg, err := loadConfig(collectors)
	if err != nil {
		return nil, err
	}
	dcfg, err := loadDigestConfig(digesters)
	if err != nil {
		return nil, err
	}
	roles := map[string][]string{}
	for role, hosts := range m.Roles.Additional {
		roles[role] = hosts
	}
	roles["app"], roles["nginx"], roles["mysql"], roles["entry"] = m.Roles.App, m.Roles.Nginx, []string{m.Roles.MySQL}, []string{m.Roles.Entry}
	var required []string
	if m.ProfilesEnabled {
		profiles, err := enabledOneshotsInGroup(cfg.Oneshots, "profiles")
		if err != nil {
			return nil, err
		}
		for _, o := range profiles {
			if len(roles[o.Hosts]) == 0 {
				return nil, fmt.Errorf("profile %s has no hosts", o.Name)
			}
			for _, host := range roles[o.Hosts] {
				required = append(required, strings.ReplaceAll(o.Output, "{host}", host))
			}
		}
	}
	for _, d := range dcfg.Digesters {
		if d.Name == "user-transitions" && d.enabledByDefault() {
			for _, o := range d.Outputs {
				required = append(required, o.File)
			}
		}
	}
	for _, s := range dcfg.Sources {
		if !strings.HasPrefix(s.Local, "{run_dir}/") {
			continue
		}
		if len(roles[s.Role]) == 0 {
			return nil, fmt.Errorf("source %s has no hosts", s.Name)
		}
		for _, host := range roles[s.Role] {
			name := strings.ReplaceAll(strings.TrimPrefix(s.Local, "{run_dir}/"), "{host}", host)
			if s.Compress {
				name += ".zst"
			}
			required = append(required, name)
		}
	}
	sort.Strings(required)
	return required, nil
}

func appendCaptureSpecs(specs []ArtifactSpec, m Manifest) []ArtifactSpec {
	result := append([]ArtifactSpec{}, specs...)
	for _, name := range m.RequiredArtifacts {
		result = append(result, ArtifactSpec{Pattern: name, Producer: "RUN capture contract"})
	}
	return result
}

func assessCaptureQuality(dir string, m Manifest, artifacts []Artifact) []Artifact {
	required := map[string]bool{}
	for _, name := range m.RequiredArtifacts {
		required[name] = true
	}
	var accessIndexes []int
	var accessRows int64
	for i := range artifacts {
		a := &artifacts[i]
		if !required[a.Name] {
			continue
		}
		switch {
		case a.Name == "user-transitions.json":
			a.Quality = inspectUserTransitions(filepath.Join(dir, a.Name))
		case strings.HasSuffix(a.Name, ".pprof"):
			a.Quality = inspectProfile(filepath.Join(dir, a.Name), m.LoadWindow)
		case strings.HasPrefix(a.Name, "raw/access-") && strings.HasSuffix(a.Name, ".log.zst"):
			a.Quality = inspectAccessLog(filepath.Join(dir, a.Name), m.LoadWindow)
			accessIndexes = append(accessIndexes, i)
			accessRows += a.Quality.InWindowSamples
		default:
			continue
		}
		if a.Quality.Status != "valid" {
			if a.Status != "missing" {
				a.Status = "failed"
			}
			a.Reason = joinReasons(a.Reason, a.Quality.Reason)
		}
	}
	if m.Passed != nil && *m.Passed && len(accessIndexes) > 0 && accessRows == 0 {
		for _, i := range accessIndexes {
			artifacts[i].Status = "failed"
			artifacts[i].Reason = "no nginx access records in the load window despite a passed benchmark"
			artifacts[i].Quality.Status = "invalid"
			artifacts[i].Quality.Reason = artifacts[i].Reason
		}
	}
	return artifacts
}

func inspectUserTransitions(path string) ArtifactQuality {
	q := ArtifactQuality{Expected: true, Status: "invalid"}
	body, err := os.ReadFile(path)
	if err != nil {
		q.Reason = err.Error()
		return q
	}
	var report struct {
		SchemaVersion int `json:"schema_version"`
		Summary       struct {
			InputFiles           int64 `json:"input_files"`
			APIRequests          int64 `json:"api_requests"`
			ClassifiedRequests   int64 `json:"classified_requests"`
			RequestsWithIdentity int64 `json:"requests_with_identity"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		q.Reason = "invalid user-transition report: " + err.Error()
		return q
	}
	s := report.Summary
	q.Rows = s.ClassifiedRequests
	if report.SchemaVersion != 3 || s.InputFiles <= 0 || s.APIRequests <= 0 || s.ClassifiedRequests <= 0 {
		q.Reason = "user-transition report has no valid classified API input; check collection and route adapter"
		return q
	}
	if s.RequestsWithIdentity <= 0 {
		q.Reason = "user-transition identity is missing; configure the access log identity field during setup"
		return q
	}
	q.Status = "valid"
	return q
}

func inspectProfile(path string, window LoadWindow) ArtifactQuality {
	q := ArtifactQuality{Expected: true, Status: "invalid", Monotonic: true, Finite: true}
	file, err := os.Open(path)
	if err != nil {
		q.Reason = err.Error()
		return q
	}
	defer file.Close()
	p, err := profile.Parse(file)
	if err == nil {
		err = p.CheckValid()
	}
	if err != nil {
		q.Reason = "invalid profile: " + err.Error()
		return q
	}
	q.Rows = int64(len(p.Sample))
	expectedType := ""
	for suffix, sampleType := range map[string]string{"-go-cpu.pprof": "cpu", "-fgprof.pprof": "time", "-go-heap.pprof": "inuse_space", "-go-allocs.pprof": "alloc_space", "-go-goroutine.pprof": "goroutine"} {
		if strings.HasSuffix(path, suffix) {
			expectedType = sampleType
		}
	}
	foundType := false
	for _, sampleType := range p.SampleType {
		if sampleType != nil && sampleType.Type == expectedType {
			foundType = true
		}
	}
	if !foundType {
		q.Reason = "profile does not contain expected sample type " + expectedType
		return q
	}
	start, err := time.Parse(time.RFC3339Nano, window.StartedAt)
	end, endErr := time.Parse(time.RFC3339Nano, window.EndedAt)
	if window.Status != "ok" || err != nil || endErr != nil || !end.After(start) {
		q.Reason = "profile load window is unavailable"
		return q
	}
	if p.TimeNanos <= 0 {
		q.Reason = "profile capture timestamp is missing"
		return q
	}
	captured := time.Unix(0, p.TimeNanos)
	sampling := strings.HasSuffix(path, "-go-cpu.pprof") || strings.HasSuffix(path, "-fgprof.pprof")
	if sampling {
		if p.DurationNanos <= 0 {
			q.Reason = "sampling profile duration is missing"
			return q
		}
		finished := captured.Add(time.Duration(p.DurationNanos))
		overlapStart, overlapEnd := start, end
		if captured.After(overlapStart) {
			overlapStart = captured
		}
		if finished.Before(overlapEnd) {
			overlapEnd = finished
		}
		q.WindowCoveragePct = math.Max(0, overlapEnd.Sub(overlapStart).Seconds()) / end.Sub(start).Seconds() * 100
		if captured.After(start) || finished.Before(end) {
			q.Reason = fmt.Sprintf("profile %s..%s does not cover load window %s..%s", captured.UTC().Format(time.RFC3339Nano), finished.UTC().Format(time.RFC3339Nano), window.StartedAt, window.EndedAt)
			return q
		}
	} else if captured.Before(start) || captured.After(end) {
		q.Reason = "snapshot is outside the load window"
		return q
	}
	q.Status = "valid"
	return q
}

func inspectAccessLog(path string, window LoadWindow) ArtifactQuality {
	q := ArtifactQuality{Expected: true, Status: "invalid", Monotonic: true, Finite: true}
	cmd := exec.Command("zstdcat", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		q.Reason = err.Error()
		return q
	}
	if err := cmd.Start(); err != nil {
		q.Reason = err.Error()
		return q
	}
	start, _ := time.Parse(time.RFC3339Nano, window.StartedAt)
	end, _ := time.Parse(time.RFC3339Nano, window.EndedAt)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	var parseErr error
	for scanner.Scan() {
		var record map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			parseErr = err
			break
		}
		for _, field := range []string{"msec", "method", "uri", "status", "response_time", "body_bytes", "upstream_time", "upstream_addr", "upstream_status", "cache_status"} {
			if _, ok := record[field]; !ok {
				parseErr = fmt.Errorf("missing access log field %s", field)
				break
			}
		}
		if parseErr != nil {
			break
		}
		var ts json.Number
		if err := json.Unmarshal(record["msec"], &ts); err != nil {
			parseErr = err
			break
		}
		seconds, err := ts.Float64()
		if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			parseErr = fmt.Errorf("invalid access timestamp")
			break
		}
		q.Rows++
		if seconds >= float64(start.UnixNano())/1e9 && seconds <= float64(end.UnixNano())/1e9 {
			q.InWindowSamples++
		}
	}
	if parseErr == nil {
		parseErr = scanner.Err()
	}
	if parseErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if parseErr != nil {
		q.Reason = "invalid access log: " + parseErr.Error()
		return q
	}
	if waitErr != nil {
		q.Reason = "access decompression failed: " + waitErr.Error()
		return q
	}
	q.Status = "valid"
	return q
}
