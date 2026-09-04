package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var allowedObjectiveStatuses = map[string]bool{"ACTIVE": true, "RETIRED": true}
var allowedObjectiveModes = map[string]bool{"SATISFY": true, "MAXIMIZE": true, "MINIMIZE": true}

type ObjectiveFilter struct {
	All    bool
	Status string
}

type NewObjective struct {
	ID                     string
	Status                 string
	Mode                   string
	Title                  string
	MetricOrPredicate      string
	RequiredForValidResult bool
	ParentObjectiveID      string
	OfficialSources        string
	Verification           string
}

type ObjectivePatch struct {
	Status                 *string
	Mode                   *string
	Title                  *string
	MetricOrPredicate      *string
	RequiredForValidResult *bool
	ParentObjectiveID      *string
	OfficialSources        *string
	Verification           *string
}

func normalizeObjectiveID(id string) string { return strings.ToUpper(strings.TrimSpace(id)) }

func objectiveIDNumber(id string) (int, error) {
	id = normalizeObjectiveID(id)
	if !strings.HasPrefix(id, "O-") {
		return 0, fmt.Errorf("invalid objective ID %q", id)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "O-"))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid objective ID %q", id)
	}
	return n, nil
}

func formatObjectiveID(number int) string { return fmt.Sprintf("O-%03d", number) }

func validateObjective(input NewObjective) error {
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	input.Mode = strings.ToUpper(strings.TrimSpace(input.Mode))
	if !allowedObjectiveStatuses[input.Status] {
		return fmt.Errorf("invalid objective status %q", input.Status)
	}
	if !allowedObjectiveModes[input.Mode] {
		return fmt.Errorf("invalid objective mode %q", input.Mode)
	}
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.MetricOrPredicate) == "" || strings.TrimSpace(input.Verification) == "" {
		return errors.New("objective requires title, metric-or-predicate, and verification")
	}
	if input.ParentObjectiveID != "" {
		if _, err := objectiveIDNumber(input.ParentObjectiveID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedInitialObjectives() error {
	seeds := []NewObjective{
		{ID: "O-001", Status: "ACTIVE", Mode: "SATISFY", Title: "ベンチマークと整合性チェックを通過する", MetricOrPredicate: "benchmark result is valid and every required correctness check passes", RequiredForValidResult: true, OfficialSources: "docs/official/", Verification: "verify the final benchmark result and correctness log against the official rules"},
		{ID: "O-002", Status: "ACTIVE", Mode: "SATISFY", Title: "再起動後の永続性と再現性条件を満たす", MetricOrPredicate: "the official restart and reproducibility requirements are satisfied", RequiredForValidResult: true, OfficialSources: "docs/official/", Verification: "restart the required servers and rerun the official verification procedure"},
		{ID: "O-003", Status: "ACTIVE", Mode: "MAXIMIZE", Title: "有効なベンチマークスコアを最大化する", MetricOrPredicate: "final score of a benchmark run that satisfies all validity requirements", OfficialSources: "docs/official/", Verification: "use the finalized benchmark score and bench log"},
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	var seeded int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM metadata WHERE key = 'objective_model_seed_version'`).Scan(&seeded); err != nil {
		tx.Rollback()
		return err
	}
	if seeded != 0 {
		return tx.Commit()
	}
	for _, seed := range seeds {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM objectives WHERE id = ?`, seed.ID).Scan(&exists); err != nil {
			tx.Rollback()
			return err
		}
		if exists != 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO objectives(id, objective_version, status, mode, title, metric_or_predicate, required_for_valid_result, parent_objective_id, official_sources, verification, updated, updated_by) VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'system:migration')`, seed.ID, seed.Status, seed.Mode, seed.Title, seed.MetricOrPredicate, boolInt(seed.RequiredForValidResult), seed.ParentObjectiveID, seed.OfficialSources, seed.Verification, now()); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`INSERT INTO objective_history(objective_id, position, occurred_at, actor, body) VALUES (?, 0, ?, 'system:migration', 'initial objective registered from official specification')`, seed.ID, now()); err != nil {
			tx.Rollback()
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE metadata SET value = 'O-004' WHERE key = 'next_objective_id' AND CAST(SUBSTR(value, 3) AS INTEGER) < 4`); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO objective_constraints(objective_id, constraint_id) SELECT 'O-003', id FROM constraints`); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT INTO metadata(key, value) VALUES ('objective_model_seed_version', '1')`); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func addObjectiveHistoryTx(tx *sql.Tx, objectiveID, actor, body string) error {
	var position int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM objective_history WHERE objective_id = ?`, objectiveID).Scan(&position); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO objective_history(objective_id, position, occurred_at, actor, body) VALUES (?, ?, ?, ?, ?)`, objectiveID, position, now(), actor, body)
	return err
}

