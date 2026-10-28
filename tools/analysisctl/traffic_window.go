package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	trafficStartRequests     int64 = 10
	trafficStartConsecutives       = 2
	// The end threshold scales with the observed load so background requests
	// (internal polling and similar) do not hide the end of the load phase.
	trafficEndRequests     int64 = 5
	trafficEndRelativeRate       = 0.05
	trafficEndConsecutives       = 3
	// A low run that reaches the collection end is accepted once it covers this
	// many seconds, because collection often stops within seconds of the load.
	trafficEndBoundarySeconds = 0.5
)

// analysisWindow is the derived window used to materialize time-series data.
// The original collection markers remain in run.json and bench.log.
type analysisWindow struct {
	StartedAt        time.Time
	EndedAt          time.Time
	Source           string
	Status           string
	Reason           string
	RequestCount     int64
	ActiveSeconds    int64
	StartThreshold   int64
	StartConsecutive int64
	EndThreshold     int64
	EndConsecutive   int64
}

type collectionWindow struct {
	StartedAt time.Time
	EndedAt   time.Time
}

// Patterns belong to the contest config; timestamps use the log's ISO prefix.
type loadPhaseConfig struct {
	StartPattern string `yaml:"start_pattern"`
	EndPattern   string `yaml:"end_pattern"`
	start        *regexp.Regexp
	end          *regexp.Regexp
}

func (p *loadPhaseConfig) compile() error {
	if p.StartPattern == "" && p.EndPattern == "" {
		return nil
	}
	if p.StartPattern == "" || p.EndPattern == "" {
		return fmt.Errorf("load_phase requires both start_pattern and end_pattern")
	}
	var err error
	if p.start, err = regexp.Compile(p.StartPattern); err != nil {
		return fmt.Errorf("load_phase.start_pattern: %w", err)
	}
	if p.end, err = regexp.Compile(p.EndPattern); err != nil {
		return fmt.Errorf("load_phase.end_pattern: %w", err)
	}
	return nil
}

func (p loadPhaseConfig) resolve(runDir string, collection collectionWindow) (collectionWindow, bool) {
	if p.start == nil || p.end == nil {
		return collectionWindow{}, false
	}
	body, err := os.ReadFile(filepath.Join(runDir, "bench.log"))
	if err != nil {
		return collectionWindow{}, false
	}
	var start, end time.Time
	var starts, ends int
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, fields[0])
		if err != nil {
			continue
		}
		if p.start.MatchString(line) {
			start, starts = at, starts+1
		}
		if p.end.MatchString(line) {
			end, ends = at, ends+1
		}
	}
	if starts != 1 || ends != 1 || !end.After(start) || start.Before(collection.StartedAt) || end.After(collection.EndedAt) {
		return collectionWindow{}, false
	}
	return collectionWindow{StartedAt: start.UTC(), EndedAt: end.UTC()}, true
}

type manifestWindow struct {
	LoadWindow struct {
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at"`
	} `json:"load_window"`
}

type accessLogSample struct {
	Msec         json.RawMessage `json:"msec"`
	ResponseTime json.RawMessage `json:"response_time"`
}

func resolveAnalysisWindows(dirs, runIDs []string, phase loadPhaseConfig) map[string]analysisWindow {
	windows := make(map[string]analysisWindow, len(runIDs))
	for index, runID := range runIDs {
		dir := ""
		if index < len(dirs) {
			dir = dirs[index]
		}
		if dir == "" {
			dir = filepath.Join("runs", runID)
		}
		windows[runID] = resolveAnalysisWindow(dir, phase)
	}
	return windows
}

func resolveAnalysisWindow(runDir string, phase loadPhaseConfig) analysisWindow {
	base := analysisWindow{
		Source:           "traffic-threshold",
		Status:           "unavailable",
		Reason:           "collection window is unavailable",
		StartThreshold:   trafficStartRequests,
		StartConsecutive: trafficStartConsecutives,
		EndThreshold:     trafficEndRequests,
		EndConsecutive:   trafficEndConsecutives,
	}

	collection, ok, reason := resolveCollectionWindow(runDir)
	if !ok {
		base.Reason = reason
		return base
	}
	declared, explicit := phase.resolve(runDir, collection)
	if explicit {
		base = analysisWindow{
			StartedAt: declared.StartedAt, EndedAt: declared.EndedAt,
			Source: "bench.log-load-phase", Status: "ok",
		}
	}

	paths := accessLogFilesForWindow(runDir)
	if len(paths) == 0 {
		if explicit {
			base.Reason = "load phase declared; access logs are unavailable"
			return base
		}
		return fallbackAnalysisWindow(base, collection, "access logs are unavailable")
	}
	statsWindow := collection
	if explicit {
		statsWindow = declared
	}
	buckets, err := requestStartBuckets(paths, statsWindow)
	if err != nil {
		if explicit {
			base.Reason = "load phase declared; access log parsing failed: " + err.Error()
			return base
		}
		return fallbackAnalysisWindow(base, collection, "access log parsing failed: "+err.Error())
	}
	if explicit {
		// Records were filtered before bucketing, so a fractional first second
		// contains only load requests and must still contribute to the count.
		base.RequestCount, base.ActiveSeconds = windowRequestStats(buckets, declared.StartedAt.Truncate(time.Second), declared.EndedAt)
		return base
	}
	start, end, ok, reason := detectTrafficWindow(collection, buckets)
	if !ok {
		return fallbackAnalysisWindow(base, collection, reason)
	}
	window := base
	window.StartedAt = start
	window.EndedAt = end
	window.Status = "ok"
	window.Reason = reason
	window.EndThreshold = trafficEndThreshold(collection, buckets)
	window.RequestCount, window.ActiveSeconds = windowRequestStats(buckets, start, end)
	return window
}

