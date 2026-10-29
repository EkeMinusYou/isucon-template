package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type staticRoute struct {
	Prefix       string `json:"prefix"`
	Directory    string `json:"directory"`
	CacheControl string `json:"cache_control"`
}

// Bootstrap supplies public routes and cache lifetimes from the contest contract.
func loadServerConfig() (serverConfig, error) {
	c := serverConfig{Address: envOr("APP_ADDR", ":8080"), MeasurementAddr: envOr("MEASUREMENT_ADDR", "127.0.0.1:6060"), AccessLogPath: envOr("ACCESS_LOG_PATH", "access.log"), MaxConnections: 32768}
	if path := os.Getenv("HTTP_CONFIG_FILE"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return c, err
		}
		defer f.Close()
		d := json.NewDecoder(f)
		d.DisallowUnknownFields()
		if err := d.Decode(&c); err != nil {
			return c, fmt.Errorf("HTTP configuration: %w", err)
		}
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return c, fmt.Errorf("both TLS certificate and key are required")
	}
	if c.MaxConnections < 1 {
		return c, fmt.Errorf("max_connections must be positive")
	}
	return c, nil
}

// Use for required upstream services or a Go forwarding role. Do not retry
// accepted state changes in application code merely because transport failed.
func newUpstreamTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 4096
	t.MaxIdleConnsPerHost = 1024
	t.IdleConnTimeout = 90 * time.Second
	return t
}
