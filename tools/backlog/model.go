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
	sectionVerification   = "Verification"
	sectionUnknowns       = "Unknowns"
)

var allowedSectionNames = map[string]bool{
	sectionObservation:    true,
	sectionHypothesis:     true,
	sectionChangeBoundary: true,
	sectionUnknowns:       true,
	sectionVerification:   true,
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

var allowedConstraintStatuses = map[string]bool{
	"ACTIVE":      true,
	"RESOLVED":    true,
	"INVALIDATED": true,
	"MERGED":      true,
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
	Name     string
	Position int
	Body     string
}

type HistoryEntry struct {
	Position   int
	OccurredAt string
	Actor      string
	Body       string
	Raw        string
}

type CardDependency struct {
	CardID          string
	DependsOnCardID string
	RequiredStatus  string
	Mode            string
	Reason          string
	TargetStatus    string
}

type Card struct {
	ID      string
	Version int
	Status  string
	Title   string
	Closed  bool

	Priority     string
	Owner        string
	Area         string
	SourceRuns   string
	CompareRun   string
	ObservedRuns string
	Fingerprint  string
	Updated      string
	UpdatedBy    string

	ConstraintAssessments map[string]string
	ConstraintRoles       map[string]string

	Sections []Section
	History  []HistoryEntry

	Dependencies        []CardDependency
	Unblocks            []CardDependency
	ConstraintIDs       []string
	ActiveConstraintIDs []string
	ObjectiveIDs        []string
}

type Constraint struct {
	ID           string
	Version      int
	Status       string
	Title        string
	Priority     string
	Fingerprint  string
	Scope        string
	SourceRuns   string
	ObservedRuns string
	Evidence     string
	Resolution   string
	MergedIntoID string
	Updated      string
	UpdatedBy    string

	CardIDs      []string
	CardRoles    map[string]string
	ObjectiveIDs []string
	History      []HistoryEntry
}

type Objective struct {
	ID                     string
	Version                int
	Status                 string
	Mode                   string
	Title                  string
	MetricOrPredicate      string
	RequiredForValidResult bool
	ParentObjectiveID      string
	OfficialSources        string
	Verification           string
	Updated                string
	UpdatedBy              string

	ConstraintIDs   []string
	InterventionIDs []string
	History         []HistoryEntry
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

func normalizeConstraintStatus(status string) string {
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
		return fmt.Errorf("unsupported section %q; allowed sections: Observation, Hypothesis, Change boundary, Unknowns, Verification, Result", name)
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

func normalizeConstraintID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != "" && !strings.HasPrefix(value, "A-") {
		value = "A-" + value
	}
	return value
}
