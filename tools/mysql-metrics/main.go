package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const defaultDSN = "isucon:isucon@tcp(127.0.0.1:3306)/"

var statusNames = []string{
	"Threads_connected",
	"Threads_running",
	"Threads_cached",
	"Threads_created",
	"Connections",
	"Questions",
	"Com_select",
	"Com_insert",
	"Com_update",
	"Com_delete",
	"Bytes_received",
	"Bytes_sent",
	"Innodb_buffer_pool_pages_total",
	"Innodb_buffer_pool_pages_free",
	"Innodb_buffer_pool_pages_dirty",
	"Innodb_buffer_pool_read_requests",
	"Innodb_buffer_pool_reads",
	"Innodb_row_lock_current_waits",
	"Innodb_row_lock_waits",
	"Innodb_row_lock_time",
	"Innodb_log_waits",
	"Created_tmp_tables",
	"Created_tmp_disk_tables",
	"Aborted_connects",
}

var statusQuery = "SHOW GLOBAL STATUS WHERE Variable_name IN ('" + strings.Join(statusNames, "','") + "')"

type statusSnapshot struct {
	at         time.Time
	statusRows int
	values     map[string]uint64
}

type collector struct {
	db           *sql.DB
	queryTimeout time.Duration
	interval     time.Duration
	output       *csv.Writer
	start        time.Time
	previous     *statusSnapshot
	sampleIndex  uint64
}

func main() {
	dsn := flag.String("dsn", defaultDSN, "MySQL DSN")
	interval := flag.Duration("interval", time.Second, "sampling interval")
	queryTimeout := flag.Duration("query-timeout", 500*time.Millisecond, "status query timeout")
	outputPath := flag.String("output", "-", "TSV output path, or - for stdout")
	lockOutputPath := flag.String("lock-output", "", "data lock wait TSV output path, or empty to disable")
	lockInterval := flag.Duration("lock-interval", 50*time.Millisecond, "data lock wait sampling interval")
	lockQueryTimeout := flag.Duration("lock-query-timeout", 40*time.Millisecond, "data lock wait query timeout")
	flag.Parse()

	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "interval must be greater than zero")
		os.Exit(2)
	}
	if *queryTimeout <= 0 {
		fmt.Fprintln(os.Stderr, "query-timeout must be greater than zero")
		os.Exit(2)
	}
	if *lockInterval <= 0 || *lockQueryTimeout <= 0 {
		fmt.Fprintln(os.Stderr, "lock intervals must be greater than zero")
		os.Exit(2)
	}

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open MySQL: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	prepareContext, prepareCancel := context.WithTimeout(context.Background(), time.Second)
	if _, err := db.ExecContext(prepareContext, "SET SESSION long_query_time = 10"); err != nil {
		fmt.Fprintf(os.Stderr, "set collector session long_query_time: %v\n", err)
	}
	prepareCancel()

	output := io.Writer(os.Stdout)
	var outputFile *os.File
	if *outputPath != "-" {
		outputFile, err = os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open output: %v\n", err)
			os.Exit(1)
		}
		defer outputFile.Close()
		output = outputFile
	}

	var lockDB *sql.DB
	var lockOutput io.Writer
	var lockOutputFile *os.File
	if *lockOutputPath != "" {
		lockDB, err = sql.Open("mysql", *dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open MySQL lock collector: %v\n", err)
			os.Exit(1)
		}
		defer lockDB.Close()
		lockDB.SetMaxOpenConns(1)
		lockDB.SetMaxIdleConns(1)
		lockContext, lockCancel := context.WithTimeout(context.Background(), time.Second)
		if _, err := lockDB.ExecContext(lockContext, "SET SESSION long_query_time = 10"); err != nil {
			fmt.Fprintf(os.Stderr, "set lock collector session long_query_time: %v\n", err)
		}
		lockCancel()

		lockOutputFile, err = os.Create(*lockOutputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open lock output: %v\n", err)
			os.Exit(1)
		}
		defer lockOutputFile.Close()
		lockOutput = lockOutputFile
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var lockWG sync.WaitGroup
	lockErr := make(chan error, 1)
	if lockOutput != nil {
		lockWG.Add(1)
		go func() {
			defer lockWG.Done()
			lockErr <- collectLockWaits(ctx, lockDB, lockOutput, *lockInterval, *lockQueryTimeout)
		}()
	}
	if err := collect(ctx, db, output, *interval, *queryTimeout); err != nil {
		stop()
		lockWG.Wait()
		fmt.Fprintf(os.Stderr, "collect MySQL metrics: %v\n", err)
		os.Exit(1)
	}
	lockWG.Wait()
	if lockOutput != nil {
		if err := <-lockErr; err != nil {
			fmt.Fprintf(os.Stderr, "collect MySQL lock waits: %v\n", err)
			os.Exit(1)
		}
	}
}

