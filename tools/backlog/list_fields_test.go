package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIListFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backlog.sqlite3")
	store, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fixtureObjectives(t, store)
	seedBacklog(t, store, 7, "B-003",
		Card{ID: "B-001", Version: 2, Status: "READY", Title: "Selected card"},
		Card{ID: "B-002", Version: 3, Status: "DOING", Title: "Excluded card"},
	)
	for _, command := range [][]string{{"intervention", "list"}} {
		args := append(command, "--status", "READY", "--format", "json", "--fields", "id, version,closed,owner,id")
		var output map[string]json.RawMessage
		requireCLIJSON(t, runBacklogCLI(t, dbPath, "", args...), &output)
		if len(output) != 2 || string(output["backlog_revision"]) != "7" {
			t.Fatalf("unexpected envelope: %s", output)
		}
		var cards []map[string]any
		if err := json.Unmarshal(output["cards"], &cards); err != nil {
			t.Fatal(err)
		}
		if len(cards) != 1 || len(cards[0]) != 4 || cards[0]["id"] != "B-001" || cards[0]["version"] != float64(2) || cards[0]["closed"] != false || cards[0]["owner"] != "" {
			t.Fatalf("selected fields/types/filter mismatch: %#v", cards)
		}
	}
	var empty struct {
		Cards []map[string]any `json:"cards"`
	}
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "list", "--owner", "missing", "--format=json", "--fields=id"), &empty)
	if empty.Cards == nil || len(empty.Cards) != 0 {
		t.Fatalf("empty cards must be []: %#v", empty)
	}
	for _, fields := range []string{"", "unknown", "history"} {
		result := runBacklogCLI(t, dbPath, "", "list", "--format=json", "--fields="+fields)
		if result.err == nil || result.stdout != "" || !strings.Contains(result.stderr, "invalid --fields entry") {
			t.Fatalf("invalid fields %q accepted: %#v", fields, result)
		}
	}
	result := runBacklogCLI(t, dbPath, "", "list", "--fields=id")
	if result.err == nil || !strings.Contains(result.stderr, "--fields requires --format json") {
		t.Fatalf("text fields accepted: %#v", result)
	}
}
