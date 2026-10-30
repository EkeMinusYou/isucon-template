package app

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type connectionKey struct{}
type observationKey struct{}
type connectionObservation struct {
	id       uint64
	accepted time.Time
	conn     net.Conn
	requests atomic.Uint64
	closed   atomic.Bool
}
type requestObservation struct {
	mu       sync.Mutex
	session  string
	upstream []UpstreamObservation
}

// HTTPObserver joins public connections and requests without trusting client
// measurement headers or assuming an authentication cookie name.
type HTTPObserver struct {
	Logger          *AccessLogger
	TCPSampleEvery  uint64
	sequence        atomic.Uint64
	requestSequence atomic.Uint64
	connections     sync.Map
}

func NewHTTPObserver(logger *AccessLogger) *HTTPObserver {
	var seed [4]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(fmt.Errorf("connection generation: %w", err))
	}
	o := &HTTPObserver{Logger: logger, TCPSampleEvery: 32}
	o.sequence.Store(uint64(binary.BigEndian.Uint32(seed[:])&0x7fffffff) << 32)
	return o
}
func (o *HTTPObserver) ConnContext(ctx context.Context, c net.Conn) context.Context {
	info := &connectionObservation{id: o.sequence.Add(1), accepted: time.Now(), conn: c}
	o.connections.Store(c, info)
	return context.WithValue(ctx, connectionKey{}, info)
}
func (o *HTTPObserver) ConnState(c net.Conn, state http.ConnState) {
	if state == http.StateClosed || state == http.StateHijacked {
		if info, ok := o.connections.LoadAndDelete(c); ok {
			info.(*connectionObservation).closed.Store(true)
		}
	}
}

// SetSessionIdentity is called after the application identifies the session.
// A one-way digest is logged; credentials and raw cookies are never exported.
func SetSessionIdentity(r *http.Request, identity string) {
	if info, ok := r.Context().Value(observationKey{}).(*requestObservation); ok {
		value := ""
		if identity != "" {
			sum := sha256.Sum256([]byte(identity))
			value = hex.EncodeToString(sum[:])
		}
		info.mu.Lock()
		info.session = value
		info.mu.Unlock()
	}
}

// PublicConnection exposes the accepted socket to bounded optional diagnostics.
func PublicConnection(r *http.Request) net.Conn {
	if info, ok := r.Context().Value(connectionKey{}).(*connectionObservation); ok {
		return info.conn
	}
	return nil
}
func (o *HTTPObserver) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		observation := &requestObservation{}
		r = r.WithContext(context.WithValue(r.Context(), observationKey{}, observation))
		body := ObserveRequestBody(r)
		rw := &observedResponse{ResponseWriter: w}
		entry := AccessLogEntry{Method: r.Method, URI: r.URL.RequestURI(), Host: r.Host, Protocol: r.Proto, RequestID: o.requestSequence.Add(1)}
		info, _ := r.Context().Value(connectionKey{}).(*connectionObservation)
		if info != nil {
			entry.Connection = info.id
			entry.ConnectionRequests = info.requests.Add(1)
			if entry.ConnectionRequests == 1 {
				entry.AcceptToFirst = started.Sub(info.accepted).Seconds()
			}
		}
		if r.TLS != nil {
			entry.SSLSessionReused = "."
			if r.TLS.DidResume {
				entry.SSLSessionReused = "r"
			}
		}
		completed := false
		defer func() {
			ended := time.Now()
			entry.Msec = float64(ended.UnixNano()) / 1e9
			entry.ResponseTime = ended.Sub(started).Seconds()
			entry.BodyBytes = rw.bytes
			entry.Status = rw.status
			if entry.Status == 0 && completed {
				entry.Status = http.StatusOK
			}
			read := body.BytesRead()
			entry.RequestBodyBytes = &read
			if r.ContentLength >= 0 {
				length := r.ContentLength
				entry.RequestContentLength = &length
			}
			observation.mu.Lock()
			entry.SessionID = observation.session
			entry.Upstream = append([]UpstreamObservation(nil), observation.upstream...)
			observation.mu.Unlock()
			if info != nil {
				entry.ConnectionClosed = info.closed.Load() || r.Close || rw.Header().Get("Connection") == "close"
				if o.TCPSampleEvery > 0 && (entry.ConnectionRequests == 1 || entry.ConnectionRequests%o.TCPSampleEvery == 0) {
					entry.TCP = sampleTCP(info.conn)
				}
			}
			if err := o.Logger.Log(entry); err != nil {
				panic(fmt.Errorf("access log: %w", err))
			}
		}()
		next.ServeHTTP(rw, r)
		completed = true
	})
}

type observedResponse struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *observedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *observedResponse) WriteHeader(status int) {
	if status >= 200 || status == http.StatusSwitchingProtocols {
		if w.status != 0 {
			return
		}
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *observedResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, e := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, e
}
func (w *observedResponse) ReadFrom(r io.Reader) (int64, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if fast, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, e := fast.ReadFrom(r)
		w.bytes += n
		return n, e
	}
	return io.Copy(struct{ io.Writer }{w}, r)
}
func (w *observedResponse) Flush() { _ = w.FlushError() }
func (w *observedResponse) FlushError() error {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *observedResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}
func (w *observedResponse) Push(target string, o *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, o)
	}
	return http.ErrNotSupported
}

// TCPObservation is absent on unsupported systems or unavailable sockets.
// Values come from the OS and are samples, not request-specific RTT estimates.
type TCPObservation struct {
	RTTMicros         uint32 `json:"rtt_us"`
	RTTVarianceMicros uint32 `json:"rtt_variance_us"`
	TotalRetransmits  uint32 `json:"total_retransmits"`
}

func unwrapTCP(c net.Conn) net.Conn {
	if t, ok := c.(*tls.Conn); ok {
		return t.NetConn()
	}
	return c
}

func ConnectionRequestCount(r *http.Request) uint64 {
	if info, ok := r.Context().Value(connectionKey{}).(*connectionObservation); ok {
		return info.requests.Load()
	}
	return 0
}
