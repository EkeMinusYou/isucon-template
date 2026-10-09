package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"time"
)

const dataLockWaitsQuery = `
SELECT
  CONCAT_WS(':',
    w.REQUESTING_ENGINE_TRANSACTION_ID,
    w.REQUESTING_EVENT_ID,
    w.REQUESTING_ENGINE_LOCK_ID,
    w.BLOCKING_ENGINE_TRANSACTION_ID,
    w.BLOCKING_EVENT_ID,
    w.BLOCKING_ENGINE_LOCK_ID
  ) AS wait_key,
  w.REQUESTING_ENGINE_TRANSACTION_ID,
  w.BLOCKING_ENGINE_TRANSACTION_ID,
  rl.OBJECT_SCHEMA,
  rl.OBJECT_NAME,
  rl.INDEX_NAME,
  rl.LOCK_TYPE,
  rl.LOCK_MODE,
  rl.LOCK_DATA,
  bl.LOCK_TYPE,
  bl.LOCK_MODE,
  bl.LOCK_DATA,
  w.REQUESTING_THREAD_ID,
  rt.PROCESSLIST_ID,
  rs.SQL_TEXT,
  w.BLOCKING_THREAD_ID,
  bt.PROCESSLIST_ID,
  bs.SQL_TEXT
FROM performance_schema.data_lock_waits AS w
JOIN performance_schema.data_locks AS rl
  ON rl.ENGINE_LOCK_ID = w.REQUESTING_ENGINE_LOCK_ID
JOIN performance_schema.data_locks AS bl
  ON bl.ENGINE_LOCK_ID = w.BLOCKING_ENGINE_LOCK_ID
LEFT JOIN performance_schema.threads AS rt
  ON rt.THREAD_ID = w.REQUESTING_THREAD_ID
LEFT JOIN performance_schema.threads AS bt
  ON bt.THREAD_ID = w.BLOCKING_THREAD_ID
LEFT JOIN performance_schema.events_statements_current AS rs
  ON rs.THREAD_ID = w.REQUESTING_THREAD_ID
LEFT JOIN performance_schema.events_statements_current AS bs
  ON bs.THREAD_ID = w.BLOCKING_THREAD_ID
ORDER BY wait_key
`

type lockWaitRow struct {
	waitKey         string
	requestingTrx   string
	blockingTrx     string
	objectSchema    string
	objectTable     string
	objectIndex     string
	requestLockType string
	requestLockMode string
	requestLockData string
	blockLockType   string
	blockLockMode   string
	blockLockData   string
	requestThread   string
	requestProcess  string
	requestSQL      string
	blockThread     string
	blockProcess    string
	blockSQL        string
}

func collectLockWaits(ctx context.Context, db *sql.DB, output io.Writer, interval, queryTimeout time.Duration) error {
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	writer.UseCRLF = false
	if err := writer.Write(lockWaitHeader()); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	startedAt := time.Now()
	var sample uint64
	poll := func() error {
		at := time.Now()
		queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
		rows, err := queryLockWaits(queryCtx, db)
		cancel()
		captureError := ""
		if err != nil {
			captureError = sanitizeLockField(err.Error())
		}
		if len(rows) == 0 {
			rows = []lockWaitRow{{}}
		}
		captured := len(rows)
		if captured == 1 && rows[0].waitKey == "" {
			captured = 0
		}
		for _, row := range rows {
			values := []string{
				strconv.FormatUint(sample, 10), at.UTC().Format(time.RFC3339Nano),
				strconv.FormatInt(at.Sub(startedAt).Milliseconds(), 10), strconv.Itoa(captured), captureError,
				row.waitKey, row.requestingTrx, row.blockingTrx, row.objectSchema, row.objectTable, row.objectIndex,
				row.requestLockType, row.requestLockMode, row.requestLockData,
				row.blockLockType, row.blockLockMode, row.blockLockData,
				row.requestThread, row.requestProcess, row.requestSQL,
				row.blockThread, row.blockProcess, row.blockSQL,
			}
			if err := writer.Write(values); err != nil {
				return err
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return err
		}
		sample++
		return nil
	}
	if err := poll(); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := poll(); err != nil {
				return err
			}
		}
	}
}

func queryLockWaits(ctx context.Context, db *sql.DB) ([]lockWaitRow, error) {
	rows, err := db.QueryContext(ctx, dataLockWaitsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]lockWaitRow, 0)
	for rows.Next() {
		values := make([]sql.NullString, 18)
		pointers := make([]interface{}, len(values))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		value := func(index int) string { return sanitizeLockField(values[index].String) }
		result = append(result, lockWaitRow{
			waitKey: value(0), requestingTrx: value(1), blockingTrx: value(2),
			objectSchema: value(3), objectTable: value(4), objectIndex: value(5),
			requestLockType: value(6), requestLockMode: value(7), requestLockData: value(8),
			blockLockType: value(9), blockLockMode: value(10), blockLockData: value(11),
			requestThread: value(12), requestProcess: value(13), requestSQL: value(14),
			blockThread: value(15), blockProcess: value(16), blockSQL: value(17),
		})
	}
	return result, rows.Err()
}

func lockWaitHeader() []string {
	return []string{
		"sample", "timestamp", "elapsed_ms", "captured_waits", "capture_error", "wait_key",
		"requesting_trx_id", "blocking_trx_id", "object_schema", "object_table", "object_index",
		"request_lock_type", "request_lock_mode", "request_lock_data",
		"blocking_lock_type", "blocking_lock_mode", "blocking_lock_data",
		"requesting_thread_id", "requesting_processlist_id", "requesting_sql",
		"blocking_thread_id", "blocking_processlist_id", "blocking_sql",
	}
}

func sanitizeLockField(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if len(value) > 4096 {
		value = value[:4096]
	}
	return value
}
