package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EkeMinusYou/isucon-template/tools/measurement/profileseries"
	"github.com/google/pprof/profile"
)

func TestSnapshotImportExcludesInitializationAndUsesAllocationDifferences(t *testing.T) {
	r, dir := profileTestRunner(t)
	base := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(dir, "host-go-snapshots.tar")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(file)
	idx := profileseries.Index{Version: 1, StartedAt: base.Add(-time.Second).UnixNano(), EndedAt: base.Add(9 * time.Second).UnixNano(), IntervalNS: int64(2 * time.Second), Status: "complete"}
	fn := &profile.Function{ID: 1, Name: "example/work", Filename: "work.go"}
	loc := &profile.Location{ID: 1, Line: []profile.Line{{Function: fn}}}
	for seq, sec := range []int{0, 2, 4, 6, 8} {
		for _, kind := range []string{"go-heap", "go-goroutine"} {
			p := &profile.Profile{TimeNanos: base.Add(time.Duration(sec) * time.Second).UnixNano(), Function: []*profile.Function{fn}, Location: []*profile.Location{loc}}
			if kind == "go-heap" {
				p.SampleType = []*profile.ValueType{{Type: "inuse_space", Unit: "bytes"}, {Type: "alloc_space", Unit: "bytes"}}
				p.Sample = []*profile.Sample{{Location: []*profile.Location{loc}, Value: []int64{int64(sec + 10), 1000 + int64(sec*5)}}}
			} else {
				p.SampleType = []*profile.ValueType{{Type: "goroutine", Unit: "count"}}
				value := int64(999)
				if sec == 4 {
					value = 5
				}
				if sec == 6 {
					value = 1
				}
				p.Sample = []*profile.Sample{{Location: []*profile.Location{loc}, Value: []int64{value}}}
			}
			var data bytes.Buffer
			if err := p.Write(&data); err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("%06d-%s.pprof", seq, kind)
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(data.Len())}); err != nil {
				t.Fatal(err)
			}
			tw.Write(data.Bytes())
			idx.Entries = append(idx.Entries, profileseries.Entry{Name: name, CapturedAt: p.TimeNanos})
		}
	}
	body, _ := json.Marshal(idx)
	tw.WriteHeader(&tar.Header{Name: "index.json", Mode: 0600, Size: int64(len(body))})
	tw.Write(body)
	tw.Close()
	file.Close()
	// The declared load interval contains t=4 and t=6 only. The large t=0/t=8
	// goroutine counts and the initial 1000 allocated bytes must not be imported.
	r.config.loadPhase = loadPhaseConfig{StartPattern: "LOAD_START", EndPattern: "LOAD_END"}
	r.config.loadPhase.compile()
	writeTestFile(t, dir, "bench.log", "2026-09-06T12:00:00Z\tBENCHMARK_START\n2026-09-06T12:00:03Z\tLOAD_START\n2026-09-06T12:00:07Z\tLOAD_END\n2026-09-06T12:00:10Z\tBENCHMARK_END\n")
	if err := r.sync(); err != nil {
		t.Fatal(err)
	}
	profileSQL(t, r, "select count(*),min(total_value),max(total_value) from go_profile_points where sample_type='goroutine'", "2|1|5")
	profileSQL(t, r, "select count(*),sum(total_value) from go_profile_points where sample_type='alloc_space_delta'", "1|10")
	profileSQL(t, r, "select count(*) from go_profile_points where captured_at_ns<"+fmt.Sprint(base.Add(3*time.Second).UnixNano())+" or captured_at_ns>="+fmt.Sprint(base.Add(7*time.Second).UnixNano()), "0")
	profileSQL(t, r, "select count(*) from go_profile_point_functions where cumulative_value=999", "0")
	profileSQL(t, r, "select count(*) from pprof_frames", "0")
}

func TestProfileWindowExcludesWholeLifecycleCPUAndUntimedCaptures(t *testing.T) {
	base := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	w := analysisWindow{Status: "ok", StartedAt: base.Add(3 * time.Second), EndedAt: base.Add(7 * time.Second)}
	for _, tc := range []struct {
		name         string
		at, duration time.Duration
		want         bool
	}{
		{"whole-go-cpu.pprof", 0, 10 * time.Second, false},
		{"inside-go-cpu.pprof", 4 * time.Second, time.Second, true},
		{"before-go-heap.pprof", 2 * time.Second, 0, false},
		{"inside-go-heap.pprof", 4 * time.Second, 0, true},
		{"cumulative-go-allocs.pprof", 4 * time.Second, 0, false},
	} {
		path := filepath.Join(t.TempDir(), tc.name)
		p := &profile.Profile{TimeNanos: base.Add(tc.at).UnixNano(), DurationNanos: int64(tc.duration), SampleType: []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}}}
		f, _ := os.Create(path)
		p.Write(f)
		f.Close()
		if got := profileWithinWindow(path, w); got != tc.want {
			t.Fatalf("%s: included=%v", tc.name, got)
		}
	}
}

func TestRuntimeCounterRatesDoNotCrossProcessRestart(t *testing.T) {
	r, dir := profileTestRunner(t)
	cfg, err := loadConfig("../analysis/sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range cfg.Sources {
		if source.View == "metrics_sql_pool" {
			r.config.Sources = append(r.config.Sources, source)
		}
	}
	r.config.SemanticSchemas = []string{"schema/go-runtime.sql"}
	writeTestFile(t, filepath.Dir(filepath.Dir(dir)), "base.sql", "set variable run_glob = getenv('ISUCON_RUN_GLOB');\ncreate view runs as select '20260906-120000'::varchar as run_id;")
	header := "sample\ttimestamp\telapsed_ms\tdatabase_host\tpool\trole\tshard\tmetric\tvalue\n"
	body := header
	for i, v := range []int{100, 120, 500, 530, 5} {
		generation := 1
		if i >= 2 {
			generation = 2
		}
		at := fmt.Sprintf("2026-09-06T12:00:%02dZ", i+1)
		body += fmt.Sprintf("%d\t%s\t%d\t-\tapp\t-\t0\tgo_process_started_unix_ms\t%d\n", i, at, (i+1)*1000, generation)
		body += fmt.Sprintf("%d\t%s\t%d\t-\tapp\t-\t0\tgo_alloc_bytes_total\t%d\n", i, at, (i+1)*1000, v)
	}
	writeTestFile(t, dir, "host-sql-pool-metrics.tsv", body)
	if err := r.sync(); err != nil {
		t.Fatal(err)
	}
	profileSQL(t, r, "select count(delta),sum(delta),sum(per_second) from go_runtime_series where metric='go_alloc_bytes_total'", "2|50.0|50.0")
	profileSQL(t, r, "select measured_delta,measured_seconds,measured_per_second from go_runtime_window where metric='go_alloc_bytes_total'", "50.0|2.0|25.0")
}
