package app

import (
	"io"
	"net/http"
	"sync/atomic"
)

// RequestBody counts bytes consumed by the application, including partial reads.
// It does not read ahead or change ContentLength or the body's Close behavior.
type RequestBody struct {
	io.ReadCloser
	bytes atomic.Int64
}

// ObserveRequestBody wraps a nonempty request body before the handler reads it.
// A nil result represents an empty body and its BytesRead method returns zero.
func ObserveRequestBody(r *http.Request) *RequestBody {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	body := &RequestBody{ReadCloser: r.Body}
	r.Body = body
	return body
}

func (b *RequestBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.bytes.Add(int64(n))
	return n, err
}

func (b *RequestBody) BytesRead() int64 {
	if b == nil {
		return 0
	}
	return b.bytes.Load()
}

// AccessLogStats observes an application's logger without owning its queue or
// output policy. Counters are process-lifetime totals and are not reset per RUN.
type AccessLogStats struct {
	enqueued atomic.Uint64
	written  atomic.Uint64
	dropped  atomic.Uint64
}

// Enqueued records an entry accepted for output, once per entry.
func (s *AccessLogStats) Enqueued() { s.enqueued.Add(1) }

// Written records a complete entry accepted by the output writer. A buffered
// writer may not have flushed it to the file yet; this is not a durability metric.
func (s *AccessLogStats) Written() { s.written.Add(1) }

// Dropped records an entry discarded before or after enqueueing. It is not
// inferred from counter differences because entries can still be in flight.
func (s *AccessLogStats) Dropped() { s.dropped.Add(1) }

// Metrics is merged into Options.Snapshot().Metrics. queueLength is the actual
// pending queue length, or zero for a synchronous logger.
func (s *AccessLogStats) Metrics(queueLength int) map[string]any {
	return map[string]any{
		"access_log_enqueued_total": s.enqueued.Load(),
		"access_log_written_total":  s.written.Load(),
		"access_log_dropped_total":  s.dropped.Load(),
		"access_log_queue_length":   queueLength,
	}
}
