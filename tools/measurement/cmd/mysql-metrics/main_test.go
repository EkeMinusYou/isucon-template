package main

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestMySQLMetricsCollectorRows(t *testing.T) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.Comma = '\t'
	collector := &collector{
		interval: time.Second,
		output:   writer,
		start:    time.Unix(0, 0),
		previous: &statusSnapshot{at: time.Unix(1, 0), values: map[string]uint64{"Innodb_buffer_pool_read_requests": 50, "Innodb_buffer_pool_reads": 1, "Created_tmp_tables": 5, "Created_tmp_disk_tables": 1}},
	}
	current := statusSnapshot{
		at: time.Unix(3, 0),
		values: map[string]uint64{
			"Innodb_buffer_pool_read_requests":  150,
			"Innodb_buffer_pool_reads":          5,
			"Created_tmp_tables":                15,
			"Created_tmp_disk_tables":           3,
			"Max_used_connections":              7,
			"Connection_errors_max_connections": 4,
		},
		maxConnections: 100,
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
	values := map[string]string{}
	for i, name := range header() {
		values[name] = row[i]
	}
	for name, want := range map[string]string{
		"buffer_pool_read_requests_per_sec":         "50.000",
		"buffer_pool_hit_pct":                       "96.000",
		"tmp_disk_ratio_pct":                        "20.000",
		"max_connections":                           "100",
		"max_used_connections":                      "7",
		"connection_errors_max_connections_total":   "4",
		"connection_errors_max_connections_per_sec": "2.000",
	} {
		if values[name] != want {
			t.Fatalf("%s = %q, want %q", name, values[name], want)
		}
	}
}
