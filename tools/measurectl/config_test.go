package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourceFetchDelayConfiguration(t *testing.T) {
	for _, delay := range []string{"0s", "2s", "-1s", "invalid"} {
		t.Run(delay, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "digesters.yaml")
			body := `sources:
  - name: access
    role: nginx
    remote: /remote/access.log
    local: '{run_dir}/access.log'
    fetch_delay: DELAY
digesters:
  - name: count
    source: access
    outputs:
      - file: count.txt
        run: wc -l {input}
`
			if err := os.WriteFile(path, []byte(strings.ReplaceAll(body, "DELAY", delay)), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := loadDigestConfig(path)
			if delay == "-1s" || delay == "invalid" {
				if err == nil {
					t.Fatal("invalid fetch_delay was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want, _ := time.ParseDuration(delay)
			if cfg.Sources[0].FetchDelay != want {
				t.Fatalf("fetch_delay=%s want=%s", cfg.Sources[0].FetchDelay, want)
			}
		})
	}
}

func TestPerHostRemoteOutputRequiresHostPlaceholder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digesters.yaml")
	body := `digesters:
  - name: journal
    remote:
      role: app
      per_host: true
      script: journalctl
    outputs:
      - file: app-journal.log
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDigestConfig(path); err == nil || !strings.Contains(err.Error(), "{host}") {
		t.Fatalf("per-host output error = %v", err)
	}
}
