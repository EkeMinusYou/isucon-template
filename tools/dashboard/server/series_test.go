package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSeriesHandlersReturnEmptyArrayWhenUnavailable(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	runID := "20260829-120000"
	if err := os.MkdirAll(filepath.Join(runsDir, runID), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &app{runsDir: runsDir}

	tests := []struct {
		name    string
		path    string
		handler http.HandlerFunc
		field   string
	}{
		{name: "mysql", path: "mysql", handler: a.handleMysql, field: "hosts"},
		{name: "alp", path: "alp", handler: a.handleAlp, field: "rows"},
		{name: "upstream", path: "upstream", handler: a.handleUpstream, field: "rows"},
		{name: "user-transitions", path: "user-transitions", handler: a.handleUserTransitions, field: "edges"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/runs/"+runID+"/"+tt.path, nil)
			req.SetPathValue("run_id", runID)
			recorder := httptest.NewRecorder()

			tt.handler(recorder, req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			var got map[string]json.RawMessage
			if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if string(got["available"]) != "false" || string(got[tt.field]) != "[]" {
				t.Fatalf("unexpected unavailable response: %s", got)
			}
		})
	}
}
