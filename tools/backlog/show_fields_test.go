package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCLIShowFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backlog.sqlite3")
	s, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fixtureObjectives(t, s)
	fixtureTarget(t, s)
	seedBacklog(t, s, 7, "B-002", Card{
		ID: "B-001", Version: 2, Status: "READY", Title: "Selected details",
		History: []HistoryEntry{{Position: 0, Actor: "human:test", Body: "Keep history order and 日本語"}},
	})
	for _, tc := range []struct {
		command []string
		id      string
		fields  string
	}{
		{[]string{"show"}, "B-001", "id,version,closed,owner,sections,history,dependencies,target_assessments"},
		{[]string{"intervention", "show"}, "B-001", "id,version,sections,history"},
		{[]string{"target", "show"}, "A-001", "id,version,goal,evaluation,objective_ids,history"},
		{[]string{"objective", "show"}, "O-001", "id,version,priority,required_for_valid_result,metric_or_predicate,history"},
	} {
		t.Run(strings.Join(tc.command, "-"), func(t *testing.T) {
			var full map[string]json.RawMessage
			requireCLIJSON(t, runBacklogCLI(t, dbPath, "", append(tc.command, tc.id, "--format=json")...), &full)
			for _, flags := range [][]string{
				{tc.id, "--fields", tc.fields + ", id ", "--format", "json"},
				{"--format=json", "--fields=" + tc.fields, tc.id},
			} {
				var got map[string]json.RawMessage
				requireCLIJSON(t, runBacklogCLI(t, dbPath, "", append(tc.command, flags...)...), &got)
				expected := make(map[string]json.RawMessage)
				for _, key := range strings.Split(tc.fields, ",") {
					expected[key] = full[key]
					if expected[key] == nil {
						expected[key] = json.RawMessage("null")
						if key == "target_assessments" {
							// getCard returns an empty map, omitted in full JSON.
							expected[key] = json.RawMessage("{}")
						}
					}
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("projection changed keys, values, or types:\ngot %s\nwant %s", got, expected)
				}
			}
			for _, fields := range []string{"", "unknown", "id,,version", "sections.name"} {
				result := runBacklogCLI(t, dbPath, "", append(tc.command, tc.id, "--format=json", "--fields="+fields)...)
				if result.err == nil || result.stdout != "" || !strings.Contains(result.stderr, "invalid --fields entry") || !strings.Contains(result.stderr, "available fields:") {
					t.Fatalf("invalid fields %q: %#v", fields, result)
				}
			}
			result := runBacklogCLI(t, dbPath, "", append(tc.command, tc.id, "--fields=id")...)
			if result.err == nil || result.stdout != "" || !strings.Contains(result.stderr, "--fields requires --format json") {
				t.Fatalf("text fields accepted: %#v", result)
			}
		})
	}
}
