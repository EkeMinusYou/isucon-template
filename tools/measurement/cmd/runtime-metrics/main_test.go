package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EkeMinusYou/isucon-template/tools/measurement/app"
)

func TestPollingStopIsQuietButRequestTimeoutRemainsAnError(t *testing.T) {
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = stderr
	t.Cleanup(func() { os.Stderr = previous; stderr.Close() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	c := collector{client: &http.Client{Timeout: time.Millisecond}, endpoint: server.URL, maxMetrics: 1024, maxResponse: 1 << 20}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.poll(ctx)
	content, err := os.ReadFile(stderr.Name())
	if err != nil || len(content) != 0 {
		t.Fatalf("normal stop reported as collection failure: %s %v", content, err)
	}
	c.poll(context.Background())
	content, err = os.ReadFile(stderr.Name())
	if err != nil || !bytes.Contains(content, []byte("runtime metrics poll:")) {
		t.Fatalf("request timeout was hidden: %s %v", content, err)
	}
}

func TestRuntimeExporterProducesBoundedCollectorRows(t *testing.T) {
	handler, err := app.NewHandler(app.Options{Snapshot: func() app.Snapshot {
		return app.Snapshot{Metrics: map[string]any{"events_total": 42}}
	}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var out bytes.Buffer
	writer := csv.NewWriter(&out)
	writer.Comma = '\t'
	c := collector{client: &http.Client{Timeout: time.Second}, endpoint: server.URL + "/debug/runtime", output: writer, maxMetrics: 1024, maxResponse: 1 << 20}
	snapshot, err := c.fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Metrics["events_total"] != 42 || snapshot.Metrics["go_process_started_unix_ms"] <= 0 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if err = c.write(snapshot); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "events_total\t42.000000") {
		t.Fatalf("rows=%s", out.String())
	}
	c.maxResponse = 8
	if _, err = c.fetch(context.Background()); err == nil {
		t.Fatal("oversized response accepted")
	}
	snapshot.Metrics["invalid"] = math.NaN()
	if err = validateSnapshot(snapshot, 1024, 1); err == nil {
		t.Fatal("non-finite metric accepted")
	}
	bounded := &limitedWriter{writer: &bytes.Buffer{}, limit: 2}
	if _, err = bounded.Write([]byte("too large")); err == nil {
		t.Fatal("output limit ignored")
	}
}
