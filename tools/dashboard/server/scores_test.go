package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseScoresRejectsOldOrMalformedFormats(t *testing.T) {
	for name, body := range map[string]string{
		"old header":    "run_id\tscore\tapp\tnginx\tmysql\n20260901-120000\t1\ta\tn\tm\n",
		"short row":     scoresHeader + "\n20260901-120000\t1\ta\tn\tm\n",
		"invalid score": scoresHeader + "\n20260901-120000\tinvalid\ta\tn\tm\ta\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "scores.tsv")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := parseScores(path); err == nil || !strings.Contains(err.Error(), "scores.tsv") {
				t.Fatalf("parseScores error = %v", err)
			}
		})
	}
}
