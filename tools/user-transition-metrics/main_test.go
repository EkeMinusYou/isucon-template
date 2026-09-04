package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func testConfig(t *testing.T) config {
	t.Helper()
	cfg := config{
		CookieField: "session_id",
		APIPrefix:   "/api/",
		Routes: []routeConfig{
			{Name: "/api/item/:id", Pattern: `^/api/item/[0-9]+$`},
			{Name: "/api/list", Pattern: `^/api/list$`},
		},
	}
	for i := range cfg.Routes {
		cfg.Routes[i].re = regexpMustCompile(t, cfg.Routes[i].Pattern)
	}
	return cfg
}

func regexpMustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return re
}

func TestAggregateTransitionsDoesNotExposeIdentity(t *testing.T) {
	input := strings.Join([]string{
		`{"msec":1000.010,"method":"GET","uri":"/api/list?limit=1","status":200,"response_time":0.002,"session_id":"secret-a"}`,
		`{"msec":"1000.020","method":"POST","uri":"/api/item/42","status":"201","response_time":0.004,"session_id":"secret-a"}`,
		`{"msec":1000.030,"method":"GET","uri":"/api/list","status":200,"response_time":0.020,"session_id":"secret-a"}`,
		`{"msec":1000.040,"method":"GET","uri":"/api/list","status":500,"response_time":0.001,"session_id":"secret-b"}`,
		`{"msec":1000.050,"method":"POST","uri":"/api/item/7","status":400,"response_time":0.001,"session_id":"secret-b"}`,
		`{"msec":1000.060,"method":"GET","uri":"/api/unknown/9","status":200,"response_time":0.001,"session_id":"secret-c"}`,
		`{"msec":1000.070,"method":"GET","uri":"/api/list","status":200,"response_time":0.001,"session_id":""}`,
		`not-json`,
	}, "\n")

	agg := newAggregator(testConfig(t), 100)
	if err := agg.scan(strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	rep := agg.buildReport(10)
	if rep.Summary.InputLines != 8 || rep.Summary.MalformedLines != 1 {
		t.Fatalf("unexpected input summary: %#v", rep.Summary)
	}
	if rep.Summary.APIRequests != 7 || rep.Summary.ClassifiedRequests != 6 || rep.Summary.UnmatchedAPIRequests != 1 {
		t.Fatalf("unexpected classification summary: %#v", rep.Summary)
	}
	if rep.Summary.RequestsWithIdentity != 5 || rep.Summary.MissingIdentityRequests != 1 || rep.Summary.Sessions != 2 {
		t.Fatalf("unexpected identity summary: %#v", rep.Summary)
	}
	if rep.Summary.Transitions != 3 || rep.Summary.OverlappingTransitions != 1 {
		t.Fatalf("unexpected transition summary: %#v", rep.Summary)
	}
	if len(rep.Edges) != 2 {
		t.Fatalf("edges=%d, want 2", len(rep.Edges))
	}
	if rep.SchemaVersion != 3 || rep.Summary.ScenarioGroups != 1 || len(rep.Scenarios) != 1 {
		t.Fatalf("unexpected scenario summary: %#v", rep)
	}
	scenario := rep.Scenarios[0]
	if scenario.Sessions != 2 || scenario.Requests != 5 || scenario.Transitions != 3 || len(scenario.Nodes) != 2 {
		t.Fatalf("unexpected scenario: %#v", scenario)
	}
	// Both sessions open on GET /api/list and close on POST /api/item/:id, so
	// the timeline positions readers lay out left to right must be 0 and 1.
	timeline := make(map[string]scenarioNodeReport, len(scenario.Nodes))
	for _, node := range scenario.Nodes {
		timeline[node.Method+" "+node.Route] = node
	}
	if got := timeline["GET /api/list"]; got.FirstPositionAvg != 0 || got.FirstOffsetAvgMS != 0 {
		t.Fatalf("unexpected entry node timeline: %#v", got)
	}
	if got := timeline["POST /api/item/:id"]; got.FirstPositionAvg != 1 || got.FirstOffsetAvgMS != 9 {
		t.Fatalf("unexpected exit node timeline: %#v", got)
	}
	body, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret-a") || strings.Contains(string(body), "secret-b") {
		t.Fatalf("identity leaked into report: %s", body)
	}
	if rep.Edges[0].FromRoute != "/api/list" || rep.Edges[0].ToRoute != "/api/item/:id" {
		t.Fatalf("route normalization failed: %#v", rep.Edges[0])
	}
}

