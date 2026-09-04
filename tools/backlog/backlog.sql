PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO metadata VALUES('backlog_revision','0');
INSERT INTO metadata VALUES('next_id','B-001');
INSERT INTO metadata VALUES('next_constraint_id','A-001');
INSERT INTO metadata VALUES('next_objective_id','O-004');
INSERT INTO metadata VALUES('objective_model_seed_version','1');
CREATE TABLE cards (
    id TEXT PRIMARY KEY,
    card_version INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('INVESTIGATE', 'READY', 'DOING', 'VERIFY', 'APPLIED', 'BLOCKED', 'VALIDATED', 'REJECTED')),
    title TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT '',
    owner TEXT NOT NULL DEFAULT '',
    area TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL DEFAULT '',
    updated TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT ''
);
CREATE TABLE card_runs (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL,
    relation TEXT NOT NULL CHECK (relation IN ('SOURCE', 'COMPARE', 'OBSERVED')),
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (card_id, run_id, relation)
);
CREATE TABLE card_dependencies (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    depends_on_card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    required_status TEXT NOT NULL CHECK (required_status IN ('VERIFY', 'APPLIED', 'VALIDATED')),
    mode TEXT NOT NULL CHECK (mode IN ('ORDERING', 'BLOCKING')),
    reason TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, depends_on_card_id),
    CHECK (card_id <> depends_on_card_id)
);
CREATE TABLE objectives (
    id TEXT PRIMARY KEY,
    objective_version INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'RETIRED')),
    mode TEXT NOT NULL CHECK (mode IN ('SATISFY', 'MAXIMIZE', 'MINIMIZE')),
    title TEXT NOT NULL,
    metric_or_predicate TEXT NOT NULL,
    required_for_valid_result INTEGER NOT NULL DEFAULT 0 CHECK (required_for_valid_result IN (0, 1)),
    parent_objective_id TEXT NOT NULL DEFAULT '',
    official_sources TEXT NOT NULL DEFAULT '',
    verification TEXT NOT NULL,
    updated TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT ''
);
INSERT INTO objectives VALUES('O-001',0,'ACTIVE','SATISFY','ベンチマークと整合性チェックを通過する','benchmark result is valid and every required correctness check passes',1,'','docs/official/','verify the final benchmark result and correctness log against the official rules','2026-09-04T17:46:45+09:00','system:migration');
INSERT INTO objectives VALUES('O-002',0,'ACTIVE','SATISFY','再起動後の永続性と再現性条件を満たす','the official restart and reproducibility requirements are satisfied',1,'','docs/official/','restart the required servers and rerun the official verification procedure','2026-09-04T17:46:45+09:00','system:migration');
INSERT INTO objectives VALUES('O-003',0,'ACTIVE','MAXIMIZE','有効なベンチマークスコアを最大化する','final score of a benchmark run that satisfies all validity requirements',0,'','docs/official/','use the finalized benchmark score and bench log','2026-09-04T17:46:45+09:00','system:migration');
CREATE TABLE objective_history (
    objective_id TEXT NOT NULL REFERENCES objectives(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    occurred_at TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (objective_id, position)
);
INSERT INTO objective_history VALUES('O-001',0,'2026-09-04T17:46:45+09:00','system:migration','initial objective registered from official specification');
INSERT INTO objective_history VALUES('O-002',0,'2026-09-04T17:46:45+09:00','system:migration','initial objective registered from official specification');
INSERT INTO objective_history VALUES('O-003',0,'2026-09-04T17:46:45+09:00','system:migration','initial objective registered from official specification');
CREATE TABLE constraints (
    id TEXT PRIMARY KEY,
    constraint_version INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'RESOLVED', 'INVALIDATED', 'MERGED')),
    title TEXT NOT NULL,
	priority TEXT NOT NULL DEFAULT '',
	fingerprint TEXT NOT NULL UNIQUE,
	scope TEXT NOT NULL DEFAULT '',
    source_runs TEXT NOT NULL DEFAULT '',
    observed_runs TEXT NOT NULL DEFAULT '',
    evidence TEXT NOT NULL DEFAULT '',
    resolution TEXT NOT NULL DEFAULT '',
	merged_into_constraint_id TEXT NOT NULL DEFAULT '',
    updated TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT ''
);
CREATE TABLE constraint_interventions (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'RESOLVES' CHECK (role IN ('RESOLVES', 'MITIGATES')),
    rationale TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, constraint_id)
);
CREATE TABLE constraint_intervention_assessments (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    assessment_json TEXT NOT NULL,
    constraint_definition_hash TEXT NOT NULL DEFAULT '',
    card_definition_hash TEXT NOT NULL DEFAULT '',
    updated TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, constraint_id)
);
CREATE TABLE constraint_history (
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    occurred_at TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (constraint_id, position)
);
CREATE TABLE objective_constraints (
    objective_id TEXT NOT NULL REFERENCES objectives(id) ON DELETE CASCADE,
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    PRIMARY KEY (objective_id, constraint_id)
);
CREATE TABLE objective_interventions (
    objective_id TEXT NOT NULL REFERENCES objectives(id) ON DELETE CASCADE,
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    rationale TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (objective_id, card_id)
);
CREATE TABLE card_sections (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, name)
);
CREATE TABLE card_history (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    occurred_at TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    raw TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, position)
);
CREATE TABLE adoption_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL CHECK (run_id <> ''),
    adopted_at TEXT NOT NULL,
    actor TEXT NOT NULL,
    forced INTEGER NOT NULL CHECK (forced IN (0, 1)),
    score INTEGER,
    passed INTEGER CHECK (passed IS NULL OR passed IN (0, 1)),
    comparison_run_id TEXT NOT NULL DEFAULT '',
    comparison_score INTEGER,
    comparison_status TEXT NOT NULL CHECK (comparison_status IN ('none', 'compatible', 'incompatible', 'unverified')),
    delta INTEGER,
    manifest_sha256 TEXT NOT NULL CHECK (
        length(manifest_sha256) = 71
        AND substr(manifest_sha256, 1, 7) = 'sha256:'
        AND substr(manifest_sha256, 8) NOT GLOB '*[^0-9a-f]*'
    ),
    snapshot_revision INTEGER NOT NULL CHECK (snapshot_revision >= 0)
);
CREATE TABLE adoption_event_cards (
    adoption_event_id INTEGER NOT NULL REFERENCES adoption_events(id) ON DELETE CASCADE,
    card_id TEXT NOT NULL REFERENCES cards(id),
    origin TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL DEFAULT '',
    definition_hash TEXT NOT NULL,
    PRIMARY KEY (adoption_event_id, card_id)
);
CREATE TABLE change_log (
    revision INTEGER PRIMARY KEY,
    occurred_at TEXT NOT NULL,
    actor TEXT NOT NULL,
    operation TEXT NOT NULL,
    card_id TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_cards_status ON cards(status);
CREATE INDEX idx_cards_area ON cards(area);
CREATE INDEX idx_cards_priority ON cards(priority);
CREATE INDEX idx_card_runs_run ON card_runs(run_id, relation, card_id);
CREATE INDEX idx_history_card ON card_history(card_id, position);
CREATE INDEX idx_adoption_events_run ON adoption_events(run_id, id);
CREATE INDEX idx_adoption_event_cards_card ON adoption_event_cards(card_id, adoption_event_id);
CREATE INDEX idx_sections_card ON card_sections(card_id, position);
CREATE INDEX idx_dependencies_target ON card_dependencies(depends_on_card_id, card_id);
CREATE INDEX idx_constraints_status ON constraints(status);
CREATE INDEX idx_constraint_interventions_constraint ON constraint_interventions(constraint_id, card_id);
CREATE INDEX idx_constraint_intervention_assessments_constraint ON constraint_intervention_assessments(constraint_id, card_id);
CREATE INDEX idx_objective_constraints_constraint ON objective_constraints(constraint_id, objective_id);
CREATE INDEX idx_objective_interventions_card ON objective_interventions(card_id, objective_id);
COMMIT;
