package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	measurement "github.com/EkeMinusYou/isucon-template/tools/measurement/app"
)

func TestStaticNegotiationAndValidators(t *testing.T) {
	dir := t.TempDir()
	body := []byte(strings.Repeat("const example = 1;\n", 100))
	if err := os.WriteFile(filepath.Join(dir, "app.js"), body, 0600); err != nil {
		t.Fatal(err)
	}
	var packed bytes.Buffer
	gz := gzip.NewWriter(&packed)
	gz.Write(body)
	gz.Close()
	os.WriteFile(filepath.Join(dir, "app.js.gz"), packed.Bytes(), 0600)
	handler, err := staticHandler([]staticRoute{{Prefix: "/assets/", Directory: dir, CacheControl: "public, max-age=86400"}}, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	var tag string
	for _, tc := range []struct {
		encoding   string
		compressed bool
	}{{"gzip", true}, {"gzip;q=0, *;q=1", false}, {"", false}} {
		r := httptest.NewRequest("GET", "/assets/app.js", nil)
		r.Header.Set("Accept-Encoding", tc.encoding)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=86400" {
			t.Fatal(w)
		}
		data := w.Body.Bytes()
		if tc.compressed {
			if w.Header().Get("Content-Encoding") != "gzip" {
				t.Fatal(w.Header())
			}
			reader, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			data, err = io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			tag = w.Header().Get("ETag")
		} else if w.Header().Get("Content-Encoding") != "" {
			t.Fatal(w.Header())
		}
		if !bytes.Equal(data, body) {
			t.Fatal("asset changed")
		}
	}
	r := httptest.NewRequest("GET", "/assets/app.js", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	r.Header.Set("If-None-Match", tag)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 304 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("GET", "/assets/app.js", nil)
	r.Header.Set("Range", "bytes=0-4")
	r.Header.Set("Accept-Encoding", "gzip")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != string(body[:5]) {
		t.Fatal(w)
	}
	for _, path := range []string{"/assets/", "/assets/../app.js", "/assets/app.js.gz", "/api/private"} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("changed"), 0600)
	if _, err := staticHandler([]staticRoute{{Prefix: "/assets/", Directory: dir, CacheControl: "no-cache"}}, http.NotFoundHandler()); err == nil {
		t.Fatal("stale compressed asset accepted")
	}
}

func TestDirectAccessLogAndStreaming(t *testing.T) {
	var output bytes.Buffer
	logger, err := measurement.NewAccessLogger(&output, measurement.AccessLoggerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	handler := measuredHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setRequestSession(r, "secret-session")
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: event\n\n"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
		}
	}), logger)
	r := httptest.NewRequest("POST", "/events", strings.NewReader("body"))
	r = r.WithContext(context.WithValue(r.Context(), connectionKey{}, &connectionObservation{id: 42}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !w.Flushed || w.Header().Get("X-Measurement-Session-ID") != "" {
		t.Fatal("streaming or private metadata failed")
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	var entry measurement.AccessLogEntry
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if len(entry.SessionID) != 64 || strings.Contains(output.String(), "secret-session") || entry.Connection != 42 || entry.ConnectionRequests != 1 || *entry.RequestBodyBytes != 4 || entry.BodyBytes != 13 {
		t.Fatal(output.String())
	}
}

func TestPublicTLSHTTP2AndGracefulStop(t *testing.T) {
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	certificate := fixture.TLS.Certificates[0]
	fixture.Close()
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0600)
	key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600)
	config := serverConfig{Address: availableAddress(t), MeasurementAddr: availableAddress(t), AccessLogPath: filepath.Join(dir, "access.log"), TLSCertFile: certFile, TLSKeyFile: keyFile, MaxConnections: 32}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- serve(ctx, config, newHandler()) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-result:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("TLS server did not stop")
		}
	})
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}))
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}, ForceAttemptHTTP2: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, err := client.Get("https://" + config.Address + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.ProtoMajor != 2 || response.StatusCode != 200 {
				t.Fatal(response.Proto, response.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	response, err := client.Get("https://" + config.Address + "/debug/runtime")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal(response.StatusCode)
	}
}

func TestConnectionLimitReleasesAndCloseUnblocksAccept(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &limitedListener{Listener: raw, slots: make(chan struct{}, 1), done: make(chan struct{})}
	defer listener.Close()
	client, err := net.Dial("tcp", raw.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	first, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := listener.Accept(); result <- err }()
	select {
	case <-result:
		t.Fatal("accepted beyond limit")
	case <-time.After(20 * time.Millisecond):
	}
	listener.Close()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected closed listener")
		}
	case <-time.After(time.Second):
		t.Fatal("accept remained blocked")
	}
	first.Close()
	first.Close()
	if listener.active.Load() != 0 || len(listener.slots) != 0 {
		t.Fatal("connection capacity leaked")
	}
}

func TestMeasuredHandlerPreservesHijack(t *testing.T) {
	logger, err := measurement.NewAccessLogger(io.Discard, measurement.AccessLoggerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	handler := measuredHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: example\r\n\r\nhello")
		buf.Flush()
	}), logger)
	server := httptest.NewServer(handler)
	defer server.Close()
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	io.WriteString(conn, "GET /socket HTTP/1.1\r\nHost: localhost\r\nConnection: Upgrade\r\nUpgrade: example\r\n\r\n")
	body, err := io.ReadAll(conn)
	if err != nil || !strings.Contains(string(body), "101 Switching Protocols") || !strings.HasSuffix(string(body), "hello") {
		t.Fatalf("hijack: %s %v", body, err)
	}
}
