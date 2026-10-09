package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"
)

const defaultMaxResponseBytes = 16 << 20

var (
	metricNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	scopePattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

type metric struct {
	Scope         string `json:"scope"`
	EntityID      int64  `json:"entity_id"`
	GroupID       int64  `json:"group_id"`
	WindowStartMS int64  `json:"window_start_ms"`
	Name          string `json:"name"`
	Value         int64  `json:"value"`
}

type snapshot struct {
	Version        int      `json:"version"`
	Generation     uint64   `json:"generation"`
	Enabled        bool     `json:"enabled"`
	StartedUnixMS  int64    `json:"started_unix_ms"`
	CapturedUnixMS int64    `json:"captured_unix_ms"`
	Detailed       bool     `json:"detailed"`
	MaxEntities    int      `json:"max_entities"`
	EntityEntries  int      `json:"entity_entries"`
	Metrics        []metric `json:"metrics"`
}

type byteLimitWriter struct {
	writer  io.Writer
	limit   int64
	written int64
}

func (writer *byteLimitWriter) Write(body []byte) (int, error) {
	if int64(len(body)) > writer.limit-writer.written {
		return 0, fmt.Errorf("output exceeds %d bytes", writer.limit)
	}
	n, err := writer.writer.Write(body)
	writer.written += int64(n)
	return n, err
}

type collector struct {
	client           *http.Client
	enableEndpoint   string
	snapshotEndpoint string
	disableEndpoint  string
	maxResponseBytes int64
	output           *csv.Writer
	sample           uint64
	generation       uint64
}

func main() {
	enableEndpoint := flag.String("enable-endpoint", "", "POST endpoint that resets and enables metrics")
	snapshotEndpoint := flag.String("snapshot-endpoint", "", "GET endpoint that renews the lease and returns a snapshot")
	disableEndpoint := flag.String("disable-endpoint", "", "POST endpoint that disables metrics and returns the final detailed snapshot")
	interval := flag.Duration("interval", time.Second, "sampling interval")
	timeout := flag.Duration("timeout", 750*time.Millisecond, "HTTP request timeout")
	outputPath := flag.String("output", "-", "TSV output path, or - for stdout")
	maxOutputBytes := flag.Int64("max-output-bytes", 32<<20, "maximum TSV output bytes")
	maxResponseBytes := flag.Int64("max-response-bytes", defaultMaxResponseBytes, "maximum JSON response bytes")
	flag.Parse()

	if *enableEndpoint == "" || *snapshotEndpoint == "" || *disableEndpoint == "" {
		fmt.Fprintln(os.Stderr, "enable-endpoint, snapshot-endpoint, and disable-endpoint are required")
		os.Exit(2)
	}
	if *interval <= 0 || *timeout <= 0 || *maxOutputBytes <= 0 || *maxResponseBytes <= 0 {
		fmt.Fprintln(os.Stderr, "interval, timeout, and byte limits must be greater than zero")
		os.Exit(2)
	}
	for _, endpoint := range []string{*enableEndpoint, *snapshotEndpoint, *disableEndpoint} {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			fmt.Fprintf(os.Stderr, "invalid endpoint %q\n", endpoint)
			os.Exit(2)
		}
	}

	output := io.Writer(os.Stdout)
	var outputFile *os.File
	if *outputPath != "-" {
		var err error
		outputFile, err = os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open output: %v\n", err)
			os.Exit(1)
		}
		defer outputFile.Close()
		output = outputFile
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	limited := &byteLimitWriter{writer: output, limit: *maxOutputBytes}
	if err := collect(ctx, limited, endpoints{*enableEndpoint, *snapshotEndpoint, *disableEndpoint}, *interval, *timeout, *maxResponseBytes); err != nil {
		fmt.Fprintf(os.Stderr, "collect JSON metrics: %v\n", err)
		os.Exit(1)
	}
}

type endpoints struct {
	enable   string
	snapshot string
	disable  string
}

