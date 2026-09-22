package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
	for _, command := range []string{"init", "add", "update", "resolve", "transition", "history", "pass"} {
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
		if !mutatesBacklog("target", []string{subcommand}) {
			t.Errorf("target %s should mutate backlog", subcommand)
		}
	}
	if mutatesBacklog("target", []string{"list"}) || mutatesBacklog("target", []string{"show"}) {
		t.Fatal("target read command was classified as a mutation")
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
	fixtureObjectives(t, store)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Title: "restored card"})
	targetID, err := store.addTarget(NewTarget{Axis: "response time", Goal: "reduce response time below 5 ms", Evaluation: "compare saved results at equal load", ObjectiveID: "O-001", Title: "restored target", Fingerprint: "target:v1:restore", Scope: "app CPU", Evidence: "1 core-s / 2 core-s", Resolution: "queue wait is absent"}, mutation{Actor: "skill:test", Operation: "target.add"}, "create restorable target")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setTargetLink(targetID, "B-001", true, "IMPROVES", "", 0, mutation{Actor: "skill:test", Operation: "target.link"}, "link restorable card"); err != nil {
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
	target, err := restored.getTarget(targetID)
	if err != nil {
		t.Fatal(err)
	}
	if target.Title != "restored target" || len(target.CardIDs) != 1 || target.CardIDs[0] != "B-001" {
		t.Fatalf("restored target = %#v", target)
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
  "Change boundary": "replace the repeated query as one implementation and adoption boundary"
}`

	got, err := readSectionStdin(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Observation":     "measured 12ms",
		"Hypothesis":      "the query is repeated",
		"Change boundary": "replace the repeated query as one implementation and adoption boundary",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sections = %#v, want %#v", got, want)
	}
}

func TestReadSectionStdinRejectsInvalidInput(t *testing.T) {
	tests := map[string]string{
		"non-object":         `[]`,
		"null":               `null`,
		"empty object":       `{}`,
		"empty section name": `{"": "body"}`,
		"unknown section":    `{"Invalid section": "body"}`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := readSectionStdin(strings.NewReader(input)); err == nil {
				t.Fatalf("readSectionStdin(%q) unexpectedly succeeded", input)
			}
		})
	}
}
