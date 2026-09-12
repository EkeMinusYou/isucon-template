package main

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type TargetFilter struct {
	All      bool
	Status   string
	Priority string
}

type NewTarget struct {
	Rationale        string
	Axis             string
	Goal             string
	Evaluation       string
	PreviousTargetID string
	ID               string
	ObjectiveID      string
	Status           string
	Title            string
	Priority         string
	Fingerprint      string
	Scope            string
	SourceRuns       string
	ObservedRuns     string
	Evidence         string
	Resolution       string
}

type TargetPatch struct {
	Axis         *string
	Goal         *string
	Evaluation   *string
	Title        *string
	Priority     *string
	Fingerprint  *string
	Scope        *string
	SourceRuns   *string
	ObservedRuns *string
	Evidence     *string
	Resolution   *string
}

type targetVersionConflictError struct {
	TargetID string
	Expected int
	Current  int
}

func (e *targetVersionConflictError) Error() string {
	return fmt.Sprintf("target version conflict: %s expected %d, current %d", e.TargetID, e.Expected, e.Current)
}

func targetIDNumber(id string) (int, error) {
	id = normalizeTargetID(id)
	if !strings.HasPrefix(id, "A-") {
		return 0, fmt.Errorf("invalid target ID %q", id)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "A-"))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid target ID %q", id)
	}
	return n, nil
}

func formatTargetID(number int) string { return fmt.Sprintf("A-%03d", number) }

func claimTargetVersionTx(tx *sql.Tx, targetID string, expected *int) error {
	query := `UPDATE targets SET target_version = target_version + 1 WHERE id = ?`
	args := []any{targetID}
	if expected != nil {
		query += ` AND target_version = ?`
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
	if err := tx.QueryRow(`SELECT target_version FROM targets WHERE id = ?`, targetID).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("target %s not found", targetID)
		}
		return err
	}
	if expected != nil {
		return &targetVersionConflictError{TargetID: targetID, Expected: *expected, Current: current}
	}
	return fmt.Errorf("target %s was not updated", targetID)
}

func addTargetHistoryTx(tx *sql.Tx, targetID, actor, body string) error {
	var position int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM target_history WHERE target_id = ?`, targetID).Scan(&position); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO target_history(target_id, position, occurred_at, actor, body) VALUES (?, ?, ?, ?, ?)`,
		targetID, position, now(), actor, body)
	return err
}

