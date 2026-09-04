package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitialObjectivesAndRelations(t *testing.T) {
	store := testStore(t)
	objectives, err := store.listObjectives(ObjectiveFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(objectives) != 3 {
		t.Fatalf("initial objectives = %d, want 3", len(objectives))
	}
	objective, err := store.getObjective("O-003")
	if err != nil {
		t.Fatal(err)
	}
	if objective.Mode != "MAXIMIZE" || objective.ParentObjectiveID != "" {
		t.Fatalf("score objective = %#v", objective)
	}
	cardID, err := store.addCard(NewCard{Title: "selection change", Actor: "skill:test", Reason: "add intervention"}, mutation{Actor: "skill:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	objective, _ = store.getObjective("O-003")
	if err := store.setObjectiveRelation("O-003", cardID, "intervention", "increase valid score", true, objective.Version, mutation{Actor: "skill:test", Operation: "objective.link"}, "link intervention to score objective"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.ObjectiveIDs) != 1 || card.ObjectiveIDs[0] != "O-003" {
		t.Fatalf("card objectives = %#v", card.ObjectiveIDs)
	}
}

func TestConstraintRequiresActiveObjectiveRelation(t *testing.T) {
	store := testStore(t)
	if _, err := store.addConstraint(NewConstraint{Title: "unscoped", Fingerprint: "constraint:v1:unscoped", Scope: "hot request response time", Evidence: "10 response-s / load-window", Resolution: "less than 5 response-s / load-window"}, mutation{Actor: "skill:test", Operation: "constraint.add"}, "reject unscoped constraint"); err == nil {
		t.Fatal("constraint without objective unexpectedly accepted")
	}
	constraintID, err := store.addConstraint(NewConstraint{ObjectiveID: "O-003", Title: "current wait", Fingerprint: "constraint:v1:test", Scope: "hot request response time", Evidence: "10 response-s / load-window", Resolution: "less than 5 response-s / load-window"}, mutation{Actor: "skill:test", Operation: "constraint.add"}, "record observed constraint")
	if err != nil {
		t.Fatal(err)
	}
	objective, err := store.getObjective("O-003")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setObjectiveRelation("O-003", constraintID, "constraint", "", false, objective.Version, mutation{Actor: "skill:test", Operation: "objective.unlink"}, "attempt to orphan active constraint"); err == nil {
		t.Fatal("orphaned ACTIVE constraint unexpectedly accepted")
	}
}

func TestFourDigitExplicitIDsAdvanceSequences(t *testing.T) {
	store := testStore(t)
	cardID, err := store.addCard(NewCard{ID: "B-1000", Title: "four digit intervention", Actor: "human:test", Reason: "verify four digit support"}, mutation{Actor: "human:test", Operation: "add"})
	if err != nil || cardID != "B-1000" {
		t.Fatalf("four digit intervention = %q, %v", cardID, err)
	}
	constraintID, err := store.addConstraint(NewConstraint{ID: "A-1000", ObjectiveID: "O-003", Title: "four digit constraint", Fingerprint: "constraint:v1:four-digit", Scope: "loop time", Evidence: "1 response-s / load-window", Resolution: "less than 1 response-s / load-window"}, mutation{Actor: "human:test", Operation: "constraint.add"}, "verify four digit support")
	if err != nil || constraintID != "A-1000" {
		t.Fatalf("four digit constraint = %q, %v", constraintID, err)
	}
	objectiveID, err := store.addObjective(NewObjective{ID: "O-1000", Status: "ACTIVE", Mode: "MAXIMIZE", Title: "four digit objective", MetricOrPredicate: "score", Verification: "benchmark score"}, mutation{Actor: "human:test", Operation: "objective.add"}, "verify four digit support")
	if err != nil || objectiveID != "O-1000" {
		t.Fatalf("four digit objective = %q, %v", objectiveID, err)
	}
	for key, want := range map[string]string{"next_id": "B-1001", "next_constraint_id": "A-1001", "next_objective_id": "O-1001"} {
		got, err := store.metadata(key)
		if err != nil || got != want {
			t.Fatalf("%s = %q, %v; want %q", key, got, err, want)
		}
	}
}

func TestLegacyMeasurementCardsAreDestroyedAndKindColumnRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backlog.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `
CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO metadata VALUES ('backlog_revision','2'), ('next_id','B-003'), ('next_constraint_id','A-001');
CREATE TABLE cards (
  id TEXT PRIMARY KEY, card_version INTEGER NOT NULL DEFAULT 0,
  kind TEXT NOT NULL DEFAULT 'CHANGE', status TEXT NOT NULL, title TEXT NOT NULL,
  priority TEXT NOT NULL DEFAULT '', owner TEXT NOT NULL DEFAULT '', area TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL DEFAULT '', updated TEXT NOT NULL DEFAULT '', updated_by TEXT NOT NULL DEFAULT '',
  assessment_kind TEXT NOT NULL DEFAULT '', assessment_detail TEXT NOT NULL DEFAULT '',
  expected_score_effect TEXT NOT NULL DEFAULT '', attribution TEXT NOT NULL DEFAULT '', blocked_contract TEXT NOT NULL DEFAULT ''
);
INSERT INTO cards(id,kind,status,title) VALUES ('B-001','CHANGE','VALIDATED','keep'), ('B-002','MEASUREMENT','VALIDATED','destroy');
CREATE TABLE card_history (card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE, position INTEGER NOT NULL, occurred_at TEXT NOT NULL DEFAULT '', actor TEXT NOT NULL DEFAULT '', body TEXT NOT NULL DEFAULT '', raw TEXT NOT NULL DEFAULT '', PRIMARY KEY(card_id,position));
INSERT INTO card_history(card_id,position,body) VALUES ('B-002',0,'measurement history');
CREATE TABLE change_log (revision INTEGER PRIMARY KEY, occurred_at TEXT NOT NULL, actor TEXT NOT NULL, operation TEXT NOT NULL, card_id TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '');
INSERT INTO change_log VALUES (1,'','skill:test','add','B-001','change'), (2,'','skill:test','add','B-002','measurement');
`
	if _, err := db.Exec(legacy); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var cardCount, historyCount, logCount, kindColumns int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&cardCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM card_history`).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM change_log WHERE card_id = 'B-002'`).Scan(&logCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cards') WHERE name = 'kind'`).Scan(&kindColumns); err != nil {
		t.Fatal(err)
	}
	if cardCount != 1 || historyCount != 0 || logCount != 0 || kindColumns != 0 {
		t.Fatalf("migration counts cards=%d history=%d log=%d kind_columns=%d", cardCount, historyCount, logCount, kindColumns)
	}
}

func TestLegacyAnchorTablesMigrateToConstraintSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backlog.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `
CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO metadata VALUES ('backlog_revision','0'), ('next_id','B-001'), ('next_anchor_id','A-1000');
CREATE TABLE bottleneck_anchors (
 id TEXT PRIMARY KEY, anchor_version INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL,
 title TEXT NOT NULL, priority TEXT NOT NULL DEFAULT '', fingerprint TEXT NOT NULL UNIQUE,
 limiting_axis TEXT NOT NULL DEFAULT '', target TEXT NOT NULL DEFAULT '', source_runs TEXT NOT NULL DEFAULT '',
 observed_runs TEXT NOT NULL DEFAULT '', evidence TEXT NOT NULL DEFAULT '', resolution TEXT NOT NULL DEFAULT '',
 merged_into_anchor_id TEXT NOT NULL DEFAULT '', updated TEXT NOT NULL DEFAULT '', updated_by TEXT NOT NULL DEFAULT '');
INSERT INTO bottleneck_anchors VALUES ('A-999',2,'ACTIVE','legacy constraint','P0','legacy:v1','cpu','app','','','90 core-s / load-window','below 60 core-s / load-window','','','system:test');
CREATE TABLE bottleneck_anchor_history (anchor_id TEXT NOT NULL, position INTEGER NOT NULL, occurred_at TEXT NOT NULL DEFAULT '', actor TEXT NOT NULL DEFAULT '', body TEXT NOT NULL DEFAULT '', PRIMARY KEY(anchor_id,position));
INSERT INTO bottleneck_anchor_history VALUES ('A-999',0,'','system:test','legacy history');
CREATE TABLE card_bottleneck_anchors (card_id TEXT NOT NULL, anchor_id TEXT NOT NULL, PRIMARY KEY(card_id,anchor_id));
CREATE TABLE anchor_candidate_assessments (card_id TEXT NOT NULL, anchor_id TEXT NOT NULL, contract_json TEXT NOT NULL, updated TEXT NOT NULL DEFAULT '', updated_by TEXT NOT NULL DEFAULT '', PRIMARY KEY(card_id,anchor_id));
`
	if _, err := db.Exec(legacy); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	constraint, err := store.getConstraint("A-999")
	if err != nil {
		t.Fatal(err)
	}
	if constraint.Version != 2 || constraint.Title != "legacy constraint" || constraint.Scope != "app — cpu" || len(constraint.History) != 1 {
		t.Fatalf("migrated constraint = %#v", constraint)
	}
	for _, oldTable := range []string{"bottleneck_anchors", "bottleneck_anchor_history", "card_bottleneck_anchors", "anchor_candidate_assessments"} {
		exists, err := store.tableExists(oldTable)
		if err != nil || exists {
			t.Fatalf("legacy table %s exists=%t err=%v", oldTable, exists, err)
		}
	}
	if next, err := store.metadata("next_constraint_id"); err != nil || next != "A-1000" {
		t.Fatalf("next_constraint_id = %q, %v", next, err)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConstraintScopeAndAssessmentPayloadMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backlog.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := `
CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO metadata VALUES ('backlog_revision','0'), ('next_id','B-002'), ('next_constraint_id','A-002'), ('next_objective_id','O-001');
CREATE TABLE cards (id TEXT PRIMARY KEY, card_version INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, title TEXT NOT NULL, priority TEXT NOT NULL DEFAULT '', owner TEXT NOT NULL DEFAULT '', area TEXT NOT NULL DEFAULT '', fingerprint TEXT NOT NULL DEFAULT '', updated TEXT NOT NULL DEFAULT '', updated_by TEXT NOT NULL DEFAULT '');
INSERT INTO cards VALUES ('B-001',0,'INVESTIGATE','candidate','','','','candidate:v1','','');
CREATE TABLE constraints (id TEXT PRIMARY KEY, constraint_version INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, title TEXT NOT NULL, priority TEXT NOT NULL DEFAULT '', fingerprint TEXT NOT NULL UNIQUE, limiting_axis TEXT NOT NULL DEFAULT '', target TEXT NOT NULL DEFAULT '', source_runs TEXT NOT NULL DEFAULT '', observed_runs TEXT NOT NULL DEFAULT '', evidence TEXT NOT NULL DEFAULT '', resolution TEXT NOT NULL DEFAULT '', merged_into_constraint_id TEXT NOT NULL DEFAULT '', updated TEXT NOT NULL DEFAULT '', updated_by TEXT NOT NULL DEFAULT '');
INSERT INTO constraints VALUES ('A-001',0,'ACTIVE','legacy scope','','constraint:v1','response time','hot request','','','10 response-s / load-window','below 2 response-s / load-window','','','');
CREATE TABLE constraint_intervention_assessments (card_id TEXT NOT NULL, constraint_id TEXT NOT NULL, assessment_json TEXT NOT NULL, constraint_definition_hash TEXT NOT NULL DEFAULT '', card_definition_hash TEXT NOT NULL DEFAULT '', updated TEXT NOT NULL DEFAULT '', updated_by TEXT NOT NULL DEFAULT '', PRIMARY KEY(card_id,constraint_id));`
	if _, err := db.Exec(legacySchema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	legacyAssessment := `{"version":1,"classification":"ROOT","constraint_current":{"value":10,"unit":"response-s","denominator":"load-window","snapshot":"RUN test"},"initial_aggregate":{"value":10,"unit":"response-s","denominator":"load-window","snapshot":"RUN test"},"boundary_reduction":{"value":9,"unit":"response-s","denominator":"load-window","basis":"code evidence"},"additional_cost":{"value":0,"unit":"response-s","denominator":"load-window","basis":"none"},"predicted_residual":{"value":1,"unit":"response-s","denominator":"load-window","basis":"calculated"},"component_demands":[{"component":"app","demand":10,"capacity":20,"unit":"core-s"}],"resolution_threshold":{"value":2,"unit":"response-s","denominator":"load-window","basis":"next contributor"},"resolution_operator":"<=","resolution_satisfies":true,"resolution_reason":"below threshold"}`
	if _, err := db.Exec(`INSERT INTO constraint_intervention_assessments(card_id,constraint_id,assessment_json) VALUES ('B-001','A-001',?)`, legacyAssessment); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	constraint, err := store.getConstraint("A-001")
	if err != nil {
		t.Fatal(err)
	}
	if constraint.Scope != "hot request — response time" {
		t.Fatalf("migrated scope = %q", constraint.Scope)
	}
	columns, err := store.tableColumns("constraints")
	if err != nil {
		t.Fatal(err)
	}
	if !columns["scope"] || columns["limiting_axis"] || columns["target"] {
		t.Fatalf("migrated constraint columns = %#v", columns)
	}
	var raw, constraintHash, cardHash string
	if err := store.db.QueryRow(`SELECT assessment_json, constraint_definition_hash, card_definition_hash FROM constraint_intervention_assessments WHERE constraint_id='A-001' AND card_id='B-001'`).Scan(&raw, &constraintHash, &cardHash); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "classification") || strings.Contains(raw, "predicted_residual") || !strings.Contains(raw, `"axis"`) {
		t.Fatalf("migrated assessment = %s", raw)
	}
	if _, _, err := parsePerformanceResidualAssessment(raw); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if constraintHash != constraintDefinitionHash(constraint) || cardHash != cardDefinitionHash(card) {
		t.Fatal("migrated assessment hashes were not refreshed")
	}
}
