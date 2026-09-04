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

func TestBuildVerificationSummaryAcceptsFreeText(t *testing.T) {
	card := Card{
		ID:     "B-001",
		Status: "APPLIED",
		Title:  "free text",
		Sections: []Section{{
			Name: sectionVerification,
			Body: "run the focused correctness check",
		}},
	}
	passed := true
	target := verifyManifest{
		RunID: "20260101-000001", Phase: "finalized", Passed: &passed,
		BacklogSnapshot: AppliedSnapshot{SchemaVersion: 3, Status: "ok", Cards: []AppliedSnapshotCard{snapshotCard(card)}},
	}
	summary, err := buildVerificationSummary(t.TempDir(), card, target, []verifyManifest{target})
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

func TestSelectDeclaredCompareManifestUsesOnlyCompatibleDeclaredRun(t *testing.T) {
	manifests := []verifyManifest{
		{RunID: "20260101-000001"},
		{RunID: "20260101-000002"},
		{RunID: "20260101-000003", Comparison: runComparison{RunID: "20260101-000001", Status: "compatible"}},
	}
	selected, warning := selectDeclaredCompareManifest(manifests[2], manifests)
	if selected == nil || selected.RunID != "20260101-000001" {
		t.Fatalf("unexpected comparison: %#v", selected)
	}
	if warning != "" {
		t.Fatalf("unexpected warning: %s", warning)
	}
	manifests[2].Comparison.Status = "incompatible"
	if selected, warning := selectDeclaredCompareManifest(manifests[2], manifests); selected != nil || warning == "" {
		t.Fatalf("incompatible comparison selected: selected=%#v warning=%q", selected, warning)
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
