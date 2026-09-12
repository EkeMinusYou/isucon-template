PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO metadata VALUES('backlog_revision','0');
INSERT INTO metadata VALUES('next_id','B-001');
INSERT INTO metadata VALUES('next_target_id','A-001');
INSERT INTO metadata VALUES('next_objective_id','O-001');
CREATE TABLE cards (
    id TEXT PRIMARY KEY,
    card_version INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('INVESTIGATE', 'READY', 'DOING', 'VERIFY', 'APPLIED', 'BLOCKED', 'VALIDATED', 'REJECTED')),
    title TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT '',
    owner TEXT NOT NULL DEFAULT '',
    area TEXT NOT NULL DEFAULT '',
    updated TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT ''
);
CREATE TABLE legacy_target_exemptions (
 card_id TEXT PRIMARY KEY REFERENCES cards(id),
 reason TEXT NOT NULL
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
CREATE TABLE objective_history (
    objective_id TEXT NOT NULL REFERENCES objectives(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    occurred_at TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (objective_id, position)
);
CREATE TABLE targets (
    id TEXT PRIMARY KEY,
    target_version INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'RESOLVED', 'RETIRED', 'MERGED')),
    title TEXT NOT NULL,
	priority TEXT NOT NULL DEFAULT '',
	fingerprint TEXT NOT NULL,
	scope TEXT NOT NULL DEFAULT '',
    source_runs TEXT NOT NULL DEFAULT '',
    observed_runs TEXT NOT NULL DEFAULT '',
    evidence TEXT NOT NULL DEFAULT '',
    resolution TEXT NOT NULL DEFAULT '',
    axis TEXT NOT NULL DEFAULT '',
    goal TEXT NOT NULL DEFAULT '',
    evaluation TEXT NOT NULL DEFAULT '',
    previous_target_id TEXT NOT NULL DEFAULT '',
    completion_evidence TEXT NOT NULL DEFAULT '',
	merged_into_target_id TEXT NOT NULL DEFAULT '',
    updated TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT ''
);
CREATE TABLE target_interventions (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'IMPROVES' CHECK (role = 'IMPROVES'),
    legacy_role TEXT NOT NULL DEFAULT '',
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    rationale TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, target_id)
);
CREATE TABLE target_intervention_assessments (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    assessment_json TEXT NOT NULL,
    target_definition_hash TEXT NOT NULL DEFAULT '',
    card_change_boundary_hash TEXT NOT NULL DEFAULT '',
    updated TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, target_id)
);
CREATE TABLE target_history (
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    occurred_at TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (target_id, position)
);
CREATE TABLE objective_targets (
    objective_id TEXT NOT NULL REFERENCES objectives(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    rationale TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (objective_id, target_id)
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
    comparison_status TEXT NOT NULL CHECK (comparison_status IN ('none', 'compatible', 'incompatible')),
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
    change_boundary_hash TEXT NOT NULL,
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
CREATE INDEX idx_targets_status ON targets(status);
CREATE INDEX idx_target_interventions_target ON target_interventions(target_id, card_id);
CREATE INDEX idx_target_intervention_assessments_target ON target_intervention_assessments(target_id, card_id);
CREATE INDEX idx_objective_targets_target ON objective_targets(target_id, objective_id);
CREATE INDEX idx_objective_interventions_card ON objective_interventions(card_id, objective_id);
COMMIT;
