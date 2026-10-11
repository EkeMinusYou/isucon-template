package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EkeMinusYou/isucon-template/tools/measurement/app"
)

type fixedStats struct{ sql.DBStats }

func (s fixedStats) Stats() sql.DBStats { return s.DBStats }

func TestAppExporterCollectorContract(t *testing.T) {
	handler, err := app.NewHandler(app.Options{
		Pools: []app.Pool{{
			DatabaseHost: "db.local", Name: "user", Role: "primary", Shard: 2,
			DB: fixedStats{sql.DBStats{
				MaxOpenConnections: 8, OpenConnections: 3, InUse: 2, Idle: 1,
				WaitCount: 7, WaitDuration: 5 * time.Millisecond,
				MaxIdleClosed: 4, MaxIdleTimeClosed: 5, MaxLifetimeClosed: 6,
			}},
		}},
		Snapshot: func() app.Snapshot {
			return app.Snapshot{
				Metrics: map[string]any{"events_total": uint64(12)},
				Extra:   map[string]any{"version": 99, "post_wal": map[string]any{"records": 12}},
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.Comma = '\t'
	c := &collector{
		client: server.Client(), endpoint: server.URL + "/debug/sql-pools",
		maxPools: defaultMaxPools, maxMetrics: defaultMaxMetrics, maxResponse: defaultMaxResponse,
		output: writer, previous: make(map[string]previousPool),
	}
	snapshot, err := c.fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := snapshot.Pools[0]
	if p.DatabaseHost != "db.local" || p.Name != "user" || p.Role != "primary" || p.Shard != 2 ||
		p.MaxOpenConnections != 8 || p.OpenConnections != 3 || p.InUse != 2 || p.Idle != 1 ||
		p.WaitCount != 7 || p.WaitDurationNS != 5_000_000 ||
		p.MaxIdleClosed != 4 || p.MaxIdleTimeClosed != 5 || p.MaxLifetimeClosed != 6 {
		t.Fatalf("pool fields changed: %+v", p)
	}
	if err := c.write(snapshot); err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{
		"db.local\tuser\tprimary\t2\twait_duration_ms_total\t5.000000",
		"local\tsnapshot\tmetrics\t0\tevents_total\t12.000000",
	} {
		if !strings.Contains(output.String(), row) {
			t.Fatalf("missing TSV measurement %q in %s", row, output.String())
		}
	}
}
