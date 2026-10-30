package main

import (
	"strings"
	"testing"
)

func TestHTTP2EnvironmentConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		enabled bool
		invalid bool
	}{
		{name: "default", enabled: true},
		{name: "enabled", value: "true", enabled: true},
		{name: "disabled", value: "false"},
		{name: "invalid", value: "tru", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HTTP2_ENABLED", tc.value)
			config, err := configFromEnv()
			if tc.invalid {
				if err == nil || !strings.Contains(err.Error(), "HTTP2_ENABLED") {
					t.Fatalf("invalid setting accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.HTTP2Enabled == nil || *config.HTTP2Enabled != tc.enabled {
				t.Fatalf("HTTP2Enabled = %v, want %v", config.HTTP2Enabled, tc.enabled)
			}
		})
	}
}
