package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pprofprofile "github.com/google/pprof/profile"
)

// pprofFunction contains the flat and cumulative wall-clock time attributed to
// one function in an fgprof profile encoded in pprof format.
type pprofFunction struct {
	Name    string  `json:"name"`
	FlatMs  float64 `json:"flat_ms"`
	FlatPct float64 `json:"flat_pct"`
	CumMs   float64 `json:"cum_ms"`
	CumPct  float64 `json:"cum_pct"`
}

type pprofProfileData struct {
	Host        string          `json:"host"`
	Source      string          `json:"source"`
	SampleType  string          `json:"sample_type"`
	SampleUnit  string          `json:"sample_unit"`
	DurationSec float64         `json:"duration_sec"`
	TotalMs     float64         `json:"total_ms"`
	Functions   []pprofFunction `json:"functions"`
}

type pprofResponse struct {
	RunID     string             `json:"run_id"`
	Available bool               `json:"available"`
	Profiles  []pprofProfileData `json:"profiles"`
}

type goPprofFunction struct {
	Name    string  `json:"name"`
	Flat    float64 `json:"flat"`
	FlatPct float64 `json:"flat_pct"`
	Cum     float64 `json:"cum"`
	CumPct  float64 `json:"cum_pct"`
}

type goPprofProfileData struct {
	Kind        string            `json:"kind"`
	Host        string            `json:"host"`
	Source      string            `json:"source"`
	SampleType  string            `json:"sample_type"`
	SampleUnit  string            `json:"sample_unit"`
	DurationSec float64           `json:"duration_sec"`
	Total       float64           `json:"total"`
	Functions   []goPprofFunction `json:"functions"`
}

type goPprofResponse struct {
	RunID     string               `json:"run_id"`
	Available bool                 `json:"available"`
	Profiles  []goPprofProfileData `json:"profiles"`
}

type goPprofKind struct {
	name                 string
	suffix               string
	preferredSampleTypes []string
}

var goPprofKinds = []goPprofKind{
	{name: "cpu", suffix: "-go-cpu.pprof", preferredSampleTypes: []string{"cpu", "time"}},
	{name: "heap", suffix: "-go-heap.pprof", preferredSampleTypes: []string{"inuse_space", "space"}},
	{name: "allocs", suffix: "-go-allocs.pprof", preferredSampleTypes: []string{"alloc_space", "space"}},
	{name: "goroutine", suffix: "-go-goroutine.pprof", preferredSampleTypes: []string{"goroutine", "samples"}},
}

type pprofFunctionValue struct {
	flat int64
	cum  int64
}

type pprofFunctionStat struct {
	name string
	flat int64
	cum  int64
}

// selectPprofSampleType prefers the wall-clock sample value emitted by fgprof.
// Supported profile converters may label the primary value through
// DefaultSampleType instead, so use that metadata before the first valid type.
func selectPprofSampleType(p *pprofprofile.Profile) (int, *pprofprofile.ValueType, error) {
	return selectPreferredPprofSampleType(p, []string{"time"})
}

func selectPreferredPprofSampleType(p *pprofprofile.Profile, preferred []string) (int, *pprofprofile.ValueType, error) {
	for _, preferredType := range preferred {
		for i, sampleType := range p.SampleType {
			if sampleType != nil && sampleType.Type == preferredType {
				return i, sampleType, nil
			}
		}
	}
	if p.DefaultSampleType != "" {
		for i, sampleType := range p.SampleType {
			if sampleType != nil && sampleType.Type == p.DefaultSampleType {
				return i, sampleType, nil
			}
		}
	}
	for i, sampleType := range p.SampleType {
		if sampleType != nil {
			return i, sampleType, nil
		}
	}
	return -1, nil, fmt.Errorf("profile has no sample type")
}

func findGoPprofKind(name string) (goPprofKind, bool) {
	for _, kind := range goPprofKinds {
		if kind.name == name {
			return kind, true
		}
	}
	return goPprofKind{}, false
}

func hasGoPprofFiles(dir string) bool {
	for _, kind := range goPprofKinds {
		if len(mustGlob(filepath.Join(dir, "*"+kind.suffix))) > 0 {
			return true
		}
	}
	return false
}

func pprofLocationName(location *pprofprofile.Location) string {
	if location != nil {
		for _, line := range location.Line {
			if line.Function != nil && line.Function.Name != "" {
				return line.Function.Name
			}
		}
		if location.Mapping != nil && location.Mapping.File != "" {
			return location.Mapping.File
		}
	}
	return "[unknown]"
}

func pprofLineNames(location *pprofprofile.Location) []string {
	if location == nil || len(location.Line) == 0 {
		return []string{pprofLocationName(location)}
	}

	names := make([]string, 0, len(location.Line))
	for _, line := range location.Line {
		if line.Function != nil && line.Function.Name != "" {
			names = append(names, line.Function.Name)
		} else {
			names = append(names, "[unknown]")
		}
	}
	return names
}

