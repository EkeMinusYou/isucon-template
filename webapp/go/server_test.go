package main

import (
	"context"
	"io"
	"net"
	"net/http"
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
	config := serverConfig{Address: availableAddress(t), MeasurementAddr: availableAddress(t), AccessLogPath: filepath.Join(t.TempDir(), "access.log")}
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