func fallbackAnalysisWindow(base analysisWindow, collection collectionWindow, reason string) analysisWindow {
	base.StartedAt = collection.StartedAt
	base.EndedAt = collection.EndedAt
	base.Source = "collection-window-fallback"
	base.Status = "ok"
	base.Reason = reason
	return base
}

func resolveCollectionWindow(runDir string) (collectionWindow, bool, string) {
	if body, err := os.ReadFile(filepath.Join(runDir, "bench.log")); err == nil {
		if window, ok := parseBenchMarkers(body); ok {
			return window, true, ""
		}
	}
	if body, err := os.ReadFile(filepath.Join(runDir, "run.json")); err == nil {
		var manifest manifestWindow
		if json.Unmarshal(body, &manifest) == nil {
			start, startErr := time.Parse(time.RFC3339Nano, manifest.LoadWindow.StartedAt)
			end, endErr := time.Parse(time.RFC3339Nano, manifest.LoadWindow.EndedAt)
			if startErr == nil && endErr == nil && end.After(start) {
				return collectionWindow{StartedAt: start.UTC(), EndedAt: end.UTC()}, true, ""
			}
		}
	}
	return collectionWindow{}, false, "bench.log and run.json do not contain a valid collection window"
}

func parseBenchMarkers(body []byte) (collectionWindow, bool) {
	var start, end time.Time
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, fields[0])
		if err != nil {
			continue
		}
		message := strings.Join(fields[1:], " ")
		switch {
		case strings.Contains(message, "BENCHMARK_START"):
			start = at
		case strings.Contains(message, "BENCHMARK_END"):
			end = at
		}
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return collectionWindow{}, false
	}
	return collectionWindow{StartedAt: start.UTC(), EndedAt: end.UTC()}, true
}

