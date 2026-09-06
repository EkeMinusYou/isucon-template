package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testLifecycleOptions(t *testing.T) lifecycleOptions {
	t.Helper()
	root := t.TempDir()
	return lifecycleOptions{
		runID: "20260901-120000", resultsDir: filepath.Join(root, "runs"), rawDir: filepath.Join(root, "raw"),
		runStateFile: filepath.Join(root, "raw", "current-run-id"), snapshotPath: filepath.Join(root, "snapshot.json"),
		scoresPath: filepath.Join(root, "runs", "scores.tsv"), collectorConfig: "collectors.yaml", digesterConfig: "digesters.yaml",
		roles: map[string][]string{
			"all": {"isucon-1"}, "app": {"isucon-1"}, "app_traffic": {"isucon-1"},
			"nginx": {"isucon-1"}, "entry": {"isucon-1"}, "mysql": {"isucon-1"},
		},
		vars: map[string]string{"services": "app,nginx,mysql"},
	}
}

func TestAdditionalRolesSurviveManifestCreation(t *testing.T) {
	opts := testLifecycleOptions(t)
	opts.roles["cache"] = []string{"isucon-1", "isucon-2"}
	opts.roles["worker"] = []string{"isucon-1", "isucon-2"}
	if err := os.WriteFile(opts.snapshotPath, []byte(`{"schema_version":3,"status":"ok","cards":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(opts.resultsDir, opts.runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	args, err := opts.manifestBeginArgs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := runManifestBegin(args); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"cache", "worker"} {
		if !reflect.DeepEqual(manifest.Roles.Additional[role], opts.roles[role]) {
			t.Fatalf("role %s lost: %#v", role, manifest.Roles)
		}
		if roleValues(manifest.Roles)[role] != "isucon-1,isucon-2" {
			t.Fatalf("comparison omits role %s", role)
		}
	}
}

func TestLifecycleBeginOwnsStateAndCollectorOrder(t *testing.T) {
	opts := testLifecycleOptions(t)
	var calls []string
	runner := lifecycleRunner{
		collect: func(args []string) error {
			calls = append(calls, "collect:"+args[0])
			if args[0] == "prepare" {
				if _, err := os.Stat(opts.runStateFile); err != nil {
					t.Fatalf("RUN state was not installed before prepare: %v", err)
				}
			}
			return nil
		},
		manifestBegin: func(args []string) error {
			calls = append(calls, "manifest")
			if !containsArgPair(args, "-app", "isucon-1") || !containsArgPair(args, "-collector-clean", "-capture-contract") {
				t.Fatalf("manifest args = %#v", args)
			}
			return nil
		},
	}
	if err := runner.begin(opts); err != nil {
		t.Fatal(err)
	}
	if want := []string{"collect:check-clean", "manifest", "collect:prepare", "collect:start"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if runID, err := readRunState(opts.runStateFile); err != nil || runID != opts.runID {
		t.Fatalf("active state = %q, %v", runID, err)
	}
}

func TestLifecycleBeginArchivesStateAfterPrepareFailure(t *testing.T) {
	opts := testLifecycleOptions(t)
	var calls []string
	runner := lifecycleRunner{
		collect: func(args []string) error {
			calls = append(calls, args[0])
			if args[0] == "prepare" {
				return errors.New("prepare failed")
			}
			return nil
		},
		manifestBegin: func([]string) error { return nil },
	}
	err := runner.begin(opts)
	if err == nil || !strings.Contains(err.Error(), "prepare failed") {
		t.Fatalf("begin error = %v", err)
	}
	if want := []string{"check-clean", "prepare", "sweep"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if _, err := os.Stat(filepath.Join(opts.rawDir, "aborted-run-id")); err != nil {
		t.Fatalf("aborted state was not archived: %v", err)
	}
}

func TestLifecycleFinalizeContinuesThroughCollectionFailures(t *testing.T) {
	opts := testLifecycleOptions(t)
	if err := writeRunState(opts.runStateFile, opts.runID); err != nil {
		t.Fatal(err)
	}
	var calls []string
	runner := lifecycleRunner{
		collect: func(args []string) error {
			calls = append(calls, "collect:"+args[0])
			return errors.New("stop failed")
		},
		digest: func([]string) error {
			calls = append(calls, "digest")
			return errors.New("digest failed")
		},
		manifestFinalize: func(args []string) error {
			calls = append(calls, "manifest")
			if !containsArgPair(args, "-dir", filepath.Join(opts.resultsDir, opts.runID)) {
				t.Fatalf("finalize args = %#v", args)
			}
			return nil
		},
	}
	if err := runner.finalize(opts); err != nil {
		t.Fatal(err)
	}
	if want := []string{"collect:stop", "digest", "manifest"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if runID, err := readRunState(filepath.Join(opts.rawDir, "last-run-id")); err != nil || runID != opts.runID {
		t.Fatalf("last state = %q, %v", runID, err)
	}
}

func containsArgPair(args []string, first, second string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == first && args[i+1] == second {
			return true
		}
	}
	return false
}

func TestCollectorModeSurvivesManifestCreation(t *testing.T) {
	for _, tc := range []struct {
		flags    string
		disabled bool
	}{
		{"", false}, {"-no-collectors", true}, {"--no-collectors=true", true},
		{"-no-collectors=false", false}, {"-no-collectors -no-collectors=false", false},
	} {
		t.Run(tc.flags, func(t *testing.T) {
			opts := testLifecycleOptions(t)
			opts.collectorFlags = tc.flags
			if err := os.WriteFile(opts.snapshotPath, []byte(`{"schema_version":3,"status":"ok","cards":[]}`), 0600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(opts.resultsDir, opts.runID)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			args, err := opts.manifestBeginArgs(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := runManifestBegin(args); err != nil {
				t.Fatal(err)
			}
			m := readTestManifest(t, filepath.Join(dir, "run.json"))
			if m.CollectorsDisabled != tc.disabled {
				t.Fatalf("mode = %v, want %v", m.CollectorsDisabled, tc.disabled)
			}
		})
	}
}
