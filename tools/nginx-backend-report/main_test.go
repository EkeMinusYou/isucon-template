package main

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestScanGroupsBackendAndCache(t *testing.T) {
	input := strings.NewReader(`{"status":"200","response_time":0.010,"upstream_time":"0.008, 0.002","upstream_addr":"10.0.0.1:8080","upstream_status":"200","cache_status":"MISS"}
{"status":304,"response_time":0.001,"upstream_time":"-","upstream_addr":"","upstream_status":"","cache_status":"HIT"}
bad-json
`)
	groups := make(map[groupKey]*aggregate)
	malformed, err := scan(input, groups)
	if err != nil {
		t.Fatal(err)
	}
	if malformed != 1 {
		t.Fatalf("malformed=%d, want 1", malformed)
	}
	miss := groups[groupKey{upstreamAddr: "10.0.0.1:8080", upstreamStatus: "200", cacheStatus: "MISS"}]
	if miss == nil || miss.requests != 1 || miss.status2xx != 1 || miss.upstreamTimeSum < 0.0099 || miss.upstreamTimeSum > 0.0101 {
		t.Fatalf("unexpected miss group: %#v", miss)
	}
	hit := groups[groupKey{upstreamAddr: "NONE", upstreamStatus: "NONE", cacheStatus: "HIT"}]
	if hit == nil || hit.requests != 1 || hit.status3xx != 1 {
		t.Fatalf("unexpected hit group: %#v", hit)
	}
}

func TestWriteReport(t *testing.T) {
	var output bytes.Buffer
	groups := map[groupKey]*aggregate{
		{upstreamAddr: "10.0.0.1:8080", upstreamStatus: "200", cacheStatus: "MISS"}: {
			requests:        2,
			status2xx:       2,
			responseTimeSum: 0.020,
			upstreamTimeSum: 0.016,
		},
	}
	if err := writeReport(&output, groups); err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(output.String()))
	reader.Comma = '\t'
	header, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	row, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(header) != 13 || len(row) != len(header) {
		t.Fatalf("header width=%d row width=%d", len(header), len(row))
	}
	if row[9] != "20.000" || row[10] != "10.000" || row[11] != "16.000" || row[12] != "8.000" {
		t.Fatalf("unexpected report timing columns: %v", row)
	}
}

func TestWriteEmptyReportGroupsByIngress(t *testing.T) {
	var output bytes.Buffer
	if err := writeReportWithIngress(&output, map[groupKey]*aggregate{}, true); err != nil {
		t.Fatal(err)
	}
	header := strings.Split(strings.TrimSpace(output.String()), "\t")
	if len(header) != 14 || header[0] != "ingress_host" {
		t.Fatalf("unexpected header: %v", header)
	}
}

func TestWriteReportGroupsByIngress(t *testing.T) {
	var output bytes.Buffer
	groups := make(map[groupKey]*aggregate)
	for _, host := range []string{"isucon-1", "isucon-2"} {
		if _, err := scanWithIngress(strings.NewReader(`{"status":200,"response_time":0.01,"upstream_time":"0.008","upstream_addr":"10.0.0.1:8080","upstream_status":"200"}`), host, groups); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeReportWithIngress(&output, groups, true); err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(output.String()))
	reader.Comma = '\t'
	header, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(header) != 14 || header[0] != "ingress_host" {
		t.Fatalf("unexpected header: %v", header)
	}
	first, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	second, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if first[0] != "isucon-1" || second[0] != "isucon-2" {
		t.Fatalf("unexpected ingress rows: %v / %v", first, second)
	}
}

func TestIngressHostFromPath(t *testing.T) {
	for _, path := range []string{"runs/x/raw/access-isucon-1.log", "runs/x/raw/access-isucon-1.log.zst"} {
		host, err := ingressHostFromPath(path)
		if err != nil || host != "isucon-1" {
			t.Fatalf("path=%s host=%s err=%v", path, host, err)
		}
	}
}
