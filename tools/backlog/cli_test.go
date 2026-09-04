package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseGlobalUsesWideDefaultListWidth(t *testing.T) {
	config, args, err := parseGlobal(nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.wide != defaultListWidth {
		t.Fatalf("default list width = %d, want %d", config.wide, defaultListWidth)
	}
	if len(args) != 0 {
		t.Fatalf("remaining args = %#v, want none", args)
	}
}

func TestParseGlobalAcceptsDumpPath(t *testing.T) {
	config, args, err := parseGlobal([]string{"-dump", "/tmp/backlog.sql", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if config.dumpPath != "/tmp/backlog.sql" {
		t.Fatalf("dump path = %q, want /tmp/backlog.sql", config.dumpPath)
	}
	if got, want := strings.Join(args, ","), "list"; got != want {
		t.Fatalf("remaining args = %q, want %q", got, want)
	}
}

func TestMutatesBacklog(t *testing.T) {
	for _, command := range []string{"init", "add", "update", "resolve", "transition", "close", "history", "pass"} {
		if !mutatesBacklog(command, nil) {
			t.Errorf("mutatesBacklog(%q) = false, want true", command)
		}
	}
	for _, command := range []string{"list", "watch", "show", "validate", "evidence"} {
		if mutatesBacklog(command, nil) {
			t.Errorf("mutatesBacklog(%q) = true, want false", command)
		}
	}
	if !mutatesBacklog("dependency", []string{"add"}) || !mutatesBacklog("dependency", []string{"remove"}) || mutatesBacklog("dependency", []string{"list"}) {
		t.Fatal("dependency mutation classification is incorrect")
	}
	for _, subcommand := range []string{"add", "update", "transition", "link", "unlink", "assess"} {
		if !mutatesBacklog("anchor", []string{subcommand}) {
			t.Errorf("anchor %s should mutate backlog", subcommand)
		}
	}
	if mutatesBacklog("anchor", []string{"list"}) || mutatesBacklog("anchor", []string{"show"}) {
		t.Fatal("anchor read command was classified as a mutation")
	}
}

func TestDumpAndRestoreDatabase(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "backlog.sqlite3")
	dumpPath := filepath.Join(dir, "backlog.sql")
	store, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Title: "restored card"})
	constraintID, err := store.addConstraint(NewConstraint{ObjectiveID: "O-003", Title: "restored constraint", Fingerprint: "constraint:v1:restore", Scope: "app CPU", Evidence: "1 core-s / 2 core-s", Resolution: "queue wait is absent"}, mutation{Actor: "skill:test", Operation: "constraint.add"}, "create restorable constraint")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setConstraintLink(constraintID, "B-001", true, "", validConstraintAssessmentJSON(), 0, mutation{Actor: "skill:test", Operation: "constraint.link"}, "link restorable card"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dumpDatabase(dbPath, dumpPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := restoreDatabaseIfMissing(dbPath, dumpPath); err != nil {
		t.Fatal(err)
	}
	restored, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	card, err := restored.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Title != "restored card" || card.Status != "READY" {
		t.Fatalf("restored card = %#v", card)
	}
	constraint, err := restored.getConstraint(constraintID)
	if err != nil {
		t.Fatal(err)
	}
	if constraint.Title != "restored constraint" || len(constraint.CardIDs) != 1 || constraint.CardIDs[0] != "B-001" {
		t.Fatalf("restored constraint = %#v", constraint)
	}
}

func TestRestoreFailureDoesNotInstallDatabase(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "backlog.sqlite3")
	dumpPath := filepath.Join(dir, "backlog.sql")
	if err := os.WriteFile(dumpPath, []byte("not valid SQL;"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := restoreDatabaseIfMissing(dbPath, dumpPath); err == nil {
		t.Fatal("invalid dump unexpectedly restored")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("database exists after failed restore: %v", err)
	}
}

func TestReadSectionStdin(t *testing.T) {
	input := `{
  " Observation ": "measured 12ms\n\n",
  "Hypothesis": "the query is repeated",
  "Change boundary": "replace the repeated query as one adoption and rollback boundary"
}`

	got, err := readSectionStdin(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Observation":     "measured 12ms",
		"Hypothesis":      "the query is repeated",
		"Change boundary": "replace the repeated query as one adoption and rollback boundary",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sections = %#v, want %#v", got, want)
	}
}

func TestReadSectionStdinRejectsInvalidInput(t *testing.T) {
	tests := map[string]string{
		"empty input":        "",
		"non-object":         `[]`,
		"null":               `null`,
		"empty object":       `{}`,
		"empty section name": `{"": "body"}`,
		"non-string body":    `{"Observation": 12}`,
		"unknown section":    `{"Invalid section": "body"}`,
		"lowercase spelling": `{"observation": "body"}`,
		"metadata section":   `{"Touches": "body"}`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := readSectionStdin(strings.NewReader(input)); err == nil {
				t.Fatalf("readSectionStdin(%q) unexpectedly succeeded", input)
			}
		})
	}
}

func TestPassCardIDsAllSelectsAppliedCards(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-004",
		Card{ID: "B-001", Status: "APPLIED", Title: "first"},
		Card{ID: "B-002", Status: "READY", Title: "not applied"},
		Card{ID: "B-003", Status: "APPLIED", Title: "second"},
		Card{ID: "B-004", Status: "APPLIED", Title: "third"})

	ids, err := passCardIDs(store, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ids, ","), "B-001,B-003,B-004"; got != want {
		t.Fatalf("pass all IDs = %s, want %s", got, want)
	}
	promoted, err := store.promoteAppliedCards(ids, "task:backlog-pass", "benchmark passed")
	if err != nil {
		t.Fatal(err)
	}
	if len(promoted) != 3 || promoted[0].ID != "B-001" || promoted[1].ID != "B-003" || promoted[2].ID != "B-004" {
		t.Fatalf("promoted cards = %#v", promoted)
	}
	for _, id := range []string{"B-001", "B-003", "B-004"} {
		card, err := store.getCard(id)
		if err != nil {
			t.Fatal(err)
		}
		if card.Status != "VALIDATED" {
			t.Fatalf("card %s status = %s, want VALIDATED", id, card.Status)
		}
	}

	ids, err = passCardIDs(store, "B-002")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ids, ","), "B-002"; got != want {
		t.Fatalf("explicit pass IDs = %s, want %s", got, want)
	}
}
