package main

import "testing"

func TestLegacyTargetMigrationPreservesHistoricalJudgments(t *testing.T) {
	s := testStore(t)
	statements := []string{
		`CREATE TABLE constraints(id TEXT PRIMARY KEY,constraint_version INTEGER,status TEXT,title TEXT,priority TEXT,fingerprint TEXT,scope TEXT,source_runs TEXT,observed_runs TEXT,evidence TEXT,resolution TEXT,merged_into_constraint_id TEXT,updated TEXT,updated_by TEXT)`,
		`CREATE TABLE constraint_history(constraint_id TEXT,position INTEGER,occurred_at TEXT,actor TEXT,body TEXT)`,
		`CREATE TABLE objective_constraints(objective_id TEXT,constraint_id TEXT)`,
		`CREATE TABLE constraint_interventions(card_id TEXT,constraint_id TEXT,role TEXT,rationale TEXT)`,
		`CREATE TABLE constraint_intervention_assessments(card_id TEXT,constraint_id TEXT,assessment_json TEXT,constraint_definition_hash TEXT,card_change_boundary_hash TEXT,updated TEXT,updated_by TEXT)`,
		`INSERT INTO metadata(key,value) VALUES('next_constraint_id','A-003')`,
		`INSERT INTO constraints VALUES('A-001',7,'INVALIDATED','old premise','','old','CPU','','','original evidence','original exit condition','','2026-09-06','human:old')`,
		`INSERT INTO constraints VALUES('A-002',8,'RESOLVED','old success','','success','CPU','','','old comparison','old resolved condition','','2026-09-06','human:old')`,
		`INSERT INTO constraint_history VALUES('A-001',0,'2026-09-06','human:old','original judgment')`,
		`INSERT INTO objective_constraints VALUES('O-001','A-001'),('O-001','A-002')`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migrateLegacyTargets(); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateLegacyTargets(); err != nil {
		t.Fatal(err)
	}
	target, err := s.getTarget("A-001")
	if err != nil {
		t.Fatal(err)
	}
	if target.Status != "RETIRED" || target.Version != 7 || target.Goal != "original exit condition" || len(target.History) != 1 || target.History[0].Body != "original judgment" {
		t.Fatalf("lost history: %#v", target)
	}
	resolved, _ := s.getTarget("A-002")
	if resolved.Status != "RESOLVED" || resolved.CompletionEvidence != "" {
		t.Fatalf("retroactively certified goal: %#v", resolved)
	}
	var oldStatus string
	if err := s.db.QueryRow(`SELECT status FROM constraints WHERE id='A-001'`).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != "INVALIDATED" {
		t.Fatalf("legacy judgment rewritten: %s", oldStatus)
	}
	next, _ := s.metadata("next_target_id")
	if next != "A-003" {
		t.Fatalf("next=%s", next)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
}
