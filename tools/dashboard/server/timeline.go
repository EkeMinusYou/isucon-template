package main

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// accessLogLine mirrors the fields of one nginx access-<host>.log JSON line
// that the benchmarker timeline needs (see nginx/ log_format json).
type accessLogLine struct {
	Msec         json.RawMessage `json:"msec"`
	Time         string          `json:"time"`
	Status       json.RawMessage `json:"status"`
	ResponseTime float64         `json:"response_time"`
	BodyBytes    int64           `json:"body_bytes"`
}

func accessLogSecond(rec accessLogLine) (int64, bool) {
	// nginx $msec is epoch seconds with a fractional part, not milliseconds.
	var number json.Number
	if err := json.Unmarshal(rec.Msec, &number); err == nil {
		seconds, err := number.Float64()
		if err == nil && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds >= 0 && seconds < float64(math.MaxInt64) {
			return int64(math.Floor(seconds)), true
		}
	}
	// Preserve historical logs that used an RFC3339 time field.
	t, err := time.Parse(time.RFC3339, rec.Time)
	return t.Unix(), err == nil
}

// accessRecord is a parsed, bucketable access-log entry.
type accessRecord struct {
	unixSec    int64
	status     int
	responseMs float64
	bodyBytes  int64
}

func parseTimelineStatus(raw json.RawMessage) int {
	var numeric int
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return numeric
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if v, err := strconv.Atoi(text); err == nil {
			return v
		}
	}
	return 0
}

// accessLogFiles returns the raw nginx logs for a run.
//
// They live in <run>/raw/ and are zstd-compressed once the digest step has
// read them, so both extensions may appear. When the same log exists in both
// forms (a previous run's .zst left behind next to a freshly fetched .log),
// the uncompressed one wins so the log is not counted twice.
func accessLogFiles(dir string) []string {
	plain := mustGlob(filepath.Join(dir, "raw", "access-*.log"))
	seen := make(map[string]bool, len(plain))
	for _, p := range plain {
		seen[p] = true
	}
	files := append([]string{}, plain...)
	for _, p := range mustGlob(filepath.Join(dir, "raw", "access-*.log.zst")) {
		if !seen[strings.TrimSuffix(p, ".zst")] {
			files = append(files, p)
		}
	}
	return files
}

// openAccessLog opens one access log, transparently decompressing .zst.
// The returned closer releases both the decoder and the file.
func openAccessLog(path string) (io.Reader, io.Closer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if !strings.HasSuffix(path, ".zst") {
		return f, f, nil
	}
	dec, err := zstd.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return dec, closerFunc(func() error {
		dec.Close()
		return f.Close()
	}), nil
}

type closerFunc func() error

func (c closerFunc) Close() error { return c() }

// readAccessRecords parses every access log of a run into a flat list of
// records, ignoring malformed lines.
func readAccessRecords(dir string) ([]accessRecord, error) {
	files := accessLogFiles(dir)
	if len(files) == 0 {
		return nil, nil
	}

	var records []accessRecord
	for _, path := range files {
		r, closer, err := openAccessLog(path)
		if err != nil {
			return nil, err
		}
		f := closer
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var rec accessLogLine
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			second, ok := accessLogSecond(rec)
			if !ok {
				continue
			}
			records = append(records, accessRecord{
				unixSec:    second,
				status:     parseTimelineStatus(rec.Status),
				responseMs: rec.ResponseTime * 1000,
				bodyBytes:  rec.BodyBytes,
			})
		}
		err = scanner.Err()
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}

// timelineBucket is a one-second aggregate of benchmarker HTTP traffic.
type timelineBucket struct {
	ElapsedS         int64   `json:"elapsed_s"`
	Requests         int64   `json:"requests"`
	Status2xx        int64   `json:"status_2xx"`
	Status3xx        int64   `json:"status_3xx"`
	Status4xx        int64   `json:"status_4xx"`
	Status5xx        int64   `json:"status_5xx"`
	AvgResponseMs    float64 `json:"avg_response_ms"`
	P95ResponseMs    float64 `json:"p95_response_ms"`
	BytesPerSec      float64 `json:"bytes_per_sec"`
	ScenarioWarnings int64   `json:"scenario_warnings"`
}

type bucketAccum struct {
	requests    int64
	status2xx   int64
	status3xx   int64
	status4xx   int64
	status5xx   int64
	responseSum float64
	responses   []float64
	bytesSum    int64
	warnings    int64
}

// countScenarioWarnings reads bench.log (tab-separated: timestamp, level,
// logger, message, ...) and returns a map of elapsed-second (relative to
// t0) to the count of warn/error level lines, ignoring anything outside
// [0, maxElapsed].
func countScenarioWarnings(dir string, t0 int64, maxElapsed int64) (map[int64]int64, error) {
	f, err := os.Open(filepath.Join(dir, "bench.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	counts := make(map[int64]int64)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		cols := strings.SplitN(line, "\t", 3)
		if len(cols) < 2 {
			continue
		}
		level := strings.TrimSpace(cols[1])
		if level != "warn" && level != "error" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, cols[0])
		if err != nil {
			continue
		}
		elapsed := t.Unix() - t0
		if elapsed < 0 || elapsed > maxElapsed {
			continue
		}
		counts[elapsed]++
	}
	return counts, scanner.Err()
}