func collect(ctx context.Context, db *sql.DB, output io.Writer, interval, queryTimeout time.Duration) error {
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	writer.UseCRLF = false
	if err := writer.Write(header()); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	collector := &collector{
		db:           db,
		queryTimeout: queryTimeout,
		interval:     interval,
		output:       writer,
	}
	collector.poll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			collector.poll(ctx)
		}
	}
}

func (c *collector) poll(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, c.queryTimeout)
	snapshot, err := queryStatus(ctx, c.db)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "MySQL status poll: %v\n", err)
		return
	}
	if c.start.IsZero() {
		c.start = snapshot.at
	}
	if err := c.write(snapshot); err != nil {
		fmt.Fprintf(os.Stderr, "MySQL metrics output: %v\n", err)
		return
	}
	c.previous = &snapshot
	c.sampleIndex++
}

func queryStatus(ctx context.Context, db *sql.DB) (statusSnapshot, error) {
	rows, err := db.QueryContext(ctx, statusQuery)
	if err != nil {
		return statusSnapshot{}, err
	}
	defer rows.Close()

	values := make(map[string]uint64, len(statusNames))
	statusRows := 0
	for rows.Next() {
		var name string
		var rawValue string
		if err := rows.Scan(&name, &rawValue); err != nil {
			return statusSnapshot{}, err
		}
		value, err := strconv.ParseUint(rawValue, 10, 64)
		if err != nil {
			return statusSnapshot{}, fmt.Errorf("parse %s=%q: %w", name, rawValue, err)
		}
		values[name] = value
		statusRows++
	}
	if err := rows.Err(); err != nil {
		return statusSnapshot{}, err
	}
	return statusSnapshot{at: time.Now(), statusRows: statusRows, values: values}, nil
}

func header() []string {
	return []string{
		"sample",
		"timestamp",
		"elapsed_ms",
		"status_rows",
		"threads_connected",
		"threads_running",
		"threads_cached",
		"threads_created_total",
		"threads_created_per_sec",
		"connections_total",
		"connections_per_sec",
		"questions_total",
		"questions_per_sec",
		"com_select_total",
		"com_select_per_sec",
		"com_insert_total",
		"com_insert_per_sec",
		"com_update_total",
		"com_update_per_sec",
		"com_delete_total",
		"com_delete_per_sec",
		"bytes_received_total",
		"bytes_received_per_sec",
		"bytes_sent_total",
		"bytes_sent_per_sec",
		"buffer_pool_pages_total",
		"buffer_pool_pages_free",
		"buffer_pool_pages_dirty",
		"buffer_pool_read_requests_total",
		"buffer_pool_read_requests_per_sec",
		"buffer_pool_reads_total",
		"buffer_pool_reads_per_sec",
		"buffer_pool_hit_pct",
		"row_lock_current_waits",
		"row_lock_waits_total",
		"row_lock_waits_per_sec",
		"row_lock_time_ms_total",
		"row_lock_time_ms_per_sec",
		"innodb_log_waits_total",
		"innodb_log_waits_per_sec",
		"created_tmp_tables_total",
		"created_tmp_tables_per_sec",
		"created_tmp_disk_tables_total",
		"created_tmp_disk_tables_per_sec",
		"tmp_disk_ratio_pct",
		"aborted_connects_total",
		"aborted_connects_per_sec",
	}
}