func accessLogFilesForWindow(runDir string) []string {
	plain, _ := filepath.Glob(filepath.Join(runDir, "raw", "access-*.log"))
	seen := make(map[string]bool, len(plain))
	for _, path := range plain {
		seen[path] = true
	}
	files := append([]string{}, plain...)
	zstd, _ := filepath.Glob(filepath.Join(runDir, "raw", "access-*.log.zst"))
	for _, path := range zstd {
		if !seen[strings.TrimSuffix(path, ".zst")] {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

func requestStartBuckets(paths []string, window collectionWindow) (map[int64]int64, error) {
	buckets := make(map[int64]int64)
	for _, path := range paths {
		if err := scanAccessLog(path, func(sample accessLogSample) {
			msec, ok := jsonFloat(sample.Msec)
			if !ok || msec < 0 || math.IsNaN(msec) || math.IsInf(msec, 0) {
				return
			}
			responseTime, _ := jsonFloat(sample.ResponseTime)
			if responseTime < 0 || math.IsNaN(responseTime) || math.IsInf(responseTime, 0) {
				responseTime = 0
			}
			requestStartSeconds := msec - responseTime
			if requestStartSeconds < 0 || requestStartSeconds >= float64(math.MaxInt64)/1e9 {
				return
			}
			requestStart := time.Unix(0, int64(math.Round(requestStartSeconds*1e9))).UTC()
			if requestStart.Before(window.StartedAt) || !requestStart.Before(window.EndedAt) {
				return
			}
			buckets[requestStart.Unix()]++
		}); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return buckets, nil
}

func scanAccessLog(path string, visit func(accessLogSample)) error {
	var (
		reader io.Reader
		file   *os.File
		cmd    *exec.Cmd
	)
	if strings.HasSuffix(path, ".zst") {
		cmd = exec.Command("zstdcat", path)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		reader = stdout
	} else {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var sample accessLogSample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			continue
		}
		visit(sample)
	}
	scanErr := scanner.Err()
	if cmd == nil {
		return scanErr
	}
	if scanErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return scanErr
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("decompression failed: %w", err)
	}
	return nil
}

func jsonFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		value, err := strconv.ParseFloat(string(number), 64)
		return value, err == nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	return value, err == nil
}

// Short traffic bursts can precede or follow the sustained workload. Keep the
// threshold interval with the most requests instead of returning the first one.
func detectTrafficWindow(collection collectionWindow, buckets map[int64]int64) (time.Time, time.Time, bool, string) {
	earliest := collection.StartedAt.Unix()
	threshold := trafficEndThreshold(collection, buckets)
	var bestStart, bestEnd time.Time
	var bestCount int64 = -1
	var bestReason string
	for {
		start, end, ok, reason := detectTrafficInterval(collection, buckets, earliest, threshold)
		if !ok {
			if bestCount < 0 {
				return start, end, false, reason
			}
			break
		}
		count, _ := windowRequestStats(buckets, start, end)
		if count > bestCount {
			bestStart, bestEnd, bestCount, bestReason = start, end, count, reason
		}
		if !end.Before(collection.EndedAt) {
			break
		}
		earliest = end.Unix()
	}
	return bestStart, bestEnd, true, bestReason
}

func detectTrafficInterval(collection collectionWindow, buckets map[int64]int64, earliest, endThreshold int64) (time.Time, time.Time, bool, string) {
	startSec := earliest
	lastBucketSec := collection.EndedAt.Unix()
	if collection.EndedAt.Nanosecond() == 0 {
		lastBucketSec--
	}
	if lastBucketSec < startSec {
		return time.Time{}, time.Time{}, false, "collection window is shorter than one second"
	}

	var startSecCandidate int64 = -1
	for sec := startSec; sec+int64(trafficStartConsecutives-1) <= lastBucketSec; sec++ {
		ok := true
		for offset := 0; offset < trafficStartConsecutives; offset++ {
			if buckets[sec+int64(offset)] < trafficStartRequests {
				ok = false
				break
			}
		}
		if ok {
			startSecCandidate = sec
			break
		}
	}
	if startSecCandidate < 0 {
		return time.Time{}, time.Time{}, false, "request rate never reached the start threshold for two consecutive seconds"
	}

	start := time.Unix(startSecCandidate, 0).UTC()
	if start.Before(collection.StartedAt) {
		start = collection.StartedAt
	}
	threshold := float64(endThreshold)
	var lowStart int64 = -1
	var lowSeconds float64
	for sec := startSecCandidate; sec <= lastBucketSec; sec++ {
		coverage := bucketCoverage(collection, sec)
		if float64(buckets[sec]) < threshold*coverage {
			if lowStart < 0 {
				lowStart, lowSeconds = sec, 0
			}
			lowSeconds += coverage
			if sec-lowStart+1 >= trafficEndConsecutives {
				end := time.Unix(lowStart, 0).UTC()
				if end.After(start) {
					return start, end, true, "traffic threshold window detected"
				}
			}
			continue
		}
		lowStart = -1
	}
	if lowStart >= 0 && lowSeconds >= trafficEndBoundarySeconds {
		end := time.Unix(lowStart, 0).UTC()
		if end.After(start) {
			return start, end, true, "traffic end detected at the collection end"
		}
	}

	if collection.EndedAt.After(start) {
		return start, collection.EndedAt, true, "traffic start detected; collection end used because the end threshold was not observed"
	}
	return time.Time{}, time.Time{}, false, "detected traffic start is not before collection end"
}

// trafficEndThreshold is the larger of the fixed floor and a fraction of the
// median per-second rate among seconds that reached the start threshold.
func trafficEndThreshold(collection collectionWindow, buckets map[int64]int64) int64 {
	var rates []int64
	for sec := collection.StartedAt.Unix(); sec <= collection.EndedAt.Unix(); sec++ {
		if bucketCoverage(collection, sec) == 1 && buckets[sec] >= trafficStartRequests {
			rates = append(rates, buckets[sec])
		}
	}
	if len(rates) == 0 {
		return trafficEndRequests
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i] < rates[j] })
	threshold := int64(math.Ceil(float64(rates[len(rates)/2]) * trafficEndRelativeRate))
	if threshold < trafficEndRequests {
		return trafficEndRequests
	}
	return threshold
}

// bucketCoverage is the fraction of a one-second bucket inside the collection
// window; only the final, partially collected second is below one.
func bucketCoverage(collection collectionWindow, sec int64) float64 {
	if sec == collection.EndedAt.Unix() {
		return float64(collection.EndedAt.Nanosecond()) / float64(time.Second)
	}
	return 1
}

func windowRequestStats(buckets map[int64]int64, start, end time.Time) (int64, int64) {
	var requests, activeSeconds int64
	for sec, count := range buckets {
		at := time.Unix(sec, 0).UTC()
		if at.Before(start) || !at.Before(end) {
			continue
		}
		requests += count
		if count >= trafficStartRequests {
			activeSeconds++
		}
	}
	return requests, activeSeconds
}
