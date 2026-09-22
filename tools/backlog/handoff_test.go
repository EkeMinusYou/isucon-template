package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorrectionHandoffAndReapplication(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "VERIFY", Owner: "worker:first", Title: "change"})
	worker, verifier, empty := "worker:first", "verifier:run", ""
	transition := func(status, owner string, release bool) Card {
		t.Helper()
		c, err := store.getCard("B-001")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.transitionCard(c.ID, status, mutation{Actor: owner, ExpectedCardVersion: &c.Version, ExpectedOwner: &owner, ReleaseOwner: release}, "checked transition"); err != nil {
			t.Fatal(err)
		}
		c, err = store.getCard(c.ID)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	claim := func(owner string, version int) error {
		return store.updateCard("B-001", CardPatch{Values: map[string]string{"owner": owner}}, mutation{Actor: owner, ExpectedOwner: &empty, ExpectedCardVersion: &version}, "claim")
	}
	applied := transition("APPLIED", worker, true)
	if applied.ApplicationID != "B-001@1" || applied.Owner != "" {
		t.Fatalf("application = %#v", applied)
	}
	measured := snapshotCard(applied)
	if err := claim(verifier, applied.Version); err != nil {
		t.Fatal(err)
	}
	doing := transition("DOING", verifier, true)
	if doing.Owner != "" || doing.Status != "DOING" || doing.ApplicationID != measured.ApplicationID {
		t.Fatalf("handoff = %#v", doing)
	}
	if err := claim(worker, doing.Version); err != nil {
		t.Fatal(err)
	}
	if err := claim("worker:second", doing.Version); err == nil {
		t.Fatal("two workers claimed one handoff")
	}
	owned, _ := store.getCard(doing.ID)
	if err := claim("worker:second", owned.Version); err == nil || !strings.Contains(err.Error(), "owner conflict") {
		t.Fatalf("ownership overwrite = %v", err)
	}
	transition("VERIFY", worker, false)
	corrected := transition("APPLIED", worker, true)
	if corrected.ApplicationID == measured.ApplicationID {
		t.Fatal("reapplication retained the old ID")
	}
	if cardChangeBoundaryHash(corrected) != measured.ChangeBoundaryHash {
		t.Fatal("test changed the declared boundary")
	}
	run := runSnapshotEnvelope{RunID: "20260901-120000", BacklogSnapshot: AppliedSnapshot{Cards: []AppliedSnapshotCard{measured}}}
	if err := validateRunSnapshotCard(run, corrected); err == nil || !strings.Contains(err.Error(), "application differs") {
		t.Fatalf("stale RUN accepted: %v", err)
	}
	if err := claim(verifier, corrected.Version); err != nil {
		t.Fatal(err)
	}
	transition("DOING", verifier, true)
	unowned, _ := store.getCard(doing.ID)
	if err := claim(worker, unowned.Version); err != nil {
		t.Fatal(err)
	}
	transition("VERIFY", worker, false)
	cleanupApplied := transition("APPLIED", worker, true)
	if cleanupApplied.ApplicationID == corrected.ApplicationID {
		t.Fatal("rejection cleanup reapplication retained the old ID")
	}
	if err := claim(verifier, cleanupApplied.Version); err != nil {
		t.Fatal(err)
	}
	rejected := transition("REJECTED", verifier, true)
	if rejected.Status != "REJECTED" || rejected.Owner != "" {
		t.Fatalf("cleanup completion = %#v", rejected)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCLIHandoffIsAtomic(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "backlog.sqlite3")
	store, err := openStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fixtureObjectives(t, store)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "APPLIED", Owner: "verifier:test", Title: "change"})
	reason := "Rejection cleanup; RUN 20260901-120000; application B-001@0; remove broken path; retain unrelated work; verify responses"
	base := []string{"transition", "B-001", "--status", "DOING", "--release-owner", "--expect-card-version", "0", "--actor", "verifier:test", "--reason", reason, "--result", "cleanup pending"}
	bad := runBacklogCLI(t, db, "", append(append([]string{}, base...), "--expect-owner", "verifier:other")...)
	if bad.err == nil || !strings.Contains(bad.stderr, "owner conflict") {
		t.Fatalf("wrong owner = %#v", bad)
	}
	c, _ := store.getCard("B-001")
	if c.Status != "APPLIED" || c.Version != 0 || len(c.History) != 0 || sectionBody(c.Sections, sectionResult) != "" {
		t.Fatalf("partial handoff = %#v", c)
	}
	good := runBacklogCLI(t, db, "", append(base, "--expect-owner", "verifier:test")...)
	if good.err != nil {
		t.Fatalf("handoff failed: %#v", good)
	}
	c, _ = store.getCard("B-001")
	if c.Status != "DOING" || c.Owner != "" || c.Version != 1 || len(c.History) != 1 || c.History[0].Body != reason || sectionBody(c.Sections, sectionResult) != "cleanup pending" {
		t.Fatalf("handoff = %#v", c)
	}
}

