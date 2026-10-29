package main

import (
	"math"
	"sort"
	"strconv"
	"time"
)

type runtimePoint struct {
	ElapsedMs int64              `json:"elapsed_ms"`
	Timestamp string             `json:"timestamp"`
	Values    map[string]float64 `json:"values"`
}

type runtimeMetrics struct {
	Host   string         `json:"host"`
	Series []runtimePoint `json:"series"`
}

// Keep the collector's sparse metric map: absent values are not zero samples.
func parseRuntimeSeries(path string, window *analysisWindow) ([]runtimePoint, error) {
	header, rows, err := readTSV(path)
	if err != nil {
		return nil, err
	}
	indices := make(map[string]int)
	for _, name := range []string{"timestamp", "elapsed_ms", "metric", "value"} {
		indices[name] = columnIndex(header, name)
		if indices[name] < 0 {
			return []runtimePoint{}, nil
		}
	}
	points := make(map[time.Time]*runtimePoint)
	for _, row := range rows {
		valid := true
		for _, index := range indices {
			if index >= len(row) {
				valid = false
			}
		}
		if !valid {
			continue
		}
		timestamp, err := time.Parse(time.RFC3339Nano, row[indices["timestamp"]])
		if err != nil || (window != nil && !window.contains(timestamp)) {
			continue
		}
		value, err := strconv.ParseFloat(row[indices["value"]], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		elapsed, err := strconv.ParseInt(row[indices["elapsed_ms"]], 10, 64)
		if err != nil || row[indices["metric"]] == "" {
			continue
		}
		timestamp = timestamp.UTC()
		point := points[timestamp]
		if point == nil {
			point = &runtimePoint{ElapsedMs: elapsed, Timestamp: timestamp.Format(time.RFC3339Nano), Values: make(map[string]float64)}
			points[timestamp] = point
		}
		point.Values[row[indices["metric"]]] = value
	}
	times := make([]time.Time, 0, len(points))
	for timestamp := range points {
		times = append(times, timestamp)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	series := make([]runtimePoint, 0, len(times))
	for _, timestamp := range times {
		series = append(series, *points[timestamp])
	}
	return series, nil
}
