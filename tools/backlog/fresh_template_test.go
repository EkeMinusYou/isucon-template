package main

import (
	"path/filepath"
	"testing"
)

func TestFreshTemplateDoesNotInventObjectives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backlog.sqlite3")
	for attempt := 0; attempt < 2; attempt++ {
		store, err := openStore(path)
		if err != nil {
			t.Fatal(err)
		}
		objectives, err := store.listObjectives(ObjectiveFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(objectives) != 0 {
			t.Fatalf("fresh template has objectives: %#v", objectives)
		}
		next, err := store.metadata("next_objective_id")
		if err != nil || next != "O-001" {
			t.Fatalf("next objective = %q: %v", next, err)
		}
		if err := store.validate(); err != nil {
			t.Fatal(err)
		}
		store.Close()
	}
}
