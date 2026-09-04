package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const benchmarkStartMarker = "BENCHMARK_START"
const benchmarkEndMarker = "BENCHMARK_END"

func resolveLoadWindow(benchLog []byte) LoadWindow {
	result := LoadWindow{Source: "bench.log", Status: "unavailable"}
	var startedAt, endedAt time.Time
	for _, line := range strings.Split(string(benchLog), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, fields[0])
		if err != nil {
			continue
		}
		message := strings.TrimSpace(fields[len(fields)-1])
		switch {
		case strings.Contains(message, benchmarkStartMarker):
			startedAt = parsed
		case strings.Contains(message, benchmarkEndMarker):
			endedAt = parsed
		}
	}
	if startedAt.IsZero() || endedAt.IsZero() {
		result.Reason = "bench.log does not contain both load start and end markers"
		return result
	}
	if !endedAt.After(startedAt) {
		result.Reason = "load end is not after load start"
		return result
	}
	result.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	result.EndedAt = endedAt.UTC().Format(time.RFC3339Nano)
	result.DurationMS = endedAt.Sub(startedAt).Milliseconds()
	result.Status = "ok"
	return result
}

func periodicArtifactInterval(name string) (time.Duration, bool) {
	switch {
	case strings.HasSuffix(name, "-proc-metrics.tsv"),
		strings.HasSuffix(name, "-service-metrics.tsv"),
		strings.HasSuffix(name, "-disk-metrics.tsv"),
		name == "mysql-status.tsv":
		return time.Second, true
	default:
		return 0, false
	}
}

func assessArtifactQuality(dir string, window LoadWindow, artifacts []Artifact) []Artifact {
	for index := range artifacts {
		artifact := &artifacts[index]
		interval, expected := periodicArtifactInterval(artifact.Name)
		artifact.Quality = ArtifactQuality{
			Expected:  expected,
			Status:    "not-applicable",
			Monotonic: true,
			Finite:    true,
		}
		if !expected {
			if artifact.Status != "ok" {
				artifact.Quality.Status = "invalid"
				artifact.Quality.Reason = artifact.Reason
			}
			continue
		}
		artifact.Quality = inspectPeriodicTSV(filepath.Join(dir, artifact.Name), window, interval)
		if artifact.Status != "ok" {
			artifact.Quality.Status = "invalid"
			artifact.Quality.Reason = joinReasons(artifact.Quality.Reason, artifact.Reason)
		}
	}
	return artifacts
}

func joinReasons(reasons ...string) string {
	var nonEmpty []string
	for _, reason := range reasons {
		if reason = strings.TrimSpace(reason); reason != "" {
			nonEmpty = append(nonEmpty, reason)
		}
	}
	return strings.Join(nonEmpty, "; ")
}

func inspectPeriodicTSV(path string, window LoadWindow, interval time.Duration) ArtifactQuality {
	quality := ArtifactQuality{Expected: true, Status: "invalid", Monotonic: true, Finite: true}
	if window.Status != "ok" {
		quality.Status = "unavailable"
		quality.Reason = "load window is unavailable"
		return quality
	}
	startedAt, startErr := time.Parse(time.RFC3339Nano, window.StartedAt)
	endedAt, endErr := time.Parse(time.RFC3339Nano, window.EndedAt)
	if startErr != nil || endErr != nil || !endedAt.After(startedAt) {
		quality.Status = "unavailable"
		quality.Reason = "load window timestamps are invalid"
		return quality
	}
	file, err := os.Open(path)
	if err != nil {
		quality.Reason = err.Error()
		return quality
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		quality.Reason = fmt.Sprintf("read header: %v", err)
		return quality
	}
	timestampColumn := -1
	for index, name := range header {
		if name == "timestamp" {
			timestampColumn = index
			break
		}
	}
	if timestampColumn < 0 {
		quality.Reason = "timestamp column is missing"
		return quality
	}
	var previous time.Time
	uniqueInWindow := map[int64]time.Time{}
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			quality.Reason = fmt.Sprintf("read row: %v", readErr)
			return quality
		}
		quality.Rows++
		if timestampColumn >= len(record) {
			quality.Monotonic = false
			continue
		}
		timestamp, parseErr := time.Parse(time.RFC3339Nano, record[timestampColumn])
		if parseErr != nil {
			quality.Monotonic = false
			continue
		}
		if !previous.IsZero() && timestamp.Before(previous) {
			quality.Monotonic = false
		}
		previous = timestamp
		if !timestamp.Before(startedAt) && timestamp.Before(endedAt) {
			uniqueInWindow[timestamp.UnixNano()] = timestamp
		}
		for _, raw := range record {
			value, parseFloatErr := strconv.ParseFloat(raw, 64)
			if parseFloatErr == nil && (math.IsNaN(value) || math.IsInf(value, 0)) {
				quality.Finite = false
			}
		}
	}
	points := make([]time.Time, 0, len(uniqueInWindow))
	for _, timestamp := range uniqueInWindow {
		points = append(points, timestamp)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })
	quality.InWindowSamples = int64(len(points))
	quality.ExpectedSamples = int64(endedAt.Sub(startedAt) / interval)
	if quality.ExpectedSamples < 1 {
		quality.ExpectedSamples = 1
	}
	quality.WindowCoveragePct = math.Min(100, float64(quality.InWindowSamples)*100/float64(quality.ExpectedSamples))
	for index := 1; index < len(points); index++ {
		gap := points[index].Sub(points[index-1]).Milliseconds()
		if gap > quality.MaxGapMS {
			quality.MaxGapMS = gap
		}
	}
	minimumSamples := quality.ExpectedSamples - 1
	if minimumSamples < 1 {
		minimumSamples = 1
	}
	var reasons []string
	if quality.InWindowSamples < minimumSamples {
		reasons = append(reasons, fmt.Sprintf("load window has %d/%d expected samples", quality.InWindowSamples, quality.ExpectedSamples))
	}
	if quality.MaxGapMS > (5 * interval / 2).Milliseconds() {
		reasons = append(reasons, fmt.Sprintf("maximum sample gap is %dms", quality.MaxGapMS))
	}
	if !quality.Monotonic {
		reasons = append(reasons, "timestamps are not monotonic")
	}
	if !quality.Finite {
		reasons = append(reasons, "non-finite numeric value detected")
	}
	if len(reasons) == 0 {
		quality.Status = "valid"
	} else {
		quality.Reason = strings.Join(reasons, "; ")
	}
	return quality
}
