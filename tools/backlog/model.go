package main

import (
	"fmt"
	"strings"
)

const (
	defaultDatabase = "tools/backlog/backlog.sqlite3"

	sectionObservation    = "Observation"
	sectionHypothesis     = "Hypothesis"
	sectionChangeBoundary = "Change boundary"
	sectionResult         = "Result"
	sectionHistory        = "History"
	sectionEvaluation     = "Evaluation"
	sectionUnknowns       = "Unknowns"
)

var allowedSectionNames = map[string]bool{
	sectionObservation:    true,
	sectionHypothesis:     true,
	sectionChangeBoundary: true,
	sectionUnknowns:       true,
	sectionEvaluation:     true,
	sectionResult:         true,
}

var allowedStatuses = map[string]bool{
	"INVESTIGATE": true,
	"READY":       true,
	"DOING":       true,
	"VERIFY":      true,
	"APPLIED":     true,
	"BLOCKED":     true,
	"VALIDATED":   true,
	"REJECTED":    true,
}

var allowedTargetStatuses = map[string]bool{
	"ACTIVE":   true,
	"RESOLVED": true,
	"RETIRED":  true,
	"MERGED":   true,
}

var allowedDependencyStatuses = map[string]bool{
	"VERIFY":    true,
	"APPLIED":   true,
	"VALIDATED": true,
}

var allowedDependencyModes = map[string]bool{
	"ORDERING": true,
	"BLOCKING": true,
}

var allowedStatusTransitions = map[string]map[string]bool{
	"INVESTIGATE": {"READY": true, "BLOCKED": true, "REJECTED": true},
	"READY":       {"DOING": true, "INVESTIGATE": true, "BLOCKED": true, "REJECTED": true},
	"DOING":       {"VERIFY": true, "INVESTIGATE": true, "BLOCKED": true, "REJECTED": true},
	"VERIFY":      {"DOING": true, "APPLIED": true, "INVESTIGATE": true, "BLOCKED": true, "REJECTED": true},
	"APPLIED":     {"DOING": true, "INVESTIGATE": true, "BLOCKED": true, "VALIDATED": true, "REJECTED": true},
	"BLOCKED":     {"INVESTIGATE": true, "DOING": true, "VERIFY": true, "APPLIED": true},
	"VALIDATED":   {},
	"REJECTED":    {},
}

type Section struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	Body     string `json:"body"`
}

type HistoryEntry struct {
	Position   int    `json:"position"`
	OccurredAt string `json:"occurred_at"`
	Actor      string `json:"actor"`
	Body       string `json:"body"`
	Raw        string `json:"raw"`
}

type CardDependency struct {
	CardID          string `json:"card_id"`
	DependsOnCardID string `json:"depends_on_card_id"`
	RequiredStatus  string `json:"required_status"`
	Mode            string `json:"mode"`
	Reason          string `json:"reason"`
	TargetStatus    string `json:"target_status"`
}

type Card struct {
	PrimaryTargetID string `json:"primary_target_id"`
	ID              string `json:"id"`
	Version         int    `json:"version"`
	Status          string `json:"status"`
	Title           string `json:"title"`
	Closed          bool   `json:"closed"`

	Priority     string `json:"priority"`
	Owner        string `json:"owner"`
	Area         string `json:"area"`
	SourceRuns   string `json:"source_runs"`
	CompareRun   string `json:"compare_run"`
	ObservedRuns string `json:"observed_runs"`
	Updated      string `json:"updated"`
	UpdatedBy    string `json:"updated_by"`

	TargetAssessments map[string]string `json:"target_assessments,omitempty"`
	TargetRoles       map[string]string `json:"target_roles"`

	Sections []Section      `json:"sections,omitempty"`
	History  []HistoryEntry `json:"history,omitempty"`

	Dependencies    []CardDependency `json:"dependencies"`
	Unblocks        []CardDependency `json:"unblocks"`
	TargetIDs       []string         `json:"target_ids"`
	ActiveTargetIDs []string         `json:"active_target_ids"`
	ObjectiveIDs    []string         `json:"objective_ids"`
}

