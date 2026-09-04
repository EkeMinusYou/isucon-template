package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseVerificationContractFence(t *testing.T) {
	contract, err := parseVerificationContract("```json\n{\"version\":1,\"artifacts\":[\"alp.json\"],\"checks\":[\"status is correct\"],\"note\":\"human guardrail\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if contract.Version != 1 || len(contract.Artifacts) != 1 || contract.Artifacts[0] != "alp.json" || len(contract.Checks) != 1 || contract.Note != "human guardrail" {
		t.Fatalf("unexpected contract: %#v", contract)
	}
}

func TestParseVerificationContractRejectsUnknownAndTrailingFields(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"unknown":true}`,
		`{"version":1} {"version":1}`,
	} {
		if _, err := parseVerificationContract(body); err == nil {
			t.Fatalf("parseVerificationContract(%q) error = nil", body)
		}
	}
}

func TestBuildVerificationSummaryKeepsLegacyTextReadable(t *testing.T) {
	card := Card{
		ID:     "B-001",
		Status: "APPLIED",
		Title:  "legacy",
		Sections: []Section{{
			Name: sectionVerification,
			Body: "run the focused correctness check",
		}},
	}
	passed := true
	summary, err := buildVerificationSummary(t.TempDir(), card, []verifyManifest{{RunID: "20260101-000001", Passed: &passed}})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Contract != "free text" || len(summary.Checks) != 1 {
		t.Fatalf("legacy summary = %#v", summary)
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
	roles := verifyRoles{App: []string{"isucon-1"}, MySQL: "isucon-2"}
	other := verifyRoles{App: []string{"isucon-3"}, MySQL: "isucon-2"}
	manifests := []verifyManifest{
		{RunID: "20260101-000001", Roles: roles},
		{RunID: "20260101-000002", Roles: other},
		{RunID: "20260101-000003", Roles: roles},
	}
	selected := selectCompareManifest("", manifests[2], manifests)
	if selected == nil || selected.RunID != "20260101-000001" {
		t.Fatalf("unexpected comparison: %#v", selected)
	}
}

func TestLoadVerifyManifestsSkipsStartedRun(t *testing.T) {
	runsDir := t.TempDir()
	fixtures := map[string]string{
		"20260101-000001": `{"run_id":"20260101-000001"}`,
		"20260101-000002": `{"schema_version":2,"phase":"started","run_id":"20260101-000002"}`,
		"20260101-000003": `{"schema_version":2,"phase":"finalized","run_id":"20260101-000003"}`,
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
	if len(manifests) != 2 || manifests[0].RunID != "20260101-000001" || manifests[1].RunID != "20260101-000003" {
		t.Fatalf("manifests = %#v", manifests)
	}
}
