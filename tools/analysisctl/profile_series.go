package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/EkeMinusYou/isucon-template/tools/measurement/profileseries"
	"github.com/google/pprof/profile"
)

// Untimed or whole-lifecycle aggregates cannot become load-only observations.
func profileWithinWindow(path string, w analysisWindow) bool {
	if w.Status != "ok" || strings.HasSuffix(path, "-go-allocs.pprof") {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	p, err := profile.Parse(f)
	if err != nil || p.TimeNanos <= 0 {
		return false
	}
	at := time.Unix(0, p.TimeNanos)
	if at.Before(w.StartedAt) || !at.Before(w.EndedAt) {
		return false
	}
	return p.DurationNanos >= 0 && !at.Add(time.Duration(p.DurationNanos)).After(w.EndedAt)
}

type pointFunction struct {
	name, file       string
	line             int
	flat, cumulative int64
}

func pointFunctions(p *profile.Profile, index int) (map[string]pointFunction, int64) {
	result := map[string]pointFunction{}
	var total int64
	for _, s := range p.Sample {
		if s == nil || len(s.Value) <= index {
			continue
		}
		value := s.Value[index]
		total += value
		frames := pprofFrames(s)
		seen := map[string]bool{}
		for depth, f := range frames {
			key := fmt.Sprintf("%s:%s:%d", f.name, f.file, f.line)
			v := result[key]
			v.name = f.name
			v.file = f.file
			v.line = f.line
			if depth == 0 {
				v.flat += value
			}
			if !seen[key] {
				v.cumulative += value
				seen[key] = true
			}
			result[key] = v
		}
	}
	return result, total
}

type pointFrame struct {
	name, file string
	line       int
}

func pprofFrames(s *profile.Sample) []pointFrame {
	var out []pointFrame
	for _, loc := range s.Location {
		for _, l := range loc.Line {
			if l.Function != nil {
				out = append(out, pointFrame{l.Function.Name, l.Function.Filename, int(l.Line)})
			}
		}
	}
	if len(out) == 0 {
		out = append(out, pointFrame{name: "[unknown]"})
	}
	return out
}
func sampleIndex(p *profile.Profile, name string) int {
	if p != nil {
		for i, t := range p.SampleType {
			if t != nil && t.Type == name {
				return i
			}
		}
	}
	return -1
}

func (r runner) prepareSnapshotSeries(output string, dirs []string, windows map[string]analysisWindow) (resultErr error) {
	headers := map[string][]string{
		"go-profile-recordings":      {"run_id", "host", "source", "status", "reason", "interval_ns", "started_at_ns", "ended_at_ns", "captures", "max_capture_ns"},
		"go-profile-points":          {"run_id", "host", "source", "profile_type", "sample_type", "value_unit", "captured_at_ns", "interval_start_ns", "interval_end_ns", "total_value", "valid"},
		"go-profile-point-functions": {"run_id", "host", "source", "profile_type", "sample_type", "value_unit", "captured_at_ns", "interval_start_ns", "interval_end_ns", "function", "file", "line", "flat_value", "cumulative_value", "valid"},
	}
	writers := map[string]*csv.Writer{}
	var files []*os.File
	defer func() {
		for _, w := range writers {
			w.Flush()
			if e := w.Error(); resultErr == nil {
				resultErr = e
			}
		}
		for _, f := range files {
			if e := f.Close(); resultErr == nil {
				resultErr = e
			}
		}
	}()
	for name, h := range headers {
		f, e := os.Create(filepath.Join(output, name+".rows"))
		if e != nil {
			return e
		}
		files = append(files, f)
		w := csv.NewWriter(f)
		w.Comma = '\t'
		writers[name] = w
		if e = w.Write(h); e != nil {
			return e
		}
	}
	integer := func(n int64) string { return strconv.FormatInt(n, 10) }
	for _, dir := range dirs {
		if r.config.Profiles.SeriesMatch == "" {
			continue
		}
		paths, e := filepath.Glob(filepath.Join(dir, r.config.Profiles.SeriesMatch))
		if e != nil {
			return e
		}
		runID := filepath.Base(dir)
		window := windows[runID]
		for _, path := range paths {
			host := strings.TrimSuffix(filepath.Base(path), "-go-snapshots.tar")
			source := filepath.Base(path)
			index, e := profileseries.Read(path, nil)
			status, reason := "complete", ""
			if e != nil {
				status = "invalid"
				reason = e.Error()
			}
			if status == "complete" && window.Status == "ok" {
				for _, kind := range []string{"go-heap", "go-goroutine"} {
					var first, last int64
					for _, entry := range index.Entries {
						if profileseries.Kind(entry.Name) != kind {
							continue
						}
						if first == 0 {
							first = entry.CapturedAt
						}
						last = entry.CapturedAt
					}
					if first > window.StartedAt.UnixNano() || last < window.EndedAt.UnixNano() {
						status, reason = "partial", "recording does not cover the complete load window"
					}
				}
			}
			var maximum int64
			for _, entry := range index.Entries {
				if entry.DurationNS > maximum {
					maximum = entry.DurationNS
				}
			}
			if e = writers["go-profile-recordings"].Write([]string{runID, host, source, status, reason, integer(index.IntervalNS), integer(index.StartedAt), integer(index.EndedAt), strconv.Itoa(len(index.Entries)), integer(maximum)}); e != nil {
				return e
			}
			if status == "invalid" || window.Status != "ok" {
				continue
			}
			var previous *profile.Profile
			_, e = profileseries.Read(path, func(entry profileseries.Entry, p *profile.Profile) error {
				kind := profileseries.Kind(entry.Name)
				at := time.Unix(0, p.TimeNanos)
				inside := !at.Before(window.StartedAt) && at.Before(window.EndedAt)
				if !inside {
					if kind == "go-heap" {
						previous = p
					}
					return nil
				}
				for i, t := range p.SampleType {
					allocation := t.Type == "alloc_space" || t.Type == "alloc_objects"
					from, to := p.TimeNanos, p.TimeNanos
					values, total := pointFunctions(p, i)
					valid := true
					metric := t.Type
					if allocation {
						oldIndex := sampleIndex(previous, t.Type)
						if oldIndex < 0 || previous.TimeNanos < window.StartedAt.UnixNano() || previous.TimeNanos >= p.TimeNanos {
							continue
						}
						old, oldTotal := pointFunctions(previous, oldIndex)
						total -= oldTotal
						from = previous.TimeNanos
						metric += "_delta"
						for key, v := range old {
							n := values[key]
							if n.name == "" {
								n.name = v.name
								n.file = v.file
								n.line = v.line
							}
							n.flat -= v.flat
							n.cumulative -= v.cumulative
							values[key] = n
						}
						for _, v := range values {
							if v.flat < 0 || v.cumulative < 0 {
								valid = false
							}
						}
						if total < 0 {
							valid = false
						}
					}
					prefix := []string{runID, host, source + "#" + entry.Name, kind, metric, t.Unit, integer(p.TimeNanos), integer(from), integer(to)}
					if e := writers["go-profile-points"].Write(append(append([]string{}, prefix...), integer(total), strconv.FormatBool(valid))); e != nil {
						return e
					}
					for _, v := range values {
						if v.flat == 0 && v.cumulative == 0 {
							continue
						}
						row := append(append([]string{}, prefix...), v.name, v.file, strconv.Itoa(v.line), integer(v.flat), integer(v.cumulative), strconv.FormatBool(valid))
						if e := writers["go-profile-point-functions"].Write(row); e != nil {
							return e
						}
					}
				}
				if kind == "go-heap" {
					previous = p
				}
				return nil
			})
			if e != nil {
				return fmt.Errorf("normalize snapshot recording %s: %w", path, e)
			}
		}
	}
	return nil
}

func legacyProfileHost(path string) string {
	name := filepath.Base(path)
	for _, kind := range []string{"go-cpu", "go-heap", "go-allocs", "go-goroutine", "fgprof"} {
		if strings.HasSuffix(name, "-"+kind+".pprof") {
			return strings.TrimSuffix(name, "-"+kind+".pprof")
		}
	}
	return ""
}
