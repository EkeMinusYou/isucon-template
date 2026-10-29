package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// AccessLogEntry is the shared log contract. Byte counts are absent when
// unknown. Referenced values must remain immutable after Log returns.
type AccessLogEntry struct {
	Msec                 float64 `json:"msec"`
	Method               string  `json:"method"`
	URI                  string  `json:"uri"`
	Status               int     `json:"status"`
	ResponseTime         float64 `json:"response_time"`
	BodyBytes            int64   `json:"body_bytes"`
	RequestBodyBytes     *int64  `json:"request_body_bytes,omitempty"`
	RequestContentLength *int64  `json:"request_content_length,omitempty"`
	SessionID            string  `json:"session_id"`
	Host                 string  `json:"host,omitempty"`
	Connection           uint64  `json:"connection,omitempty"`
	ConnectionRequests   uint64  `json:"connection_requests,omitempty"`
	ConnectionClosed     bool    `json:"connection_closed,omitempty"`
	AcceptToFirst        float64 `json:"accept_to_first_request,omitempty"`
	SSLSessionReused     string  `json:"ssl_session_reused,omitempty"`
	Details              any     `json:"details,omitempty"`
}

type AccessLoggerOptions struct {
	QueueSize     int
	BufferSize    int
	BatchInterval time.Duration
	FlushInterval time.Duration
}

var ErrAccessLoggerClosed = errors.New("access logger is closed")

type accessLogFailure struct{ err error }

// AccessLogger batches typed entries off the response path. A full queue blocks
// producers rather than dropping entries. Close drains and flushes the queue.
// The caller owns the writer and stops request handlers before closing it.
type AccessLogger struct {
	entries    chan AccessLogEntry
	stop, done chan struct{}
	mu         sync.RWMutex
	closed     bool
	closeOnce  sync.Once
	stats      AccessLogStats
	failure    atomic.Pointer[accessLogFailure]
}

func NewAccessLogger(out io.Writer, o AccessLoggerOptions) (*AccessLogger, error) {
	if out == nil {
		return nil, errors.New("access log writer is required")
	}
	if o.QueueSize == 0 {
		o.QueueSize = 4096
	}
	if o.BufferSize == 0 {
		o.BufferSize = 256 << 10
	}
	if o.BatchInterval == 0 {
		o.BatchInterval = 5 * time.Millisecond
	}
	if o.FlushInterval == 0 {
		o.FlushInterval = 100 * time.Millisecond
	}
	if o.QueueSize < 1 || o.BufferSize < 1 || o.BatchInterval <= 0 || o.FlushInterval < o.BatchInterval || o.FlushInterval > time.Second {
		return nil, errors.New("access logger requires positive sizes and 0 < batch <= flush <= 1s")
	}
	l := &AccessLogger{entries: make(chan AccessLogEntry, o.QueueSize), stop: make(chan struct{}), done: make(chan struct{})}
	go l.run(out, o)
	return l, nil
}

func (l *AccessLogger) Log(entry AccessLogEntry) error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed {
		return ErrAccessLoggerClosed
	}
	if err := l.Err(); err != nil {
		return err
	}
	select {
	case l.entries <- entry:
		l.stats.Enqueued()
		return l.Err()
	case <-l.done:
		return l.Err()
	}
}
func (l *AccessLogger) Err() error {
	if f := l.failure.Load(); f != nil {
		return f.err
	}
	return nil
}
func (l *AccessLogger) Metrics() map[string]any {
	m := l.stats.Metrics(len(l.entries))
	m["access_log_errors_total"] = 0
	if l.Err() != nil {
		m["access_log_errors_total"] = 1
	}
	return m
}
func (l *AccessLogger) fail(err error) {
	l.failure.CompareAndSwap(nil, &accessLogFailure{err: fmt.Errorf("access log output: %w", err)})
}
func (l *AccessLogger) run(out io.Writer, o AccessLoggerOptions) {
	defer close(l.done)
	buffer := bufio.NewWriterSize(out, o.BufferSize)
	encoder := json.NewEncoder(buffer)
	write := func(entry AccessLogEntry) bool {
		if err := encoder.Encode(entry); err != nil {
			l.stats.Dropped()
			l.fail(err)
			return false
		}
		l.stats.Written()
		return true
	}
	flush := func() bool {
		if err := buffer.Flush(); err != nil {
			l.fail(err)
			return false
		}
		return true
	}
	batch := time.NewTicker(o.BatchInterval)
	flushTick := time.NewTicker(o.FlushInterval)
	defer batch.Stop()
	defer flushTick.Stop()
	for {
		select {
		case <-batch.C:
			// Bound a batch so that flush and stop requests cannot starve.
			for i := 0; i < cap(l.entries); i++ {
				select {
				case entry := <-l.entries:
					if !write(entry) {
						return
					}
				default:
					i = cap(l.entries)
				}
			}
		case <-flushTick.C:
			if !flush() {
				return
			}
		case <-l.stop:
			for {
				select {
				case entry := <-l.entries:
					if !write(entry) {
						return
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// Close is idempotent and does not close the caller's writer.
func (l *AccessLogger) Close() error {
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		close(l.stop)
		l.mu.Unlock()
		<-l.done
		// Failed output can leave accepted entries that were never encoded.
		for {
			select {
			case <-l.entries:
				l.stats.Dropped()
			default:
				return
			}
		}
	})
	return l.Err()
}
