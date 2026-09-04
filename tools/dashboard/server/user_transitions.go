package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

type userTransitionSummary struct {
	InputFiles              int64 `json:"input_files"`
	InputLines              int64 `json:"input_lines"`
	MalformedLines          int64 `json:"malformed_lines"`
	InvalidTimeLines        int64 `json:"invalid_time_lines"`
	APIRequests             int64 `json:"api_requests"`
	ClassifiedRequests      int64 `json:"classified_requests"`
	UnmatchedAPIRequests    int64 `json:"unmatched_api_requests"`
	RequestsWithIdentity    int64 `json:"requests_with_identity"`
	MissingIdentityRequests int64 `json:"missing_identity_requests"`
	Sessions                int64 `json:"sessions"`
	Transitions             int64 `json:"transitions"`
	OverlappingTransitions  int64 `json:"overlapping_transitions"`
	AmbiguousTransitions    int64 `json:"ambiguous_order_transitions"`
	WindowStartUnixMS       int64 `json:"window_start_unix_ms"`
	WindowEndUnixMS         int64 `json:"window_end_unix_ms"`
	ScenarioGroups          int64 `json:"scenario_groups"`
	ScenariosEmitted        int64 `json:"scenarios_emitted"`
	ScenarioSessionsOmitted int64 `json:"scenario_sessions_omitted"`
}

type userTransitionEdge struct {
	FromMethod           string  `json:"from_method"`
	FromRoute            string  `json:"from_route"`
	ToMethod             string  `json:"to_method"`
	ToRoute              string  `json:"to_route"`
	Transitions          int64   `json:"transitions"`
	Sessions             int64   `json:"sessions"`
	OverlapTransitions   int64   `json:"overlap_transitions"`
	AmbiguousTransitions int64   `json:"ambiguous_order_transitions"`
	StartGapAvgMS        float64 `json:"start_gap_avg_ms"`
	StartGapP50MS        float64 `json:"start_gap_p50_ms"`
	StartGapP95MS        float64 `json:"start_gap_p95_ms"`
	IdleGapAvgMS         float64 `json:"idle_gap_avg_ms"`
	FromStatus2xx        int64   `json:"from_status_2xx"`
	FromStatus3xx        int64   `json:"from_status_3xx"`
	FromStatus4xx        int64   `json:"from_status_4xx"`
	FromStatus5xx        int64   `json:"from_status_5xx"`
	FromStatusOther      int64   `json:"from_status_other"`
	ToStatus2xx          int64   `json:"to_status_2xx"`
	ToStatus3xx          int64   `json:"to_status_3xx"`
	ToStatus4xx          int64   `json:"to_status_4xx"`
	ToStatus5xx          int64   `json:"to_status_5xx"`
	ToStatusOther        int64   `json:"to_status_other"`
}

type userScenarioNode struct {
	Method           string  `json:"method"`
	Route            string  `json:"route"`
	Requests         int64   `json:"requests"`
	Sessions         int64   `json:"sessions"`
	FirstSessions    int64   `json:"first_sessions"`
	LastSessions     int64   `json:"last_sessions"`
	FirstPositionAvg float64 `json:"first_position_avg"`
	FirstOffsetAvgMS float64 `json:"first_offset_avg_ms"`
}

type userScenario struct {
	ID                   string               `json:"id"`
	Signature            []string             `json:"signature"`
	Sessions             int64                `json:"sessions"`
	Requests             int64                `json:"requests"`
	Transitions          int64                `json:"transitions"`
	OverlapTransitions   int64                `json:"overlap_transitions"`
	AmbiguousTransitions int64                `json:"ambiguous_order_transitions"`
	RequestsPerSession   float64              `json:"requests_per_session_avg"`
	DurationP50MS        float64              `json:"duration_p50_ms"`
	DurationP95MS        float64              `json:"duration_p95_ms"`
	Nodes                []userScenarioNode   `json:"nodes"`
	Edges                []userTransitionEdge `json:"edges"`
}

type userTransitionsArtifact struct {
	SchemaVersion    int                   `json:"schema_version"`
	IdentityField    string                `json:"identity_field"`
	Ordering         string                `json:"ordering"`
	ScenarioGrouping string                `json:"scenario_grouping"`
	Summary          userTransitionSummary `json:"summary"`
	Edges            []userTransitionEdge  `json:"edges"`
	Scenarios        []userScenario        `json:"scenarios"`
}

type userTransitionsResponse struct {
	RunID     string `json:"run_id"`
	Available bool   `json:"available"`
	userTransitionsArtifact
}

func parseUserTransitions(path string) (userTransitionsArtifact, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return userTransitionsArtifact{Edges: []userTransitionEdge{}}, false, nil
		}
		return userTransitionsArtifact{}, false, err
	}
	defer f.Close()

	var artifact userTransitionsArtifact
	if err := json.NewDecoder(f).Decode(&artifact); err != nil {
		return userTransitionsArtifact{}, false, err
	}
	if artifact.Edges == nil {
		artifact.Edges = []userTransitionEdge{}
	}
	if artifact.Scenarios == nil {
		artifact.Scenarios = []userScenario{}
	}
	return artifact, artifact.SchemaVersion >= 1 && artifact.Summary.InputFiles > 0, nil
}

func (a *app) handleUserTransitions(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}
	artifact, available, err := parseUserTransitions(filepath.Join(dir, "user-transitions.json"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, userTransitionsResponse{
		RunID:                   runID,
		Available:               available,
		userTransitionsArtifact: artifact,
	})
}
