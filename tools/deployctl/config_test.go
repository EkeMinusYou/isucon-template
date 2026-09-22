package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	contestOverlayConfig = "../contest/deployments.yaml"
	genericConfig        = "deployments.yaml"
)

// The repository deploys through the contest overlay once one exists, and
// through the generic graph before that. Both must stay loadable, so the tests
// that check the deployed declaration follow whichever is in use.
func repositoryConfig(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(contestOverlayConfig); err == nil {
		return contestOverlayConfig
	}
	return genericConfig
}

func TestGenericConfigStaysLoadableWithoutTheOverlay(t *testing.T) {
	cfg, err := loadConfig(genericConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app", "nginx", "mysql", "sysctl", "db-schema"} {
		if _, ok := cfg.Deployments[name]; !ok {
			t.Errorf("deployment %q is missing from the generic graph", name)
		}
	}
	script := cfg.Deployments["db-schema"].Activations[0].Script
	if strings.Contains(script, "shards/") {
		t.Error("the generic db-schema must not depend on the sharded layout")
	}
}

func writeConfigFixture(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIncludeLetsTheIncludingFileWin(t *testing.T) {
	dir := t.TempDir()
	writeConfigFixture(t, dir, "base.yaml", `deployments:
  sample:
    activations:
      - label: generic
        role: all
        script: echo generic
  untouched:
    activations:
      - label: kept
        role: all
        script: echo kept
`)
	overlay := writeConfigFixture(t, dir, "overlay.yaml", `include:
  - base.yaml
deployments:
  sample:
    activations:
      - label: overlay
        role: all
        script: echo overlay
  added:
    activations:
      - label: added
        role: all
        script: echo added
`)
	cfg, err := loadConfig(overlay)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"sample": "overlay", "untouched": "kept", "added": "added"} {
		got := cfg.Deployments[name].Activations[0].Label
		if got != want {
			t.Errorf("deployment %q label = %q, want %q", name, got, want)
		}
	}
}

func TestIncludeRejectsCycle(t *testing.T) {
	dir := t.TempDir()
	writeConfigFixture(t, dir, "b.yaml", `include:
  - a.yaml
deployments:
  sample:
    activations:
      - label: b
        role: all
        script: echo b
`)
	a := writeConfigFixture(t, dir, "a.yaml", `include:
  - b.yaml
deployments:
  sample:
    activations:
      - label: a
        role: all
        script: echo a
`)
	_, err := loadConfig(a)
	if err == nil || !strings.Contains(err.Error(), "循環") {
		t.Fatalf("err = %v, want a cycle report", err)
	}
}

func TestIncludeReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFixture(t, dir, "overlay.yaml", `include:
  - absent.yaml
deployments:
  sample:
    activations:
      - label: sample
        role: all
        script: echo sample
`)
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected an error for a missing include")
	}
}
