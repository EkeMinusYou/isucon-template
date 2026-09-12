package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEvaluationContractFence(t *testing.T) {
	contract, err := parseEvaluationContract("```json\n{\"version\":1,\"artifacts\":[\"alp.json\"],\"checks\":[\"status is correct\"],\"note\":\"human guardrail\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if contract.Version != 1 || len(contract.Artifacts) != 1 || contract.Artifacts[0] != "alp.json" || len(contract.Checks) != 1 || contract.Note != "human guardrail" {
		t.Fatalf("unexpected contract: %#v", contract)
	}
}

func TestParseEvaluationContractRejectsUnknownAndTrailingFields(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"unknown":true}`,
		`{"version":1} {"version":1}`,
	} {
		if _, err := parseEvaluationContract(body); err == nil {
			t.Fatalf("parseEvaluationContract(%q) error = nil", body)
		}
	}
}

func TestBuildEvaluationSummaryAcceptsFreeText(t *testing.T) {
	card := Card{
		ID:     "B-001",
		Status: "APPLIED",
		Title:  "free text",
		Sections: []Section{{
			Name: sectionEvaluation,
			Body: "run the focused correctness check",
		}},
	}
	passed := true
	target := verifyManifest{
		RunID: "20260101-000001", Phase: "finalized", Passed: &passed,
		BacklogSnapshot: AppliedSnapshot{SchemaVersion: 3, Status: "ok", Cards: []AppliedSnapshotCard{snapshotCard(card)}},
	}
	summary, err := buildEvaluationSummary(t.TempDir(), card, target, []verifyManifest{target})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Contract != "free text" || len(summary.Checks) != 1 {
		t.Fatalf("free-text summary = %#v", summary)
	}
}

func TestReadALPRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alp.json")
	body := `[["count","method","uri","sum","p99"],[12,"POST","/api/test",1.5,0.2]]`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	values, err := readALPRow(path, "POST", "/api/test", []string{"count", "sum", "p99"})
	if err != nil {
		t.Fatal(err)
	}
	if values["count"] != "12" || values["sum"] != "1.5" || values["p99"] != "0.2" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestSelectCompareManifest(t *testing.T) {
	roles := verifyRoles{App: []string{"h1"}, Entry: "h1", Additional: map[string][]string{"pdns": {"h2"}}}
	manifests := []verifyManifest{
		{RunID: "20260101-000001", Roles: roles},
		{RunID: "20260101-000002", Roles: roles},
		{RunID: "20260101-000003", Roles: verifyRoles{App: []string{"h1"}, Entry: "h2", Additional: roles.Additional}},
		{RunID: "20260101-000004", Roles: verifyRoles{App: []string{"h1"}, Entry: "h1", Additional: map[string][]string{"pdns": {"h3"}}}},
		{RunID: "20260101-000005", Roles: roles, Comparison: runComparison{RunID: "20260101-000001", Status: "compatible"}},
		{RunID: "20260101-000006", Roles: roles},
	}
	for _, tc := range []struct{ name, explicit, want string }{
		{"latest earlier identical roles", "", "20260101-000002"},
		{"explicit baseline", "20260101-000001", "20260101-000001"},
		{"explicit role change for review", "20260101-000003", "20260101-000003"},
		{"multiple explicit baselines", "20260101-000002,20260101-000001", "20260101-000002"},
		{"missing explicit baseline", "20250101-000001", ""},
		{"self comparison", "20260101-000005", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, warning := selectCompareManifest(tc.explicit, manifests[4], manifests)
			if tc.want == "" {
				if selected != nil || warning == "" {
					t.Fatalf("selected=%#v warning=%q", selected, warning)
				}
			} else if selected == nil || selected.RunID != tc.want || warning != "" {
				t.Fatalf("selected=%#v warning=%q want=%s", selected, warning, tc.want)
			}
		})
	}
	if selected, warning := selectCompareManifest("", manifests[0], manifests); selected != nil || warning == "" {
		t.Fatalf("unexpected predecessor: %#v, %q", selected, warning)
	}
	card := Card{ID: "B-001", Status: "APPLIED", CompareRun: manifests[0].RunID}
	summary, err := buildEvaluationSummary(t.TempDir(), card, manifests[4], manifests)
	if err != nil || summary.CompareRun == nil || summary.CompareRun.ID != card.CompareRun {
		t.Fatalf("card comparison not used: %#v, %v", summary, err)
	}
}

func TestLoadVerifyManifestsSkipsStartedRun(t *testing.T) {
	runsDir := t.TempDir()
	fixtures := map[string]string{
		"20260101-000001": `{"run_id":"20260101-000001"}`,
		"20260101-000002": `{"schema_version":4,"phase":"started","run_id":"20260101-000002"}`,
		"20260101-000003": `{"schema_version":4,"phase":"finalized","run_id":"20260101-000003"}`,
	}
	for runID, body := range fixtures {
		dir := filepath.Join(runsDir, runID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifests, err := loadVerifyManifests(runsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 || manifests[0].RunID != "20260101-000003" {
		t.Fatalf("manifests = %#v", manifests)
	}
}

func TestSelectEvidenceManifestUsesLatestRunContainingCard(t *testing.T) {
	card := Card{ID: "B-001", Status: "APPLIED"}
	item := snapshotCard(card)
	manifests := []verifyManifest{
		{RunID: "20260101-000001", BacklogSnapshot: AppliedSnapshot{SchemaVersion: 3, Status: "ok", Cards: []AppliedSnapshotCard{item}}},
		{RunID: "20260101-000002", BacklogSnapshot: AppliedSnapshot{SchemaVersion: 3, Status: "ok", Cards: []AppliedSnapshotCard{}}},
	}
	selected, err := selectEvidenceManifest(card.ID, "", manifests)
	if err != nil {
		t.Fatal(err)
	}
	if selected.RunID != "20260101-000001" {
		t.Fatalf("selected unrelated latest RUN: %#v", selected)
	}
	if _, err := selectEvidenceManifest(card.ID, "20260101-000002", manifests); err == nil {
		t.Fatal("explicit unrelated RUN was accepted")
	}
}

func TestParseVerifyArgsAcceptsExplicitRun(t *testing.T) {
	format, runID, ids, err := parseVerifyArgs([]string{"--format", "json", "--run", "20260101-000001", "B-001,B-002"})
	if err != nil {
		t.Fatal(err)
	}
	if format != "json" || runID != "20260101-000001" || ids != "B-001,B-002" {
		t.Fatalf("parsed args = %q %q %q", format, runID, ids)
	}
}
