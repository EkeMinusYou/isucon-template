package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestDashboardReadsCurrentBacklogSchema(t *testing.T) {
	schema, err := os.ReadFile(filepath.Join("..", "..", "backlog", "backlog.sql"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "backlog.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO cards(id, status, title, priority, owner, area, fingerprint, updated, updated_by) VALUES ('B-001', 'READY', 'current intervention', 'P1', '', 'app', 'query:v1', '2026-09-04T00:00:00Z', 'skill:test')`,
		`INSERT INTO cards(id, status, title) VALUES ('B-002', 'APPLIED', 'prerequisite')`,
		`INSERT INTO card_sections(card_id, name, position, body) VALUES ('B-001', 'Hypothesis', 0, 'remove repeated query')`,
		`INSERT INTO card_history(card_id, position, occurred_at, actor, body) VALUES ('B-001', 0, '2026-09-04T00:00:00Z', 'skill:test', 'investigated')`,
		`INSERT INTO card_dependencies(card_id, depends_on_card_id, required_status, mode, reason) VALUES ('B-001', 'B-002', 'APPLIED', 'BLOCKING', 'deploy prerequisite first')`,
		`INSERT INTO objective_interventions(objective_id, card_id, rationale) VALUES ('O-003', 'B-001', 'increase throughput')`,
		`INSERT INTO constraints(id, status, title, fingerprint, scope, evidence, resolution) VALUES ('A-001', 'ACTIVE', 'database wait', 'db-wait:v1', 'app requests', 'slow query log', 'wait is no longer limiting')`,
		`INSERT INTO constraint_interventions(card_id, constraint_id, role, rationale) VALUES ('B-001', 'A-001', 'MITIGATES', 'reduce database work')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	list, err := queryBacklog(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Cards) != 2 || list.Counts["READY"] != 1 || list.Counts["APPLIED"] != 1 {
		t.Fatalf("unexpected backlog list: %#v", list)
	}
	detail, err := queryBacklogCard(dbPath, "B-001")
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil || len(detail.Objectives) != 1 || len(detail.Constraints) != 1 || len(detail.Dependencies) != 1 {
		t.Fatalf("unexpected backlog detail: %#v", detail)
	}
	if detail.Objectives[0].ID != "O-003" || detail.Constraints[0].ID != "A-001" || detail.Dependencies[0].DependsOnCardID != "B-002" {
		t.Fatalf("unexpected relations: %#v", detail)
	}
}
