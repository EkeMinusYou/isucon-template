package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckRunDirRejectsMissingRequiredArtifact(t *testing.T) {
	dir := t.TempDir()
	specs := []ArtifactSpec{
		{Pattern: "required.tsv", Producer: "collector:required"},
		{Pattern: "optional.tsv", Producer: "collector:optional", Optional: true},
	}
	if err := checkRunDir(dir, specs); err == nil {
		t.Fatal("checkRunDir accepted a missing required artifact")
	}
	if err := os.WriteFile(filepath.Join(dir, "required.tsv"), []byte("value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkRunDir(dir, specs); err != nil {
		t.Fatalf("checkRunDir rejected complete required artifacts: %v", err)
	}
}

func TestAppendMissingArtifactsRecordsRequiredAndSkipsOptional(t *testing.T) {
	dir := t.TempDir()
	got, err := appendMissingArtifacts(dir, nil, []ArtifactSpec{
		{Pattern: "required.tsv", Producer: "collector:required"},
		{Pattern: "optional.tsv", Producer: "oneshot:optional", Optional: true},
		{Pattern: "raw/access-*.log.zst", Producer: "source:access"},
		{Pattern: "run.json", Producer: "measurectl:manifest"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "raw/access-*.log.zst" || got[1].Name != "required.tsv" ||
		got[0].Status != "missing" || got[1].Status != "missing" || got[0].Reason == "" || got[1].Reason == "" {
		t.Fatalf("missing artifacts = %#v", got)
	}
}

func TestOptionalOneshotProducesOptionalArtifactSpec(t *testing.T) {
	optional := false
	if (Oneshot{EnabledByDefault: &optional}).enabledByDefault() {
		t.Fatal("disabled oneshot reported enabled by default")
	}
}
