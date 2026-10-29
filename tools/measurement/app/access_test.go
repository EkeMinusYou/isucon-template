package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestRequestBodyMeasuresConsumptionWithoutReadingAhead(t *testing.T) {
	r := httptest.NewRequest("POST", "/upload", strings.NewReader("abcdef"))
	r.ContentLength = -1 // Chunked or otherwise unknown length.
	observed := ObserveRequestBody(r)
	if observed.BytesRead() != 0 {
		t.Fatal("wrapper read ahead")
	}
	p := make([]byte, 2)
	if n, err := r.Body.Read(p); n != 2 || err != nil {
		t.Fatalf("read %d: %v", n, err)
	}
	if observed.BytesRead() != 2 || r.ContentLength != -1 {
		t.Fatal("partial read or declared length changed")
	}
	rest, err := io.ReadAll(r.Body)
	if err != nil || string(rest) != "cdef" || observed.BytesRead() != 6 {
		t.Fatalf("remaining body %q: %v", rest, err)
	}
	if err := r.Body.Close(); err != nil {
		t.Fatal(err)
	}
	empty := &http.Request{Body: http.NoBody}
	if counter := ObserveRequestBody(empty); counter != nil || counter.BytesRead() != 0 {
		t.Fatal("empty body changed")
	}
}

func TestAccessLogStatsConcurrentAndPendingEntries(t *testing.T) {
	var stats AccessLogStats
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); stats.Enqueued(); stats.Written() }()
	}
	workers.Wait()
	stats.Enqueued() // Still queued, not lost.
	stats.Dropped()
	m := stats.Metrics(1)
	if m["access_log_enqueued_total"] != uint64(101) || m["access_log_written_total"] != uint64(100) ||
		m["access_log_dropped_total"] != uint64(1) || m["access_log_queue_length"] != 1 {
		t.Fatalf("metrics: %#v", m)
	}
}
