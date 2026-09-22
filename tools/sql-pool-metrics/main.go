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
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultEndpoint    = "http://127.0.0.1:6060/debug/sql-pools"
	defaultMaxPools    = 128
	defaultMaxResponse = 1 << 20
	defaultMaxOutput   = 32 << 20
)

var (
	labelPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	hostPattern  = regexp.MustCompile(`^[A-Za-z0-9_.:\[\]-]{1,128}$`)
)

type snapshot struct {
	Version        int    `json:"version"`
	StartedUnixMS  int64  `json:"started_unix_ms"`
	CapturedUnixMS int64  `json:"captured_unix_ms"`
	Pools          []pool `json:"pools"`
}

type pool struct {
	DatabaseHost       string `json:"database_host"`
	Name               string `json:"name"`
	Role               string `json:"role"`
	Shard              int    `json:"shard"`
	MaxOpenConnections int    `json:"max_open_connections"`
	OpenConnections    int    `json:"open_connections"`
	InUse              int    `json:"in_use"`
	Idle               int    `json:"idle"`
	WaitCount          int64  `json:"wait_count"`
	WaitDurationNS     int64  `json:"wait_duration_ns"`
	MaxIdleClosed      int64  `json:"max_idle_closed"`
	MaxIdleTimeClosed  int64  `json:"max_idle_time_closed"`
	MaxLifetimeClosed  int64  `json:"max_lifetime_closed"`
}

type previousPool struct {
	capturedUnixMS int64
	value          pool
}

