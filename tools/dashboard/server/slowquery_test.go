package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSlpTSVFilesIncludesEveryDatabaseHost(t *testing.T) {
	dir := t.TempDir()
	header := "Count\tQuery\tSum(QueryTime)\tAvg(QueryTime)\tMax(QueryTime)\tMin(QueryTime)\tP95(QueryTime)\tSum(LockTime)\tAvg(LockTime)\tMax(LockTime)\tSum(RowsExamined)\tAvg(RowsExamined)\tMax(RowsExamined)\tSum(RowsSent)\tAvg(RowsSent)\n"
	paths := []string{
		filepath.Join(dir, "isucon-2-slp.tsv"),
		filepath.Join(dir, "isucon-3-slp.tsv"),
	}
	for _, path := range paths {
		body := header + "2\tSELECT 1\t1.5\t0.75\t1.0\t0.5\t0.9\t0.1\t0.05\t0.1\t4\t2\t3\t2\t1\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	resp, err := parseSlpTSVFiles(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Available || resp.TotalQueryCount != 4 || resp.TotalClasses != 2 {
		t.Fatalf("response = %#v", resp)
	}
	if len(resp.Classes) != 2 || resp.Classes[0].Host != "isucon-2" || resp.Classes[1].Host != "isucon-3" {
		t.Fatalf("classes = %#v", resp.Classes)
	}
	if resp.TotalQueryTimeSum != 3 {
		t.Fatalf("total query time = %v", resp.TotalQueryTimeSum)
	}
}

func TestSlpPathsIncludesLegacyArtifact(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "slp.tsv")
	if err := os.WriteFile(legacy, []byte("Count\tQuery\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := slpPaths(dir)
	if len(paths) != 1 || paths[0] != legacy {
		t.Fatalf("paths = %#v, want %q", paths, legacy)
	}
	if got := slpHost(legacy, "isucon-2"); got != "isucon-2" {
		t.Fatalf("legacy host = %q", got)
	}
}
