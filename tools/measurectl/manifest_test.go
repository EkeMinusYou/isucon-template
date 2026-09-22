package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePassed(t *testing.T) {
	tests := []struct {
		name string
		log  string
		want *bool
	}{
		{
			name: "final check succeeded",
			log:  "整合性チェックが成功しました\n最終チェックを実施します\n最終チェックが成功しました\n",
			want: boolPointer(true),
		},
		{
			name: "structured failure",
			log:  `{"pass":false,"score":0,"messages":["整合性チェックに失敗しました"]}`,
			want: boolPointer(false),
		},
		{
			name: "plain final check failure",
			log:  "最終チェックに失敗しました\n",
			want: boolPointer(false),
		},
		{
			name: "preflight success only",
			log:  "整合性チェックが成功しました\n",
			want: nil,
		},
		{
			name: "missing log",
			log:  "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePassed([]byte(tt.log))
			if got == nil || tt.want == nil {
				if got != nil || tt.want != nil {
					t.Fatalf("resolvePassed() = %v, want %v", got, tt.want)
				}
				return
			}
			if *got != *tt.want {
				t.Fatalf("resolvePassed() = %t, want %t", *got, *tt.want)
			}
		})
	}
}

func TestManifestBeginAndFinalizePreserveSourceAndRoles(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "20260901-120000")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runManifestBegin([]string{
		"-dir", runDir,
		"-app", "isucon-1,isucon-2",
		"-app-traffic", "isucon-1",
		"-nginx", "isucon-2",
		"-mysql", "isucon-3",
	}); err != nil {
		t.Fatal(err)
	}
	manifestBody, err := os.ReadFile(filepath.Join(runDir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifestBody), `"comparison"`) {
		t.Fatalf("manifest contains removed comparison field: %s", manifestBody)
	}
	if strings.Contains(string(manifestBody), `"dirty"`) {
		t.Fatalf("manifest contains removed source dirty field: %s", manifestBody)
	}
	started := readTestManifest(t, filepath.Join(runDir, "run.json"))
	if started.Phase != "started" {
		t.Fatalf("started manifest = %#v", started)
	}
	benchLog := "2026-09-01T03:00:10.000Z\tBENCHMARK_START\n" +
		"2026-09-01T03:01:10.250Z\tBENCHMARK_END\n" +
		"スコア: 123\n最終チェックが成功しました\n"
	if err := os.WriteFile(filepath.Join(runDir, "bench.log"), []byte(benchLog), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "probe.tsv"), []byte("value\n1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scores := filepath.Join(root, "scores.tsv")
	if err := runManifestFinalize([]string{"-dir", runDir, "-scores", scores}); err != nil {
		t.Fatal(err)
	}
	finalized := readTestManifest(t, filepath.Join(runDir, "run.json"))
	if finalized.Phase != "finalized" || finalized.FinalizedAt == "" || finalized.Score == nil || *finalized.Score != 123 {
		t.Fatalf("finalized manifest = %#v", finalized)
	}
	if finalized.LoadWindow.Status != "ok" || finalized.LoadWindow.DurationMS != 60250 {
		t.Fatalf("load window = %#v", finalized.LoadWindow)
	}
	if finalized.Source != started.Source || finalized.Roles.MySQL != "isucon-3" {
		t.Fatalf("begin fields changed: started=%#v finalized=%#v", started, finalized)
	}
	artifactStatuses := map[string]string{}
	for _, artifact := range finalized.Artifacts {
		artifactStatuses[artifact.Name] = artifact.Status
	}
	if artifactStatuses["alp.json"] != "missing" || artifactStatuses["raw/access-*.log.zst"] != "missing" {
		t.Fatalf("required missing artifacts were not recorded: %#v", artifactStatuses)
	}
	if _, exists := artifactStatuses["*-fgprof.pprof"]; exists {
		t.Fatalf("optional fgprof was recorded as missing: %#v", artifactStatuses)
	}
}

func readTestManifest(t *testing.T, path string) Manifest {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestManifestFinalizeRequiresBegin(t *testing.T) {
	dir := t.TempDir()
	if err := runManifestFinalize([]string{"-dir", dir}); err == nil {
		t.Fatal("finalize without begin unexpectedly succeeded")
	}
}

func TestAppendScoresDistinguishesUnknownFromZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scores.tsv")
	if err := appendScores(path, Manifest{RunID: "20260901-120000"}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	if err := appendScores(path, Manifest{RunID: "20260901-120100", Score: &zero}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("scores lines = %q", lines)
	}
	unknown := strings.Split(lines[1], "\t")
	realZero := strings.Split(lines[2], "\t")
	if unknown[1] != "" || realZero[1] != "0" {
		t.Fatalf("unknown=%q zero=%q", unknown[1], realZero[1])
	}
}