func (s *Store) addObjective(input NewObjective, options mutation, reason string) (string, error) {
	if err := ensureReason(options.Actor, reason); err != nil {
		return "", err
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "ACTIVE"
	}
	input.Mode = strings.ToUpper(strings.TrimSpace(input.Mode))
	input.ParentObjectiveID = normalizeObjectiveID(input.ParentObjectiveID)
	if err := validateObjective(input); err != nil {
		return "", err
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return "", err
	}
	id := normalizeObjectiveID(input.ID)
	if id == "" {
		if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'next_objective_id'`).Scan(&id); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	if _, err := objectiveIDNumber(id); err != nil {
		tx.Rollback()
		return "", err
	}
	if input.ParentObjectiveID != "" {
		if _, err := getObjectiveFrom(tx, input.ParentObjectiveID); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	_, err = tx.Exec(`INSERT INTO objectives(id, objective_version, status, mode, title, metric_or_predicate, required_for_valid_result, parent_objective_id, official_sources, verification, updated, updated_by) VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, input.Status, input.Mode, strings.TrimSpace(input.Title), strings.TrimSpace(input.MetricOrPredicate), boolInt(input.RequiredForValidResult), input.ParentObjectiveID, strings.TrimSpace(input.OfficialSources), strings.TrimSpace(input.Verification), now(), options.Actor)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	if err := addObjectiveHistoryTx(tx, id, options.Actor, reason); err != nil {
		tx.Rollback()
		return "", err
	}
	insertedNumber, _ := objectiveIDNumber(id)
	var configuredNext string
	if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'next_objective_id'`).Scan(&configuredNext); err != nil {
		tx.Rollback()
		return "", err
	}
	configuredNumber, err := objectiveIDNumber(configuredNext)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	if input.ID == "" || insertedNumber >= configuredNumber {
		if _, err := tx.Exec(`UPDATE metadata SET value = ? WHERE key = 'next_objective_id'`, formatObjectiveID(insertedNumber+1)); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	options.CardID, options.Summary = id, reason
	if err := finishMutation(tx, current, options); err != nil {
		return "", err
	}
	return id, nil
}

func getObjectiveFrom(q queryer, id string) (Objective, error) {
	id = normalizeObjectiveID(id)
	var objective Objective
	var required int
	err := q.QueryRow(`SELECT id, objective_version, status, mode, title, metric_or_predicate, required_for_valid_result, parent_objective_id, official_sources, verification, updated, updated_by FROM objectives WHERE id = ?`, id).Scan(&objective.ID, &objective.Version, &objective.Status, &objective.Mode, &objective.Title, &objective.MetricOrPredicate, &required, &objective.ParentObjectiveID, &objective.OfficialSources, &objective.Verification, &objective.Updated, &objective.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Objective{}, fmt.Errorf("objective %s not found", id)
	}
	if err != nil {
		return Objective{}, err
	}
	objective.RequiredForValidResult = required != 0
	rows, err := q.Query(`SELECT constraint_id FROM objective_constraints WHERE objective_id = ? ORDER BY constraint_id`, id)
	if err != nil {
		return Objective{}, err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return Objective{}, err
		}
		objective.ConstraintIDs = append(objective.ConstraintIDs, value)
	}
	rows.Close()
	rows, err = q.Query(`SELECT card_id FROM objective_interventions WHERE objective_id = ? ORDER BY card_id`, id)
	if err != nil {
		return Objective{}, err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return Objective{}, err
		}
		objective.InterventionIDs = append(objective.InterventionIDs, value)
	}
	rows.Close()
	rows, err = q.Query(`SELECT position, occurred_at, actor, body FROM objective_history WHERE objective_id = ? ORDER BY position`, id)
	if err != nil {
		return Objective{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry HistoryEntry
		if err := rows.Scan(&entry.Position, &entry.OccurredAt, &entry.Actor, &entry.Body); err != nil {
			return Objective{}, err
		}
		objective.History = append(objective.History, entry)
	}
	return objective, rows.Err()
}

func (s *Store) getObjective(id string) (Objective, error) { return getObjectiveFrom(s.db, id) }

func (s *Store) listObjectives(filter ObjectiveFilter) ([]Objective, error) {
	rows, err := s.db.Query(`SELECT id FROM objectives ORDER BY id`)
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
	rows.Close()
	var result []Objective
	for _, id := range ids {
		objective, err := getObjectiveFrom(s.db, id)
		if err != nil {
			return nil, err
		}
		if !filter.All && objective.Status != "ACTIVE" {
			continue
		}
		if filter.Status != "" && objective.Status != strings.ToUpper(strings.TrimSpace(filter.Status)) {
			continue
		}
		result = append(result, objective)
	}
	return result, nil
}

func claimObjectiveVersionTx(tx *sql.Tx, id string, expected int) error {
	result, err := tx.Exec(`UPDATE objectives SET objective_version = objective_version + 1 WHERE id = ? AND objective_version = ?`, id, expected)
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
	if err := tx.QueryRow(`SELECT objective_version FROM objectives WHERE id = ?`, id).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("objective %s not found", id)
		}
		return err
	}
	return fmt.Errorf("objective version conflict: %s expected %d, current %d", id, expected, current)
}

func (s *Store) updateObjective(id string, patch ObjectivePatch, expected int, options mutation, reason string) error {
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	id = normalizeObjectiveID(id)
	currentObjective, err := s.getObjective(id)
	if err != nil {
		return err
	}
	target := NewObjective{ID: id, Status: currentObjective.Status, Mode: currentObjective.Mode, Title: currentObjective.Title, MetricOrPredicate: currentObjective.MetricOrPredicate, RequiredForValidResult: currentObjective.RequiredForValidResult, ParentObjectiveID: currentObjective.ParentObjectiveID, OfficialSources: currentObjective.OfficialSources, Verification: currentObjective.Verification}
	if patch.Status != nil {
		target.Status = strings.ToUpper(strings.TrimSpace(*patch.Status))
	}
	if patch.Mode != nil {
		target.Mode = strings.ToUpper(strings.TrimSpace(*patch.Mode))
	}
	if patch.Title != nil {
		target.Title = *patch.Title
	}
	if patch.MetricOrPredicate != nil {
		target.MetricOrPredicate = *patch.MetricOrPredicate
	}
	if patch.RequiredForValidResult != nil {
		target.RequiredForValidResult = *patch.RequiredForValidResult
	}
	if patch.ParentObjectiveID != nil {
		target.ParentObjectiveID = normalizeObjectiveID(*patch.ParentObjectiveID)
	}
	if patch.OfficialSources != nil {
		target.OfficialSources = *patch.OfficialSources
	}
	if patch.Verification != nil {
		target.Verification = *patch.Verification
	}
	if target.ParentObjectiveID == id {
		return errors.New("objective cannot be its own parent")
	}
	if err := validateObjective(target); err != nil {
		return err
	}
	tx, revision, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	if target.ParentObjectiveID != "" {
		if _, err := getObjectiveFrom(tx, target.ParentObjectiveID); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := claimObjectiveVersionTx(tx, id, expected); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE objectives SET status=?, mode=?, title=?, metric_or_predicate=?, required_for_valid_result=?, parent_objective_id=?, official_sources=?, verification=?, updated=?, updated_by=? WHERE id=?`, target.Status, target.Mode, strings.TrimSpace(target.Title), strings.TrimSpace(target.MetricOrPredicate), boolInt(target.RequiredForValidResult), target.ParentObjectiveID, strings.TrimSpace(target.OfficialSources), strings.TrimSpace(target.Verification), now(), options.Actor, id); err != nil {
		tx.Rollback()
		return err
	}
	if err := addObjectiveHistoryTx(tx, id, options.Actor, reason); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = id, reason
	return finishMutation(tx, revision, options)
}

