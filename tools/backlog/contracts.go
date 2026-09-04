package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

type AssessmentAxis struct {
	Unit        string `json:"unit"`
	Denominator string `json:"denominator"`
}

type AssessmentCurrent struct {
	Value    float64 `json:"value"`
	Snapshot string  `json:"snapshot"`
}

type AssessmentEstimate struct {
	Value float64 `json:"value"`
	Basis string  `json:"basis"`
}

type PerformanceResidualAssessment struct {
	Version   int                `json:"version"`
	Axis      AssessmentAxis     `json:"axis"`
	Current   AssessmentCurrent  `json:"current"`
	Reduction AssessmentEstimate `json:"reduction"`
	AddedCost AssessmentEstimate `json:"added_cost"`
	Threshold AssessmentEstimate `json:"threshold"`
}

func decodeStrictJSON(raw string, destination any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return nil
}

func parsePerformanceResidualAssessment(raw string) (PerformanceResidualAssessment, string, error) {
	var assessment PerformanceResidualAssessment
	if err := decodeStrictJSON(raw, &assessment); err != nil {
		return assessment, "", fmt.Errorf("invalid constraint assessment: %w", err)
	}
	if assessment.Version != 1 {
		return assessment, "", fmt.Errorf("constraint assessment version must be 1, got %d", assessment.Version)
	}
	assessment.Axis.Unit = strings.TrimSpace(assessment.Axis.Unit)
	assessment.Axis.Denominator = strings.TrimSpace(assessment.Axis.Denominator)
	assessment.Current.Snapshot = strings.TrimSpace(assessment.Current.Snapshot)
	if assessment.Axis.Unit == "" || assessment.Axis.Denominator == "" {
		return assessment, "", errors.New("constraint assessment axis requires unit and denominator")
	}
	if assessment.Current.Snapshot == "" {
		return assessment, "", errors.New("constraint assessment current requires snapshot")
	}
	for _, item := range []struct {
		name  string
		value float64
		basis string
	}{
		{"current", assessment.Current.Value, "current observation"},
		{"reduction", assessment.Reduction.Value, assessment.Reduction.Basis},
		{"added_cost", assessment.AddedCost.Value, assessment.AddedCost.Basis},
		{"threshold", assessment.Threshold.Value, assessment.Threshold.Basis},
	} {
		if math.IsNaN(item.value) || math.IsInf(item.value, 0) || item.value < 0 {
			return assessment, "", fmt.Errorf("%s value must be finite and non-negative", item.name)
		}
		if item.name != "current" && strings.TrimSpace(item.basis) == "" {
			return assessment, "", fmt.Errorf("%s requires basis", item.name)
		}
	}
	assessment.Reduction.Basis = strings.TrimSpace(assessment.Reduction.Basis)
	assessment.AddedCost.Basis = strings.TrimSpace(assessment.AddedCost.Basis)
	assessment.Threshold.Basis = strings.TrimSpace(assessment.Threshold.Basis)
	if assessment.predictedResidual() < 0 {
		return assessment, "", errors.New("predicted residual must be non-negative")
	}
	canonical, err := json.Marshal(assessment)
	if err != nil {
		return assessment, "", err
	}
	return assessment, string(canonical), nil
}

func (assessment PerformanceResidualAssessment) predictedResidual() float64 {
	return assessment.Current.Value - assessment.Reduction.Value + assessment.AddedCost.Value
}

func (assessment PerformanceResidualAssessment) resolves() bool {
	tolerance := math.Max(1e-9, math.Abs(assessment.Current.Value)*0.001)
	return assessment.predictedResidual() <= assessment.Threshold.Value+tolerance
}

