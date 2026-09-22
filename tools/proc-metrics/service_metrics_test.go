package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestParseServiceNames(t *testing.T) {
	got := parseServiceNames("app, nginx.service mysql app.service")
	want := []string{"app.service", "nginx.service", "mysql.service"}
	if len(got) != len(want) {
		t.Fatalf("parseServiceNames() = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("parseServiceNames() = %#v, want %#v", got, want)
		}
	}
}

func TestReadServiceCgroup(t *testing.T) {
	root := t.TempDir()
	servicePath := filepath.Join(root, "system.slice", "nginx.service")
	if err := os.MkdirAll(servicePath, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"cpu.stat":       "usage_usec 3000000\nuser_usec 2100000\nsystem_usec 900000\n",
		"memory.current": "4096\n",
		"memory.peak":    "8192\n",
		"io.stat":        "259:0 rbytes=100 wbytes=200 rios=1 wios=2\n8:0 rbytes=300 wbytes=400\n",
		"pids.current":   "3\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(servicePath, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	collector := newServiceCollector(root, []string{"nginx"})
	values := collector.read()
	got, ok := values["nginx.service"]
	if !ok || !got.available {
		t.Fatalf("service snapshot = %#v, want available snapshot", got)
	}
	if got.memoryCurrent != 4096 || got.memoryPeak != 8192 || got.tasksCurrent != 3 {
		t.Fatalf("service snapshot = %#v", got)
	}
	if got.cpu != (serviceCPUCounters{usageUsec: 3000000, userUsec: 2100000, systemUsec: 900000}) {
		t.Fatalf("service CPU = %#v", got.cpu)
	}
	if got.io.readBytes != 400 || got.io.writeBytes != 600 {
		t.Fatalf("service I/O = %#v", got.io)
	}
}

func TestServiceHeaderAndRowsWidth(t *testing.T) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.Comma = '\t'
	if err := writer.Write(serviceHeader()); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	previousService := serviceSnapshot{
		available: true,
		cpu:       serviceCPUCounters{usageUsec: 1000000, userUsec: 700000, systemUsec: 300000},
		io:        serviceIOCounters{readBytes: 100, writeBytes: 200},
	}
	currentService := serviceSnapshot{
		available: true,
		cpu:       serviceCPUCounters{usageUsec: 3000000, userUsec: 1700000, systemUsec: 1300000},
		io:        serviceIOCounters{readBytes: 500, writeBytes: 800},
	}

	currentAt := time.Unix(10, 0)
	previous := snapshot{at: time.Unix(8, 0)}
	current := snapshot{at: currentAt, memTotal: 1024}
	services := map[string]serviceSnapshot{
		"nginx.service": currentService,
	}
	if err := writeServiceRows(writer, 0, time.Unix(0, 0), current, services, map[string]serviceSnapshot{"nginx.service": previousService}, &previous, []string{"nginx.service"}); err != nil {
		t.Fatal(err)
	}

	reader := csv.NewReader(&output)
	reader.Comma = '\t'
	readHeader, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	readRow, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(readHeader) != len(readRow) {
		t.Fatalf("header width=%d, row width=%d", len(readHeader), len(readRow))
	}
	values := map[string]string{}
	for i, name := range readHeader {
		values[name] = readRow[i]
	}
	for name, want := range map[string]float64{"cpu_pct": 100, "cpu_user_pct": 50, "cpu_system_pct": 50, "io_read_bytes_per_sec": 200, "io_write_bytes_per_sec": 300} {
		got, err := strconv.ParseFloat(values[name], 64)
		if err != nil || got != want {
			t.Fatalf("%s = %q, want %g", name, values[name], want)
		}
	}
}
