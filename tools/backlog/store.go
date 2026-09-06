package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cards (
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

CREATE TABLE IF NOT EXISTS card_runs (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL,
    relation TEXT NOT NULL CHECK (relation IN ('SOURCE', 'COMPARE', 'OBSERVED')),
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (card_id, run_id, relation)
);

CREATE TABLE IF NOT EXISTS card_dependencies (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    depends_on_card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    required_status TEXT NOT NULL CHECK (required_status IN ('VERIFY', 'APPLIED', 'VALIDATED')),
    mode TEXT NOT NULL CHECK (mode IN ('ORDERING', 'BLOCKING')),
    reason TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, depends_on_card_id),
    CHECK (card_id <> depends_on_card_id)
);

CREATE TABLE IF NOT EXISTS objectives (
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

CREATE TABLE IF NOT EXISTS objective_history (
    objective_id TEXT NOT NULL REFERENCES objectives(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    occurred_at TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (objective_id, position)
);

CREATE TABLE IF NOT EXISTS constraints (
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

CREATE TABLE IF NOT EXISTS constraint_interventions (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'RESOLVES' CHECK (role IN ('RESOLVES', 'MITIGATES')),
    rationale TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, constraint_id)
);

CREATE TABLE IF NOT EXISTS constraint_intervention_assessments (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    assessment_json TEXT NOT NULL,
    constraint_definition_hash TEXT NOT NULL DEFAULT '',
    card_change_boundary_hash TEXT NOT NULL DEFAULT '',
    updated TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, constraint_id)
);

CREATE TABLE IF NOT EXISTS constraint_history (
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    occurred_at TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (constraint_id, position)
);

CREATE TABLE IF NOT EXISTS objective_constraints (
    objective_id TEXT NOT NULL REFERENCES objectives(id) ON DELETE CASCADE,
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    PRIMARY KEY (objective_id, constraint_id)
);

CREATE TABLE IF NOT EXISTS objective_interventions (
    objective_id TEXT NOT NULL REFERENCES objectives(id) ON DELETE CASCADE,
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    rationale TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (objective_id, card_id)
);

CREATE TABLE IF NOT EXISTS card_sections (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, name)
);

CREATE TABLE IF NOT EXISTS card_history (
    card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    occurred_at TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    raw TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (card_id, position)
);

CREATE TABLE IF NOT EXISTS adoption_events (
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

CREATE TABLE IF NOT EXISTS adoption_event_cards (
    adoption_event_id INTEGER NOT NULL REFERENCES adoption_events(id) ON DELETE CASCADE,
    card_id TEXT NOT NULL REFERENCES cards(id),
    origin TEXT NOT NULL DEFAULT '',
    change_boundary_hash TEXT NOT NULL,
    PRIMARY KEY (adoption_event_id, card_id)
);

CREATE TABLE IF NOT EXISTS change_log (
    revision INTEGER PRIMARY KEY,
    occurred_at TEXT NOT NULL,
    actor TEXT NOT NULL,
    operation TEXT NOT NULL,
    card_id TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_cards_status ON cards(status);
CREATE INDEX IF NOT EXISTS idx_cards_area ON cards(area);
CREATE INDEX IF NOT EXISTS idx_cards_priority ON cards(priority);
CREATE INDEX IF NOT EXISTS idx_card_runs_run ON card_runs(run_id, relation, card_id);
CREATE INDEX IF NOT EXISTS idx_history_card ON card_history(card_id, position);
CREATE INDEX IF NOT EXISTS idx_adoption_events_run ON adoption_events(run_id, id);
CREATE INDEX IF NOT EXISTS idx_adoption_event_cards_card ON adoption_event_cards(card_id, adoption_event_id);
CREATE INDEX IF NOT EXISTS idx_sections_card ON card_sections(card_id, position);
CREATE INDEX IF NOT EXISTS idx_dependencies_target ON card_dependencies(depends_on_card_id, card_id);
CREATE INDEX IF NOT EXISTS idx_constraints_status ON constraints(status);
CREATE INDEX IF NOT EXISTS idx_constraint_interventions_constraint ON constraint_interventions(constraint_id, card_id);
CREATE INDEX IF NOT EXISTS idx_constraint_intervention_assessments_constraint ON constraint_intervention_assessments(constraint_id, card_id);
CREATE INDEX IF NOT EXISTS idx_objective_constraints_constraint ON objective_constraints(constraint_id, objective_id);
CREATE INDEX IF NOT EXISTS idx_objective_interventions_card ON objective_interventions(card_id, objective_id);
`

type Store struct {
	db *sql.DB
}

func openStore(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	dsn := path + "?_txlock=immediate"
	if strings.Contains(path, "?") {
		dsn = path + "&_txlock=immediate"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open backlog data: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db}
	if err := store.initialize(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) initialize() error {
	for _, statement := range []string{"PRAGMA busy_timeout = 5000", "PRAGMA foreign_keys = ON"} {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("configure backlog data: %w", err)
		}
	}
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("create backlog schema: %w", err)
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO metadata(key, value) VALUES
		('backlog_revision', '0'), ('next_id', 'B-001'), ('next_constraint_id', 'A-001'), ('next_objective_id', 'O-001')`); err != nil {
		return fmt.Errorf("initialize metadata: %w", err)
	}
	if err := s.initializeBaseObjectives(); err != nil {
		return fmt.Errorf("initialize objectives: %w", err)
	}
	return nil
}

func parseRunIDsStrict(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "none") {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' })
	if len(parts) == 0 {
		return nil, errors.New("RUN references require comma-separated RUN IDs in YYYYMMDD-HHMMSS form")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "runs/"))
		if len(value) != len("20060102-150405") {
			return nil, fmt.Errorf("invalid RUN ID %q; expected YYYYMMDD-HHMMSS", strings.TrimSpace(part))
		}
		if _, err := time.Parse("20060102-150405", value); err != nil {
			return nil, fmt.Errorf("invalid RUN ID %q; expected YYYYMMDD-HHMMSS", strings.TrimSpace(part))
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) metadata(key string) (string, error) {
	var value string
	if err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&value); err != nil {
		return "", fmt.Errorf("read metadata %s: %w", key, err)
	}
	return value, nil
}

func (s *Store) revision() (int, error) {
	value, err := s.metadata("backlog_revision")
	if err != nil {
		return 0, err
	}
	revision, err := strconv.Atoi(value)
	if err != nil || revision < 0 {
		return 0, fmt.Errorf("invalid backlog revision %q", value)
	}
	return revision, nil
}

func (s *Store) reconcileNextCardID() error {
	next, err := s.metadata("next_id")
	if err != nil {
		return err
	}
	nextNumber, err := idNumber(next)
	if err != nil {
		return fmt.Errorf("invalid next_id: %w", err)
	}
	rows, err := s.db.Query(`SELECT id FROM cards`)
	if err != nil {
		return err
	}
	defer rows.Close()
	maxNumber := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		number, err := idNumber(id)
		if err != nil {
			return fmt.Errorf("invalid card ID %q: %w", id, err)
		}
		if number > maxNumber {
			maxNumber = number
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if nextNumber > maxNumber {
		return nil
	}
	_, err = s.db.Exec(`UPDATE metadata SET value = ? WHERE key = 'next_id'`, formatID(maxNumber+1))
	return err
}

func (s *Store) cardCount() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) getCard(id string) (Card, error) { return getCardFrom(s.db, id) }

