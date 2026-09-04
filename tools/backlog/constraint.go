package main

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type ConstraintFilter struct {
	All      bool
	Status   string
	Priority string
}

type NewConstraint struct {
	ID           string
	ObjectiveID  string
	Status       string
	Title        string
	Priority     string
	Fingerprint  string
	Scope        string
	SourceRuns   string
	ObservedRuns string
	Evidence     string
	Resolution   string
}

type ConstraintPatch struct {
	Title        *string
	Priority     *string
	Fingerprint  *string
	Scope        *string
	SourceRuns   *string
	ObservedRuns *string
	Evidence     *string
	Resolution   *string
}

type constraintVersionConflictError struct {
	ConstraintID string
	Expected     int
	Current      int
}

func (e *constraintVersionConflictError) Error() string {
	return fmt.Sprintf("constraint version conflict: %s expected %d, current %d", e.ConstraintID, e.Expected, e.Current)
}

func constraintIDNumber(id string) (int, error) {
	id = normalizeConstraintID(id)
	if !strings.HasPrefix(id, "A-") {
		return 0, fmt.Errorf("invalid constraint ID %q", id)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "A-"))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid constraint ID %q", id)
	}
	return n, nil
}

func formatConstraintID(number int) string { return fmt.Sprintf("A-%03d", number) }

func claimConstraintVersionTx(tx *sql.Tx, constraintID string, expected *int) error {
	query := `UPDATE constraints SET constraint_version = constraint_version + 1 WHERE id = ?`
	args := []any{constraintID}
	if expected != nil {
		query += ` AND constraint_version = ?`
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
	if err := tx.QueryRow(`SELECT constraint_version FROM constraints WHERE id = ?`, constraintID).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("constraint %s not found", constraintID)
		}
		return err
	}
	if expected != nil {
		return &constraintVersionConflictError{ConstraintID: constraintID, Expected: *expected, Current: current}
	}
	return fmt.Errorf("constraint %s was not updated", constraintID)
}

func addConstraintHistoryTx(tx *sql.Tx, constraintID, actor, body string) error {
	var position int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM constraint_history WHERE constraint_id = ?`, constraintID).Scan(&position); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO constraint_history(constraint_id, position, occurred_at, actor, body) VALUES (?, ?, ?, ?, ?)`,
		constraintID, position, now(), actor, body)
	return err
}

