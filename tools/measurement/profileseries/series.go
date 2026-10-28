// Package profileseries defines bounded, timestamped Go snapshot recordings.
package profileseries

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/google/pprof/profile"
)

const (
	DefaultInterval = 2 * time.Second
	MaxProfileBytes = 8 << 20
	MaxBundleBytes  = 64 << 20
	MaxEntries      = 4000
)

type Entry struct {
	Name       string `json:"name"`
	CapturedAt int64  `json:"captured_unix_nano"`
	DurationNS int64  `json:"capture_duration_ns"`
}

type Index struct {
	Version    int     `json:"version"`
	StartedAt  int64   `json:"started_unix_nano"`
	EndedAt    int64   `json:"ended_unix_nano"`
	IntervalNS int64   `json:"interval_ns"`
	Status     string  `json:"status"`
	Error      string  `json:"error,omitempty"`
	Entries    []Entry `json:"entries"`
}

var memberPattern = regexp.MustCompile(`^([0-9]{6})-(go-heap|go-goroutine)\.pprof$`)

func Kind(name string) string {
	m := memberPattern.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return m[2]
}

// Read validates the complete recording. Visitors may stream results into
// temporary files; callers must discard them if final validation fails.
func Read(path string, visit func(Entry, *profile.Profile) error) (Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return Index{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Index{}, err
	}
	if stat.Size() > MaxBundleBytes {
		return Index{}, fmt.Errorf("snapshot archive exceeds byte limit")
	}
	tr := tar.NewReader(f)
	seen := map[string]int64{}
	var index Index
	indexed := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return index, err
		}
		if indexed {
			return index, fmt.Errorf("data after snapshot index")
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > MaxProfileBytes {
			return index, fmt.Errorf("invalid snapshot member %q", h.Name)
		}
		body, err := io.ReadAll(io.LimitReader(tr, MaxProfileBytes+1))
		if err != nil {
			return index, err
		}
		if h.Name == "index.json" {
			if err := json.Unmarshal(body, &index); err != nil {
				return index, err
			}
			indexed = true
			continue
		}
		kind := Kind(h.Name)
		if kind == "" || seen[h.Name] != 0 || len(seen) >= MaxEntries {
			return index, fmt.Errorf("invalid or duplicate snapshot %q", h.Name)
		}
		p, err := profile.ParseData(body)
		if err == nil {
			err = p.CheckValid()
		}
		if err != nil {
			return index, fmt.Errorf("snapshot %s: %w", h.Name, err)
		}
		if p.TimeNanos <= 0 {
			return index, fmt.Errorf("snapshot timestamp missing")
		}
		expected := "inuse_space"
		if kind == "go-goroutine" {
			expected = "goroutine"
		}
		found := false
		for _, t := range p.SampleType {
			if t != nil && t.Type == expected {
				found = true
			}
		}
		if !found {
			return index, fmt.Errorf("snapshot %s lacks %s", h.Name, expected)
		}
		seen[h.Name] = p.TimeNanos
		if visit != nil {
			if err := visit(Entry{Name: h.Name, CapturedAt: p.TimeNanos}, p); err != nil {
				return index, err
			}
		}
	}
	if !indexed || index.Version != 1 || index.Status != "complete" || index.Error != "" || index.IntervalNS <= 0 || index.EndedAt <= index.StartedAt || len(index.Entries) == 0 || len(index.Entries) != len(seen) {
		return index, fmt.Errorf("incomplete snapshot recording: %s %s", index.Status, index.Error)
	}
	for _, e := range index.Entries {
		if seen[e.Name] != e.CapturedAt || e.CapturedAt < index.StartedAt || e.CapturedAt > index.EndedAt || e.DurationNS < 0 {
			return index, fmt.Errorf("snapshot index mismatch for %s", e.Name)
		}
		delete(seen, e.Name)
	}
	if len(seen) != 0 {
		return index, fmt.Errorf("snapshot index contains duplicate entries")
	}
	for _, kind := range []string{"go-heap", "go-goroutine"} {
		var previous int64
		count := 0
		for _, e := range index.Entries {
			if Kind(e.Name) != kind {
				continue
			}
			count++
			if previous != 0 && (e.CapturedAt <= previous || e.CapturedAt-previous > 3*index.IntervalNS) {
				return index, fmt.Errorf("missing or non-monotonic snapshot observations")
			}
			previous = e.CapturedAt
		}
		if count < 2 {
			return index, fmt.Errorf("fewer than two snapshots for %s", kind)
		}
	}
	return index, nil
}