func (s *Store) setObjectiveRelation(objectiveID, targetID, targetType, rationale string, add bool, expected int, options mutation, reason string) error {
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	objectiveID = normalizeObjectiveID(objectiveID)
	targetType = strings.ToLower(strings.TrimSpace(targetType))
	tx, revision, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	if _, err := getObjectiveFrom(tx, objectiveID); err != nil {
		tx.Rollback()
		return err
	}
	if err := claimObjectiveVersionTx(tx, objectiveID, expected); err != nil {
		tx.Rollback()
		return err
	}
	switch targetType {
	case "constraint":
		targetID = normalizeConstraintID(targetID)
		if _, err := getConstraintFrom(tx, targetID); err != nil {
			tx.Rollback()
			return err
		}
		if add {
			_, err = tx.Exec(`INSERT OR IGNORE INTO objective_constraints(objective_id, constraint_id) VALUES (?, ?)`, objectiveID, targetID)
		} else {
			_, err = tx.Exec(`DELETE FROM objective_constraints WHERE objective_id = ? AND constraint_id = ?`, objectiveID, targetID)
		}
	case "intervention":
		targetID = normalizeID(targetID)
		if _, err := getCardFrom(tx, targetID); err != nil {
			tx.Rollback()
			return err
		}
		if add {
			_, err = tx.Exec(`INSERT INTO objective_interventions(objective_id, card_id, rationale) VALUES (?, ?, ?) ON CONFLICT(objective_id, card_id) DO UPDATE SET rationale=excluded.rationale`, objectiveID, targetID, strings.TrimSpace(rationale))
		} else {
			_, err = tx.Exec(`DELETE FROM objective_interventions WHERE objective_id = ? AND card_id = ?`, objectiveID, targetID)
		}
	default:
		tx.Rollback()
		return fmt.Errorf("invalid objective relation target %q", targetType)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	verb := "linked"
	if !add {
		verb = "unlinked"
	}
	if err := addObjectiveHistoryTx(tx, objectiveID, options.Actor, fmt.Sprintf("%s %s %s: %s", verb, targetType, targetID, reason)); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = objectiveID, reason
	return finishMutation(tx, revision, options)
}

func validateObjectiveRelations(q queryer) error {
	rows, err := q.Query(`SELECT id, objective_version, status, mode, title, metric_or_predicate, required_for_valid_result, parent_objective_id, official_sources, verification FROM objectives ORDER BY id`)
	if err != nil {
		return err
	}
	maxObjectiveNumber := 0
	for rows.Next() {
		var input NewObjective
		var version, required int
		if err := rows.Scan(&input.ID, &version, &input.Status, &input.Mode, &input.Title, &input.MetricOrPredicate, &required, &input.ParentObjectiveID, &input.OfficialSources, &input.Verification); err != nil {
			rows.Close()
			return err
		}
		input.RequiredForValidResult = required != 0
		number, err := objectiveIDNumber(input.ID)
		if err != nil {
			rows.Close()
			return err
		}
		if version < 0 {
			rows.Close()
			return fmt.Errorf("objective %s has invalid version %d", input.ID, version)
		}
		if number > maxObjectiveNumber {
			maxObjectiveNumber = number
		}
		if err := validateObjective(input); err != nil {
			rows.Close()
			return fmt.Errorf("objective %s: %w", input.ID, err)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var nextObjectiveID string
	if err := q.QueryRow(`SELECT value FROM metadata WHERE key = 'next_objective_id'`).Scan(&nextObjectiveID); err != nil {
		return err
	}
	nextObjectiveNumber, err := objectiveIDNumber(nextObjectiveID)
	if err != nil {
		return fmt.Errorf("invalid next_objective_id: %w", err)
	}
	if nextObjectiveNumber <= maxObjectiveNumber {
		return fmt.Errorf("next_objective_id %s must be greater than maximum objective ID %s", nextObjectiveID, formatObjectiveID(maxObjectiveNumber))
	}
	var missingParents int
	if err := q.QueryRow(`SELECT COUNT(*) FROM objectives child LEFT JOIN objectives parent ON parent.id = child.parent_objective_id WHERE child.parent_objective_id <> '' AND parent.id IS NULL`).Scan(&missingParents); err != nil {
		return err
	}
	if missingParents != 0 {
		return fmt.Errorf("%d objectives have missing parents", missingParents)
	}
	var cycles int
	if err := q.QueryRow(`WITH RECURSIVE chain(start, current) AS (
		SELECT id, parent_objective_id FROM objectives WHERE parent_objective_id <> ''
		UNION
		SELECT chain.start, o.parent_objective_id FROM chain JOIN objectives o ON o.id = chain.current WHERE o.parent_objective_id <> ''
	) SELECT COUNT(*) FROM chain WHERE start = current`).Scan(&cycles); err != nil {
		return err
	}
	if cycles != 0 {
		return fmt.Errorf("objective hierarchy contains %d cycles", cycles)
	}
	var unscoped int
	if err := q.QueryRow(`SELECT COUNT(*) FROM constraints c WHERE c.status = 'ACTIVE' AND NOT EXISTS (
		SELECT 1 FROM objective_constraints r JOIN objectives o ON o.id = r.objective_id
		WHERE r.constraint_id = c.id AND o.status = 'ACTIVE'
	)`).Scan(&unscoped); err != nil {
		return err
	}
	if unscoped != 0 {
		return fmt.Errorf("%d ACTIVE constraints are not connected to an ACTIVE objective", unscoped)
	}
	return nil
}
