package main

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestParseProcStat(t *testing.T) {
	input := `cpu  100 20 30 400 50 6 7 8 0 0
ctxt 1234
intr 5678 1 2
procs_running 3
procs_blocked 4
`

	cpu, ctxt, intr, running, blocked, err := parseProcStat(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if cpu.user != 120 || cpu.system != 43 || cpu.idle != 400 || cpu.iowait != 50 || cpu.steal != 8 {
		t.Fatalf("unexpected cpu counters: %+v", cpu)
	}
	if ctxt != 1234 || intr != 5678 || running != 3 || blocked != 4 {
		t.Fatalf("unexpected proc counters: ctxt=%d intr=%d running=%d blocked=%d", ctxt, intr, running, blocked)
	}
}

func TestParseMeminfo(t *testing.T) {
	values, err := parseMeminfo(strings.NewReader("MemTotal:       1024 kB\nMemAvailable:    512 kB\nHugePages_Total: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if values["MemTotal"] != 1024*1024 || values["MemAvailable"] != 512*1024 || values["HugePages_Total"] != 2 {
		t.Fatalf("unexpected memory values: %#v", values)
	}
}

func TestParseNetDev(t *testing.T) {
	input := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 100 10 1 2 0 0 0 0 200 20 3 4 0 0 0 0
      lo: 300 30 5 6 0 0 0 0 400 40 7 8 0 0 0 0
`

	values, err := parseNetDev(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if values.rxBytes != 400 || values.txBytes != 600 || values.rxErrors != 6 || values.txDrops != 12 {
		t.Fatalf("unexpected network values: %+v", values)
	}
}

func TestParsePressure(t *testing.T) {
	input := "some avg10=1.25 avg60=2.50 avg300=3.75 total=100\nfull avg10=0.50 avg60=1.00 avg300=1.50 total=20\n"
	values := parsePressure(strings.NewReader(input))
	if values.someAvg10 != 1.25 || values.fullAvg10 != 0.50 {
		t.Fatalf("unexpected pressure values: %+v", values)
	}
}

func TestCounterRate(t *testing.T) {
	if got := counterRate(150, 100, 2); got != 25 {
		t.Fatalf("counterRate() = %v, want 25", got)
	}
	if got := counterRate(100, 150, 2); got != 0 {
		t.Fatalf("counterRate() after reset = %v, want 0", got)
	}
}

func TestHeaderAndRowWidth(t *testing.T) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.Comma = '\t'
	if err := writer.Write(header()); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	if err := writeRow(writer, 0, time.Unix(0, 0), snapshot{at: time.Unix(0, 0)}, nil); err != nil {
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

func TestParseDiskstats(t *testing.T) {
	input := "8 0 sda 10 2 20 40 30 4 40 80 2 300 500 6 1 8 16 7 70\n"
	values, devices, err := parseDiskstats(strings.NewReader(input), map[string]struct{}{"sda": {}})
	if err != nil {
		t.Fatal(err)
	}
	if values.readBytes != 20*sectorSizeBytes || values.writeBytes != 40*sectorSizeBytes || values.ioInProgress != 2 || values.ioTimeMillis != 300 {
		t.Fatalf("unexpected disk values: %+v", values)
	}
	sda := devices["sda"]
	if sda.readsCompleted != 10 || sda.readTimeMillis != 40 || sda.writesCompleted != 30 || sda.writeTimeMillis != 80 || sda.weightedIOTimeMillis != 500 {
		t.Fatalf("unexpected per-device values: %+v", sda)
	}
	if sda.discardsCompleted != 6 || sda.discardBytes != 8*sectorSizeBytes || sda.flushesCompleted != 7 || sda.flushTimeMillis != 70 {
		t.Fatalf("unexpected optional disk values: %+v", sda)
	}
}

func TestDiskDeviceMetrics(t *testing.T) {
	previous := diskCounters{
		readsCompleted: 10, readBytes: 1000, readTimeMillis: 40,
		writesCompleted: 20, writeBytes: 2000, writeTimeMillis: 100,
		ioTimeMillis: 200, weightedIOTimeMillis: 300,
	}
	current := diskCounters{
		readsCompleted: 14, readBytes: 3048, readTimeMillis: 60,
		writesCompleted: 22, writeBytes: 6096, writeTimeMillis: 120,
		ioTimeMillis: 1200, weightedIOTimeMillis: 2300,
	}
	got := diskDeviceMetrics(current, &previous, 2)
	if got.readIOPS != 2 || got.writeIOPS != 1 {
		t.Fatalf("unexpected IOPS: %+v", got)
	}
	if got.readAwaitMillis != 5 || got.writeAwaitMillis != 10 {
		t.Fatalf("unexpected await: %+v", got)
	}
	if got.avgReadRequestBytes != 512 || got.avgWriteRequestBytes != 2048 {
		t.Fatalf("unexpected request size: %+v", got)
	}
	if got.ioUtilPct != 50 || got.avgQueueSize != 1 {
		t.Fatalf("unexpected utilization/queue: %+v", got)
	}
}

func TestDiskHeaderAndRowsWidth(t *testing.T) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.Comma = '\t'
	if err := writer.Write(diskHeader()); err != nil {
		t.Fatal(err)
	}
	current := snapshot{at: time.Unix(1, 0), disks: map[string]diskCounters{"sda": {ioInProgress: 2}}}
	previous := snapshot{at: time.Unix(0, 0), disks: map[string]diskCounters{"sda": {}}}
	if err := writeDiskRows(writer, 1, time.Unix(0, 0), current, &previous); err != nil {
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