func (s *Store) addConstraint(input NewConstraint, options mutation, reason string) (string, error) {
	if err := ensureReason(options.Actor, reason); err != nil {
		return "", err
	}
	input.Status = normalizeConstraintStatus(input.Status)
	if input.Status == "" {
		input.Status = "ACTIVE"
	}
	if input.Status != "ACTIVE" {
		return "", errors.New("new constraints must start as ACTIVE")
	}
	input.ObjectiveID = normalizeObjectiveID(input.ObjectiveID)
	if input.ObjectiveID == "" {
		return "", errors.New("constraint requires an ACTIVE objective")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Fingerprint = strings.TrimSpace(input.Fingerprint)
	if input.Title == "" || input.Fingerprint == "" || strings.TrimSpace(input.Scope) == "" || strings.TrimSpace(input.Evidence) == "" || strings.TrimSpace(input.Resolution) == "" {
		return "", errors.New("constraint requires title, fingerprint, scope, evidence, and resolution")
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return "", err
	}
	id := normalizeConstraintID(input.ID)
	if id == "" {
		if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'next_constraint_id'`).Scan(&id); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	if _, err := constraintIDNumber(id); err != nil {
		tx.Rollback()
		return "", err
	}
	objective, err := getObjectiveFrom(tx, input.ObjectiveID)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	if objective.Status != "ACTIVE" {
		tx.Rollback()
		return "", fmt.Errorf("constraint requires an ACTIVE objective; %s is %s", objective.ID, objective.Status)
	}
	_, err = tx.Exec(`INSERT INTO constraints(
        id, constraint_version, status, title, priority, fingerprint, scope,
	        source_runs, observed_runs, evidence, resolution, merged_into_constraint_id, updated, updated_by
	    ) VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		id, input.Status, input.Title, input.Priority, input.Fingerprint, input.Scope,
		input.SourceRuns, input.ObservedRuns, input.Evidence, input.Resolution, now(), options.Actor)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	if err := addConstraintHistoryTx(tx, id, options.Actor, reason); err != nil {
		tx.Rollback()
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO objective_constraints(objective_id, constraint_id) VALUES (?, ?)`, objective.ID, id); err != nil {
		tx.Rollback()
		return "", err
	}
	insertedNumber, _ := constraintIDNumber(id)
	var configuredNext string
	if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'next_constraint_id'`).Scan(&configuredNext); err != nil {
		tx.Rollback()
		return "", err
	}
	configuredNumber, err := constraintIDNumber(configuredNext)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	if input.ID == "" || insertedNumber >= configuredNumber {
		if _, err := tx.Exec(`UPDATE metadata SET value = ? WHERE key = 'next_constraint_id'`, formatConstraintID(insertedNumber+1)); err != nil {
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

func getConstraintFrom(q queryer, id string) (Constraint, error) {
	id = normalizeConstraintID(id)
	var constraint Constraint
	err := q.QueryRow(`SELECT id, constraint_version, status, title, priority, fingerprint, scope,
	        source_runs, observed_runs, evidence, resolution, merged_into_constraint_id, updated, updated_by
	        FROM constraints WHERE id = ?`, id).Scan(
		&constraint.ID, &constraint.Version, &constraint.Status, &constraint.Title, &constraint.Priority, &constraint.Fingerprint,
		&constraint.Scope, &constraint.SourceRuns, &constraint.ObservedRuns,
		&constraint.Evidence, &constraint.Resolution, &constraint.MergedIntoID, &constraint.Updated, &constraint.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Constraint{}, fmt.Errorf("constraint %s not found", id)
	}
	if err != nil {
		return Constraint{}, err
	}
	rows, err := q.Query(`SELECT card_id, role FROM constraint_interventions WHERE constraint_id = ? ORDER BY card_id`, id)
	if err != nil {
		return Constraint{}, err
	}
	constraint.CardRoles = map[string]string{}
	for rows.Next() {
		var cardID, role string
		if err := rows.Scan(&cardID, &role); err != nil {
			rows.Close()
			return Constraint{}, err
		}
		constraint.CardIDs = append(constraint.CardIDs, cardID)
		constraint.CardRoles[cardID] = role
	}
	if err := rows.Close(); err != nil {
		return Constraint{}, err
	}
	rows, err = q.Query(`SELECT objective_id FROM objective_constraints WHERE constraint_id = ? ORDER BY objective_id`, id)
	if err != nil {
		return Constraint{}, err
	}
	for rows.Next() {
		var objectiveID string
		if err := rows.Scan(&objectiveID); err != nil {
			rows.Close()
			return Constraint{}, err
		}
		constraint.ObjectiveIDs = append(constraint.ObjectiveIDs, objectiveID)
	}
	if err := rows.Close(); err != nil {
		return Constraint{}, err
	}
	rows, err = q.Query(`SELECT position, occurred_at, actor, body FROM constraint_history WHERE constraint_id = ? ORDER BY position`, id)
	if err != nil {
		return Constraint{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry HistoryEntry
		if err := rows.Scan(&entry.Position, &entry.OccurredAt, &entry.Actor, &entry.Body); err != nil {
			return Constraint{}, err
		}
		constraint.History = append(constraint.History, entry)
	}
	return constraint, rows.Err()
}

func (s *Store) getConstraint(id string) (Constraint, error) { return getConstraintFrom(s.db, id) }

func (s *Store) listConstraints(filter ConstraintFilter) ([]Constraint, error) {
	rows, err := s.db.Query(`SELECT id FROM constraints ORDER BY id`)
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
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var constraints []Constraint
	for _, id := range ids {
		constraint, err := getConstraintFrom(s.db, id)
		if err != nil {
			return nil, err
		}
		if !filter.All && constraint.Status != "ACTIVE" {
			continue
		}
		if filter.Status != "" && normalizeConstraintStatus(filter.Status) != constraint.Status {
			continue
		}
		if filter.Priority != "" && !strings.EqualFold(filter.Priority, constraint.Priority) {
			continue
		}
		constraints = append(constraints, constraint)
	}
	return constraints, nil
}

func (s *Store) updateConstraint(id string, patch ConstraintPatch, expected int, options mutation, reason string) error {
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	id = normalizeConstraintID(id)
	parts := []string{}
	args := []any{}
	fields := []struct {
		column string
		value  *string
	}{{"title", patch.Title}, {"priority", patch.Priority}, {"fingerprint", patch.Fingerprint}, {"scope", patch.Scope}, {"source_runs", patch.SourceRuns}, {"observed_runs", patch.ObservedRuns}, {"evidence", patch.Evidence}, {"resolution", patch.Resolution}}
	required := map[string]bool{"title": true, "fingerprint": true, "scope": true, "evidence": true, "resolution": true}
	for _, field := range fields {
		if field.value != nil {
			if required[field.column] && strings.TrimSpace(*field.value) == "" {
				return fmt.Errorf("constraint field %s cannot be empty", field.column)
			}
			parts = append(parts, field.column+" = ?")
			args = append(args, *field.value)
		}
	}
	if len(parts) == 0 {
		return errors.New("no constraint changes supplied")
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	constraint, err := getConstraintFrom(tx, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	if constraint.Status != "ACTIVE" {
		tx.Rollback()
		return fmt.Errorf("terminal constraint %s (%s) is immutable", id, constraint.Status)
	}
	if err := claimConstraintVersionTx(tx, id, &expected); err != nil {
		tx.Rollback()
		return err
	}
	parts = append(parts, "updated = ?", "updated_by = ?")
	args = append(args, now(), options.Actor, id)
	if _, err := tx.Exec(`UPDATE constraints SET `+strings.Join(parts, ", ")+` WHERE id = ?`, args...); err != nil {
		tx.Rollback()
		return err
	}
	if err := addConstraintHistoryTx(tx, id, options.Actor, reason); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = id, reason
	return finishMutation(tx, current, options)
}

func (s *Store) transitionConstraint(id, status, mergedIntoID string, expected int, options mutation, reason string) error {
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	id, status = normalizeConstraintID(id), normalizeConstraintStatus(status)
	if !allowedConstraintStatuses[status] {
		return fmt.Errorf("invalid constraint status %q", status)
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	constraint, err := getConstraintFrom(tx, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	allowed := map[string]map[string]bool{
		"ACTIVE": {"RESOLVED": true, "INVALIDATED": true, "MERGED": true},
	}
	if constraint.Status == status || !allowed[constraint.Status][status] {
		tx.Rollback()
		return fmt.Errorf("invalid constraint status transition %s -> %s", constraint.Status, status)
	}
	mergedIntoID = normalizeConstraintID(mergedIntoID)
	if status == "MERGED" {
		if mergedIntoID == "" {
			tx.Rollback()
			return errors.New("MERGED transition requires --merged-into")
		}
		if mergedIntoID == id {
			tx.Rollback()
			return errors.New("a constraint cannot merge into itself")
		}
		survivor, err := getConstraintFrom(tx, mergedIntoID)
		if err != nil {
			tx.Rollback()
			return err
		}
		if survivor.Status != "ACTIVE" {
			tx.Rollback()
			return fmt.Errorf("merge survivor %s must be ACTIVE, got %s", mergedIntoID, survivor.Status)
		}
	} else if mergedIntoID != "" {
		tx.Rollback()
		return errors.New("--merged-into is valid only for a MERGED transition")
	}
	if err := claimConstraintVersionTx(tx, id, &expected); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE constraints SET status = ?, merged_into_constraint_id = ?, updated = ?, updated_by = ? WHERE id = ?`, status, mergedIntoID, now(), options.Actor, id); err != nil {
		tx.Rollback()
		return err
	}
	if err := addConstraintHistoryTx(tx, id, options.Actor, reason); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = id, status+": "+reason
	return finishMutation(tx, current, options)
}

func (s *Store) setConstraintLink(constraintID, cardID string, link bool, role, assessmentJSON string, expected int, options mutation, reason string) error {
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	constraintID, cardID = normalizeConstraintID(constraintID), normalizeID(cardID)
	role = strings.ToUpper(strings.TrimSpace(role))
	if link && role != "" && role != "RESOLVES" && role != "MITIGATES" {
		return fmt.Errorf("invalid constraint relation role %q", role)
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	if err := claimConstraintVersionTx(tx, constraintID, &expected); err != nil {
		tx.Rollback()
		return err
	}
	constraint, err := getConstraintFrom(tx, constraintID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if link && constraint.Status != "ACTIVE" {
		tx.Rollback()
		return fmt.Errorf("new candidate links require an ACTIVE constraint; %s is %s", constraintID, constraint.Status)
	}
	if !link && constraint.Status != "ACTIVE" {
		tx.Rollback()
		return fmt.Errorf("terminal constraint %s (%s) is immutable", constraintID, constraint.Status)
	}
	card, err := getCardFrom(tx, cardID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if link {
		var parsed *PerformanceResidualAssessment
		if strings.TrimSpace(assessmentJSON) != "" {
			contract, canonical, contractErr := parsePerformanceResidualAssessment(assessmentJSON)
			if contractErr != nil {
				tx.Rollback()
				return contractErr
			}
			parsed = &contract
			if _, contractErr = tx.Exec(`INSERT INTO constraint_intervention_assessments(card_id, constraint_id, assessment_json, constraint_definition_hash, card_change_boundary_hash, updated, updated_by)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(card_id, constraint_id) DO UPDATE SET assessment_json=excluded.assessment_json, constraint_definition_hash=excluded.constraint_definition_hash,
				card_change_boundary_hash=excluded.card_change_boundary_hash, updated=excluded.updated, updated_by=excluded.updated_by`,
				cardID, constraintID, canonical, constraintDefinitionHash(constraint), cardAssessmentChangeBoundaryHash(card), now(), options.Actor); contractErr != nil {
				tx.Rollback()
				return contractErr
			}
		}
		if role == "" {
			if parsed == nil {
				tx.Rollback()
				return errors.New("--role is required when no performance residual assessment is supplied")
			}
			if parsed.resolves() {
				role = "RESOLVES"
			} else {
				role = "MITIGATES"
			}
		}
		if role == "RESOLVES" && parsed != nil && !parsed.resolves() {
			tx.Rollback()
			return errors.New("a performance RESOLVES assessment must satisfy the constraint resolution threshold")
		}
		if role == "MITIGATES" && parsed != nil && parsed.resolves() {
			tx.Rollback()
			return errors.New("a MITIGATES assessment must leave the constraint unresolved")
		}
		_, err = tx.Exec(`INSERT INTO constraint_interventions(card_id, constraint_id, role, rationale) VALUES (?, ?, ?, ?)`, cardID, constraintID, role, reason)
	} else {
		var result sql.Result
		result, err = tx.Exec(`DELETE FROM constraint_interventions WHERE card_id = ? AND constraint_id = ?`, cardID, constraintID)
		if err == nil {
			if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
				if rowsErr != nil {
					err = rowsErr
				} else {
					err = fmt.Errorf("constraint link %s -> %s not found", constraintID, cardID)
				}
			}
		}
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	action := "linked"
	if !link {
		action = "unlinked"
	}
	if err := addConstraintHistoryTx(tx, constraintID, options.Actor, fmt.Sprintf("%s %s: %s", action, cardID, reason)); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE constraints SET updated = ?, updated_by = ? WHERE id = ?`, now(), options.Actor, constraintID); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = constraintID, fmt.Sprintf("%s %s: %s", action, cardID, reason)
	return finishMutation(tx, current, options)
}

func activeConstraintIDs(constraintIDs []string, constraints map[string]Constraint) []string {
	var result []string
	for _, id := range constraintIDs {
		if constraint, ok := constraints[id]; ok && constraint.Status == "ACTIVE" {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}