// summarizePprof mirrors pprof's function-level flat/cumulative accounting:
// locations are ordered leaf-first, so the first line of the first location
// receives flat time, while every distinct frame in the sample receives cum
// time.
func summarizePprof(p *pprofprofile.Profile, sampleIndex int) ([]pprofFunctionStat, int64) {
	values := make(map[string]*pprofFunctionValue)
	var total int64

	for _, sample := range p.Sample {
		if sample == nil || sampleIndex < 0 || sampleIndex >= len(sample.Value) {
			continue
		}
		value := sample.Value[sampleIndex]
		if value <= 0 {
			continue
		}
		total += value

		leafName := "[unknown]"
		if len(sample.Location) > 0 {
			leaf := sample.Location[0]
			if leaf != nil && len(leaf.Line) > 0 && leaf.Line[0].Function != nil && leaf.Line[0].Function.Name != "" {
				leafName = leaf.Line[0].Function.Name
			} else {
				leafName = pprofLocationName(leaf)
			}
		}
		leafStats := values[leafName]
		if leafStats == nil {
			leafStats = &pprofFunctionValue{}
			values[leafName] = leafStats
		}
		leafStats.flat += value

		seen := make(map[string]struct{})
		for _, location := range sample.Location {
			for _, name := range pprofLineNames(location) {
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				stats := values[name]
				if stats == nil {
					stats = &pprofFunctionValue{}
					values[name] = stats
				}
				stats.cum += value
			}
		}
	}

	functions := make([]pprofFunctionStat, 0, len(values))
	for name, value := range values {
		functions = append(functions, pprofFunctionStat{
			name: name,
			flat: value.flat,
			cum:  value.cum,
		})
	}
	sort.Slice(functions, func(i, j int) bool {
		if functions[i].flat != functions[j].flat {
			return functions[i].flat > functions[j].flat
		}
		if functions[i].cum != functions[j].cum {
			return functions[i].cum > functions[j].cum
		}
		return functions[i].name < functions[j].name
	})
	return functions, total
}

func pprofValueToMillis(value int64, unit string) float64 {
	switch strings.ToLower(unit) {
	case "nanoseconds":
		return float64(value) / 1_000_000
	case "microseconds":
		return float64(value) / 1_000
	case "milliseconds":
		return float64(value)
	case "seconds":
		return float64(value) * 1_000
	default:
		return float64(value)
	}
}

func pprofPercent(value, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

func parsePprofFile(path string) (pprofProfileData, error) {
	profile, err := readPprofFile(path)
	if err != nil {
		return pprofProfileData{}, err
	}
	sampleIndex, sampleType, err := selectPprofSampleType(profile)
	if err != nil {
		return pprofProfileData{}, err
	}

	functions, total := summarizePprof(profile, sampleIndex)
	outputFunctions := make([]pprofFunction, 0, len(functions))
	for _, function := range functions {
		outputFunctions = append(outputFunctions, pprofFunction{
			Name:    function.name,
			FlatMs:  pprofValueToMillis(function.flat, sampleType.Unit),
			FlatPct: pprofPercent(function.flat, total),
			CumMs:   pprofValueToMillis(function.cum, sampleType.Unit),
			CumPct:  pprofPercent(function.cum, total),
		})
	}

	return pprofProfileData{
		Source:      filepath.Base(path),
		SampleType:  sampleType.Type,
		SampleUnit:  sampleType.Unit,
		DurationSec: float64(profile.DurationNanos) / 1_000_000_000,
		TotalMs:     pprofValueToMillis(total, sampleType.Unit),
		Functions:   outputFunctions,
	}, nil
}

func readPprofFile(path string) (*pprofprofile.Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	profile, err := pprofprofile.Parse(f)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

func parseGoPprofFile(path string, kind goPprofKind) (goPprofProfileData, error) {
	profile, err := readPprofFile(path)
	if err != nil {
		return goPprofProfileData{}, err
	}
	sampleIndex, sampleType, err := selectPreferredPprofSampleType(profile, kind.preferredSampleTypes)
	if err != nil {
		return goPprofProfileData{}, err
	}

	functions, total := summarizePprof(profile, sampleIndex)
	outputFunctions := make([]goPprofFunction, 0, len(functions))
	for _, function := range functions {
		outputFunctions = append(outputFunctions, goPprofFunction{
			Name:    function.name,
			Flat:    float64(function.flat),
			FlatPct: pprofPercent(function.flat, total),
			Cum:     float64(function.cum),
			CumPct:  pprofPercent(function.cum, total),
		})
	}

	return goPprofProfileData{
		Kind:        kind.name,
		Source:      filepath.Base(path),
		SampleType:  sampleType.Type,
		SampleUnit:  sampleType.Unit,
		DurationSec: float64(profile.DurationNanos) / 1_000_000_000,
		Total:       float64(total),
		Functions:   outputFunctions,
	}, nil
}

func (a *app) handleFgprof(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}

	resp := pprofResponse{RunID: runID, Available: false, Profiles: []pprofProfileData{}}
	for _, path := range mustGlob(filepath.Join(dir, "*-fgprof.pprof")) {
		profile, err := parsePprofFile(path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse %s: %v", filepath.Base(path), err))
			return
		}
		profile.Host = strings.TrimSuffix(filepath.Base(path), "-fgprof.pprof")
		resp.Profiles = append(resp.Profiles, profile)
	}
	sort.Slice(resp.Profiles, func(i, j int) bool {
		return resp.Profiles[i].Host < resp.Profiles[j].Host
	})
	resp.Available = len(resp.Profiles) > 0
	writeJSON(w, resp)
}

func (a *app) resolveFgprofFile(runID, host string) (string, bool) {
	if host == "" || filepath.Base(host) != host || strings.ContainsAny(host, `/\\`) {
		return "", false
	}
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		return "", false
	}
	path := filepath.Join(dir, host+"-fgprof.pprof")
	if !fileExists(path) {
		return "", false
	}
	return path, true
}