func (s *Store) addTarget(input NewTarget, options mutation, reason string) (string, error) {
	if strings.TrimSpace(input.Rationale) == "" {
		input.Rationale = reason
	}
	if err := ensureReason(options.Actor, reason); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Goal) == "" {
		input.Goal = input.Resolution
	}
	if strings.TrimSpace(input.Axis) == "" || strings.TrimSpace(input.Goal) == "" || strings.TrimSpace(input.Evaluation) == "" {
		return "", errors.New("target requires axis, goal, and evaluation")
	}
	input.Resolution = input.Goal
	input.Status = normalizeTargetStatus(input.Status)
	if input.Status == "" {
		input.Status = "ACTIVE"
	}
	if input.Status != "ACTIVE" {
		return "", errors.New("new targets must start as ACTIVE")
	}
	input.ObjectiveID = normalizeObjectiveID(input.ObjectiveID)
	if input.ObjectiveID == "" {
		return "", errors.New("target requires an ACTIVE objective")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Fingerprint = strings.TrimSpace(input.Fingerprint)
	if input.Title == "" || input.Fingerprint == "" || strings.TrimSpace(input.Scope) == "" || strings.TrimSpace(input.Evidence) == "" || strings.TrimSpace(input.Resolution) == "" {
		return "", errors.New("target requires title, fingerprint, scope, evidence, and goal")
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return "", err
	}
	id := normalizeTargetID(input.ID)
	if id == "" {
		if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'next_target_id'`).Scan(&id); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	if _, err := targetIDNumber(id); err != nil {
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
		return "", fmt.Errorf("target requires an ACTIVE objective; %s is %s", objective.ID, objective.Status)
	}
	_, err = tx.Exec(`INSERT INTO targets(
        id, target_version, status, title, priority, fingerprint, scope,
	        source_runs, observed_runs, evidence, resolution, merged_into_target_id, updated, updated_by
	    ) VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		id, input.Status, input.Title, input.Priority, input.Fingerprint, input.Scope,
		input.SourceRuns, input.ObservedRuns, input.Evidence, input.Resolution, now(), options.Actor)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	if input.PreviousTargetID != "" {
		previous, err := getTargetFrom(tx, input.PreviousTargetID)
		if err != nil {
			tx.Rollback()
			return "", err
		}
		if previous.Status == "ACTIVE" {
			tx.Rollback()
			return "", errors.New("previous target must be terminal; reuse an ACTIVE target")
		}
	}
	if _, err := tx.Exec(`UPDATE targets SET axis=?,goal=?,evaluation=?,previous_target_id=? WHERE id=?`, input.Axis, input.Goal, input.Evaluation, normalizeTargetID(input.PreviousTargetID), id); err != nil {
		tx.Rollback()
		return "", err
	}
	if err := addTargetHistoryTx(tx, id, options.Actor, reason); err != nil {
		tx.Rollback()
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO objective_targets(objective_id, target_id,is_primary,rationale) VALUES (?, ?,1,?)`, objective.ID, id, input.Rationale); err != nil {
		tx.Rollback()
		return "", err
	}
	insertedNumber, _ := targetIDNumber(id)
	var configuredNext string
	if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'next_target_id'`).Scan(&configuredNext); err != nil {
		tx.Rollback()
		return "", err
	}
	configuredNumber, err := targetIDNumber(configuredNext)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	if input.ID == "" || insertedNumber >= configuredNumber {
		if _, err := tx.Exec(`UPDATE metadata SET value = ? WHERE key = 'next_target_id'`, formatTargetID(insertedNumber+1)); err != nil {
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

func getTargetFrom(q queryer, id string) (Target, error) {
	id = normalizeTargetID(id)
	var target Target
	err := q.QueryRow(`SELECT id, target_version, status, title, priority, fingerprint, scope,
	        source_runs, observed_runs, evidence, resolution, merged_into_target_id, updated, updated_by
	        FROM targets WHERE id = ?`, id).Scan(
		&target.ID, &target.Version, &target.Status, &target.Title, &target.Priority, &target.Fingerprint,
		&target.Scope, &target.SourceRuns, &target.ObservedRuns,
		&target.Evidence, &target.Resolution, &target.MergedIntoID, &target.Updated, &target.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Target{}, fmt.Errorf("target %s not found", id)
	}
	if err != nil {
		return Target{}, err
	}
	if err := q.QueryRow(`SELECT axis,goal,evaluation,previous_target_id,completion_evidence,COALESCE((SELECT objective_id FROM objective_targets WHERE target_id=targets.id AND is_primary=1),'') FROM targets WHERE id=?`, id).Scan(&target.Axis, &target.Goal, &target.Evaluation, &target.PreviousTargetID, &target.CompletionEvidence, &target.PrimaryObjectiveID); err != nil {
		return Target{}, err
	}
	rows, err := q.Query(`SELECT card_id, role FROM target_interventions WHERE target_id = ? ORDER BY card_id`, id)
	if err != nil {
		return Target{}, err
	}
	target.CardRoles = map[string]string{}
	for rows.Next() {
		var cardID, role string
		if err := rows.Scan(&cardID, &role); err != nil {
			rows.Close()
			return Target{}, err
		}
		target.CardIDs = append(target.CardIDs, cardID)
		target.CardRoles[cardID] = role
	}
	if err := rows.Close(); err != nil {
		return Target{}, err
	}
	rows, err = q.Query(`SELECT objective_id,rationale FROM objective_targets WHERE target_id = ? ORDER BY objective_id`, id)
	if err != nil {
		return Target{}, err
	}
	for rows.Next() {
		var objectiveID, rationale string
		if err := rows.Scan(&objectiveID, &rationale); err != nil {
			rows.Close()
			return Target{}, err
		}
		if target.ObjectiveRationales == nil {
			target.ObjectiveRationales = map[string]string{}
		}
		target.ObjectiveRationales[objectiveID] = rationale
		target.ObjectiveIDs = append(target.ObjectiveIDs, objectiveID)
	}
	if err := rows.Close(); err != nil {
		return Target{}, err
	}
	rows, err = q.Query(`SELECT position, occurred_at, actor, body FROM target_history WHERE target_id = ? ORDER BY position`, id)
	if err != nil {
		return Target{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry HistoryEntry
		if err := rows.Scan(&entry.Position, &entry.OccurredAt, &entry.Actor, &entry.Body); err != nil {
			return Target{}, err
		}
		target.History = append(target.History, entry)
	}
	return target, rows.Err()
}

func (s *Store) getTarget(id string) (Target, error) { return getTargetFrom(s.db, id) }

func (s *Store) listTargets(filter TargetFilter) ([]Target, error) {
	rows, err := s.db.Query(`SELECT id FROM targets ORDER BY id`)
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
	var targets []Target
	for _, id := range ids {
		target, err := getTargetFrom(s.db, id)
		if err != nil {
			return nil, err
		}
		if !filter.All && target.Status != "ACTIVE" {
			continue
		}
		if filter.Status != "" && normalizeTargetStatus(filter.Status) != target.Status {
			continue
		}
		if filter.Priority != "" && !strings.EqualFold(filter.Priority, target.Priority) {
			continue
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func (s *Store) updateTarget(id string, patch TargetPatch, expected int, options mutation, reason string) error {
	if patch.Resolution != nil {
		if patch.Goal == nil {
			patch.Goal = patch.Resolution
		}
		patch.Resolution = nil
	}
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	id = normalizeTargetID(id)
	parts := []string{}
	args := []any{}
	fields := []struct {
		column string
		value  *string
	}{{"axis", patch.Axis}, {"goal", patch.Goal}, {"evaluation", patch.Evaluation}, {"title", patch.Title}, {"priority", patch.Priority}, {"fingerprint", patch.Fingerprint}, {"scope", patch.Scope}, {"source_runs", patch.SourceRuns}, {"observed_runs", patch.ObservedRuns}, {"evidence", patch.Evidence}, {"resolution", patch.Resolution}}
	required := map[string]bool{"axis": true, "goal": true, "evaluation": true, "title": true, "fingerprint": true, "scope": true, "evidence": true, "resolution": true}
	for _, field := range fields {
		if field.value != nil {
			if required[field.column] && strings.TrimSpace(*field.value) == "" {
				return fmt.Errorf("target field %s cannot be empty", field.column)
			}
			parts = append(parts, field.column+" = ?")
			args = append(args, *field.value)
		}
	}
	if len(parts) == 0 {
		return errors.New("no target changes supplied")
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	target, err := getTargetFrom(tx, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	if target.Status != "ACTIVE" {
		tx.Rollback()
		return fmt.Errorf("terminal target %s (%s) is immutable", id, target.Status)
	}
	if err := claimTargetVersionTx(tx, id, &expected); err != nil {
		tx.Rollback()
		return err
	}
	parts = append(parts, "updated = ?", "updated_by = ?")
	args = append(args, now(), options.Actor, id)
	if _, err := tx.Exec(`UPDATE targets SET `+strings.Join(parts, ", ")+` WHERE id = ?`, args...); err != nil {
		tx.Rollback()
		return err
	}
	if patch.Goal != nil {
		reason = fmt.Sprintf("%s\nGoal changed from: %s\nGoal changed to: %s", reason, target.Goal, *patch.Goal)
	}
	if err := addTargetHistoryTx(tx, id, options.Actor, reason); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = id, reason
	return finishMutation(tx, current, options)
}

func (s *Store) transitionTarget(id, status, mergedIntoID string, expected int, options mutation, reason string) error {
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	id, status = normalizeTargetID(id), normalizeTargetStatus(status)
	if status == "RESOLVED" && strings.TrimSpace(options.CompletionEvidence) == "" {
		return errors.New("RESOLVED requires --completion-evidence showing goal attainment")
	}
	if !allowedTargetStatuses[status] {
		return fmt.Errorf("invalid target status %q", status)
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	target, err := getTargetFrom(tx, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	allowed := map[string]map[string]bool{
		"ACTIVE": {"RESOLVED": true, "RETIRED": true, "MERGED": true},
	}
	if target.Status == status || !allowed[target.Status][status] {
		tx.Rollback()
		return fmt.Errorf("invalid target status transition %s -> %s", target.Status, status)
	}
	mergedIntoID = normalizeTargetID(mergedIntoID)
	if status == "MERGED" {
		if mergedIntoID == "" {
			tx.Rollback()
			return errors.New("MERGED transition requires --merged-into")
		}
		if mergedIntoID == id {
			tx.Rollback()
			return errors.New("a target cannot merge into itself")
		}
		survivor, err := getTargetFrom(tx, mergedIntoID)
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
	if status == "RESOLVED" {
		if _, err := tx.Exec(`UPDATE targets SET completion_evidence=? WHERE id=?`, options.CompletionEvidence, id); err != nil {
			tx.Rollback()
			return err
		}
		reason += "\nGoal attainment evidence: " + options.CompletionEvidence
	}
	if err := claimTargetVersionTx(tx, id, &expected); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE targets SET status = ?, merged_into_target_id = ?, updated = ?, updated_by = ? WHERE id = ?`, status, mergedIntoID, now(), options.Actor, id); err != nil {
		tx.Rollback()
		return err
	}
	if err := addTargetHistoryTx(tx, id, options.Actor, reason); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = id, status+": "+reason
	return finishMutation(tx, current, options)
}

func (s *Store) setTargetLink(targetID, cardID string, link bool, role, assessmentJSON string, expected int, options mutation, reason string) error {
	if options.Primary && options.ExpectedCardVersion == nil {
		return errors.New("--primary requires --expect-card-version to protect the intervention target set")
	}
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	targetID, cardID = normalizeTargetID(targetID), normalizeID(cardID)
	role = "IMPROVES"
	if strings.TrimSpace(assessmentJSON) != "" {
		return errors.New("target links do not accept residual assessments; record expected contribution in --reason")
	}
	tx, current, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	if err := claimTargetVersionTx(tx, targetID, &expected); err != nil {
		tx.Rollback()
		return err
	}
	target, err := getTargetFrom(tx, targetID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if link && target.Status != "ACTIVE" {
		tx.Rollback()
		return fmt.Errorf("new candidate links require an ACTIVE target; %s is %s", targetID, target.Status)
	}
	if !link && target.Status != "ACTIVE" {
		tx.Rollback()
		return fmt.Errorf("terminal target %s (%s) is immutable", targetID, target.Status)
	}
	if err := claimCardVersionTx(tx, cardID, options.ExpectedCardVersion); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := getCardFrom(tx, cardID); err != nil {
		tx.Rollback()
		return err
	}
	if link && options.Primary {
		if _, err := tx.Exec(`UPDATE target_interventions SET is_primary=0 WHERE card_id=?`, cardID); err != nil {
			tx.Rollback()
			return err
		}
	}
	if link {
		_, err = tx.Exec(`INSERT INTO target_interventions(card_id,target_id,role,rationale,is_primary) VALUES (?,?,'IMPROVES',?,CASE WHEN EXISTS(SELECT 1 FROM target_interventions WHERE card_id=? AND is_primary=1) THEN 0 ELSE 1 END) ON CONFLICT(card_id,target_id) DO UPDATE SET rationale=excluded.rationale,is_primary=CASE WHEN excluded.is_primary=1 THEN 1 ELSE target_interventions.is_primary END`, cardID, targetID, reason, cardID)
	} else {
		var result sql.Result
		result, err = tx.Exec(`DELETE FROM target_interventions WHERE card_id = ? AND target_id = ?`, cardID, targetID)
		if err == nil {
			if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
				if rowsErr != nil {
					err = rowsErr
				} else {
					err = fmt.Errorf("target link %s -> %s not found", targetID, cardID)
				}
			}
		}
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	if !link {
		if _, err := tx.Exec(`UPDATE target_interventions SET is_primary=1 WHERE card_id=? AND target_id=(SELECT MIN(target_id) FROM target_interventions WHERE card_id=?) AND NOT EXISTS(SELECT 1 FROM target_interventions WHERE card_id=? AND is_primary=1)`, cardID, cardID, cardID); err != nil {
			tx.Rollback()
			return err
		}
	}
	action := "linked"
	if !link {
		action = "unlinked"
	}
	if err := addTargetHistoryTx(tx, targetID, options.Actor, fmt.Sprintf("%s %s: %s", action, cardID, reason)); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE targets SET updated = ?, updated_by = ? WHERE id = ?`, now(), options.Actor, targetID); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = targetID, fmt.Sprintf("%s %s: %s", action, cardID, reason)
	return finishMutation(tx, current, options)
}

func activeTargetIDs(targetIDs []string, targets map[string]Target) []string {
	var result []string
	for _, id := range targetIDs {
		if target, ok := targets[id]; ok && target.Status == "ACTIVE" {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}
