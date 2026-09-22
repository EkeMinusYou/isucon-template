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
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	trafficStartRequests     int64 = 10
	trafficStartConsecutives       = 2
	trafficEndRequests       int64 = 5
	trafficEndConsecutives         = 3
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

func resolveAnalysisWindows(dirs, runIDs []string) map[string]analysisWindow {
	windows := make(map[string]analysisWindow, len(runIDs))
	for index, runID := range runIDs {
		dir := ""
		if index < len(dirs) {
			dir = dirs[index]
		}
		if dir == "" {
			dir = filepath.Join("runs", runID)
		}
		windows[runID] = resolveAnalysisWindow(dir)
	}
	return windows
}

func resolveAnalysisWindow(runDir string) analysisWindow {
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

	paths := accessLogFilesForWindow(runDir)
	if len(paths) == 0 {
		return fallbackAnalysisWindow(base, collection, "access logs are unavailable")
	}
	buckets, err := requestStartBuckets(paths, collection)
	if err != nil {
		return fallbackAnalysisWindow(base, collection, "access log parsing failed: "+err.Error())
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

func detectTrafficWindow(collection collectionWindow, buckets map[int64]int64) (time.Time, time.Time, bool, string) {
	startSec := collection.StartedAt.Unix()
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

	var lowStart int64 = -1
	for sec := startSecCandidate; sec <= lastBucketSec; sec++ {
		if buckets[sec] < trafficEndRequests {
			if lowStart < 0 {
				lowStart = sec
			}
			if sec-lowStart+1 >= trafficEndConsecutives {
				start := time.Unix(startSecCandidate, 0).UTC()
				if start.Before(collection.StartedAt) {
					start = collection.StartedAt
				}
				end := time.Unix(lowStart, 0).UTC()
				if end.After(start) {
					return start, end, true, "traffic threshold window detected"
				}
			}
			continue
		}
		lowStart = -1
	}

	start := time.Unix(startSecCandidate, 0).UTC()
	if collection.EndedAt.After(start) {
		return start, collection.EndedAt, true, "traffic start detected; collection end used because the end threshold was not observed"
	}
	return time.Time{}, time.Time{}, false, "detected traffic start is not before collection end"
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