func (c *collector) write(current statusSnapshot) error {
	if current.at.IsZero() {
		return errors.New("MySQL status timestamp is missing")
	}
	seconds := 0.0
	var previous *statusSnapshot
	if c.previous != nil {
		previous = c.previous
		seconds = current.at.Sub(previous.at).Seconds()
		if seconds <= 0 {
			seconds = c.interval.Seconds()
		}
	}

	value := func(name string) uint64 { return current.values[name] }
	previousValue := func(name string) uint64 {
		if previous == nil {
			return 0
		}
		return previous.values[name]
	}
	rate := func(name string) string {
		return formatRate(counterDifference(value(name), previousValue(name)), seconds)
	}
	bufferPoolRequestsDelta := counterDifference(value("Innodb_buffer_pool_read_requests"), previousValue("Innodb_buffer_pool_read_requests"))
	bufferPoolReadsDelta := counterDifference(value("Innodb_buffer_pool_reads"), previousValue("Innodb_buffer_pool_reads"))
	tmpTablesDelta := counterDifference(value("Created_tmp_tables"), previousValue("Created_tmp_tables"))
	tmpDiskTablesDelta := counterDifference(value("Created_tmp_disk_tables"), previousValue("Created_tmp_disk_tables"))
	tmpDiskRatio := "0.000"
	if seconds > 0 {
		tmpDiskRatio = formatRatio(tmpDiskTablesDelta, tmpTablesDelta)
	}
	values := []string{
		strconv.FormatUint(c.sampleIndex, 10),
		current.at.UTC().Format(time.RFC3339Nano),
		strconv.FormatInt(current.at.Sub(c.start).Milliseconds(), 10),
		strconv.Itoa(current.statusRows),
		strconv.FormatUint(value("Threads_connected"), 10),
		strconv.FormatUint(value("Threads_running"), 10),
		strconv.FormatUint(value("Threads_cached"), 10),
		strconv.FormatUint(value("Threads_created"), 10),
		rate("Threads_created"),
		strconv.FormatUint(value("Connections"), 10),
		rate("Connections"),
		strconv.FormatUint(value("Questions"), 10),
		rate("Questions"),
		strconv.FormatUint(value("Com_select"), 10),
		rate("Com_select"),
		strconv.FormatUint(value("Com_insert"), 10),
		rate("Com_insert"),
		strconv.FormatUint(value("Com_update"), 10),
		rate("Com_update"),
		strconv.FormatUint(value("Com_delete"), 10),
		rate("Com_delete"),
		strconv.FormatUint(value("Bytes_received"), 10),
		rate("Bytes_received"),
		strconv.FormatUint(value("Bytes_sent"), 10),
		rate("Bytes_sent"),
		strconv.FormatUint(value("Innodb_buffer_pool_pages_total"), 10),
		strconv.FormatUint(value("Innodb_buffer_pool_pages_free"), 10),
		strconv.FormatUint(value("Innodb_buffer_pool_pages_dirty"), 10),
		strconv.FormatUint(value("Innodb_buffer_pool_read_requests"), 10),
		formatRate(bufferPoolRequestsDelta, seconds),
		strconv.FormatUint(value("Innodb_buffer_pool_reads"), 10),
		formatRate(bufferPoolReadsDelta, seconds),
		formatHitRate(bufferPoolRequestsDelta, bufferPoolReadsDelta, seconds),
		strconv.FormatUint(value("Innodb_row_lock_current_waits"), 10),
		strconv.FormatUint(value("Innodb_row_lock_waits"), 10),
		rate("Innodb_row_lock_waits"),
		strconv.FormatUint(value("Innodb_row_lock_time"), 10),
		rate("Innodb_row_lock_time"),
		strconv.FormatUint(value("Innodb_log_waits"), 10),
		rate("Innodb_log_waits"),
		strconv.FormatUint(value("Created_tmp_tables"), 10),
		rate("Created_tmp_tables"),
		strconv.FormatUint(value("Created_tmp_disk_tables"), 10),
		rate("Created_tmp_disk_tables"),
		tmpDiskRatio,
		strconv.FormatUint(value("Aborted_connects"), 10),
		rate("Aborted_connects"),
	}
	if len(values) != len(header()) {
		return fmt.Errorf("MySQL metrics row width=%d, header width=%d", len(values), len(header()))
	}
	if err := c.output.Write(values); err != nil {
		return err
	}
	c.output.Flush()
	return c.output.Error()
}

func counterDifference(current, previous uint64) uint64 {
	if current < previous {
		return current
	}
	return current - previous
}

func formatRate(value uint64, seconds float64) string {
	if seconds <= 0 {
		return "0.000"
	}
	return formatFloat(float64(value) / seconds)
}

func formatHitRate(requests, reads uint64, seconds float64) string {
	if seconds <= 0 || requests == 0 {
		return "0.000"
	}
	if reads > requests {
		reads = requests
	}
	return formatFloat(float64(requests-reads) * 100 / float64(requests))
}

func formatRatio(numerator, denominator uint64) string {
	if denominator == 0 {
		return "0.000"
	}
	return formatFloat(float64(numerator) * 100 / float64(denominator))
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