type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

const cardColumns = `
id, card_version, status, title, priority, owner, area, updated, updated_by`

func getCardFrom(q queryer, id string) (Card, error) {
	var card Card
	row := q.QueryRow(`SELECT `+cardColumns+` FROM cards WHERE id = ?`, id)
	err := row.Scan(
		&card.ID, &card.Version, &card.Status, &card.Title, &card.Priority, &card.Owner, &card.Area,
		&card.Updated, &card.UpdatedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Card{}, fmt.Errorf("card %s not found", id)
		}
		return Card{}, err
	}
	card.Closed = isClosedStatus(card.Status)
	if err := loadCardContent(q, &card); err != nil {
		return Card{}, err
	}
	return card, nil
}

func loadCardContent(q queryer, card *Card) error {
	if err := loadCardRuns(q, card); err != nil {
		return err
	}
	rows, err := q.Query(`SELECT constraint_id, assessment_json FROM constraint_intervention_assessments WHERE card_id = ? ORDER BY constraint_id`, card.ID)
	if err != nil {
		return err
	}
	card.ConstraintAssessments = map[string]string{}
	for rows.Next() {
		var anchorID, raw string
		if err := rows.Scan(&anchorID, &raw); err != nil {
			rows.Close()
			return err
		}
		card.ConstraintAssessments[anchorID] = raw
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = q.Query(`SELECT name, position, body FROM card_sections WHERE card_id = ? ORDER BY position`, card.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var section Section
		if err := rows.Scan(&section.Name, &section.Position, &section.Body); err != nil {
			rows.Close()
			return err
		}
		card.Sections = append(card.Sections, section)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = q.Query(`SELECT position, occurred_at, actor, body, raw FROM card_history WHERE card_id = ? ORDER BY position`, card.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var entry HistoryEntry
		if err := rows.Scan(&entry.Position, &entry.OccurredAt, &entry.Actor, &entry.Body, &entry.Raw); err != nil {
			rows.Close()
			return err
		}
		card.History = append(card.History, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = q.Query(`SELECT d.card_id, d.depends_on_card_id, d.required_status, d.mode, d.reason,
        target.status
        FROM card_dependencies d JOIN cards target ON target.id = d.depends_on_card_id
        WHERE d.card_id = ? ORDER BY d.depends_on_card_id`, card.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var dependency CardDependency
		if err := rows.Scan(&dependency.CardID, &dependency.DependsOnCardID, &dependency.RequiredStatus,
			&dependency.Mode, &dependency.Reason, &dependency.TargetStatus); err != nil {
			rows.Close()
			return err
		}
		card.Dependencies = append(card.Dependencies, dependency)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = q.Query(`SELECT d.card_id, d.depends_on_card_id, d.required_status, d.mode, d.reason,
        dependent.status
        FROM card_dependencies d JOIN cards dependent ON dependent.id = d.card_id
        WHERE d.depends_on_card_id = ? ORDER BY d.card_id`, card.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var dependency CardDependency
		if err := rows.Scan(&dependency.CardID, &dependency.DependsOnCardID, &dependency.RequiredStatus,
			&dependency.Mode, &dependency.Reason, &dependency.TargetStatus); err != nil {
			rows.Close()
			return err
		}
		card.Unblocks = append(card.Unblocks, dependency)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	rows, err = q.Query(`SELECT a.id, a.status, l.role FROM constraint_interventions l
        JOIN constraints a ON a.id = l.constraint_id
        WHERE l.card_id = ? ORDER BY a.id`, card.ID)
	if err != nil {
		return err
	}
	card.ConstraintRoles = map[string]string{}
	defer rows.Close()
	for rows.Next() {
		var id, status, role string
		if err := rows.Scan(&id, &status, &role); err != nil {
			return err
		}
		card.ConstraintIDs = append(card.ConstraintIDs, id)
		card.ConstraintRoles[id] = role
		if status == "ACTIVE" {
			card.ActiveConstraintIDs = append(card.ActiveConstraintIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = q.Query(`SELECT objective_id FROM objective_interventions WHERE card_id = ? ORDER BY objective_id`, card.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		card.ObjectiveIDs = append(card.ObjectiveIDs, id)
	}
	return rows.Err()
}

func loadCardRuns(q queryer, card *Card) error {
	rows, err := q.Query(`SELECT run_id, relation FROM card_runs WHERE card_id = ? ORDER BY relation, position, run_id`, card.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var sourceRuns, compareRuns, observedRuns []string
	for rows.Next() {
		var runID, relation string
		if err := rows.Scan(&runID, &relation); err != nil {
			return err
		}
		switch relation {
		case "SOURCE":
			sourceRuns = append(sourceRuns, runID)
		case "COMPARE":
			compareRuns = append(compareRuns, runID)
		case "OBSERVED":
			observedRuns = append(observedRuns, runID)
		}
	}
	card.SourceRuns = strings.Join(sourceRuns, ",")
	card.CompareRun = strings.Join(compareRuns, ",")
	card.ObservedRuns = strings.Join(observedRuns, ",")
	return rows.Err()
}

type ListFilter struct {
	All          bool
	Status       string
	Area         string
	Priority     string
	Owner        string
	ConstraintID string
	Unowned      bool
}

func (s *Store) listCards(filter ListFilter) ([]Card, error) {
	rows, err := s.db.Query(`SELECT ` + cardColumns + ` FROM cards ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var cards []Card
	for rows.Next() {
		var card Card
		if err := rows.Scan(
			&card.ID, &card.Version, &card.Status, &card.Title, &card.Priority, &card.Owner, &card.Area,
			&card.Updated, &card.UpdatedBy,
		); err != nil {
			return nil, err
		}
		card.Closed = isClosedStatus(card.Status)
		if !filter.All && card.Closed {
			continue
		}
		if filter.Status != "" && !strings.EqualFold(filter.Status, card.Status) {
			continue
		}
		if filter.Area != "" && !strings.EqualFold(filter.Area, card.Area) {
			continue
		}
		if filter.Priority != "" && !strings.EqualFold(filter.Priority, card.Priority) {
			continue
		}
		if filter.Unowned && card.Owner != "" {
			continue
		}
		if filter.Owner != "" && card.Owner != filter.Owner {
			continue
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := loadListDependencies(s.db, cards); err != nil {
		return nil, err
	}
	for i := range cards {
		if err := loadCardRuns(s.db, &cards[i]); err != nil {
			return nil, err
		}
	}
	if err := loadListConstraints(s.db, cards); err != nil {
		return nil, err
	}
	if filter.ConstraintID != "" {
		filtered := cards[:0]
		for _, card := range cards {
			for _, anchorID := range card.ConstraintIDs {
				if anchorID == filter.ConstraintID {
					filtered = append(filtered, card)
					break
				}
			}
		}
		cards = filtered
	}
	return cards, nil
}

func loadListConstraints(q queryer, cards []Card) error {
	if len(cards) == 0 {
		return nil
	}
	cardIndexes := make(map[string]int, len(cards))
	for i := range cards {
		cardIndexes[cards[i].ID] = i
	}
	rows, err := q.Query(`SELECT l.card_id, a.id, a.status, l.role FROM constraint_interventions l
        JOIN constraints a ON a.id = l.constraint_id ORDER BY l.card_id, a.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cardID, anchorID, status, role string
		if err := rows.Scan(&cardID, &anchorID, &status, &role); err != nil {
			return err
		}
		if index, ok := cardIndexes[cardID]; ok {
			if cards[index].ConstraintRoles == nil {
				cards[index].ConstraintRoles = map[string]string{}
			}
			cards[index].ConstraintIDs = append(cards[index].ConstraintIDs, anchorID)
			cards[index].ConstraintRoles[anchorID] = role
			if status == "ACTIVE" {
				cards[index].ActiveConstraintIDs = append(cards[index].ActiveConstraintIDs, anchorID)
			}
		}
	}
	return rows.Err()
}

func loadListDependencies(q queryer, cards []Card) error {
	if len(cards) == 0 {
		return nil
	}
	cardIndexes := make(map[string]int, len(cards))
	for i := range cards {
		cardIndexes[cards[i].ID] = i
	}
	rows, err := q.Query(`SELECT d.card_id, d.depends_on_card_id, d.required_status, d.mode, d.reason,
		target.status, dependent.status
        FROM card_dependencies d
        JOIN cards target ON target.id = d.depends_on_card_id
        JOIN cards dependent ON dependent.id = d.card_id
        ORDER BY d.card_id, d.depends_on_card_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var dependency CardDependency
		var dependentStatus string
		if err := rows.Scan(&dependency.CardID, &dependency.DependsOnCardID, &dependency.RequiredStatus,
			&dependency.Mode, &dependency.Reason, &dependency.TargetStatus, &dependentStatus); err != nil {
			return err
		}
		if index, ok := cardIndexes[dependency.CardID]; ok {
			cards[index].Dependencies = append(cards[index].Dependencies, dependency)
		}
		if index, ok := cardIndexes[dependency.DependsOnCardID]; ok {
			reverse := dependency
			reverse.TargetStatus = dependentStatus
			cards[index].Unblocks = append(cards[index].Unblocks, reverse)
		}
	}
	return rows.Err()
}

type mutation struct {
	ExpectedCardVersion *int
	Actor               string
	Operation           string
	CardID              string
	Summary             string
}

type cardVersionConflictError struct {
	CardID   string
	Expected int
	Current  int
}

func (e *cardVersionConflictError) Error() string {
	return fmt.Sprintf("card version conflict: %s expected %d, current %d", e.CardID, e.Expected, e.Current)
}

func (s *Store) beginMutation(options mutation) (*sql.Tx, int, error) {
	if strings.TrimSpace(options.Actor) == "" {
		return nil, 0, errors.New("actor is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, err
	}
	var value string
	if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'backlog_revision'`).Scan(&value); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	current, err := strconv.Atoi(value)
	if err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("invalid backlog revision %q", value)
	}
	return tx, current, nil
}

func claimCardVersionTx(tx *sql.Tx, cardID string, expected *int) error {
	query := `UPDATE cards SET card_version = card_version + 1 WHERE id = ?`
	args := []any{cardID}
	if expected != nil {
		query += ` AND card_version = ?`
		args = append(args, *expected)
	}
	result, err := tx.Exec(query, args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	var current int
	if err := tx.QueryRow(`SELECT card_version FROM cards WHERE id = ?`, cardID).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("card %s not found", cardID)
		}
		return err
	}
	if expected != nil {
		return &cardVersionConflictError{CardID: cardID, Expected: *expected, Current: current}
	}
	return fmt.Errorf("card %s was not updated", cardID)
}

func finishMutation(tx *sql.Tx, current int, options mutation) error {
	if err := validateActiveConstraintBindingsTx(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := validateObjectiveRelations(tx); err != nil {
		tx.Rollback()
		return err
	}
	next := current + 1
	if _, err := tx.Exec(`UPDATE metadata SET value = ? WHERE key = 'backlog_revision'`, strconv.Itoa(next)); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT INTO change_log(revision, occurred_at, actor, operation, card_id, summary) VALUES (?, ?, ?, ?, ?, ?)`,
		next, now(), options.Actor, options.Operation, options.CardID, options.Summary); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func validateActiveConstraintBindingsTx(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT l.constraint_id, l.card_id, l.role FROM constraint_interventions l
		JOIN constraints a ON a.id = l.constraint_id WHERE a.status = 'ACTIVE' ORDER BY l.constraint_id, l.card_id`)
	if err != nil {
		return err
	}
	type link struct{ constraintID, cardID, role string }
	var links []link
	for rows.Next() {
		var item link
		if err := rows.Scan(&item.constraintID, &item.cardID, &item.role); err != nil {
			rows.Close()
			return err
		}
		links = append(links, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, link := range links {
		if link.role == "MITIGATES" {
			continue
		}
		constraint, err := getConstraintFrom(tx, link.constraintID)
		if err != nil {
			return err
		}
		card, err := getCardFrom(tx, link.cardID)
		if err != nil {
			return err
		}
		var assessmentCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM constraint_intervention_assessments WHERE constraint_id = ? AND card_id = ?`, constraint.ID, card.ID).Scan(&assessmentCount); err != nil {
			return err
		}
		if assessmentCount == 0 {
			continue
		}
		assessment, err := getPerformanceResidualAssessmentFrom(tx, constraint, card)
		if err != nil {
			return err
		}
		if !assessment.resolves() {
			return fmt.Errorf("active constraint RESOLVES relation %s -> %s does not satisfy the resolution threshold", constraint.ID, card.ID)
		}
	}
	return nil
}

func now() string { return time.Now().Format(time.RFC3339) }

func ensureReason(actor, reason string) error {
	if strings.TrimSpace(actor) == "" {
		return errors.New("--actor is required")
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("--reason is required")
	}
	return nil
}

func ensureReadyActor(actor, status string) error {
	actor = strings.TrimSpace(actor)
	isSkillRun := strings.HasPrefix(actor, "skill:") || strings.HasPrefix(actor, "agent:isucon-")
	if status == "READY" && isSkillRun && actor != "skill:isucon-investigate" {
		return errors.New("skill actors may create or promote READY cards only as skill:isucon-investigate")
	}
	return nil
}

func requiresReadyContract(status string) bool {
	switch normalizeStatus(status) {
	case "READY", "DOING", "VERIFY", "APPLIED", "VALIDATED":
		return true
	default:
		return false
	}
}

func validateReadyContract(q queryer, cardID string) error {
	card, err := getCardFrom(q, cardID)
	if err != nil {
		return err
	}
	for _, name := range []string{sectionHypothesis, sectionChangeBoundary, sectionVerification} {
		if strings.TrimSpace(sectionBody(card.Sections, name)) == "" {
			return fmt.Errorf("card %s cannot be READY without a non-empty %s section", card.ID, name)
		}
	}
	var activeObjectives int
	if err := q.QueryRow(`SELECT COUNT(*) FROM objective_interventions oi
		JOIN objectives o ON o.id = oi.objective_id
		WHERE oi.card_id = ? AND o.status = 'ACTIVE'`, card.ID).Scan(&activeObjectives); err != nil {
		return err
	}
	if activeObjectives == 0 {
		return fmt.Errorf("card %s cannot be READY without an ACTIVE Objective relation", card.ID)
	}
	rows, err := q.Query(`SELECT d.depends_on_card_id, d.required_status, target.status
		FROM card_dependencies d JOIN cards target ON target.id = d.depends_on_card_id
		WHERE d.card_id = ? AND d.mode = 'BLOCKING' ORDER BY d.depends_on_card_id`, card.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var targetID, required, status string
		if err := rows.Scan(&targetID, &required, &status); err != nil {
			return err
		}
		if status == "REJECTED" {
			return fmt.Errorf("dependency %s is REJECTED; return %s to INVESTIGATE", targetID, card.ID)
		}
		if !dependencyStatusSatisfied(status, required) {
			return fmt.Errorf("dependency %s requires %s and is %s", targetID, required, status)
		}
	}
	return rows.Err()
}

func validateReadyDependents(q queryer, changedCardID string) error {
	rows, err := q.Query(`SELECT DISTINCT dependent.id
		FROM card_dependencies d JOIN cards dependent ON dependent.id = d.card_id
		WHERE d.depends_on_card_id = ? AND d.mode = 'BLOCKING'
		AND dependent.status IN ('READY', 'DOING', 'VERIFY', 'APPLIED', 'VALIDATED')
		ORDER BY dependent.id`, changedCardID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := validateReadyContract(q, id); err != nil {
			return fmt.Errorf("card %s status change would invalidate dependent READY contract: %w", changedCardID, err)
		}
	}
	return nil
}

func addHistoryTx(tx *sql.Tx, cardID, occurredAt, actor, body string) error {
	var position int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM card_history WHERE card_id = ?`, cardID).Scan(&position); err != nil {
		return err
	}
	raw := fmt.Sprintf("- %s [%s] %s", occurredAt, actor, body)
	_, err := tx.Exec(`INSERT INTO card_history(card_id, position, occurred_at, actor, body, raw) VALUES (?, ?, ?, ?, ?, ?)`,
		cardID, position, occurredAt, actor, body, raw)
	return err
}

type CardPatch struct {
	Values   map[string]string
	Sections map[string]string
}

var patchColumns = map[string]string{
	"status":     "status",
	"title":      "title",
	"priority":   "priority",
	"owner":      "owner",
	"area":       "area",
	"updated":    "updated",
	"updated-by": "updated_by",
}

var runRelationFields = map[string]string{"source-runs": "SOURCE", "compare-run": "COMPARE", "observed-runs": "OBSERVED"}

func (s *Store) updateCard(id string, patch CardPatch, options mutation, reason string) error {
	_, err := s.mutateCard(id, patch, options, reason, false)
	return err
}

func (s *Store) resolveCardAndWake(id, status string, patch CardPatch, options mutation, reason string) ([]string, error) {
	status = normalizeStatus(status)
	if status != "READY" && status != "BLOCKED" && status != "REJECTED" {
		return nil, fmt.Errorf("resolve status must be READY, BLOCKED, or REJECTED; got %q", status)
	}
	if patch.Values == nil {
		patch.Values = map[string]string{}
	}
	if supplied, ok := patch.Values["status"]; ok && normalizeStatus(supplied) != status {
		return nil, errors.New("resolve status conflicts with patch status")
	}
	patch.Values["status"] = status
	patch.Values["owner"] = ""
	return s.mutateCard(id, patch, options, reason, true)
}

func (s *Store) mutateCard(id string, patch CardPatch, options mutation, reason string, resolve bool) ([]string, error) {
	if err := ensureReason(options.Actor, reason); err != nil {
		return nil, err
	}
	if value, ok := patch.Values["status"]; ok {
		value = normalizeStatus(value)
		if !allowedStatuses[value] {
			return nil, fmt.Errorf("invalid status %q", value)
		}
		patch.Values["status"] = value
	}
	if len(patch.Values) == 0 && len(patch.Sections) == 0 {
		return nil, errors.New("no changes supplied")
	}
	options.CardID = id
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return nil, err
	}
	if err := claimCardVersionTx(tx, id, options.ExpectedCardVersion); err != nil {
		tx.Rollback()
		return nil, err
	}
	currentCard, err := getCardFrom(tx, id)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	targetStatus := currentCard.Status
	if status, ok := patch.Values["status"]; ok {
		targetStatus = status
	}
	if targetStatus != currentCard.Status {
		if resolve {
			if currentCard.Status != "INVESTIGATE" || (targetStatus != "READY" && targetStatus != "BLOCKED" && targetStatus != "REJECTED") {
				tx.Rollback()
				return nil, fmt.Errorf("resolve requires INVESTIGATE -> READY, BLOCKED, or REJECTED; card %s is %s", id, currentCard.Status)
			}
		} else if currentCard.Status != "READY" || targetStatus != "DOING" || strings.TrimSpace(patch.Values["owner"]) == "" {
			tx.Rollback()
			return nil, errors.New("update may change status only for an atomic READY -> DOING claim with a non-empty owner; use transition or resolve for other status changes")
		}
		if err := validateCardStatusTransition(currentCard, targetStatus); err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, dependency := range currentCard.Dependencies {
			if targetStatus != "READY" && targetStatus != "DOING" {
				break
			}
			if targetStatus == "READY" && dependency.Mode != "BLOCKING" {
				continue
			}
			if dependency.TargetStatus == "REJECTED" {
				tx.Rollback()
				return nil, fmt.Errorf("dependency %s is REJECTED; return %s to INVESTIGATE", dependency.DependsOnCardID, currentCard.ID)
			}
			if !dependencyStatusSatisfied(dependency.TargetStatus, dependency.RequiredStatus) {
				tx.Rollback()
				return nil, fmt.Errorf("dependency %s requires %s and is %s", dependency.DependsOnCardID, dependency.RequiredStatus, dependency.TargetStatus)
			}
		}
	}
	if targetStatus == "READY" && currentCard.Status != "READY" {
		if err := ensureReadyActor(options.Actor, targetStatus); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if targetStatus == "READY" {
		if owner, ok := patch.Values["owner"]; ok && strings.TrimSpace(owner) != "" {
			tx.Rollback()
			return nil, errors.New("READY cards must have an empty owner")
		}
		if patch.Values == nil {
			patch.Values = map[string]string{}
		}
		patch.Values["owner"] = ""
	}
	setParts := make([]string, 0, len(patch.Values))
	args := make([]any, 0, len(patch.Values)+2)
	keys := make([]string, 0, len(patch.Values))
	for key := range patch.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := runRelationFields[key]; ok {
			continue
		}
		column, ok := patchColumns[key]
		if !ok {
			tx.Rollback()
			return nil, fmt.Errorf("unsupported card field %q", key)
		}
		setParts = append(setParts, column+" = ?")
		args = append(args, patch.Values[key])
	}
	for key, relation := range runRelationFields {
		if value, ok := patch.Values[key]; ok {
			if err := setCardRunsTx(tx, id, relation, value); err != nil {
				tx.Rollback()
				return nil, err
			}
		}
	}
	if len(setParts) > 0 {
		args = append(args, id)
		if _, err := tx.Exec(`UPDATE cards SET `+strings.Join(setParts, ", ")+` WHERE id = ?`, args...); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	for name, body := range patch.Sections {
		if err := validateSectionName(name); err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := upsertSectionTx(tx, id, name, body); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if requiresReadyContract(targetStatus) {
		if err := validateReadyContract(tx, id); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if targetStatus != currentCard.Status {
		if err := validateReadyDependents(tx, id); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	metadataParts := make([]string, 0, 2)
	metadataArgs := make([]any, 0, 3)
	if _, explicitlyUpdated := patch.Values["updated"]; !explicitlyUpdated {
		metadataParts = append(metadataParts, "updated = ?")
		metadataArgs = append(metadataArgs, now())
	}
	if _, explicitlyUpdatedBy := patch.Values["updated-by"]; !explicitlyUpdatedBy {
		metadataParts = append(metadataParts, "updated_by = ?")
		metadataArgs = append(metadataArgs, options.Actor)
	}
	if len(metadataParts) > 0 {
		metadataArgs = append(metadataArgs, id)
		if _, err := tx.Exec(`UPDATE cards SET `+strings.Join(metadataParts, ", ")+` WHERE id = ?`, metadataArgs...); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err := addHistoryTx(tx, id, now(), options.Actor, reason); err != nil {
		tx.Rollback()
		return nil, err
	}
	var woke []string
	if resolve && targetStatus != currentCard.Status {
		woke, err = wakeDependentsTx(tx, id, options.Actor)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	options.Summary = reason
	if resolve {
		options.Summary = fmt.Sprintf("%s: %s", targetStatus, reason)
		if len(woke) > 0 {
			options.Summary += "; woke " + strings.Join(woke, ",") + " to INVESTIGATE"
		}
	}
	if err := finishMutation(tx, current, options); err != nil {
		return nil, err
	}
	return woke, nil
}

func upsertSectionTx(tx *sql.Tx, cardID, name, body string) error {
	if strings.TrimSpace(body) == "" {
		_, err := tx.Exec(`DELETE FROM card_sections WHERE card_id = ? AND name = ?`, cardID, name)
		return err
	}
	var position int
	err := tx.QueryRow(`SELECT position FROM card_sections WHERE card_id = ? AND name = ?`, cardID, name).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM card_sections WHERE card_id = ?`, cardID).Scan(&position); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO card_sections(card_id, name, position, body) VALUES (?, ?, ?, ?)
        ON CONFLICT(card_id, name) DO UPDATE SET body = excluded.body`, cardID, name, position, body)
	return err
}

func setCardRunsTx(tx *sql.Tx, cardID, relation, raw string) error {
	runs, err := parseRunIDsStrict(raw)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM card_runs WHERE card_id = ? AND relation = ?`, cardID, relation); err != nil {
		return err
	}
	for position, runID := range runs {
		if _, err := tx.Exec(`INSERT INTO card_runs(card_id, run_id, relation, position) VALUES (?, ?, ?, ?)`, cardID, runID, relation, position); err != nil {
			return err
		}
	}
	return nil
}

type DependencyInput struct {
	CardID          string
	DependsOnCardID string
	RequiredStatus  string
	Mode            string
	Reason          string
}

func dependencyStatusSatisfied(current, required string) bool {
	switch required {
	case "VERIFY":
		return current == "VERIFY" || current == "APPLIED" || current == "VALIDATED"
	case "APPLIED":
		return current == "APPLIED" || current == "VALIDATED"
	case "VALIDATED":
		return current == "VALIDATED"
	default:
		return false
	}
}

func (s *Store) addDependency(input DependencyInput, options mutation) (bool, error) {
	if err := ensureReason(options.Actor, input.Reason); err != nil {
		return false, err
	}
	input.CardID = normalizeID(input.CardID)
	input.DependsOnCardID = normalizeID(input.DependsOnCardID)
	if input.CardID == input.DependsOnCardID {
		return false, errors.New("a card cannot depend on itself")
	}
	options.CardID = input.CardID
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return false, err
	}
	if err := claimCardVersionTx(tx, input.CardID, options.ExpectedCardVersion); err != nil {
		tx.Rollback()
		return false, err
	}
	card, err := getCardFrom(tx, input.CardID)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	if _, err := getCardFrom(tx, input.DependsOnCardID); err != nil {
		tx.Rollback()
		return false, err
	}
	input.RequiredStatus = normalizeStatus(input.RequiredStatus)
	input.Mode = strings.ToUpper(strings.TrimSpace(input.Mode))
	if input.RequiredStatus == "" || input.Mode == "" {
		tx.Rollback()
		return false, errors.New("dependencies require --required-status and --mode")
	}
	if !allowedDependencyStatuses[input.RequiredStatus] {
		tx.Rollback()
		return false, fmt.Errorf("invalid dependency required status %q", input.RequiredStatus)
	}
	if !allowedDependencyModes[input.Mode] {
		tx.Rollback()
		return false, fmt.Errorf("invalid dependency mode %q", input.Mode)
	}
	var cycle int
	err = tx.QueryRow(`WITH RECURSIVE reachable(id) AS (
        SELECT depends_on_card_id FROM card_dependencies WHERE card_id = ?
        UNION
        SELECT d.depends_on_card_id FROM card_dependencies d JOIN reachable r ON d.card_id = r.id
    ) SELECT COUNT(*) FROM reachable WHERE id = ?`, input.DependsOnCardID, input.CardID).Scan(&cycle)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	if cycle > 0 {
		tx.Rollback()
		return false, fmt.Errorf("dependency %s -> %s would create a cycle", input.CardID, input.DependsOnCardID)
	}
	if _, err := tx.Exec(`INSERT INTO card_dependencies(card_id, depends_on_card_id, required_status, mode, reason)
        VALUES (?, ?, ?, ?, ?)`, input.CardID, input.DependsOnCardID, input.RequiredStatus, input.Mode, input.Reason); err != nil {
		tx.Rollback()
		return false, err
	}
	if requiresReadyContract(card.Status) {
		if err := validateReadyContract(tx, card.ID); err != nil {
			tx.Rollback()
			return false, fmt.Errorf("dependency would invalidate READY contract: %w", err)
		}
	}
	updated := now()
	if _, err := tx.Exec(`UPDATE cards SET updated = ?, updated_by = ? WHERE id = ?`, updated, options.Actor, input.CardID); err != nil {
		tx.Rollback()
		return false, err
	}
	history := fmt.Sprintf("dependency added: %s requires %s at %s (%s): %s", input.CardID, input.DependsOnCardID, input.RequiredStatus, input.Mode, input.Reason)
	if err := addHistoryTx(tx, input.CardID, updated, options.Actor, history); err != nil {
		tx.Rollback()
		return false, err
	}
	woke := false
	if card.Status == "BLOCKED" && input.Mode == "BLOCKING" {
		shouldWake, wakeReason, err := shouldWakeDependentTx(tx, input.CardID)
		if err != nil {
			tx.Rollback()
			return false, err
		}
		if shouldWake {
			if err := wakeDependentTx(tx, input.CardID, options.Actor, wakeReason); err != nil {
				tx.Rollback()
				return false, err
			}
			woke = true
		}
	}
	options.Summary = history
	if woke {
		options.Summary += "; woke dependent to INVESTIGATE"
	}
	if err := finishMutation(tx, current, options); err != nil {
		return false, err
	}
	return woke, nil
}

func (s *Store) removeDependency(cardID, dependsOnCardID string, options mutation, reason string) (bool, error) {
	if err := ensureReason(options.Actor, reason); err != nil {
		return false, err
	}
	cardID, dependsOnCardID = normalizeID(cardID), normalizeID(dependsOnCardID)
	options.CardID = cardID
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return false, err
	}
	if err := claimCardVersionTx(tx, cardID, options.ExpectedCardVersion); err != nil {
		tx.Rollback()
		return false, err
	}
	var removedMode string
	if err := tx.QueryRow(`SELECT mode FROM card_dependencies WHERE card_id = ? AND depends_on_card_id = ?`, cardID, dependsOnCardID).Scan(&removedMode); err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("dependency %s -> %s not found", cardID, dependsOnCardID)
		}
		return false, err
	}
	result, err := tx.Exec(`DELETE FROM card_dependencies WHERE card_id = ? AND depends_on_card_id = ?`, cardID, dependsOnCardID)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		tx.Rollback()
		if err != nil {
			return false, err
		}
		return false, fmt.Errorf("dependency %s -> %s not found", cardID, dependsOnCardID)
	}
	updated := now()
	if _, err := tx.Exec(`UPDATE cards SET updated = ?, updated_by = ? WHERE id = ?`, updated, options.Actor, cardID); err != nil {
		tx.Rollback()
		return false, err
	}
	history := fmt.Sprintf("dependency removed: %s no longer depends on %s: %s", cardID, dependsOnCardID, reason)
	if err := addHistoryTx(tx, cardID, updated, options.Actor, history); err != nil {
		tx.Rollback()
		return false, err
	}
	woke := false
	if removedMode == "BLOCKING" {
		var status string
		if err := tx.QueryRow(`SELECT status FROM cards WHERE id = ?`, cardID).Scan(&status); err != nil {
			tx.Rollback()
			return false, err
		}
		if status == "BLOCKED" {
			wakeReason := fmt.Sprintf("blocking dependency %s was removed; re-evaluate the card", dependsOnCardID)
			if err := wakeDependentTx(tx, cardID, options.Actor, wakeReason); err != nil {
				tx.Rollback()
				return false, err
			}
			woke = true
		}
	}
	options.Summary = history
	if woke {
		options.Summary += "; woke dependent to INVESTIGATE"
	}
	if err := finishMutation(tx, current, options); err != nil {
		return false, err
	}
	return woke, nil
}

func shouldWakeDependentTx(tx *sql.Tx, cardID string) (bool, string, error) {
	rows, err := tx.Query(`SELECT d.required_status, target.id, target.status
        FROM card_dependencies d JOIN cards target ON target.id = d.depends_on_card_id
        WHERE d.card_id = ? AND d.mode = 'BLOCKING' ORDER BY target.id`, cardID)
	if err != nil {
		return false, "", err
	}
	defer rows.Close()
	count := 0
	allSatisfied := true
	for rows.Next() {
		var required, targetID, targetStatus string
		if err := rows.Scan(&required, &targetID, &targetStatus); err != nil {
			return false, "", err
		}
		count++
		if targetStatus == "REJECTED" {
			return true, fmt.Sprintf("blocking dependency %s was REJECTED; re-evaluate the dependency path", targetID), nil
		}
		if !dependencyStatusSatisfied(targetStatus, required) {
			allSatisfied = false
		}
	}
	if err := rows.Err(); err != nil {
		return false, "", err
	}
	if count > 0 && allSatisfied {
		return true, "all structured BLOCKING dependencies reached their required status", nil
	}
	return false, "", nil
}

func wakeDependentTx(tx *sql.Tx, cardID, actor, reason string) error {
	updated := now()
	result, err := tx.Exec(`UPDATE cards SET status = 'INVESTIGATE', owner = '', card_version = card_version + 1,
        updated = ?, updated_by = ? WHERE id = ? AND status = 'BLOCKED'`, updated, actor, cardID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return nil
	}
	return addHistoryTx(tx, cardID, updated, actor, "automatically woke BLOCKED -> INVESTIGATE: "+reason)
}

func wakeDependentsTx(tx *sql.Tx, changedCardID, actor string) ([]string, error) {
	rows, err := tx.Query(`SELECT DISTINCT dependent.id
        FROM card_dependencies d JOIN cards dependent ON dependent.id = d.card_id
        WHERE d.depends_on_card_id = ? AND d.mode = 'BLOCKING' AND dependent.status = 'BLOCKED'
        ORDER BY dependent.id`, changedCardID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	var woke []string
	for _, id := range ids {
		shouldWake, reason, err := shouldWakeDependentTx(tx, id)
		if err != nil {
			return nil, err
		}
		if !shouldWake {
			continue
		}
		if err := wakeDependentTx(tx, id, actor, reason); err != nil {
			return nil, err
		}
		woke = append(woke, id)
	}
	return woke, nil
}

func (s *Store) transitionCard(id, status string, options mutation, reason string) error {
	_, err := s.transitionCardAndWake(id, status, options, reason)
	return err
}

func (s *Store) transitionCardAndWake(id, status string, options mutation, reason string) ([]string, error) {
	status = normalizeStatus(status)
	if !allowedStatuses[status] {
		return nil, fmt.Errorf("invalid status %q", status)
	}
	if err := ensureReason(options.Actor, reason); err != nil {
		return nil, err
	}
	if err := ensureReadyActor(options.Actor, status); err != nil {
		return nil, err
	}
	options.CardID = id
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return nil, err
	}
	if err := claimCardVersionTx(tx, id, options.ExpectedCardVersion); err != nil {
		tx.Rollback()
		return nil, err
	}
	card, err := getCardFrom(tx, id)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := validateCardStatusTransition(card, status); err != nil {
		tx.Rollback()
		return nil, err
	}
	if requiresReadyContract(status) {
		if err := validateReadyContract(tx, id); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if card.Status == "READY" && status == "DOING" {
		tx.Rollback()
		return nil, errors.New("READY -> DOING must use update with a non-empty owner for an atomic claim")
	}
	updated := now()
	query := `UPDATE cards SET status = ?, updated = ?, updated_by = ? WHERE id = ?`
	args := []any{status, updated, options.Actor, id}
	if status == "READY" || status == "INVESTIGATE" || (card.Status == "INVESTIGATE" && status == "REJECTED") {
		query = `UPDATE cards SET status = ?, owner = ?, updated = ?, updated_by = ? WHERE id = ?`
		args = []any{status, "", updated, options.Actor, id}
	}
	if _, err := tx.Exec(query, args...); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := validateReadyDependents(tx, id); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := addHistoryTx(tx, id, updated, options.Actor, reason); err != nil {
		tx.Rollback()
		return nil, err
	}
	woke, err := wakeDependentsTx(tx, id, options.Actor)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	options.Summary = fmt.Sprintf("%s: %s", status, reason)
	if len(woke) > 0 {
		options.Summary += "; woke " + strings.Join(woke, ",") + " to INVESTIGATE"
	}
	if err := finishMutation(tx, current, options); err != nil {
		return nil, err
	}
	return woke, nil
}

func (s *Store) addHistory(id, actor, body, occurredAt string, options mutation) error {
	if err := ensureReason(actor, body); err != nil {
		return err
	}
	if occurredAt == "" {
		occurredAt = now()
	}
	options.CardID = id
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	if _, err := getCardFrom(tx, id); err != nil {
		tx.Rollback()
		return err
	}
	if err := addHistoryTx(tx, id, occurredAt, actor, body); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE cards SET updated = ?, updated_by = ? WHERE id = ?`, now(), actor, id); err != nil {
		tx.Rollback()
		return err
	}
	options.Actor, options.Summary = actor, body
	return finishMutation(tx, current, options)
}

type NewCard struct {
	ID string
	CardPatch
	Title  string
	Actor  string
	Reason string
}

func (s *Store) addCard(input NewCard, options mutation) (string, error) {
	if err := ensureReason(input.Actor, input.Reason); err != nil {
		return "", err
	}
	if input.Values == nil {
		input.Values = map[string]string{}
	}
	status := normalizeStatus(input.Values["status"])
	if status == "" {
		status = "INVESTIGATE"
	}
	if !allowedStatuses[status] {
		return "", fmt.Errorf("invalid status %q", status)
	}
	if status != "INVESTIGATE" {
		return "", fmt.Errorf("new cards must start as INVESTIGATE, got %s; use resolve after completing the READY contract", status)
	}
	if err := ensureReadyActor(input.Actor, status); err != nil {
		return "", err
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return "", err
	}
	var nextID string
	if input.ID != "" {
		nextID = normalizeID(input.ID)
	} else if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'next_id'`).Scan(&nextID); err != nil {
		tx.Rollback()
		return "", err
	}
	if nextID == "" {
		tx.Rollback()
		return "", errors.New("next_id is empty")
	}
	var existing string
	if err := tx.QueryRow(`SELECT id FROM cards WHERE id = ?`, nextID).Scan(&existing); err == nil {
		tx.Rollback()
		return "", fmt.Errorf("card %s already exists", nextID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		return "", err
	}
	values := map[string]string{}
	for key, value := range input.Values {
		values[key] = value
	}
	values["status"] = status
	title := strings.TrimSpace(input.Title)
	if title == "" {
		tx.Rollback()
		return "", errors.New("--title is required")
	}
	columns := []string{"id", "card_version", "status", "title"}
	args := []any{nextID, 0, status, title}
	keys := make([]string, 0, len(patchColumns))
	for key := range patchColumns {
		if key != "status" && key != "title" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := runRelationFields[key]; ok {
			continue
		}
		columns = append(columns, patchColumns[key])
		args = append(args, values[key])
	}
	columns = append(columns, "updated", "updated_by")
	args = append(args, now(), input.Actor)
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	query := "INSERT INTO cards(" + strings.Join(columns, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	if _, err := tx.Exec(query, args...); err != nil {
		tx.Rollback()
		return "", err
	}
	for key, relation := range runRelationFields {
		if value, ok := input.Values[key]; ok {
			if err := setCardRunsTx(tx, nextID, relation, value); err != nil {
				tx.Rollback()
				return "", err
			}
		}
	}
	for name, body := range input.Sections {
		if err := validateSectionName(name); err != nil {
			tx.Rollback()
			return "", err
		}
		if err := upsertSectionTx(tx, nextID, name, body); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	if err := addHistoryTx(tx, nextID, now(), input.Actor, input.Reason); err != nil {
		tx.Rollback()
		return "", err
	}
	insertedNumber, err := idNumber(nextID)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	var configuredNext string
	if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'next_id'`).Scan(&configuredNext); err != nil {
		tx.Rollback()
		return "", err
	}
	configuredNumber, err := idNumber(configuredNext)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	if input.ID == "" || insertedNumber >= configuredNumber {
		if _, err := tx.Exec(`UPDATE metadata SET value = ? WHERE key = 'next_id'`, formatID(insertedNumber+1)); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	options.CardID, options.Summary = nextID, input.Reason
	if err := finishMutation(tx, current, options); err != nil {
		return "", err
	}
	return nextID, nil
}

func idNumber(id string) (int, error) {
	id = normalizeID(id)
	if !strings.HasPrefix(id, "B-") {
		return 0, fmt.Errorf("invalid card ID %q", id)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "B-"))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid card ID %q", id)
	}
	return n, nil
}

func formatID(number int) string { return fmt.Sprintf("B-%03d", number) }

func (s *Store) validate() error {
	var integrity string
	if err := s.db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("backlog integrity_check: %s", integrity)
	}
	if _, err := s.revision(); err != nil {
		return err
	}
	if err := validateObjectiveRelations(s.db); err != nil {
		return err
	}
	next, err := s.metadata("next_id")
	if err != nil {
		return err
	}
	nextNumber, err := idNumber(next)
	if err != nil {
		return fmt.Errorf("invalid next_id: %w", err)
	}
	var maxCardNumber int
	cardIDRows, err := s.db.Query(`SELECT id FROM cards`)
	if err != nil {
		return err
	}
	for cardIDRows.Next() {
		var id string
		if err := cardIDRows.Scan(&id); err != nil {
			cardIDRows.Close()
			return err
		}
		number, err := idNumber(id)
		if err != nil {
			cardIDRows.Close()
			return fmt.Errorf("invalid card ID %q: %w", id, err)
		}
		if number > maxCardNumber {
			maxCardNumber = number
		}
	}
	if err := cardIDRows.Close(); err != nil {
		return err
	}
	if nextNumber <= maxCardNumber {
		return fmt.Errorf("next_id %s must be greater than maximum card ID %s", next, formatID(maxCardNumber))
	}
	rows, err := s.db.Query(`SELECT id, status, card_version, owner FROM cards`)
	if err != nil {
		return err
	}
	var readyContractIDs []string
	for rows.Next() {
		var id, status, owner string
		var cardVersion int
		if err := rows.Scan(&id, &status, &cardVersion, &owner); err != nil {
			return err
		}
		if !allowedStatuses[status] {
			return fmt.Errorf("card %s has invalid status %s", id, status)
		}
		if cardVersion < 0 {
			return fmt.Errorf("card %s has invalid card_version %d", id, cardVersion)
		}
		trimmedOwner := strings.TrimSpace(owner)
		if owner != trimmedOwner {
			return fmt.Errorf("card %s has invalid owner %q: owner must not contain surrounding whitespace", id, owner)
		}
		switch strings.ToLower(owner) {
		case "none", "null", "-":
			return fmt.Errorf("card %s has invalid owner %q: use an empty owner for an unowned card", id, owner)
		}
		if status == "READY" && strings.TrimSpace(owner) != "" {
			return fmt.Errorf("card %s is READY but has non-empty owner %q", id, owner)
		}
		if requiresReadyContract(status) {
			readyContractIDs = append(readyContractIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for _, id := range readyContractIDs {
		if err := validateReadyContract(s.db, id); err != nil {
			return err
		}
	}

	rows, err = s.db.Query(`SELECT card_id, run_id, relation FROM card_runs ORDER BY card_id, relation, position`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cardID, runID, relation string
		if err := rows.Scan(&cardID, &runID, &relation); err != nil {
			rows.Close()
			return err
		}
		if relation != "SOURCE" && relation != "COMPARE" && relation != "OBSERVED" {
			rows.Close()
			return fmt.Errorf("card %s has invalid RUN relation %q", cardID, relation)
		}
		parsed, err := parseRunIDsStrict(runID)
		if err != nil || len(parsed) != 1 || parsed[0] != runID {
			rows.Close()
			return fmt.Errorf("card %s has invalid %s RUN ID %q", cardID, relation, runID)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT d.card_id, d.depends_on_card_id, d.required_status, d.mode
		FROM card_dependencies d`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cardID, targetID, required, mode string
		if err := rows.Scan(&cardID, &targetID, &required, &mode); err != nil {
			rows.Close()
			return err
		}
		if !allowedDependencyStatuses[required] || !allowedDependencyModes[mode] {
			rows.Close()
			return fmt.Errorf("dependency %s -> %s has invalid contract %s/%s", cardID, targetID, required, mode)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	var cycles int
	if err := s.db.QueryRow(`WITH RECURSIVE path(start, current) AS (
        SELECT card_id, depends_on_card_id FROM card_dependencies
        UNION
        SELECT path.start, d.depends_on_card_id FROM path JOIN card_dependencies d ON d.card_id = path.current
    ) SELECT COUNT(*) FROM path WHERE start = current`).Scan(&cycles); err != nil {
		return err
	}
	if cycles > 0 {
		return fmt.Errorf("backlog contains %d dependency cycles", cycles)
	}
	constraintRows, err := s.db.Query(`SELECT a.id, a.constraint_version, a.status, a.title, a.fingerprint, a.scope,
		a.evidence, a.resolution, a.merged_into_constraint_id, COALESCE(survivor.status, '')
		FROM constraints a LEFT JOIN constraints survivor ON survivor.id = a.merged_into_constraint_id`)
	if err != nil {
		return err
	}
	maxConstraintNumber := 0
	for constraintRows.Next() {
		var id, status, title, fingerprint, scope, evidence, resolution, mergedIntoID, survivorStatus string
		var version int
		if err := constraintRows.Scan(&id, &version, &status, &title, &fingerprint, &scope, &evidence, &resolution, &mergedIntoID, &survivorStatus); err != nil {
			return err
		}
		number, err := constraintIDNumber(id)
		if err != nil {
			return err
		}
		if number > maxConstraintNumber {
			maxConstraintNumber = number
		}
		if version < 0 || !allowedConstraintStatuses[status] {
			return fmt.Errorf("constraint %s has invalid version/status %d/%s", id, version, status)
		}
		if strings.TrimSpace(title) == "" || strings.TrimSpace(fingerprint) == "" || strings.TrimSpace(scope) == "" || strings.TrimSpace(evidence) == "" || strings.TrimSpace(resolution) == "" {
			return fmt.Errorf("constraint %s is missing required identity, evidence, or resolution fields", id)
		}
		if status == "MERGED" && strings.TrimSpace(mergedIntoID) == "" {
			return fmt.Errorf("merged constraint %s is missing its surviving constraint", id)
		}
		if status != "MERGED" && strings.TrimSpace(mergedIntoID) != "" {
			return fmt.Errorf("non-merged constraint %s has a merged-into constraint", id)
		}
		if mergedIntoID == id {
			return fmt.Errorf("constraint %s cannot merge into itself", id)
		}
		if mergedIntoID != "" {
			if survivorStatus == "" {
				return fmt.Errorf("merged constraint %s has missing survivor %s", id, mergedIntoID)
			}
			if survivorStatus != "ACTIVE" {
				return fmt.Errorf("merged constraint %s points to non-live survivor %s (%s)", id, mergedIntoID, survivorStatus)
			}
		}
	}
	if err := constraintRows.Err(); err != nil {
		constraintRows.Close()
		return err
	}
	if err := constraintRows.Close(); err != nil {
		return err
	}
	nextConstraint, err := s.metadata("next_constraint_id")
	if err != nil {
		return err
	}
	nextConstraintNumber, err := constraintIDNumber(nextConstraint)
	if err != nil {
		return fmt.Errorf("invalid next_constraint_id: %w", err)
	}
	if nextConstraintNumber <= maxConstraintNumber {
		return fmt.Errorf("next_constraint_id %s must be greater than maximum constraint ID %s", nextConstraint, formatConstraintID(maxConstraintNumber))
	}
	assessmentRows, err := s.db.Query(`SELECT l.constraint_id, l.card_id, l.role FROM constraint_interventions l
		JOIN constraints a ON a.id = l.constraint_id WHERE a.status = 'ACTIVE' ORDER BY l.constraint_id, l.card_id`)
	if err != nil {
		return err
	}
	type activeConstraintLink struct{ constraintID, cardID, role string }
	var activeLinks []activeConstraintLink
	for assessmentRows.Next() {
		var link activeConstraintLink
		if err := assessmentRows.Scan(&link.constraintID, &link.cardID, &link.role); err != nil {
			assessmentRows.Close()
			return err
		}
		activeLinks = append(activeLinks, link)
	}
	if err := assessmentRows.Err(); err != nil {
		assessmentRows.Close()
		return err
	}
	if err := assessmentRows.Close(); err != nil {
		return err
	}
	for _, link := range activeLinks {
		if link.role == "MITIGATES" {
			continue
		}
		constraint, err := getConstraintFrom(s.db, link.constraintID)
		if err != nil {
			return err
		}
		card, err := getCardFrom(s.db, link.cardID)
		if err != nil {
			return err
		}
		var assessmentCount int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM constraint_intervention_assessments WHERE constraint_id = ? AND card_id = ?`, constraint.ID, card.ID).Scan(&assessmentCount); err != nil {
			return err
		}
		if assessmentCount == 0 {
			continue
		}
		assessment, err := getPerformanceResidualAssessmentFrom(s.db, constraint, card)
		if err != nil {
			return err
		}
		if !assessment.resolves() {
			return fmt.Errorf("active constraint RESOLVES relation %s -> %s does not satisfy the resolution threshold", link.constraintID, link.cardID)
		}
	}
	return nil
}