func TestEqualStartTimesAreMarkedAmbiguous(t *testing.T) {
	input := strings.Join([]string{
		`{"msec":1000.010,"method":"GET","uri":"/api/list","status":200,"response_time":0.010,"session_id":"same"}`,
		`{"msec":1000.020,"method":"POST","uri":"/api/item/1","status":201,"response_time":0.020,"session_id":"same"}`,
	}, "\n")
	agg := newAggregator(testConfig(t), 10)
	if err := agg.scan(strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	rep := agg.buildReport(10)
	if rep.Summary.AmbiguousTransitions != 1 || rep.Edges[0].AmbiguousTransitions != 1 {
		t.Fatalf("ambiguous transition not counted: %#v", rep)
	}
}

func TestScenariosGroupByExactNodeSetAndTrackObservedBoundaries(t *testing.T) {
	input := strings.Join([]string{
		`{"msec":1000.010,"method":"GET","uri":"/api/list","status":200,"response_time":0.001,"session_id":"a"}`,
		`{"msec":1000.020,"method":"POST","uri":"/api/item/1","status":201,"response_time":0.001,"session_id":"a"}`,
		`{"msec":1000.030,"method":"GET","uri":"/api/list","status":200,"response_time":0.001,"session_id":"a"}`,
		`{"msec":1000.040,"method":"POST","uri":"/api/item/2","status":201,"response_time":0.001,"session_id":"b"}`,
		`{"msec":1000.050,"method":"GET","uri":"/api/list","status":200,"response_time":0.001,"session_id":"b"}`,
		`{"msec":1000.060,"method":"GET","uri":"/api/list","status":200,"response_time":0.001,"session_id":"c"}`,
		`{"msec":1000.070,"method":"GET","uri":"/api/list","status":200,"response_time":0.001,"session_id":""}`,
	}, "\n")

	agg := newAggregator(testConfig(t), 100)
	if err := agg.scan(strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	rep := agg.buildReport(10)
	if rep.Summary.Sessions != 3 || rep.Summary.ScenarioGroups != 2 || len(rep.Scenarios) != 2 {
		t.Fatalf("unexpected groups: %#v", rep.Summary)
	}
	group := rep.Scenarios[0]
	if group.Sessions != 2 || group.Requests != 5 || group.Transitions != 3 {
		t.Fatalf("unexpected largest group: %#v", group)
	}
	var firstTotal, lastTotal uint64
	for _, node := range group.Nodes {
		firstTotal += node.FirstSessions
		lastTotal += node.LastSessions
	}
	if firstTotal != group.Sessions || lastTotal != group.Sessions {
		t.Fatalf("observed boundaries do not cover sessions: first=%d last=%d sessions=%d", firstTotal, lastTotal, group.Sessions)
	}
	if rep.Summary.MissingIdentityRequests != 1 {
		t.Fatalf("Cookie-less request was not excluded: %#v", rep.Summary)
	}
}

func TestEventLimitFailsClosed(t *testing.T) {
	agg := newAggregator(testConfig(t), 1)
	input := strings.Join([]string{
		`{"msec":1000.010,"method":"GET","uri":"/api/list","status":200,"response_time":0.001,"session_id":"a"}`,
		`{"msec":1000.020,"method":"GET","uri":"/api/list","status":200,"response_time":0.001,"session_id":"a"}`,
	}, "\n")
	if err := agg.scan(strings.NewReader(input)); err == nil {
		t.Fatal("event limit did not fail")
	}
}
