// Command dashboardserver is a read-only local API server that aggregates
// ISUCON measurement results from runs/ into JSON for the dashboard.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"path/filepath"
)

type app struct {
	root       string
	runsDir    string
	analysisDB string
}

func main() {
	root := flag.String("root", ".", "repository root directory")
	analysisDB := flag.String("analysis-db", "", "path to analysis.duckdb")
	addr := flag.String("addr", "127.0.0.1:8091", "listen address")
	flag.Parse()

	a := &app{
		root:       *root,
		analysisDB: *analysisDB,
		runsDir:    filepath.Join(*root, "runs"),
	}
	if a.analysisDB == "" {
		a.analysisDB = filepath.Join(*root, "runs", "analysis.duckdb")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/runs", a.handleRuns)
	mux.HandleFunc("GET /api/scores", a.handleScores)
	mux.HandleFunc("GET /api/runs/{run_id}/alp", a.handleAlp)
	mux.HandleFunc("GET /api/runs/{run_id}/slowquery", a.handleSlowQuery)
	mux.HandleFunc("GET /api/runs/{run_id}/metrics", a.handleMetrics)
	mux.HandleFunc("GET /api/runs/{run_id}/fgprof", a.handleFgprof)
	mux.HandleFunc("GET /api/runs/{run_id}/fgprof/{host}/graph.svg", a.handleFgprofGraph)
	mux.HandleFunc("GET /api/runs/{run_id}/pprof", a.handleGoPprof)
	mux.HandleFunc("GET /api/runs/{run_id}/pprof/{kind}/{host}/graph.svg", a.handleGoPprofGraph)
	mux.HandleFunc("GET /api/runs/{run_id}/timeline", a.handleTimeline)
	mux.HandleFunc("GET /api/runs/{run_id}/mysql", a.handleMysql)
	mux.HandleFunc("GET /api/runs/{run_id}/upstream", a.handleUpstream)
	mux.HandleFunc("GET /api/runs/{run_id}/user-transitions", a.handleUserTransitions)

	log.Printf("dashboardserver listening on %s (root=%s)", *addr, a.root)
	if err := http.ListenAndServe(*addr, withLogging(mux)); err != nil {
		log.Fatal(err)
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		log.Printf("%s %s", r.Method, r.URL.Path)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
