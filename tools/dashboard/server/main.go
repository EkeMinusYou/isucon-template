// Command dashboardserver is a read-only local API server that aggregates
// ISUCON measurement results (runs/, docs/reports/, tools/backlog) into
// JSON for the tools/dashboard/web React app.
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
	reportsDir string
	dbPath     string
}

func main() {
	root := flag.String("root", ".", "repository root directory")
	dbPath := flag.String("db", "", "path to backlog.sqlite3 (default: <root>/tools/backlog/backlog.sqlite3)")
	addr := flag.String("addr", "127.0.0.1:8091", "listen address")
	flag.Parse()

	a := &app{
		root:       *root,
		runsDir:    filepath.Join(*root, "runs"),
		reportsDir: filepath.Join(*root, "docs", "reports"),
		dbPath:     *dbPath,
	}
	if a.dbPath == "" {
		a.dbPath = filepath.Join(*root, "tools", "backlog", "backlog.sqlite3")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/runs", a.handleRuns)
	mux.HandleFunc("GET /api/scores", a.handleScores)
	mux.HandleFunc("GET /api/runs/{run_id}/alp", a.handleAlp)
	mux.HandleFunc("GET /api/runs/{run_id}/slowquery", a.handleSlowQuery)
	mux.HandleFunc("GET /api/runs/{run_id}/metrics", a.handleMetrics)
	mux.HandleFunc("GET /api/runs/{run_id}/fgprof", a.handleFgprof)
	mux.HandleFunc("GET /api/runs/{run_id}/fgprof/{host}/graph.svg", a.handleFgprofGraph)
	mux.HandleFunc("GET /api/runs/{run_id}/timeline", a.handleTimeline)
	mux.HandleFunc("GET /api/runs/{run_id}/mysql", a.handleMysql)
	mux.HandleFunc("GET /api/runs/{run_id}/upstream", a.handleUpstream)
	mux.HandleFunc("GET /api/runs/{run_id}/user-transitions", a.handleUserTransitions)
	mux.HandleFunc("GET /api/backlog", a.handleBacklog)
	mux.HandleFunc("GET /api/backlog/{id}", a.handleBacklogDetail)
	mux.HandleFunc("GET /api/reports", a.handleReports)
	mux.HandleFunc("GET /api/reports/{path...}", a.handleReportContent)

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
