package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultEndpoint = "http://127.0.0.1:6060/debug/runtime"

	defaultMaxMetrics  = 1024
	defaultMaxResponse = 1 << 20
	defaultMaxOutput   = 32 << 20
)

var (
	labelPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

type snapshot struct {
	Version        int                `json:"version"`
	StartedUnixMS  int64              `json:"started_unix_ms"`
	CapturedUnixMS int64              `json:"captured_unix_ms"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
}

type collector struct {
	client      *http.Client
	endpoint    string
	output      *csv.Writer
	sample      uint64
	maxMetrics  int
	maxResponse int64
}

type limitedWriter struct {
	writer io.Writer
	limit  int64
	used   int64
}

func (w *limitedWriter) Write(value []byte) (int, error) {
	if int64(len(value)) > w.limit-w.used {
		return 0, fmt.Errorf("output exceeds %d bytes", w.limit)
	}
	n, err := w.writer.Write(value)
	w.used += int64(n)
	return n, err
}

func main() {
	endpoint := flag.String("endpoint", defaultEndpoint, "runtime metrics endpoint")
	interval := flag.Duration("interval", time.Second, "sampling interval")
	timeout := flag.Duration("timeout", 750*time.Millisecond, "HTTP request timeout")
	outputPath := flag.String("output", "-", "TSV output path, or - for stdout")
	maxMetrics := flag.Int("max-metrics", defaultMaxMetrics, "maximum numeric application metrics per snapshot")
	maxResponse := flag.Int64("max-response-bytes", defaultMaxResponse, "maximum response body size")
	maxOutput := flag.Int64("max-output-bytes", defaultMaxOutput, "maximum TSV output size")
	flag.Parse()

	if *interval <= 0 || *timeout <= 0 || *maxMetrics <= 0 || *maxResponse <= 0 || *maxOutput <= 0 {
		fmt.Fprintln(os.Stderr, "interval, timeout, metric, and byte limits must be greater than zero")
		os.Exit(2)
	}
	if strings.TrimSpace(*endpoint) == "" {
		fmt.Fprintln(os.Stderr, "endpoint must not be empty")
		os.Exit(2)
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
	limited := &limitedWriter{writer: output, limit: *maxOutput}
	if err := collect(context.Background(), limited, *endpoint, *interval, *timeout, *maxMetrics, *maxResponse); err != nil {
		fmt.Fprintf(os.Stderr, "collect runtime metrics: %v\n", err)
		os.Exit(1)
	}
}

func collect(parent context.Context, output io.Writer, endpoint string, interval, timeout time.Duration, maxMetrics int, maxResponse int64) error {
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	writer.UseCRLF = false
	if err := writer.Write([]string{"sample", "timestamp", "elapsed_ms", "metric", "value"}); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	collector := &collector{
		client:      &http.Client{Timeout: timeout},
		endpoint:    endpoint,
		output:      writer,
		maxMetrics:  maxMetrics,
		maxResponse: maxResponse,
	}
	collector.poll(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			writer.Flush()
			return writer.Error()
		case <-ticker.C:
			collector.poll(ctx)
		}
	}
}

func (c *collector) poll(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, c.client.Timeout)
	current, err := c.fetch(ctx)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "runtime metrics poll: %v\n", err)
		return
	}
	if err := c.write(current); err != nil {
		fmt.Fprintf(os.Stderr, "runtime metrics output: %v\n", err)
	}
}

func (c *collector) fetch(ctx context.Context) (snapshot, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return snapshot{}, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return snapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return snapshot{}, fmt.Errorf("endpoint returned HTTP %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponse+1))
	if err != nil {
		return snapshot{}, err
	}
	if int64(len(body)) > c.maxResponse {
		return snapshot{}, fmt.Errorf("response exceeds %d bytes", c.maxResponse)
	}
	var current snapshot
	if err := json.Unmarshal(body, &current); err != nil {
		return snapshot{}, err
	}
	if err := validateSnapshot(current, c.maxMetrics, int64(len(body))); err != nil {
		return snapshot{}, err
	}
	return current, nil
}

func validateSnapshot(current snapshot, maxMetrics int, contentLength int64) error {
	if contentLength < 0 {
		return errors.New("negative content length")
	}
	if current.Version != 1 || current.StartedUnixMS <= 0 || current.CapturedUnixMS < current.StartedUnixMS {
		return errors.New("invalid snapshot metadata")
	}
	if len(current.Metrics) == 0 {
		return errors.New("snapshot has no metrics")
	}
	if len(current.Metrics) > maxMetrics {
		return errors.New("too many snapshot metrics")
	}
	for name, value := range current.Metrics {
		if !labelPattern.MatchString(name) || math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("invalid snapshot metric")
		}
	}
	return nil
}

func (c *collector) write(current snapshot) error {
	currentAt := current.CapturedUnixMS
	timestamp := time.UnixMilli(currentAt).UTC().Format(time.RFC3339Nano)
	elapsedMS := currentAt - current.StartedUnixMS
	for name, value := range current.Metrics {
		if err := c.output.Write([]string{strconv.FormatUint(c.sample, 10), timestamp, strconv.FormatInt(elapsedMS, 10), name, strconv.FormatFloat(value, 'f', 6, 64)}); err != nil {
			return err
		}
	}
	c.output.Flush()
	if err := c.output.Error(); err != nil {
		return err
	}
	c.sample++
	return nil
}
