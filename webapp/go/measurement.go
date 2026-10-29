package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	measurement "github.com/EkeMinusYou/isucon-template/tools/measurement/app"
)

type observationKey struct{}
type connectionKey struct{}
type requestObservation struct{ sessionID string }
type connectionObservation struct {
	id       uint64
	requests atomic.Uint64
}

// Bootstrap calls this after authentication. The value is private to the log;
// neither authentication tokens nor measurement headers reach the response.
func setRequestSession(r *http.Request, identity string) {
	if o, ok := r.Context().Value(observationKey{}).(*requestObservation); ok {
		o.sessionID = ""
		if identity != "" {
			digest := sha256.Sum256([]byte(identity))
			o.sessionID = hex.EncodeToString(digest[:])
		}
	}
}

type observedResponse struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *observedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *observedResponse) WriteHeader(code int) {
	if code >= 100 && code < 200 && code != 101 {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *observedResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}
func (w *observedResponse) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *observedResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, b, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil && w.status == 0 {
		w.status = 101
	}
	return c, b, err
}

func measuredHandler(next http.Handler, logger *measurement.AccessLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		observation := &requestObservation{}
		r = r.WithContext(context.WithValue(r.Context(), observationKey{}, observation))
		body := measurement.ObserveRequestBody(r)
		response := &observedResponse{ResponseWriter: w}
		var id, sequence uint64
		if conn, ok := r.Context().Value(connectionKey{}).(*connectionObservation); ok {
			id = conn.id
			sequence = conn.requests.Add(1)
		}
		next.ServeHTTP(response, r)
		status := response.status
		if status == 0 {
			status = 200
		}
		read := body.BytesRead()
		entry := measurement.AccessLogEntry{Msec: float64(time.Now().UnixMicro()) / 1e6, Method: r.Method, URI: r.URL.RequestURI(), Host: r.Host, Status: status, ResponseTime: time.Since(start).Seconds(), BodyBytes: response.bytes, RequestBodyBytes: &read, SessionID: observation.sessionID, Connection: id, ConnectionRequests: sequence}
		if r.ContentLength >= 0 {
			n := r.ContentLength
			entry.RequestContentLength = &n
		}
		if r.TLS != nil {
			entry.SSLSessionReused = "."
			if r.TLS.DidResume {
				entry.SSLSessionReused = "r"
			}
		}
		if err := logger.Log(entry); err != nil {
			panic(err)
		}
	})
}
