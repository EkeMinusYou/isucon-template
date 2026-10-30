package app

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"time"
)

type UpstreamObservation struct {
	Address        string  `json:"address"`
	Reused         bool    `json:"reused"`
	ConnectSeconds float64 `json:"connect_seconds"`
	TLSSeconds     float64 `json:"tls_seconds"`
	HeaderSeconds  float64 `json:"header_seconds"`
	TotalSeconds   float64 `json:"total_seconds"`
	BodyBytes      int64   `json:"body_bytes"`
	Error          string  `json:"error,omitempty"`
}

// NewUpstreamTransport applies the reusable upstream-connection defaults.
// Use requests derived from the incoming request context to join observations.
func NewUpstreamTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 1024
	t.MaxIdleConnsPerHost = 1024
	t.IdleConnTimeout = 120 * time.Second
	return ObserveUpstream(t)
}
func ObserveUpstream(base http.RoundTripper) http.RoundTripper { return &observedTransport{base: base} }

type observedTransport struct{ base http.RoundTripper }

func (t *observedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	observation, ok := r.Context().Value(observationKey{}).(*requestObservation)
	if !ok {
		return t.base.RoundTrip(r)
	}
	start := time.Now()
	var mu sync.Mutex
	var connect, tlsStart time.Time
	span := UpstreamObservation{Address: r.URL.Host}
	trace := &httptrace.ClientTrace{
		ConnectStart: func(_, _ string) { mu.Lock(); connect = time.Now(); mu.Unlock() },
		ConnectDone: func(_, _ string, _ error) {
			mu.Lock()
			if !connect.IsZero() {
				span.ConnectSeconds += time.Since(connect).Seconds()
			}
			mu.Unlock()
		},
		TLSHandshakeStart: func() { mu.Lock(); tlsStart = time.Now(); mu.Unlock() },
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			mu.Lock()
			span.TLSSeconds = time.Since(tlsStart).Seconds()
			mu.Unlock()
		},
		GotConn: func(i httptrace.GotConnInfo) {
			mu.Lock()
			span.Reused = i.Reused
			span.Address = i.Conn.RemoteAddr().String()
			mu.Unlock()
		},
	}
	response, err := t.base.RoundTrip(r.WithContext(httptrace.WithClientTrace(r.Context(), trace)))
	mu.Lock()
	span.HeaderSeconds = time.Since(start).Seconds()
	if err != nil {
		span.Error = err.Error()
	}
	mu.Unlock()
	finish := func(bytes int64) {
		mu.Lock()
		span.BodyBytes = bytes
		span.TotalSeconds = time.Since(start).Seconds()
		copy := span
		mu.Unlock()
		observation.mu.Lock()
		observation.upstream = append(observation.upstream, copy)
		observation.mu.Unlock()
	}
	if err != nil {
		finish(0)
		return nil, err
	}
	response.Body = &observedUpstreamBody{ReadCloser: response.Body, finish: finish}
	return response, nil
}

type observedUpstreamBody struct {
	io.ReadCloser
	bytes  atomic.Int64
	once   sync.Once
	finish func(int64)
}

func (b *observedUpstreamBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	b.bytes.Add(int64(n))
	if e != nil {
		b.once.Do(func() { b.finish(b.bytes.Load()) })
	}
	return n, e
}
func (b *observedUpstreamBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.finish(b.bytes.Load()) })
	return err
}

func (t *observedTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
