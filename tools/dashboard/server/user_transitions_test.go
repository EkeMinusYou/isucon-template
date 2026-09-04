package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleUserTransitions(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	runID := "20260903-120000"
	runDir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version":2,"identity_field":"session_id","ordering":"request_start_time","scenario_grouping":"exact set","summary":{"input_files":2,"sessions":3,"transitions":4,"scenario_groups":1,"scenarios_emitted":1},"edges":[{"from_method":"GET","from_route":"/api/a","to_method":"POST","to_route":"/api/b","transitions":4,"sessions":3}],"scenarios":[{"id":"scenario-test","signature":["GET /api/a","POST /api/b"],"sessions":3,"requests":7,"transitions":4,"nodes":[{"method":"GET","route":"/api/a","requests":3,"sessions":3,"first_sessions":3,"last_sessions":0}],"edges":[{"from_method":"GET","from_route":"/api/a","to_method":"POST","to_route":"/api/b","transitions":4,"sessions":3}]}]}`
	if err := os.WriteFile(filepath.Join(runDir, "user-transitions.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &app{runsDir: runsDir}
	req := httptest.NewRequest("GET", "/api/runs/"+runID+"/user-transitions", nil)
	req.SetPathValue("run_id", runID)
	recorder := httptest.NewRecorder()
	a.handleUserTransitions(recorder, req)
	if recorder.Code != 200 {
		t.Fatalf("status=%d", recorder.Code)
	}
	var got userTransitionsResponse
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.Summary.Sessions != 3 || len(got.Edges) != 1 || len(got.Scenarios) != 1 {
		t.Fatalf("unexpected response: %#v", got)
	}
	if got.Scenarios[0].ID != "scenario-test" || got.Scenarios[0].Nodes[0].FirstSessions != 3 {
		t.Fatalf("unexpected scenario: %#v", got.Scenarios[0])
	}
}

func TestHandleUserTransitionsUnavailable(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	runID := "20260903-120001"
	if err := os.MkdirAll(filepath.Join(runsDir, runID), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &app{runsDir: runsDir}
	req := httptest.NewRequest("GET", "/api/runs/"+runID+"/user-transitions", nil)
	req.SetPathValue("run_id", runID)
	recorder := httptest.NewRecorder()
	a.handleUserTransitions(recorder, req)

	var got userTransitionsResponse
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Available || got.Edges == nil {
		t.Fatalf("unexpected unavailable response: %#v", got)
	}
}
