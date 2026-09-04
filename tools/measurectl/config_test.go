package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
