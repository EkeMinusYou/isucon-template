package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"strings"
	"testing"
)

func testSnapshot(captured int64, waitCount int64, waitDurationNS int64) snapshot {
	return snapshot{
		Version:        1,
		StartedUnixMS:  1000,
		CapturedUnixMS: captured,
		Pools: []pool{{
			DatabaseHost:       "127.0.0.1",
			Name:               "user",
			Role:               "shard",
			Shard:              0,
			MaxOpenConnections: 3,
			OpenConnections:    3,
			InUse:              2,
			Idle:               1,
			WaitCount:          waitCount,
			WaitDurationNS:     waitDurationNS,
		}},
	}
}

func TestValidateSnapshot(t *testing.T) {
	if err := validateSnapshot(testSnapshot(1000, 0, 0), 128, defaultMaxMetrics, 10); err != nil {
		t.Fatalf("validate snapshot: %v", err)
	}
	invalid := testSnapshot(1000, 0, 0)
	invalid.Pools[0].Name = "user\tsecret"
	if err := validateSnapshot(invalid, 128, defaultMaxMetrics, 10); err == nil {
		t.Fatal("accepted an unsafe label")
	}
	invalid = testSnapshot(1000, 0, 0)
	invalid.Pools[0].InUse = 4
	if err := validateSnapshot(invalid, 128, defaultMaxMetrics, 10); err == nil {
		t.Fatal("accepted usage above open connections")
	}
}

func TestSnapshotMetricsAreBoundedAndRetained(t *testing.T) {
	s := testSnapshot(1000, 0, 0)
	s.Metrics = map[string]float64{"events_total": 12, "pending_bytes": 34}
	if err := validateSnapshot(s, 128, defaultMaxMetrics, 100); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	w := csv.NewWriter(&output)
	w.Comma = '\t'
	c := &collector{output: w, previous: make(map[string]previousPool)}
	if err := c.write(s); err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{"local\tsnapshot\tmetrics\t0\tevents_total\t12.000000", "local\tsnapshot\tmetrics\t0\tpending_bytes\t34.000000"} {
		if !strings.Contains(output.String(), row) {
			t.Fatal("snapshot metric was dropped")
		}
	}
	s.Metrics["bad"] = math.Inf(1)
	if err := validateSnapshot(s, 128, defaultMaxMetrics, 100); err == nil {
		t.Fatal("nonfinite metric accepted")
	}
}

func TestWriteComputesPoolWaitRates(t *testing.T) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.Comma = '\t'
	collector := &collector{output: writer, previous: make(map[string]previousPool)}
	if err := collector.write(testSnapshot(1000, 0, 0)); err != nil {
		t.Fatalf("write first sample: %v", err)
	}
	if err := collector.write(testSnapshot(2000, 2, 5_000_000)); err != nil {
		t.Fatalf("write second sample: %v", err)
	}
	writer.Flush()
	reader := csv.NewReader(strings.NewReader(output.String()))
	reader.Comma = '\t'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(rows) != 2*13 {
		t.Fatalf("rows = %d, want %d", len(rows), 2*13)
	}
	countRateChecked := false
	averageWaitChecked := false
	for _, row := range rows {
		if len(row) >= 8 && row[7] == "wait_count_per_sec" && row[8] != "2.000000" {
			if row[8] != "0.000000" {
				t.Fatalf("wait_count_per_sec = %s", row[8])
			}
		} else if len(row) >= 8 && row[7] == "wait_count_per_sec" && row[8] == "2.000000" {
			countRateChecked = true
		}
		if len(row) >= 8 && row[7] == "average_wait_ms" && row[8] != "2.500000" {
			if row[8] != "0.000000" {
				t.Fatalf("average_wait_ms = %s", row[8])
			}
		} else if len(row) >= 8 && row[7] == "average_wait_ms" && row[8] == "2.500000" {
			averageWaitChecked = true
		}
	}
	if !countRateChecked || !averageWaitChecked {
		t.Fatalf("derived wait metrics were not observed: count=%t average=%t", countRateChecked, averageWaitChecked)
	}
}

func TestSnapshotMetricAndPoolLimitsAreIndependent(t *testing.T) {
	s := testSnapshot(1000, 0, 0)
	s.Metrics = make(map[string]float64)
	for i := 0; i < 129; i++ {
		s.Metrics[fmt.Sprintf("metric_%d", i)] = float64(i)
	}
	if err := validateSnapshot(s, 1, 129, 100); err != nil {
		t.Fatal(err)
	}
	if err := validateSnapshot(s, 1, 128, 100); err == nil {
		t.Fatal("metric limit ignored")
	}
	s.Pools = append(s.Pools, s.Pools[0])
	if err := validateSnapshot(s, 1, 1024, 100); err == nil {
		t.Fatal("metric limit relaxed pool bound")
	}
}
