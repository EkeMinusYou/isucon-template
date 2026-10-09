package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/pprof"
	runtimepprof "runtime/pprof"
	"strconv"
	"sync"
	"time"

	"github.com/felixge/fgprof"
)

func profileHandler(maxSeconds int) http.Handler {
	return profileHandlerWithStarters(maxSeconds, func(w io.Writer) (func() error, error) {
		if err := runtimepprof.StartCPUProfile(w); err != nil {
			return nil, err
		}
		return func() error { runtimepprof.StopCPUProfile(); return nil }, nil
	}, func(w io.Writer) (func() error, error) {
		return fgprof.Start(w, fgprof.FormatPprof), nil
	})
}

type samplingStarter func(io.Writer) (stop func() error, err error)

func profileHandlerWithStarters(maxSeconds int, startCPU, startFG samplingStarter) http.Handler {
	mux := http.NewServeMux()
	var stateMu sync.Mutex
	active := map[string]string{"cpu": "", "fgprof": ""}
	stopRequests := make(map[string]chan struct{})
	mux.HandleFunc("/debug/profiles/status", func(w http.ResponseWriter, r *http.Request) {
		stateMu.Lock()
		defer stateMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(active)
	})
	mux.HandleFunc("/debug/profiles/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "POST is required", http.StatusMethodNotAllowed)
			return
		}
		kind, runID := r.URL.Query().Get("kind"), r.URL.Query().Get("run_id")
		if (kind != "cpu" && kind != "fgprof") || runID == "" {
			http.Error(w, "kind must be cpu or fgprof and run_id is required", http.StatusBadRequest)
			return
		}
		stateMu.Lock()
		defer stateMu.Unlock()
		if active[kind] != "" && active[kind] != runID {
			http.Error(w, "profile belongs to another RUN", http.StatusConflict)
			return
		}
		// Signal the owning handler; it exports through the original HTTP response.
		// Repeated stops and already completed captures are harmless.
		if stop := stopRequests[kind]; stop != nil {
			close(stop)
			delete(stopRequests, kind)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// CPU and goroutine wall-clock sampling can run together. Reject duplicate
	// captures of the same kind and expose RUN ownership to the collector gate.
	bounded := func(kind string, start samplingStarter) http.Handler {
		var sampling sync.Mutex
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seconds := maxSeconds
			if value := r.URL.Query().Get("seconds"); value != "" {
				var err error
				seconds, err = strconv.Atoi(value)
				if err != nil || seconds < 1 || seconds > maxSeconds {
					http.Error(w, fmt.Sprintf("seconds must be between 1 and %d", maxSeconds), http.StatusBadRequest)
					return
				}
			}
			if !sampling.TryLock() {
				http.Error(w, "a sampling profile is already running", http.StatusConflict)
				return
			}
			defer sampling.Unlock()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			stop, err := start(w)
			if err != nil {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				http.Error(w, "could not start "+kind+" profile: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// Publish RUN ownership only after the profiler has started successfully.
			stopRequested := make(chan struct{})
			stateMu.Lock()
			active[kind] = r.URL.Query().Get("run_id")
			if active[kind] == "" {
				active[kind] = "manual"
			}
			stopRequests[kind] = stopRequested
			stateMu.Unlock()
			defer func() {
				stateMu.Lock()
				active[kind] = ""
				delete(stopRequests, kind)
				stateMu.Unlock()
			}()
			defer func() {
				if err := stop(); err != nil {
					log.Printf("%s profile export failed: %v", kind, err)
				}
			}()
			timer := time.NewTimer(time.Duration(seconds) * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-stopRequested:
			case <-r.Context().Done():
			}
		})
	}
	mux.Handle("/debug/pprof/profile", bounded("cpu", startCPU))
	mux.Handle("/debug/fgprof", bounded("fgprof", startFG))
	for _, name := range []string{"heap", "allocs", "goroutine"} {
		mux.Handle("/debug/pprof/"+name, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// These endpoints are snapshots; delta profiles need a separate policy.
			if r.URL.Query().Has("seconds") {
				http.Error(w, "snapshot profiles do not accept seconds", http.StatusBadRequest)
				return
			}
			pprof.Handler(r.URL.Path[len("/debug/pprof/"):]).ServeHTTP(w, r)
		}))
	}
	return mux
}
