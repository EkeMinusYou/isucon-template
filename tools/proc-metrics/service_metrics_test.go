package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
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

func TestParseServiceCPUStat(t *testing.T) {
	input := `usage_usec 3000000
user_usec 2100000
system_usec 900000
nr_periods 10
`

	got, err := parseServiceCPUStat(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := serviceCPUCounters{usageUsec: 3000000, userUsec: 2100000, systemUsec: 900000}
	if got != want {
		t.Fatalf("parseServiceCPUStat() = %#v, want %#v", got, want)
	}
}

func TestParseServiceIOStat(t *testing.T) {
	input := `259:0 rbytes=100 wbytes=200 rios=1 wios=2
8:0 rbytes=300 wbytes=400
`

	got, err := parseServiceIOStat(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := serviceIOCounters{readBytes: 400, writeBytes: 600}
	if got != want {
		t.Fatalf("parseServiceIOStat() = %#v, want %#v", got, want)
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
		"io.stat":        "259:0 rbytes=100 wbytes=200\n",
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
	if got.io.readBytes != 100 || got.io.writeBytes != 200 {
		t.Fatalf("service I/O = %#v", got.io)
	}
}

func TestServiceCounterRates(t *testing.T) {
	previous := serviceSnapshot{
		available: true,
		cpu:       serviceCPUCounters{usageUsec: 1000000, userUsec: 700000, systemUsec: 300000},
		io:        serviceIOCounters{readBytes: 100, writeBytes: 200},
	}
	current := serviceSnapshot{
		available: true,
		cpu:       serviceCPUCounters{usageUsec: 3000000, userUsec: 1700000, systemUsec: 1300000},
		io:        serviceIOCounters{readBytes: 500, writeBytes: 800},
	}

	got := serviceCounterRates(current, &previous, 2)
	if got.cpuPct != 100 || got.cpuUserPct != 50 || got.cpuSystemPct != 50 {
		t.Fatalf("service CPU rates = %#v", got)
	}
	if got.ioReadBytes != 200 || got.ioWriteBytes != 300 {
		t.Fatalf("service I/O rates = %#v", got)
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
	currentAt := time.Unix(10, 0)
	current := snapshot{at: currentAt, memTotal: 1024}
	services := map[string]serviceSnapshot{
		"nginx.service": {available: true, memoryCurrent: 128, memoryPeak: 256, tasksCurrent: 2},
	}
	if err := writeServiceRows(writer, 0, time.Unix(0, 0), current, services, nil, nil, []string{"nginx.service"}); err != nil {
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
}
