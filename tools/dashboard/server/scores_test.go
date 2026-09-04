package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseScoresPreservesUnknownAndZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scores.tsv")
	body := "run_id\tscore\tapp\tnginx\tmysql\tapp_traffic\n" +
		"20260901-120000\t\tisucon-1\tisucon-1\tisucon-1\tisucon-1\n" +
		"20260901-120100\t0\tisucon-1\tisucon-1\tisucon-1\tisucon-1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := parseScores(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Score != nil {
		t.Fatalf("unknown score was not preserved: %#v", entries)
	}
	if entries[1].Score == nil || *entries[1].Score != 0 {
		t.Fatalf("real zero score was not preserved: %#v", entries)
	}
}
