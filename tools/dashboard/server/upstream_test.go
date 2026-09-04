package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleUpstreamReturnsEmptyArrayWhenUnavailable(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	runID := "20260829-120000"
	if err := os.MkdirAll(filepath.Join(runsDir, runID), 0o755); err != nil {
		t.Fatal(err)
	}

	a := &app{runsDir: runsDir}
	req := httptest.NewRequest("GET", "/api/runs/"+runID+"/upstream", nil)
	req.SetPathValue("run_id", runID)
	recorder := httptest.NewRecorder()

	a.handleUpstream(recorder, req)

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var got struct {
		Available bool          `json:"available"`
		Rows      []upstreamRow `json:"rows"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Available {
		t.Fatal("available = true, want false")
	}
	if got.Rows == nil {
		t.Fatal("rows = nil, want an empty JSON array")
	}
	if len(got.Rows) != 0 {
		t.Fatalf("len(rows) = %d, want 0", len(got.Rows))
	}
}
