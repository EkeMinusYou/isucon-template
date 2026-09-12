package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardReadsCurrentBacklogSchema(t *testing.T) {
	schema, err := os.ReadFile(filepath.Join("..", "..", "backlog", "backlog.sql"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "backlog.sqlite3")
	// Restore dumps with the same CLI as backlog; its SQL dialect may be newer
	// than the embedded read-only dashboard driver (for example, unistr()).
	cmd := exec.Command("sqlite3", ":memory:")
	cmd.Stdin = strings.NewReader(string(schema) + "\nSELECT sql || ';' FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' AND sql IS NOT NULL ORDER BY rowid;\n")
	ddl, err := cmd.Output()
	if err != nil {
		t.Fatalf("read backlog schema: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(ddl)); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO objectives(id, status, mode, title, metric_or_predicate, verification) VALUES ('O-003', 'ACTIVE', 'MAXIMIZE', 'score', 'score', 'benchmark result')`,
		`INSERT INTO cards(id, status, title, priority, owner, area, updated, updated_by) VALUES ('B-001', 'READY', 'current intervention', 'P1', '', 'app', '2026-09-04T00:00:00Z', 'skill:test')`,
		`INSERT INTO cards(id, status, title) VALUES ('B-002', 'APPLIED', 'prerequisite')`,
		`INSERT INTO cards(id, status, title) VALUES ('B-003', 'VALIDATED', 'legacy terminal intervention')`,
		`INSERT INTO card_sections(card_id, name, position, body) VALUES ('B-001', 'Hypothesis', 0, 'remove repeated query')`,
		`INSERT INTO card_history(card_id, position, occurred_at, actor, body) VALUES ('B-001', 0, '2026-09-04T00:00:00Z', 'skill:test', 'investigated')`,
		`INSERT INTO card_dependencies(card_id, depends_on_card_id, required_status, mode, reason) VALUES ('B-001', 'B-002', 'APPLIED', 'BLOCKING', 'deploy prerequisite first')`,
		`INSERT INTO targets(id, status, title, fingerprint, scope, evidence, axis, goal, evaluation) VALUES ('A-001', 'ACTIVE', 'database requests', 'db-wait:v1', 'app requests', 'slow query log', 'latency', 'reduce round trips', 'compare matching workloads')`,
		`INSERT INTO target_interventions(card_id, target_id, role, rationale, is_primary) VALUES ('B-001', 'A-001', 'IMPROVES', 'reduce database work', 1)`,
		`INSERT INTO objective_targets(objective_id, target_id, rationale, is_primary) VALUES ('O-003', 'A-001', 'increase completed scoring actions', 1)`,
		`INSERT INTO objectives(id, status, mode, title, metric_or_predicate, verification) VALUES ('O-005', 'RETIRED', 'MINIMIZE', 'historical classification', 'wait', 'historical evidence')`,
		`INSERT INTO objective_targets(objective_id, target_id, rationale, is_primary) VALUES ('O-005', 'A-001', 'historical relation', 0)`,
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
	if detail == nil || len(detail.Objectives) != 1 || len(detail.Targets) != 1 || len(detail.Dependencies) != 1 {
		t.Fatalf("unexpected backlog detail: %#v", detail)
	}
	if !detail.Targets[0].IsPrimary || detail.Targets[0].Goal != "reduce round trips" || detail.Targets[0].Evaluation != "compare matching workloads" || detail.Targets[0].Axis != "latency" {
		t.Fatalf("missing target goal or primary relation: %#v", detail.Targets)
	}
	if detail.Objectives[0].TargetID != "A-001" || !detail.Objectives[0].IsPrimary {
		t.Fatalf("objective must be derived through its target: %#v", detail.Objectives)
	}
	if detail.Objectives[0].ID != "O-003" || detail.Targets[0].ID != "A-001" || detail.Dependencies[0].DependsOnCardID != "B-002" {
		t.Fatalf("unexpected relations: %#v", detail)
	}
	legacy, err := queryBacklogCard(dbPath, "B-003")
	if err != nil {
		t.Fatal(err)
	}
	if legacy == nil || !legacy.Closed || legacy.Status != "VALIDATED" || len(legacy.Targets) != 0 || len(legacy.Objectives) != 0 {
		t.Fatalf("legacy terminal card must remain readable without invented relations: %#v", legacy)
	}
}
