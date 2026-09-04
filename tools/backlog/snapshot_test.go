package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppliedSnapshotIncludesOnlyAppliedCards(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 42, "B-005",
		Card{ID: "B-001", Status: "APPLIED", Title: "change", Fingerprint: "change:v1", Sections: []Section{{Name: sectionChangeBoundary, Body: "replace query"}}},
		Card{ID: "B-002", Status: "APPLIED", Title: "second change", Fingerprint: "change:v2", Sections: []Section{{Name: sectionVerification, Body: `{"version":1}`}}},
		Card{ID: "B-003", Status: "READY", Title: "ready"},
		Card{ID: "B-004", Status: "VALIDATED", Title: "validated"})

	snapshot, err := store.appliedSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != "ok" || snapshot.SchemaVersion != 1 || snapshot.Revision != 42 {
		t.Fatalf("snapshot metadata = %#v", snapshot)
	}
	if len(snapshot.Cards) != 2 || snapshot.Cards[0].ID != "B-001" || snapshot.Cards[1].ID != "B-002" {
		t.Fatalf("snapshot cards = %#v", snapshot.Cards)
	}
	for _, card := range snapshot.Cards {
		if !strings.HasPrefix(card.DefinitionHash, "sha256:") {
			t.Fatalf("definition hash = %q", card.DefinitionHash)
		}
	}
}

func TestAppliedSnapshotUsesEmptyArray(t *testing.T) {
	store := testStore(t)
	snapshot, err := store.appliedSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"cards":[]`) {
		t.Fatalf("empty cards were not encoded as an array: %s", body)
	}
}

func TestValidateRunSnapshotCardAllowsHistoryOnlyAndRejectsDefinitionChange(t *testing.T) {
	card := Card{ID: "B-001", Status: "APPLIED", Fingerprint: "change:v1", Sections: []Section{{Name: sectionVerification, Body: `{"version":1}`}}}
	run := runSnapshotEnvelope{
		Phase:           "finalized",
		RunID:           "20260901-120000",
		BacklogSnapshot: AppliedSnapshot{Status: "ok", Cards: []AppliedSnapshotCard{snapshotCard(card)}},
	}
	card.History = append(card.History, HistoryEntry{Body: "artifact checked"})
	if err := validateRunSnapshotCard(run, card); err != nil {
		t.Fatalf("history-only change rejected: %v", err)
	}
	card.Sections[0].Body = `{"version":1,"artifacts":["probe.tsv"]}`
	if err := validateRunSnapshotCard(run, card); err == nil || !strings.Contains(err.Error(), "definition differs") {
		t.Fatalf("definition change error = %v", err)
	}
}

func TestLoadRunAppliedSnapshotRejectsLegacyAndStartedRuns(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		runID string
		body  string
	}{
		{runID: "20260901-120000", body: `{"run_id":"20260901-120000"}`},
		{runID: "20260901-120001", body: `{"schema_version":2,"phase":"started","run_id":"20260901-120001","backlog_snapshot":{"schema_version":1,"status":"ok","cards":[]}}`},
	} {
		dir := filepath.Join(root, "runs", test.runID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(test.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadRunAppliedSnapshot(root, test.runID, false); err == nil {
			t.Fatalf("run %s unexpectedly accepted", test.runID)
		}
		if _, err := loadRunAppliedSnapshot(root, test.runID, true); err == nil {
			t.Fatalf("unfinished run %s unexpectedly accepted with force", test.runID)
		}
	}
}

func TestLoadRunAppliedSnapshotRequiresPassedKnownScoreAndCompatibleComparison(t *testing.T) {
	root := t.TempDir()
	runID := "20260901-120010"
	dir := filepath.Join(root, "runs", runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(body string) error {
		return os.WriteFile(filepath.Join(dir, "run.json"), []byte(body), 0o644)
	}
	base := `{"schema_version":4,"phase":"finalized","run_id":"20260901-120010","backlog_snapshot":{"schema_version":1,"status":"ok","cards":[]}`
	for name, suffix := range map[string]string{
		"failed":               `,"score":100,"passed":false}`,
		"unknown pass":         `,"score":100,"passed":null}`,
		"unknown score":        `,"score":null,"passed":true}`,
		"incompatible control": `,"score":100,"passed":true,"comparison":{"run_id":"20260901-110000","status":"incompatible"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := write(base + suffix); err != nil {
				t.Fatal(err)
			}
			if _, err := loadRunAppliedSnapshot(root, runID, false); err == nil {
				t.Fatalf("%s RUN unexpectedly accepted", name)
			}
			if _, err := loadRunAppliedSnapshot(root, runID, true); err != nil {
				t.Fatalf("%s RUN was not accepted with force: %v", name, err)
			}
		})
	}
	if err := write(base + `,"score":0,"passed":true,"comparison":{"status":"none"}}`); err != nil {
		t.Fatal(err)
	}
	if run, err := loadRunAppliedSnapshot(root, runID, false); err != nil || run.Score == nil || *run.Score != 0 {
		t.Fatalf("valid zero-score RUN rejected: run=%#v err=%v", run, err)
	}
	if err := write(`{"schema_version":4,"phase":"finalized","run_id":"20260901-120010","passed":false,"backlog_snapshot":{"schema_version":1,"status":"error","cards":[]}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRunAppliedSnapshot(root, runID, true); err == nil {
		t.Fatal("force accepted a RUN without a usable APPLIED snapshot")
	}
}

func TestPassSnapshotCardIDsUsesRunMembership(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 10, "B-004",
		Card{ID: "B-001", Status: "APPLIED", Title: "measured change"},
		Card{ID: "B-002", Status: "APPLIED", Title: "applied later"},
		Card{ID: "B-003", Status: "APPLIED", Title: "third change"})
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	run := runSnapshotEnvelope{
		Phase:           "finalized",
		RunID:           "20260901-120000",
		BacklogSnapshot: AppliedSnapshot{Status: "ok", Cards: []AppliedSnapshotCard{snapshotCard(card)}},
	}
	ids, err := passSnapshotCardIDs(store, run, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids, ","); got != "B-001" {
		t.Fatalf("pass IDs = %s", got)
	}
	if _, err := passSnapshotCardIDs(store, run, "B-002"); err == nil || !strings.Contains(err.Error(), "was not APPLIED") {
		t.Fatalf("snapshot-external card error = %v", err)
	}
}
