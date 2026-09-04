package main

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestStatusQueryContainsAllSelectedMetrics(t *testing.T) {
	for _, name := range statusNames {
		if !strings.Contains(statusQuery, "'"+name+"'") {
			t.Fatalf("status query does not contain %q: %s", name, statusQuery)
		}
	}
}

func TestMySQLMetricsCollectorRowWidth(t *testing.T) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.Comma = '\t'
	collector := &collector{
		interval: time.Second,
		output:   writer,
		start:    time.Unix(0, 0),
	}
	current := statusSnapshot{
		at: time.Unix(1, 0),
		values: map[string]uint64{
			"Innodb_buffer_pool_read_requests": 100,
			"Innodb_buffer_pool_reads":         1,
			"Created_tmp_tables":               10,
			"Created_tmp_disk_tables":          2,
		},
	}
	if err := collector.write(current); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	reader := csv.NewReader(strings.NewReader(output.String()))
	reader.Comma = '\t'
	row, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(row) != len(header()) {
		t.Fatalf("row width=%d, header width=%d", len(row), len(header()))
	}
}

func TestMySQLMetricRates(t *testing.T) {
	if got := formatRate(50, 2); got != "25.000" {
		t.Fatalf("formatRate()=%q, want 25.000", got)
	}
	if got := formatHitRate(100, 4, 1); got != "96.000" {
		t.Fatalf("formatHitRate()=%q, want 96.000", got)
	}
	if got := formatRatio(2, 10); got != "20.000" {
		t.Fatalf("formatRatio()=%q, want 20.000", got)
	}
}
