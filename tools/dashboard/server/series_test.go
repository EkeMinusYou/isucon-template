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
	}{
		{name: "mysql", path: "mysql", handler: a.handleMysql},
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
			var got struct {
				Available bool  `json:"available"`
				Series    []any `json:"series"`
			}
			if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.Available {
				t.Fatal("available = true, want false")
			}
			if got.Series == nil {
				t.Fatal("series = nil, want an empty JSON array")
			}
			if len(got.Series) != 0 {
				t.Fatalf("len(series) = %d, want 0", len(got.Series))
			}
		})
	}
}