func collect(ctx context.Context, output io.Writer, targets endpoints, interval, timeout time.Duration, maxResponseBytes int64) (resultErr error) {
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	writer.UseCRLF = false
	if err := writer.Write([]string{"sample", "timestamp", "elapsed_ms", "generation", "final", "scope", "entity_id", "group_id", "window_start_ms", "metric", "value"}); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	collector := &collector{
		client:           &http.Client{Timeout: timeout},
		enableEndpoint:   targets.enable,
		snapshotEndpoint: targets.snapshot,
		disableEndpoint:  targets.disable,
		maxResponseBytes: maxResponseBytes,
		output:           writer,
	}
	initial, err := collector.fetch(context.Background(), http.MethodPost, collector.enableEndpoint)
	if err != nil {
		return fmt.Errorf("enable: %w", err)
	}
	if !initial.Enabled {
		return errors.New("enable endpoint returned disabled snapshot")
	}
	collector.generation = initial.Generation
	if err := collector.write(initial, false); err != nil {
		return err
	}
	defer func() {
		finalCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		final, err := collector.fetch(finalCtx, http.MethodPost, collector.disableEndpoint)
		if err == nil && final.Enabled {
			err = errors.New("disable endpoint returned enabled snapshot")
		}
		if err == nil {
			err = collector.write(final, true)
		}
		if resultErr == nil && err != nil {
			resultErr = fmt.Errorf("disable: %w", err)
		}
		writer.Flush()
		if resultErr == nil {
			resultErr = writer.Error()
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			current, err := collector.fetch(ctx, http.MethodGet, collector.snapshotEndpoint)
			if err != nil {
				return fmt.Errorf("snapshot: %w", err)
			}
			if !current.Enabled {
				return errors.New("snapshot lease expired while collector was active")
			}
			if err := collector.write(current, false); err != nil {
				return err
			}
		}
	}
}

func (collector *collector) fetch(ctx context.Context, method, endpoint string) (snapshot, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return snapshot{}, err
	}
	response, err := collector.client.Do(request)
	if err != nil {
		return snapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return snapshot{}, fmt.Errorf("endpoint returned HTTP %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, collector.maxResponseBytes+1))
	if err != nil {
		return snapshot{}, err
	}
	if int64(len(body)) > collector.maxResponseBytes {
		return snapshot{}, fmt.Errorf("response exceeds %d bytes", collector.maxResponseBytes)
	}
	var current snapshot
	if err := json.Unmarshal(body, &current); err != nil {
		return snapshot{}, err
	}
	if err := validateSnapshot(current, collector.maxResponseBytes, int64(len(body))); err != nil {
		return snapshot{}, err
	}
	return current, nil
}

func validateSnapshot(current snapshot, maxResponseBytes, contentLength int64) error {
	if contentLength > maxResponseBytes {
		return fmt.Errorf("response exceeds %d bytes", maxResponseBytes)
	}
	if current.Version != 1 || current.Generation == 0 || current.StartedUnixMS <= 0 || current.CapturedUnixMS < current.StartedUnixMS {
		return errors.New("invalid snapshot metadata")
	}
	if current.MaxEntities <= 0 || current.EntityEntries < 0 || current.EntityEntries > current.MaxEntities {
		return errors.New("invalid snapshot cardinality")
	}
	for _, value := range current.Metrics {
		if !scopePattern.MatchString(value.Scope) || !metricNamePattern.MatchString(value.Name) || value.Value < 0 || value.WindowStartMS < 0 {
			return fmt.Errorf("invalid metric label or value: %+v", value)
		}
		if value.Scope == "global" && (value.EntityID != 0 || value.GroupID != 0) {
			return fmt.Errorf("global metric has entity identifiers: %+v", value)
		}
		if value.Scope != "global" && (value.EntityID <= 0 || value.GroupID <= 0) {
			return fmt.Errorf("entity metric lacks bounded identifiers: %+v", value)
		}
	}
	return nil
}

func (collector *collector) write(current snapshot, final bool) error {
	if collector.generation != 0 && current.Generation != collector.generation {
		return fmt.Errorf("snapshot generation changed from %d to %d", collector.generation, current.Generation)
	}
	collector.generation = current.Generation
	timestamp := time.UnixMilli(current.CapturedUnixMS).UTC().Format(time.RFC3339Nano)
	elapsedMS := current.CapturedUnixMS - current.StartedUnixMS
	for _, value := range current.Metrics {
		row := []string{
			strconv.FormatUint(collector.sample, 10),
			timestamp,
			strconv.FormatInt(elapsedMS, 10),
			strconv.FormatUint(current.Generation, 10),
			strconv.FormatBool(final),
			value.Scope,
			strconv.FormatInt(value.EntityID, 10),
			strconv.FormatInt(value.GroupID, 10),
			strconv.FormatInt(value.WindowStartMS, 10),
			value.Name,
			strconv.FormatInt(value.Value, 10),
		}
		if err := collector.output.Write(row); err != nil {
			return err
		}
	}
	collector.output.Flush()
	if err := collector.output.Error(); err != nil {
		return err
	}
	collector.sample++
	return nil
}
