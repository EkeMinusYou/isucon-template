package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestImplementationEstimateCLI(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backlog.sqlite3")
	var receipt cardMutationReceipt
	var card Card
	read := func() {
		t.Helper()
		requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "show", "B-001", "--format", "json"), &card)
	}
	result := runBacklogCLI(t, dbPath, "", "add", "--title", "estimate workflow", "--implementation-estimate-minutes", "20", "--actor", "human:test", "--reason", "create")
	if result.err == nil || !strings.Contains(result.stderr, "flag provided but not defined") {
		t.Fatalf("add accepted an implementation estimate: %#v", result)
	}
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "add", "--title", "estimate workflow", "--actor", "human:test", "--reason", "create", "--format", "json"), &receipt)
	read()
	if card.ImplementationEstimateMinutes != nil {
		t.Fatalf("new cards must not have an estimate: %#v", card)
	}
	for _, value := range []string{"0", "-1", "1.5", "30m", "unknown", "999999999999999999999999999"} {
		before := card
		result := runBacklogCLI(t, dbPath, "", "update", "B-001", "--implementation-estimate-minutes", value, "--title", "must not persist", "--expect-card-version", "0", "--actor", "human:test", "--reason", "invalid")
		if result.err == nil || !strings.Contains(result.stderr, "positive whole number") {
			t.Fatalf("invalid %q: %#v", value, result)
		}
		read()
		if !reflect.DeepEqual(before, card) {
			t.Fatalf("invalid update changed card: %#v", card)
		}
		result = runBacklogCLI(t, dbPath, "", "add", "--title", "invalid", "--implementation-estimate-minutes", value, "--actor", "human:test", "--reason", "invalid")
		if result.err == nil || !strings.Contains(result.stderr, "flag provided but not defined") {
			t.Fatalf("add accepted %q: %#v", value, result)
		}
	}
	contract := `{"Hypothesis":"reduce repeated work","Change boundary":"batch reads; implementation 20m + local checks 10m","Evaluation":"compare saved RUNs"}`
	requireCLIJSON(t, runBacklogCLI(t, dbPath, contract, "resolve", "B-001", "--status", "READY", "--priority", "P1", "--implementation-estimate-minutes", "30", "--section-stdin", "--expect-card-version", "0", "--actor", "skill:isucon-investigate", "--reason", "estimate final scope", "--format", "json"), &receipt)
	read()
	if card.Status != "READY" || card.Version != 1 || *card.ImplementationEstimateMinutes != 30 || !strings.Contains(sectionBody(card.Sections, "Change boundary"), "local checks") {
		t.Fatalf("resolved card: %#v", card)
	}
	before := card
	result = runBacklogCLI(t, dbPath, "", "update", "B-001", "--implementation-estimate-minutes", "60", "--expect-card-version", "0", "--actor", "human:test", "--reason", "stale")
	if result.err == nil || !strings.Contains(result.stderr, "card version conflict") {
		t.Fatalf("stale update: %#v", result)
	}
	read()
	if !reflect.DeepEqual(before, card) {
		t.Fatal("stale update changed card")
	}
	for _, args := range [][]string{nil, {"list"}, {"show", "B-001"}} {
		result := runBacklogCLI(t, dbPath, "", args...)
		if result.err != nil || !strings.Contains(result.stdout, "30m") {
			t.Fatalf("estimate missing from %v: %#v", args, result)
		}
	}
	result = runBacklogCLI(t, dbPath, "", "list", "--format", "json", "--fields", "id,implementation_estimate_minutes")
	if result.err != nil || !strings.Contains(result.stdout, `"implementation_estimate_minutes": 30`) {
		t.Fatalf("projected list: %#v", result)
	}
	if got := implementationEstimateLabel(Card{}); got != "-" {
		t.Fatalf("unset list estimate label = %q, want %q", got, "-")
	}
	// The persisted SQL dump must restore the numeric estimate.
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	result = runBacklogCLI(t, dbPath, "", "show", "B-001")
	if result.err != nil || !strings.Contains(result.stdout, "30m") {
		t.Fatalf("restored estimate: %#v", result)
	}
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "update", "B-001", "--title", "renamed", "--expect-card-version", "1", "--actor", "human:test", "--reason", "rename", "--format", "json"), &receipt)
	read()
	if *card.ImplementationEstimateMinutes != 30 {
		t.Fatal("omission cleared estimate")
	}
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "update", "B-001", "--implementation-estimate-minutes", "", "--expect-card-version", "2", "--actor", "human:test", "--reason", "scope uncertain", "--format", "json"), &receipt)
	read()
	if card.ImplementationEstimateMinutes != nil {
		t.Fatal("estimate was not cleared")
	}
	result = runBacklogCLI(t, dbPath, "")
	if result.err != nil || strings.Contains(result.stdout, "未見積り") {
		t.Fatalf("unset estimate: %#v", result)
	}
}

func TestImplementationEstimateLegacyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite3")
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.addCard(NewCard{Title: "legacy", Actor: "human:test", Reason: "create"}, mutation{Actor: "human:test"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.getCard(id)
	if err != nil {
		t.Fatal(err)
	}
	revision, _ := store.metadata("backlog_revision")
	if _, err := store.db.Exec(`ALTER TABLE cards DROP COLUMN implementation_estimate_minutes`); err != nil {
		t.Fatal(err)
	}
	store.Close()
	for i := 0; i < 2; i++ {
		store, err = openStore(path)
		if err != nil {
			t.Fatal(err)
		}
		after, err := store.getCard(id)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("migration changed legacy card: %#v; %v", after, err)
		}
		gotRevision, _ := store.metadata("backlog_revision")
		if gotRevision != revision {
			t.Fatal("migration changed backlog revision")
		}
		store.Close()
	}
}
