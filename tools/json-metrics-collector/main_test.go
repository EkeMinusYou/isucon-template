package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testSnapshot(enabled, detailed bool, captured int64) snapshot {
	metrics := []metric{{Scope: "global", Name: "event_count", Value: captured - 1000}}
	if detailed {
		metrics = append(metrics, metric{Scope: "resource", EntityID: 10, GroupID: 20, Name: "event_count", Value: 500})
	}
	return snapshot{
		Version:        1,
		Generation:     7,
		Enabled:        enabled,
		StartedUnixMS:  1000,
		CapturedUnixMS: captured,
		Detailed:       detailed,
		MaxEntities:    2048,
		EntityEntries:  1,
		Metrics:        metrics,
	}
}

func TestCollectWritesPeriodicAndFinalRows(t *testing.T) {
	var polls atomic.Int64
	var disables atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/enable":
			_ = json.NewEncoder(writer).Encode(testSnapshot(true, false, 1000))
		case "/snapshot":
			count := polls.Add(1)
			_ = json.NewEncoder(writer).Encode(testSnapshot(true, false, 1000+count*1000))
		case "/disable":
			disables.Add(1)
			_ = json.NewEncoder(writer).Encode(testSnapshot(false, true, 4000))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	err := collect(ctx, &output, endpoints{
		enable:   server.URL + "/enable",
		snapshot: server.URL + "/snapshot",
		disable:  server.URL + "/disable",
	}, 5*time.Millisecond, time.Second, 1<<20)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if polls.Load() == 0 || disables.Load() != 1 {
		t.Fatalf("polls=%d disables=%d", polls.Load(), disables.Load())
	}

	reader := csv.NewReader(strings.NewReader(output.String()))
	reader.Comma = '\t'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read TSV: %v", err)
	}
	if len(rows) < 4 {
		t.Fatalf("rows = %d, want header plus periodic and final rows", len(rows))
	}
	if got := rows[0]; strings.Join(got, ",") != "sample,timestamp,elapsed_ms,generation,final,scope,entity_id,group_id,window_start_ms,metric,value" {
		t.Fatalf("header = %v", got)
	}
	finalRows := 0
	for _, row := range rows[1:] {
		if row[4] == "true" {
			finalRows++
			if row[3] != "7" {
				t.Fatalf("final generation = %s", row[3])
			}
		}
	}
	if finalRows != 2 {
		t.Fatalf("final rows = %d, want 2", finalRows)
	}
}

func TestValidateSnapshotRejectsUnboundedLabels(t *testing.T) {
	current := testSnapshot(true, false, 1000)
	current.Metrics[0].Name = "metric\tsecret"
	if err := validateSnapshot(current, 1024, -1); err == nil {
		t.Fatal("validateSnapshot accepted an unsafe metric label")
	}
	current = testSnapshot(true, true, 1000)
	current.Metrics[1].GroupID = 0
	if err := validateSnapshot(current, 1024, -1); err == nil {
		t.Fatal("validateSnapshot accepted an unowned grouped metric")
	}
}

func TestByteLimitWriter(t *testing.T) {
	var output bytes.Buffer
	writer := &byteLimitWriter{writer: &output, limit: 4}
	if _, err := writer.Write([]byte("1234")); err != nil {
		t.Fatalf("write within limit: %v", err)
	}
	if _, err := writer.Write([]byte("5")); err == nil {
		t.Fatal("writer accepted output above limit")
	}
	if got := strconv.Itoa(output.Len()); got != "4" {
		t.Fatalf("output length = %s", got)
	}
}