type collector struct {
	client      *http.Client
	endpoint    string
	output      *csv.Writer
	previous    map[string]previousPool
	sample      uint64
	maxPools    int
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
	endpoint := flag.String("endpoint", defaultEndpoint, "SQL pool metrics endpoint")
	interval := flag.Duration("interval", time.Second, "sampling interval")
	timeout := flag.Duration("timeout", 750*time.Millisecond, "HTTP request timeout")
	outputPath := flag.String("output", "-", "TSV output path, or - for stdout")
	maxPools := flag.Int("max-pools", defaultMaxPools, "maximum pool entries per snapshot")
	maxResponse := flag.Int64("max-response-bytes", defaultMaxResponse, "maximum response body size")
	maxOutput := flag.Int64("max-output-bytes", defaultMaxOutput, "maximum TSV output size")
	flag.Parse()

	if *interval <= 0 || *timeout <= 0 || *maxPools <= 0 || *maxResponse <= 0 || *maxOutput <= 0 {
		fmt.Fprintln(os.Stderr, "interval, timeout, pool, and byte limits must be greater than zero")
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
	if err := collect(context.Background(), limited, *endpoint, *interval, *timeout, *maxPools, *maxResponse); err != nil {
		fmt.Fprintf(os.Stderr, "collect SQL pool metrics: %v\n", err)
		os.Exit(1)
	}
}

func collect(parent context.Context, output io.Writer, endpoint string, interval, timeout time.Duration, maxPools int, maxResponse int64) error {
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	writer.UseCRLF = false
	if err := writer.Write([]string{"sample", "timestamp", "elapsed_ms", "database_host", "pool", "role", "shard", "metric", "value"}); err != nil {
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
		previous:    make(map[string]previousPool),
		maxPools:    maxPools,
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
		fmt.Fprintf(os.Stderr, "SQL pool metrics poll: %v\n", err)
		return
	}
	if err := c.write(current); err != nil {
		fmt.Fprintf(os.Stderr, "SQL pool metrics output: %v\n", err)
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
	if err := validateSnapshot(current, c.maxPools, int64(len(body))); err != nil {
		return snapshot{}, err
	}
	return current, nil
}

func validateSnapshot(current snapshot, maxPools int, contentLength int64) error {
	if contentLength < 0 {
		return errors.New("negative content length")
	}
	if current.Version != 1 || current.StartedUnixMS <= 0 || current.CapturedUnixMS < current.StartedUnixMS {
		return errors.New("invalid snapshot metadata")
	}
	if len(current.Pools) == 0 || len(current.Pools) > maxPools {
		return fmt.Errorf("invalid pool count %d", len(current.Pools))
	}
	seen := make(map[string]struct{}, len(current.Pools))
	for _, value := range current.Pools {
		if !labelPattern.MatchString(value.Name) || !labelPattern.MatchString(value.Role) || value.Shard < -1 {
			return fmt.Errorf("invalid pool label: %+v", value)
		}
		if !hostPattern.MatchString(value.DatabaseHost) {
			return fmt.Errorf("invalid database host: %+v", value)
		}
		key := fmt.Sprintf("%s/%s/%s/%d", value.DatabaseHost, value.Role, value.Name, value.Shard)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate pool %q", key)
		}
		seen[key] = struct{}{}
		if value.MaxOpenConnections < 0 || value.OpenConnections < 0 || value.InUse < 0 || value.Idle < 0 ||
			value.WaitCount < 0 || value.WaitDurationNS < 0 || value.MaxIdleClosed < 0 ||
			value.MaxIdleTimeClosed < 0 || value.MaxLifetimeClosed < 0 {
			return fmt.Errorf("negative pool metric: %+v", value)
		}
		if value.InUse > value.OpenConnections || value.Idle > value.OpenConnections {
			return fmt.Errorf("pool usage exceeds open connections: %+v", value)
		}
	}
	return nil
}

func (c *collector) write(current snapshot) error {
	currentAt := current.CapturedUnixMS
	timestamp := time.UnixMilli(currentAt).UTC().Format(time.RFC3339Nano)
	elapsedMS := currentAt - current.StartedUnixMS
	for _, value := range current.Pools {
		key := fmt.Sprintf("%s/%s/%s/%d", value.DatabaseHost, value.Role, value.Name, value.Shard)
		previous, hasPrevious := c.previous[key]
		elapsedSeconds := 0.0
		if hasPrevious && currentAt > previous.capturedUnixMS {
			elapsedSeconds = float64(currentAt-previous.capturedUnixMS) / 1000.0
		}
		waitCountRate := 0.0
		waitDurationRate := 0.0
		averageWaitMS := 0.0
		if hasPrevious && elapsedSeconds > 0 && value.WaitCount >= previous.value.WaitCount && value.WaitDurationNS >= previous.value.WaitDurationNS {
			waitCountDelta := value.WaitCount - previous.value.WaitCount
			waitDurationDelta := value.WaitDurationNS - previous.value.WaitDurationNS
			waitCountRate = float64(waitCountDelta) / elapsedSeconds
			waitDurationRate = float64(waitDurationDelta) / 1e6 / elapsedSeconds
			if waitCountDelta > 0 {
				averageWaitMS = float64(waitDurationDelta) / 1e6 / float64(waitCountDelta)
			}
		}
		utilization := 0.0
		if value.MaxOpenConnections > 0 {
			utilization = float64(value.InUse) * 100.0 / float64(value.MaxOpenConnections)
		}
		metrics := []metricValue{
			{"max_open_connections", float64(value.MaxOpenConnections)},
			{"open_connections", float64(value.OpenConnections)},
			{"in_use", float64(value.InUse)},
			{"idle", float64(value.Idle)},
			{"pool_utilization_pct", utilization},
			{"wait_count_total", float64(value.WaitCount)},
			{"wait_duration_ms_total", float64(value.WaitDurationNS) / 1e6},
			{"max_idle_closed_total", float64(value.MaxIdleClosed)},
			{"max_idle_time_closed_total", float64(value.MaxIdleTimeClosed)},
			{"max_lifetime_closed_total", float64(value.MaxLifetimeClosed)},
			{"wait_count_per_sec", waitCountRate},
			{"wait_duration_ms_per_sec", waitDurationRate},
			{"average_wait_ms", averageWaitMS},
		}
		for _, metric := range metrics {
			row := []string{
				strconv.FormatUint(c.sample, 10),
				timestamp,
				strconv.FormatInt(elapsedMS, 10),
				value.DatabaseHost,
				value.Name,
				value.Role,
				strconv.Itoa(value.Shard),
				metric.name,
				strconv.FormatFloat(metric.value, 'f', 6, 64),
			}
			if err := c.output.Write(row); err != nil {
				return err
			}
		}
		c.previous[key] = previousPool{capturedUnixMS: currentAt, value: value}
	}
	c.output.Flush()
	if err := c.output.Error(); err != nil {
		return err
	}
	c.sample++
	return nil
}

type metricValue struct {
	name  string
	value float64
}