type Target struct {
	Axis                string            `json:"axis"`
	Goal                string            `json:"goal"`
	Evaluation          string            `json:"evaluation"`
	PreviousTargetID    string            `json:"previous_target_id"`
	CompletionEvidence  string            `json:"completion_evidence"`
	PrimaryObjectiveID  string            `json:"primary_objective_id"`
	ObjectiveRationales map[string]string `json:"objective_rationales"`
	ID                  string            `json:"id"`
	Version             int               `json:"version"`
	Status              string            `json:"status"`
	Title               string            `json:"title"`
	Priority            string            `json:"priority"`
	Fingerprint         string            `json:"fingerprint"`
	Scope               string            `json:"scope"`
	SourceRuns          string            `json:"source_runs"`
	ObservedRuns        string            `json:"observed_runs"`
	Evidence            string            `json:"evidence"`
	Resolution          string            `json:"resolution"`
	MergedIntoID        string            `json:"merged_into_id"`
	Updated             string            `json:"updated"`
	UpdatedBy           string            `json:"updated_by"`

	CardIDs      []string          `json:"card_ids"`
	CardRoles    map[string]string `json:"card_roles"`
	ObjectiveIDs []string          `json:"objective_ids"`
	History      []HistoryEntry    `json:"history,omitempty"`
}

type Objective struct {
	ID                     string `json:"id"`
	Version                int    `json:"version"`
	Status                 string `json:"status"`
	Mode                   string `json:"mode"`
	Title                  string `json:"title"`
	MetricOrPredicate      string `json:"metric_or_predicate"`
	RequiredForValidResult bool   `json:"required_for_valid_result"`
	ParentObjectiveID      string `json:"parent_objective_id"`
	OfficialSources        string `json:"official_sources"`
	Verification           string `json:"verification"`
	Updated                string `json:"updated"`
	UpdatedBy              string `json:"updated_by"`

	TargetIDs       []string       `json:"target_ids"`
	InterventionIDs []string       `json:"intervention_ids"`
	History         []HistoryEntry `json:"history,omitempty"`
}

func (c Card) Field(sectionName, key string) string {
	for _, section := range c.Sections {
		if section.Name == sectionName {
			return sectionField(section.Body, key)
		}
	}
	return ""
}

func sectionField(body, key string) string {
	value, _ := sectionFieldValue(body, key)
	return value
}

func sectionFieldValue(body, key string) (string, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		name, value, ok := strings.Cut(strings.TrimPrefix(line, "- "), ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func sectionBody(sections []Section, name string) string {
	for _, section := range sections {
		if section.Name == name {
			return section.Body
		}
	}
	return ""
}

func normalizeStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

func normalizeTargetStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

func isClosedStatus(status string) bool {
	return status == "VALIDATED" || status == "REJECTED"
}

func validateStatusTransition(from, to string) error {
	from, to = normalizeStatus(from), normalizeStatus(to)
	if from == to {
		return nil
	}
	if !allowedStatusTransitions[from][to] {
		return fmt.Errorf("invalid status transition %s -> %s", from, to)
	}
	return nil
}

func validateCardStatusTransition(card Card, to string) error {
	to = normalizeStatus(to)
	return validateStatusTransition(card.Status, to)
}

func validateSectionName(name string) error {
	if name == sectionHistory {
		return fmt.Errorf("History is managed by 'history add'")
	}
	if !allowedSectionNames[name] {
		return fmt.Errorf("unsupported section %q; allowed sections: Observation, Hypothesis, Change boundary, Unknowns, Evaluation, Result", name)
	}
	return nil
}

func normalizeID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "B-")
	value = strings.TrimPrefix(value, "B")
	if value == "" {
		return ""
	}
	var number int
	if _, err := fmt.Sscanf(value, "%d", &number); err == nil {
		return fmt.Sprintf("B-%03d", number)
	}
	return "B-" + value
}

func normalizeTargetID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != "" && !strings.HasPrefix(value, "A-") {
		value = "A-" + value
	}
	return value
}
