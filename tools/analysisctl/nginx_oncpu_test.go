package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNginxOnCPUStacksSplitFramesAndShares(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	schema, err := os.ReadFile("../analysis/schema/nginx-oncpu.sql")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(t.TempDir(), "runs", "20260901-120000")
	writeTestFile(t, runDir, "entry-a-nginx-oncpu.folded", "nginx;ngx_worker+0x10;SSL_do_handshake+0x2;bn_mul_mont+0x5 3\nnginx;ngx_worker+0x10;ngx_http_log 1\n")
	query := "set variable run_glob = " + sqlString(filepath.ToSlash(runDir)) + ";\n" + string(schema) + `
select host, leaf_function, samples, sample_share, list_has_any(frames, ['SSL_do_handshake'])
from nginx_oncpu_stacks order by samples desc;
`
	cmd := exec.Command(duckdb, "-noheader", "-list")
	cmd.Stdin = strings.NewReader(query)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("read nginx on-CPU profile: %v: %s", err, output)
	}
	want := "entry-a|bn_mul_mont|3|0.75|true\nentry-a|ngx_http_log|1|0.25|false"
	if got := strings.TrimSpace(string(output)); got != want {
		t.Fatalf("nginx on-CPU stacks = %q, want %q", got, want)
	}
}
