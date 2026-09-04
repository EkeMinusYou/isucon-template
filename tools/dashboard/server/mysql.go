package main

import (
	"net/http"
	"os"
	"path/filepath"
)

// mysqlPoint mirrors a useful subset of `mysql-status.tsv` (one row per
// sample of `SHOW GLOBAL STATUS`/`SHOW GLOBAL VARIABLES` deltas on the
// single MySQL host). Monotonic *_total counters are intentionally skipped
// in favor of their *_per_sec rate, which is what actually reads as a time
// series; gauges (thread/pool/lock counts, hit/ratio percentages) are kept
// as-is.
type mysqlPoint struct {
	ElapsedMs int64 `json:"elapsed_ms"`

	ThreadsConnected      float64 `json:"threads_connected"`
	ThreadsRunning        float64 `json:"threads_running"`
	ThreadsCached         float64 `json:"threads_cached"`
	ThreadsCreatedPerSec  float64 `json:"threads_created_per_sec"`
	ConnectionsPerSec     float64 `json:"connections_per_sec"`
	AbortedConnectsPerSec float64 `json:"aborted_connects_per_sec"`

	QuestionsPerSec float64 `json:"questions_per_sec"`
	ComSelectPerSec float64 `json:"com_select_per_sec"`
	ComInsertPerSec float64 `json:"com_insert_per_sec"`
	ComUpdatePerSec float64 `json:"com_update_per_sec"`
	ComDeletePerSec float64 `json:"com_delete_per_sec"`

	BytesReceivedPerSec float64 `json:"bytes_received_per_sec"`
	BytesSentPerSec     float64 `json:"bytes_sent_per_sec"`

	BufferPoolPagesTotal         float64 `json:"buffer_pool_pages_total"`
	BufferPoolPagesFree          float64 `json:"buffer_pool_pages_free"`
	BufferPoolPagesDirty         float64 `json:"buffer_pool_pages_dirty"`
	BufferPoolReadRequestsPerSec float64 `json:"buffer_pool_read_requests_per_sec"`
	BufferPoolReadsPerSec        float64 `json:"buffer_pool_reads_per_sec"`
	BufferPoolHitPct             float64 `json:"buffer_pool_hit_pct"`

	RowLockCurrentWaits  float64 `json:"row_lock_current_waits"`
	RowLockWaitsPerSec   float64 `json:"row_lock_waits_per_sec"`
	RowLockTimeMsPerSec  float64 `json:"row_lock_time_ms_per_sec"`
	InnodbLogWaitsPerSec float64 `json:"innodb_log_waits_per_sec"`

	CreatedTmpTablesPerSec     float64 `json:"created_tmp_tables_per_sec"`
	CreatedTmpDiskTablesPerSec float64 `json:"created_tmp_disk_tables_per_sec"`
	TmpDiskRatioPct            float64 `json:"tmp_disk_ratio_pct"`
}

type mysqlResponse struct {
	RunID     string       `json:"run_id"`
	Available bool         `json:"available"`
	Series    []mysqlPoint `json:"series"`
}

func parseMysqlSeries(path string) ([]mysqlPoint, error) {
	header, rows, err := readTSV(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if columnIndex(header, "elapsed_ms") < 0 {
		return nil, nil
	}

	series := make([]mysqlPoint, 0, len(rows))
	for _, row := range rows {
		c := &colIndexer{header: header, row: row}
		series = append(series, mysqlPoint{
			ElapsedMs: c.int64("elapsed_ms"),

			ThreadsConnected:      c.float("threads_connected"),
			ThreadsRunning:        c.float("threads_running"),
			ThreadsCached:         c.float("threads_cached"),
			ThreadsCreatedPerSec:  c.float("threads_created_per_sec"),
			ConnectionsPerSec:     c.float("connections_per_sec"),
			AbortedConnectsPerSec: c.float("aborted_connects_per_sec"),

			QuestionsPerSec: c.float("questions_per_sec"),
			ComSelectPerSec: c.float("com_select_per_sec"),
			ComInsertPerSec: c.float("com_insert_per_sec"),
			ComUpdatePerSec: c.float("com_update_per_sec"),
			ComDeletePerSec: c.float("com_delete_per_sec"),

			BytesReceivedPerSec: c.float("bytes_received_per_sec"),
			BytesSentPerSec:     c.float("bytes_sent_per_sec"),

			BufferPoolPagesTotal:         c.float("buffer_pool_pages_total"),
			BufferPoolPagesFree:          c.float("buffer_pool_pages_free"),
			BufferPoolPagesDirty:         c.float("buffer_pool_pages_dirty"),
			BufferPoolReadRequestsPerSec: c.float("buffer_pool_read_requests_per_sec"),
			BufferPoolReadsPerSec:        c.float("buffer_pool_reads_per_sec"),
			BufferPoolHitPct:             c.float("buffer_pool_hit_pct"),

			RowLockCurrentWaits:  c.float("row_lock_current_waits"),
			RowLockWaitsPerSec:   c.float("row_lock_waits_per_sec"),
			RowLockTimeMsPerSec:  c.float("row_lock_time_ms_per_sec"),
			InnodbLogWaitsPerSec: c.float("innodb_log_waits_per_sec"),

			CreatedTmpTablesPerSec:     c.float("created_tmp_tables_per_sec"),
			CreatedTmpDiskTablesPerSec: c.float("created_tmp_disk_tables_per_sec"),
			TmpDiskRatioPct:            c.float("tmp_disk_ratio_pct"),
		})
	}
	return series, nil
}

func (a *app) handleMysql(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}
	series, err := parseMysqlSeries(filepath.Join(dir, "mysql-status.tsv"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	available := series != nil
	if series == nil {
		series = make([]mysqlPoint, 0)
	}
	writeJSON(w, mysqlResponse{RunID: runID, Available: available, Series: series})
}