// handleFgprofGraph delegates graph layout to go tool pprof and Graphviz. The
// SVG is generated on demand so the dashboard does not modify any RUN output.
func (a *app) handleFgprofGraph(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	host := r.PathValue("host")
	profilePath, ok := a.resolveFgprofFile(runID, host)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown fgprof profile")
		return
	}

	profile, err := readPprofFile(profilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse %s: %v", filepath.Base(profilePath), err))
		return
	}
	_, sampleType, err := selectPprofSampleType(profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("select sample type: %v", err))
		return
	}
	a.writePprofGraph(w, r, profilePath, sampleType.Type, "fgprof")
}

func (a *app) handleGoPprof(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}

	resp := goPprofResponse{RunID: runID, Available: false, Profiles: []goPprofProfileData{}}
	for _, kind := range goPprofKinds {
		for _, path := range mustGlob(filepath.Join(dir, "*"+kind.suffix)) {
			profile, err := parseGoPprofFile(path, kind)
			if err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse %s: %v", filepath.Base(path), err))
				return
			}
			profile.Host = strings.TrimSuffix(filepath.Base(path), kind.suffix)
			resp.Profiles = append(resp.Profiles, profile)
		}
	}
	sort.Slice(resp.Profiles, func(i, j int) bool {
		if resp.Profiles[i].Kind != resp.Profiles[j].Kind {
			return resp.Profiles[i].Kind < resp.Profiles[j].Kind
		}
		return resp.Profiles[i].Host < resp.Profiles[j].Host
	})
	resp.Available = len(resp.Profiles) > 0
	writeJSON(w, resp)
}

func (a *app) resolveGoPprofFile(runID, kindName, host string) (string, goPprofKind, bool) {
	kind, ok := findGoPprofKind(kindName)
	if !ok || host == "" || filepath.Base(host) != host || strings.ContainsAny(host, `/\\`) {
		return "", goPprofKind{}, false
	}
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		return "", goPprofKind{}, false
	}
	path := filepath.Join(dir, host+kind.suffix)
	if !fileExists(path) {
		return "", goPprofKind{}, false
	}
	return path, kind, true
}

func (a *app) handleGoPprofGraph(w http.ResponseWriter, r *http.Request) {
	profilePath, kind, ok := a.resolveGoPprofFile(r.PathValue("run_id"), r.PathValue("kind"), r.PathValue("host"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown Go pprof profile")
		return
	}
	profile, err := readPprofFile(profilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse %s: %v", filepath.Base(profilePath), err))
		return
	}
	_, sampleType, err := selectPreferredPprofSampleType(profile, kind.preferredSampleTypes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("select sample type: %v", err))
		return
	}
	a.writePprofGraph(w, r, profilePath, sampleType.Type, "Go pprof")
}

func (a *app) writePprofGraph(w http.ResponseWriter, r *http.Request, profilePath, sampleType, label string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	args := []string{
		"tool",
		"pprof",
		"-svg",
		"-symbolize=fastlocal",
		"-functions",
		"-nodecount=50",
		"-edgefraction=0.01",
		"-sample_index=" + sampleType,
	}
	binaryPath := findApplicationBinary(filepath.Join(a.root, "webapp", "go"))
	if fileExists(binaryPath) {
		args = append(args, binaryPath)
	}
	args = append(args, profilePath)

	cmd := exec.CommandContext(ctx, "go", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	svg, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("generate %s graph: %s", label, message))
		return
	}
	if !bytes.Contains(svg, []byte("<svg")) {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("generate %s graph: SVG output is empty", label))
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(svg)
}

func findApplicationBinary(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), ".mod") || strings.HasSuffix(entry.Name(), ".sum") || strings.HasSuffix(entry.Name(), ".pgo") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return filepath.Join(dir, entry.Name())
		}
	}
	return ""
}
