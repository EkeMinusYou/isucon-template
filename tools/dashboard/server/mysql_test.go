package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMysqlHostSeriesKeepsDatabaseHostsSeparate(t *testing.T) {
	dir := t.TempDir()
	body := "sample\telapsed_ms\tthreads_running\n0\t0\t1\n"
	paths := []string{
		filepath.Join(dir, "isucon-3-mysql-status.tsv"),
		filepath.Join(dir, "isucon-2-mysql-status.tsv"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	hosts, err := parseMysqlHostSeries(paths, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0].Host != "isucon-2" || hosts[1].Host != "isucon-3" {
		t.Fatalf("hosts = %#v", hosts)
	}
	for _, host := range hosts {
		if len(host.Series) != 1 || host.Series[0].ThreadsRunning != 1 {
			t.Fatalf("series for %s = %#v", host.Host, host.Series)
		}
	}
}

func TestMysqlStatusPathsIncludesLegacyArtifact(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "mysql-status.tsv")
	if err := os.WriteFile(legacy, []byte("sample\telapsed_ms\n0\t0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := mysqlStatusPaths(dir)
	if len(paths) != 1 || paths[0] != legacy {
		t.Fatalf("paths = %#v, want %q", paths, legacy)
	}
	if got := mysqlStatusHost(legacy, "isucon-2"); got != "isucon-2" {
		t.Fatalf("legacy host = %q", got)
	}
}