func constraintDefinitionHash(constraint Constraint) string {
	definition := struct {
		Fingerprint string `json:"fingerprint"`
		Scope       string `json:"scope"`
		Evidence    string `json:"evidence"`
		Resolution  string `json:"resolution"`
	}{constraint.Fingerprint, constraint.Scope, constraint.Evidence, constraint.Resolution}
	body, _ := json.Marshal(definition)
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cardAssessmentChangeBoundaryHash(card Card) string {
	return cardChangeBoundaryHash(card)
}

func getPerformanceResidualAssessmentFrom(q queryer, constraint Constraint, card Card) (PerformanceResidualAssessment, error) {
	var raw, constraintHash, changeBoundaryHash string
	if err := q.QueryRow(`SELECT assessment_json, constraint_definition_hash, card_change_boundary_hash FROM constraint_intervention_assessments WHERE constraint_id = ? AND card_id = ?`, constraint.ID, card.ID).Scan(&raw, &constraintHash, &changeBoundaryHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PerformanceResidualAssessment{}, fmt.Errorf("card %s requires a structured constraint candidate assessment before linking to %s", card.ID, constraint.ID)
		}
		return PerformanceResidualAssessment{}, err
	}
	if constraintHash != constraintDefinitionHash(constraint) {
		return PerformanceResidualAssessment{}, fmt.Errorf("assessment for %s -> %s is stale because the constraint definition changed", constraint.ID, card.ID)
	}
	if changeBoundaryHash != cardAssessmentChangeBoundaryHash(card) {
		return PerformanceResidualAssessment{}, fmt.Errorf("assessment for %s -> %s is stale because the card change boundary changed", constraint.ID, card.ID)
	}
	assessment, _, err := parsePerformanceResidualAssessment(raw)
	return assessment, err
}

func (s *Store) setConstraintInterventionAssessment(constraintID, cardID, raw string, expected int, options mutation, reason string) error {
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	constraintID, cardID = normalizeConstraintID(constraintID), normalizeID(cardID)
	assessment, canonical, err := parsePerformanceResidualAssessment(raw)
	if err != nil {
		return err
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
	if constraint.Status != "ACTIVE" {
		tx.Rollback()
		return fmt.Errorf("candidate assessments require an ACTIVE constraint; %s is %s", constraintID, constraint.Status)
	}
	card, err := getCardFrom(tx, cardID)
	if err != nil {
		tx.Rollback()
		return err
	}
	var role string
	if err := tx.QueryRow(`SELECT role FROM constraint_interventions WHERE card_id = ? AND constraint_id = ?`, cardID, constraintID).Scan(&role); err != nil && !errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		return err
	}
	if role == "RESOLVES" && !assessment.resolves() {
		tx.Rollback()
		return errors.New("a RESOLVES relation must retain an assessment satisfying the resolution threshold")
	}
	if role == "MITIGATES" && assessment.resolves() {
		tx.Rollback()
		return errors.New("a MITIGATES relation must retain an assessment that does not satisfy the resolution threshold")
	}
	if _, err := tx.Exec(`INSERT INTO constraint_intervention_assessments(card_id, constraint_id, assessment_json, constraint_definition_hash, card_change_boundary_hash, updated, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(card_id, constraint_id) DO UPDATE SET assessment_json=excluded.assessment_json, constraint_definition_hash=excluded.constraint_definition_hash,
		card_change_boundary_hash=excluded.card_change_boundary_hash, updated=excluded.updated, updated_by=excluded.updated_by`,
		cardID, constraintID, canonical, constraintDefinitionHash(constraint), cardAssessmentChangeBoundaryHash(card), now(), options.Actor); err != nil {
		tx.Rollback()
		return err
	}
	if err := addConstraintHistoryTx(tx, constraintID, options.Actor, fmt.Sprintf("assessed %s: %s", cardID, reason)); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE constraints SET updated = ?, updated_by = ? WHERE id = ?`, now(), options.Actor, constraintID); err != nil {
		tx.Rollback()
		return err
	}
	options.CardID, options.Summary = constraintID, fmt.Sprintf("assessed %s: %s", cardID, reason)
	return finishMutation(tx, current, options)
}
