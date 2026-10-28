// Package app exposes application measurements on a dedicated HTTP listener.
package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// Pool identifies a database pool with stable labels. *sql.DB and *sqlx.DB
// both satisfy its Stats interface.
type Pool struct {
	DatabaseHost string
	Name         string
	Role         string
	Shard        int
	DB           interface{ Stats() sql.DBStats }
}

// Snapshot supplies application-specific numeric metrics and optional extra
// JSON fields. Standard snapshot fields always take precedence over Extra.
type Snapshot struct {
	Metrics map[string]any
	Extra   map[string]any
}

type Options struct {
	// Address defaults to 127.0.0.1:6060. It must not expose private metrics
	// to public traffic.
	Address string
	// MaxProfileSeconds defaults to 3600 and must be between 1 and 3600.
	MaxProfileSeconds int
	Pools             []Pool
	Snapshot          func() Snapshot
}

func normalize(options Options) (Options, error) {
	if options.Address == "" {
		options.Address = "127.0.0.1:6060"
	}
	if options.MaxProfileSeconds == 0 {
		options.MaxProfileSeconds = 3600
	}
	if options.MaxProfileSeconds < 1 || options.MaxProfileSeconds > 3600 {
		return options, fmt.Errorf("profile duration limit must be between 1 and 3600 seconds")
	}
	for _, pool := range options.Pools {
		if pool.DB == nil {
			return options, fmt.Errorf("pool %q has no database", pool.Name)
		}
	}
	options.Pools = append([]Pool(nil), options.Pools...)
	return options, nil
}

// NewHandler creates the profile and SQL pool endpoints without starting a
// listener. Each application process should create only one handler.
func NewHandler(options Options) (http.Handler, error) {
	options, err := normalize(options)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	mux := http.NewServeMux()
	mux.Handle("/", profileHandler(options.MaxProfileSeconds))
	mux.HandleFunc("/debug/sql-pools", func(w http.ResponseWriter, r *http.Request) {
		snapshot := Snapshot{}
		if options.Snapshot != nil {
			snapshot = options.Snapshot()
		}
		payload := make(map[string]any, len(snapshot.Extra)+5)
		for key, value := range snapshot.Extra {
			payload[key] = value
		}
		pools := make([]map[string]any, 0, len(options.Pools))
		for _, pool := range options.Pools {
			stats := pool.DB.Stats()
			pools = append(pools, map[string]any{
				"database_host": pool.DatabaseHost, "name": pool.Name, "role": pool.Role, "shard": pool.Shard,
				"max_open_connections": stats.MaxOpenConnections,
				"open_connections":     stats.OpenConnections, "in_use": stats.InUse, "idle": stats.Idle,
				"wait_count": stats.WaitCount, "wait_duration_ns": stats.WaitDuration.Nanoseconds(),
				"max_idle_closed": stats.MaxIdleClosed, "max_idle_time_closed": stats.MaxIdleTimeClosed,
				"max_lifetime_closed": stats.MaxLifetimeClosed,
			})
		}
		payload["version"] = 1
		payload["started_unix_ms"] = started.UnixMilli()
		payload["captured_unix_ms"] = time.Now().UnixMilli()
		payload["pools"] = pools
		observations := make(map[string]any, len(snapshot.Metrics)+8)
		for name, value := range snapshot.Metrics {
			observations[name] = value
		}
		for name, value := range runtimeMetrics() {
			observations[name] = value
		}
		observations["go_process_started_unix_ms"] = started.UnixMilli()
		payload["metrics"] = observations
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Printf("measurement snapshot export failed: %v", err)
		}
	})
	return mux, nil
}

// Start binds before returning, so a bind failure is reported synchronously.
// The caller owns the returned server and can Close or Shutdown it.
func Start(options Options) (*http.Server, error) {
	options, err := normalize(options)
	if err != nil {
		return nil, err
	}
	handler, err := NewHandler(options)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", options.Address)
	if err != nil {
		return nil, fmt.Errorf("measurement listener unavailable: %w", err)
	}
	server := &http.Server{
		Addr: options.Address, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      time.Duration(options.MaxProfileSeconds+15) * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("measurement server stopped: %v", err)
		}
	}()
	return server, nil
}