func writeAdoptionRun(t *testing.T, root, runID string, cards ...Card) {
	t.Helper()
	score := int64(100)
	passed := true
	snapshot := AppliedSnapshot{SchemaVersion: 3, Status: "ok", Cards: []AppliedSnapshotCard{}}
	for _, c := range cards {
		snapshot.Cards = append(snapshot.Cards, snapshotCard(c))
	}
	run := runSnapshotEnvelope{SchemaVersion: 4, Phase: "finalized", RunID: runID, Score: &score, Passed: &passed, BacklogSnapshot: snapshot}
	body, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "runs", runID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.json"), body, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCLIPassPinsRunOwnerVersionsAndApplication(t *testing.T) {
	for _, scenario := range []string{"success", "missing run", "latest", "all", "wrong owner", "stale version", "old application", "missing application", "missing version", "direct transition"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			db := filepath.Join(dir, "backlog.sqlite3")
			store, err := openStore(db)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			fixtureObjectives(t, store)
			seedBacklog(t, store, 0, "B-003", Card{ID: "B-001", Status: "APPLIED", Owner: "verifier:test", Title: "measured"}, Card{ID: "B-002", Status: "APPLIED", Title: "later"})
			card, _ := store.getCard("B-001")
			measured := card
			if scenario == "old application" {
				measured.ApplicationID = "B-001@older"
			}
			if scenario == "missing application" {
				measured.ApplicationID = ""
			}
			writeAdoptionRun(t, dir, "20260901-120000", measured)
			later, _ := store.getCard("B-002")
			writeAdoptionRun(t, dir, "20260901-130000", later)
			run, owner, version, ids := "20260901-120000", "verifier:test", "B-001=0", "B-001"
			switch scenario {
			case "missing run":
				run = ""
			case "latest":
				run = "latest"
			case "all":
				ids = "all"
			case "wrong owner":
				owner = "verifier:other"
			case "stale version":
				version = "B-001=1"
			case "missing version":
				version = ""
			}
			args := []string{"pass", ids, "--actor", "task:pass", "--owner", owner, "--expect-card-versions", version, "--evidence-run", run, "--force=true"}
			if scenario == "direct transition" {
				args = []string{"transition", "B-001", "--status", "VALIDATED", "--expect-card-version", "0", "--actor", owner, "--reason", "bypass"}
			}
			result := runBacklogCLI(t, db, "", args...)
			if scenario == "success" {
				if result.err != nil || !strings.Contains(result.stdout, "run_id=20260901-120000") {
					t.Fatalf("explicit pass failed: %#v", result)
				}
				adopted, _ := store.getCard(card.ID)
				if adopted.Status != "VALIDATED" || adopted.Owner != "" {
					t.Fatalf("adoption = %#v", adopted)
				}
				var application string
				if err := store.db.QueryRow(`SELECT application_id FROM adoption_event_cards`).Scan(&application); err != nil || application != card.ApplicationID {
					t.Fatalf("recorded application=%s, err=%v", application, err)
				}
			} else {
				if result.err == nil {
					t.Fatalf("unsafe pass accepted: %#v", result)
				}
				unchanged, _ := store.getCard(card.ID)
				if unchanged.Status != "APPLIED" || unchanged.Version != 0 {
					t.Fatalf("failed pass mutated card: %#v", unchanged)
				}
				var events int
				store.db.QueryRow(`SELECT COUNT(*) FROM adoption_events`).Scan(&events)
				if events != 0 {
					t.Fatal("failed pass recorded adoption")
				}
			}
			later, _ = store.getCard(later.ID)
			if later.Status != "APPLIED" {
				t.Fatal("pass changed a card outside the selected RUN")
			}
		})
	}
}

func TestApplicationMigrationPreservesHistoryAndIsIdempotent(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 12, "B-003", Card{ID: "B-001", Status: "APPLIED", Version: 9, Owner: "old-worker", Title: "legacy", History: []HistoryEntry{{Body: "historical deployment"}}}, Card{ID: "B-002", Status: "DOING", Title: "unfinished"})
	for _, table := range []string{"cards", "adoption_event_cards"} {
		if _, err := store.db.Exec(`ALTER TABLE ` + table + ` DROP COLUMN application_id`); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := store.migrateApplicationIDs(); err != nil {
			t.Fatal(err)
		}
	}
	c, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if c.ApplicationID != "legacy:B-001@9" || c.Version != 9 || c.Owner != "old-worker" || len(c.History) != 1 {
		t.Fatalf("migration changed evidence: %#v", c)
	}
	rev, _ := store.revision()
	if rev != 12 {
		t.Fatalf("migration revision = %d", rev)
	}
	pending, _ := store.getCard("B-002")
	if pending.ApplicationID != "" {
		t.Fatal("migration invented an application")
	}
}

func TestAppliedHasNoCountLimit(t *testing.T) {
	store := testStore(t)
	var cards []Card
	for i := 1; i <= 11; i++ {
		cards = append(cards, Card{ID: fmt.Sprintf("B-%03d", i), Status: "VERIFY", Owner: "worker:test", Title: "change"})
	}
	seedBacklog(t, store, 0, "B-012", cards...)
	for _, c := range cards {
		if err := store.transitionCard(c.ID, "APPLIED", mutation{Actor: "worker:test", ExpectedCardVersion: intPtr(0)}, "checked application"); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.appliedSnapshot()
	if err != nil || len(snapshot.Cards) != 11 {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
}
