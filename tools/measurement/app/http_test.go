package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPObserverJoinsPublicRequestsAndUpstreamWithoutSecrets(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewAccessLogger(&output, AccessLoggerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	observer := NewHTTPObserver(logger)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "upstream") }))
	defer upstream.Close()
	transport := NewUpstreamTransport()
	server := httptest.NewUnstartedServer(observer.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetSessionIdentity(r, "session-secret")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream.URL, nil)
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Error(err)
			return
		}
		defer response.Body.Close()
		if _, err := io.Copy(io.Discard, response.Body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write(body)
	})))
	server.Config.ConnContext = observer.ConnContext
	server.Config.ConnState = observer.ConnState
	server.Start()
	client := server.Client()
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/items", strings.NewReader("payload"))
		req.Header.Set("X-Measurement-Session-ID", "spoofed-secret")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 201 || string(data) != "payload" || response.Header.Get("X-Measurement-Session-ID") != "" {
			t.Fatalf("response: %d %q", response.StatusCode, data)
		}
	}
	client.CloseIdleConnections()
	server.Close()
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var rows []AccessLogEntry
	for decoder.More() {
		var e AccessLogEntry
		if err := decoder.Decode(&e); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, e)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	for i, e := range rows {
		if e.Protocol != "HTTP/1.1" || e.Connection > 1<<63-1 || e.Connection == 0 || e.Connection != rows[0].Connection || e.ConnectionRequests != uint64(i+1) {
			t.Fatalf("connection: %+v", e)
		}
		if e.Status != 201 || e.BodyBytes != 7 || e.RequestBodyBytes == nil || *e.RequestBodyBytes != 7 || e.RequestContentLength == nil || *e.RequestContentLength != 7 {
			t.Fatalf("bytes/status: %+v", e)
		}
		if len(e.SessionID) != 64 || strings.Contains(e.SessionID, "secret") {
			t.Fatal(e.SessionID)
		}
		if len(e.Upstream) != 1 || e.Upstream[0].BodyBytes != 8 || e.Upstream[0].TotalSeconds <= 0 {
			t.Fatalf("upstream: %+v", e.Upstream)
		}
	}
	if !rows[1].Upstream[0].Reused {
		t.Fatal("upstream keepalive was not reused")
	}
}
func TestHTTPObserverPreservesFlushAndMeasuresImplicitSuccess(t *testing.T) {
	var output bytes.Buffer
	logger, _ := NewAccessLogger(&output, AccessLoggerOptions{})
	observer := NewHTTPObserver(logger)
	handler := observer.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
		}
		io.WriteString(w, "event\n")
	}))
	server := httptest.NewServer(handler)
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	server.Close()
	logger.Close()
	var entry AccessLogEntry
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Status != 200 || entry.BodyBytes != 6 || entry.SessionID != "" {
		t.Fatalf("entry: %+v", entry)
	}
}
