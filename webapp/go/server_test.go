package main

import (
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
)

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

func TestServerSeparatesTrafficAndMeasurementsAndStops(t *testing.T) {
	config := serverConfig{Address: availableAddress(t), MeasurementAddr: availableAddress(t), AccessLog: filepath.Join(t.TempDir(), "access.log")}
	for config.MeasurementAddr == config.Address {
		config.MeasurementAddr = availableAddress(t)
	}
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
			t.Error("server did not stop")
		}
	})
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, err := client.Get("http://" + config.Address + "/healthz")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusOK || string(body) != "ok\n" {
				t.Fatalf("healthz: %d %q", response.StatusCode, body)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, path := range []string{"/debug/runtime", "/debug/pprof/"} {
		response, err := client.Post("http://"+config.Address+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("unexpected public endpoint %s: %d", path, response.StatusCode)
		}
	}
	response, err := client.Get("http://" + config.MeasurementAddr + "/debug/profiles/status")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"cpu":""`) {
		t.Fatalf("profile status: %d %s", response.StatusCode, body)
	}
	response, err = client.Get("http://" + config.MeasurementAddr + "/debug/runtime")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "go_process_started_unix_ms") {
		t.Fatalf("runtime metrics: %d %s", response.StatusCode, body)
	}
}

func TestServerRejectsPublicMeasurementListener(t *testing.T) {
	err := serve(context.Background(), serverConfig{Address: "127.0.0.1:0", MeasurementAddr: "0.0.0.0:6060"}, newHandler())
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("public measurement listener accepted: %v", err)
	}
}

func TestDirectTLSProtocolConfigurationAndPublicConnectionLog(t *testing.T) {
	enabled, disabled := true, false
	for _, tc := range []struct {
		name     string
		enabled  *bool
		protocol string
		alpn     string
	}{
		{name: "default", protocol: "HTTP/2.0", alpn: "h2"},
		{name: "enabled", enabled: &enabled, protocol: "HTTP/2.0", alpn: "h2"},
		{name: "disabled", enabled: &disabled, protocol: "HTTP/1.1", alpn: "http/1.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			certificate := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			cert := certificate.TLS.Certificates[0]
			certificate.Close()
			root := t.TempDir()
			certPath := filepath.Join(root, "server.crt")
			keyPath := filepath.Join(root, "server.key")
			var certPEM []byte
			for _, der := range cert.Certificate {
				certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
			}
			key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(certPath, certPEM, 0600)
			os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600)
			config := serverConfig{HTTP2Enabled: tc.enabled, TLSAddress: availableAddress(t), TLSCertFile: certPath, TLSKeyFile: keyPath, MeasurementAddr: availableAddress(t), AccessLog: filepath.Join(root, "access.log")}
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() { result <- serve(ctx, config, newHandler()) }()
			transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: true}
			client := &http.Client{Transport: transport, Timeout: time.Second}
			t.Cleanup(func() {
				client.CloseIdleConnections()
				cancel()
				select {
				case err := <-result:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(3 * time.Second):
					t.Error("shutdown timed out")
					return
				}
				data, err := os.ReadFile(config.AccessLog)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				for _, line := range lines {
					var entry struct {
						Protocol   string `json:"protocol"`
						Connection uint64 `json:"connection"`
					}
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						t.Fatal(err)
					}
					if entry.Protocol != tc.protocol || entry.Connection == 0 {
						t.Fatalf("public connection log: %+v; want %s and a connection ID", entry, tc.protocol)
					}
				}
			})
			deadline := time.Now().Add(3 * time.Second)
			for {
				response, err := client.Get("https://" + config.TLSAddress + "/healthz")
				if err == nil {
					io.Copy(io.Discard, response.Body)
					response.Body.Close()
					if response.Proto != tc.protocol {
						t.Fatalf("TLS protocol: got %s, want %s", response.Proto, tc.protocol)
					}
					if response.TLS.NegotiatedProtocol != tc.alpn {
						t.Fatalf("ALPN: got %q, want %q", response.TLS.NegotiatedProtocol, tc.alpn)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal(err)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
