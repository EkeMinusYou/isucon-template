package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeLogsDoNotReadBinarySQLReports(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	schema, err := os.ReadFile("../analysis/schema/runtime-logs.sql")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(t.TempDir(), "runs", "20260901-120000")
	writeTestFile(t, runDir, "host1-pt-query-digest.log", "INSERT INTO images VALUES (_binary'\xff\xd8');\n")
	query := "set variable run_glob = " + sqlString(filepath.ToSlash(runDir)) + ";\n" + string(schema) + "select run_id, host, source_artifact, content from runtime_logs_raw"
	check := func(want string) {
		t.Helper()
		output, err := exec.Command(duckdb, "-noheader", "-list", "-c", query).CombinedOutput()
		if err != nil {
			t.Fatalf("read fault logs beside a binary SQL report: %v: %s", err, output)
		}
		if strings.TrimSpace(string(output)) != want {
			t.Fatalf("runtime logs: got %q, want %q", output, want)
		}
	}
	check("")
	writeTestFile(t, runDir, "host1-app-journal.log", "日本語の障害ログ\n")
	check("20260901-120000|host1|host1-app-journal.log|日本語の障害ログ")
}
