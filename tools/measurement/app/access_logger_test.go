package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

func TestAccessLoggerDrainsConcurrentEntriesWithoutLoss(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewAccessLogger(&output, AccessLoggerOptions{QueueSize: 7})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for i := 0; i < 50; i++ {
				size := int64(i)
				entry := AccessLogEntry{Msec: 1000, Method: "GET", URI: fmt.Sprintf("/a\nb/%d/%d", worker, i), Status: 200, SessionID: "opaque-id", RequestBodyBytes: &size}
				if err := logger.Log(entry); err != nil {
					t.Error(err)
					return
				}
			}
		}(worker)
	}
	workers.Wait()
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	scan := bufio.NewScanner(&output)
	for scan.Scan() {
		var entry AccessLogEntry
		if err := json.Unmarshal(scan.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if seen[entry.URI] {
			t.Fatal("duplicate entry")
		}
		seen[entry.URI] = true
		if entry.RequestBodyBytes == nil || entry.RequestContentLength != nil {
			t.Fatal("unknown byte count was filled in")
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 400 {
		t.Fatalf("got %d entries", len(seen))
	}
	m := logger.Metrics()
	if m["access_log_enqueued_total"] != uint64(400) || m["access_log_written_total"] != uint64(400) || m["access_log_queue_length"] != 0 {
		t.Fatalf("metrics: %#v", m)
	}
	if err := logger.Log(AccessLogEntry{}); !errors.Is(err, ErrAccessLoggerClosed) {
		t.Fatalf("write after close: %v", err)
	}
}

type brokenAccessWriter struct{}

func (brokenAccessWriter) Write([]byte) (int, error) { return 0, errors.New("disk unavailable") }

func TestAccessLoggerReportsWriteFailureAndUnblocksProducers(t *testing.T) {
	logger, err := NewAccessLogger(brokenAccessWriter{}, AccessLoggerOptions{QueueSize: 1, BufferSize: 1, BatchInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		for i := 0; i < 100; i++ {
			if logger.Log(AccessLogEntry{URI: "/x"}) != nil {
				return
			}
		}
	}()
	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatal("producer blocked after writer failed")
	}
	if err := logger.Close(); err == nil {
		t.Fatal("output failure hidden")
	}
	if logger.Metrics()["access_log_errors_total"] != 1 {
		t.Fatal("output error metric missing")
	}
}
func TestAccessLoggerRejectsInvalidOptions(t *testing.T) {
	if _, err := NewAccessLogger(io.Discard, AccessLoggerOptions{BatchInterval: time.Second, FlushInterval: time.Millisecond}); err == nil {
		t.Fatal("invalid intervals accepted")
	}
}
func BenchmarkAccessLogger(b *testing.B) {
	logger, err := NewAccessLogger(io.Discard, AccessLoggerOptions{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := logger.Log(AccessLogEntry{Msec: 1000, Method: "GET", URI: "/api/item", Status: 200, SessionID: "opaque"}); err != nil {
			b.Fatal(err)
		}
	}
	if err := logger.Close(); err != nil {
		b.Fatal(err)
	}
}

func TestAccessLoggerSurfacesFlushFailureAfterEncoding(t *testing.T) {
	logger, err := NewAccessLogger(brokenAccessWriter{}, AccessLoggerOptions{BufferSize: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Log(AccessLogEntry{URI: "/x"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err == nil {
		t.Fatal("flush error hidden")
	}
	m := logger.Metrics()
	if m["access_log_written_total"] != uint64(1) || m["access_log_errors_total"] != 1 {
		t.Fatalf("encoded entry and failed flush: %#v", m)
	}
}

func BenchmarkMapAccessLog(b *testing.B) {
	output := bufio.NewWriterSize(io.Discard, 256<<10)
	encoder := json.NewEncoder(output)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := map[string]any{"msec": 1000.0, "method": "GET", "uri": "/api/item", "status": 200, "response_time": 0.0, "body_bytes": int64(0), "session_id": "opaque"}
		if err := encoder.Encode(entry); err != nil {
			b.Fatal(err)
		}
	}
	if err := output.Flush(); err != nil {
		b.Fatal(err)
	}
}