// timelineHostPoint is a compact CPU/load/memory sample aligned to the same
// elapsed-second axis as timelineBucket, using each proc-metrics.tsv row's
// wall-clock timestamp rather than its sampler-local elapsed_ms.
type timelineHostPoint struct {
	ElapsedS   int64   `json:"elapsed_s"`
	CPUBusyPct float64 `json:"cpu_busy_pct"`
	Load1      float64 `json:"load1"`
	MemUsedPct float64 `json:"mem_used_pct"`
}

type timelineHost struct {
	Host   string              `json:"host"`
	Series []timelineHostPoint `json:"series"`
}

func parseTimelineHostSeries(path string, t0 int64) ([]timelineHostPoint, error) {
	header, rows, err := readTSV(path)
	if err != nil {
		return nil, err
	}
	if columnIndex(header, "timestamp") < 0 {
		return nil, nil
	}

	series := make([]timelineHostPoint, 0, len(rows))
	for _, row := range rows {
		c := &colIndexer{header: header, row: row}
		tsIdx := columnIndex(header, "timestamp")
		if tsIdx < 0 || tsIdx >= len(row) {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, row[tsIdx])
		if err != nil {
			continue
		}
		memTotal := c.float("mem_total_bytes")
		memUsed := c.float("mem_used_bytes")
		memUsedPct := 0.0
		if memTotal > 0 {
			memUsedPct = memUsed / memTotal * 100
		}
		series = append(series, timelineHostPoint{
			ElapsedS:   t.Unix() - t0,
			CPUBusyPct: c.float("cpu_busy_pct"),
			Load1:      c.float("load1"),
			MemUsedPct: memUsedPct,
		})
	}
	return series, nil
}

type timelineResponse struct {
	RunID     string           `json:"run_id"`
	Available bool             `json:"available"`
	StartTime string           `json:"start_time"`
	Buckets   []timelineBucket `json:"buckets"`
	Hosts     []timelineHost   `json:"hosts"`
}

func (a *app) handleTimeline(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}

	records, err := readAccessRecords(dir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(records) == 0 {
		writeJSON(w, timelineResponse{RunID: runID, Available: false, Buckets: []timelineBucket{}, Hosts: []timelineHost{}})
		return
	}

	t0 := records[0].unixSec
	maxElapsed := int64(0)
	for _, rec := range records {
		if rec.unixSec < t0 {
			t0 = rec.unixSec
		}
	}
	accums := make(map[int64]*bucketAccum)
	for _, rec := range records {
		elapsed := rec.unixSec - t0
		if elapsed > maxElapsed {
			maxElapsed = elapsed
		}
		acc := accums[elapsed]
		if acc == nil {
			acc = &bucketAccum{}
			accums[elapsed] = acc
		}
		acc.requests++
		acc.responseSum += rec.responseMs
		acc.responses = append(acc.responses, rec.responseMs)
		acc.bytesSum += rec.bodyBytes
		switch rec.status / 100 {
		case 2:
			acc.status2xx++
		case 3:
			acc.status3xx++
		case 4:
			acc.status4xx++
		case 5:
			acc.status5xx++
		}
	}

	warnings, err := countScenarioWarnings(dir, t0, maxElapsed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	buckets := make([]timelineBucket, 0, maxElapsed+1)
	for elapsed := int64(0); elapsed <= maxElapsed; elapsed++ {
		acc := accums[elapsed]
		b := timelineBucket{ElapsedS: elapsed, ScenarioWarnings: warnings[elapsed]}
		if acc != nil {
			sort.Float64s(acc.responses)
			b.Requests = acc.requests
			b.Status2xx = acc.status2xx
			b.Status3xx = acc.status3xx
			b.Status4xx = acc.status4xx
			b.Status5xx = acc.status5xx
			b.AvgResponseMs = acc.responseSum / float64(acc.requests)
			p95Idx := int(float64(len(acc.responses)) * 0.95)
			if p95Idx >= len(acc.responses) {
				p95Idx = len(acc.responses) - 1
			}
			b.P95ResponseMs = acc.responses[p95Idx]
			b.BytesPerSec = float64(acc.bytesSum)
		}
		buckets = append(buckets, b)
	}

	hosts := []timelineHost{}
	for _, f := range mustGlob(filepath.Join(dir, "*-proc-metrics.tsv")) {
		host := strings.TrimSuffix(filepath.Base(f), "-proc-metrics.tsv")
		series, err := parseTimelineHostSeries(f, t0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		hosts = append(hosts, timelineHost{Host: host, Series: series})
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Host < hosts[j].Host })

	writeJSON(w, timelineResponse{
		RunID:     runID,
		Available: true,
		StartTime: time.Unix(t0, 0).UTC().Format(time.RFC3339),
		Buckets:   buckets,
		Hosts:     hosts,
	})
}
