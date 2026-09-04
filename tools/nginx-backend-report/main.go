package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type accessLog struct {
	Status         json.RawMessage `json:"status"`
	ResponseTime   float64         `json:"response_time"`
	UpstreamTime   string          `json:"upstream_time"`
	UpstreamAddr   string          `json:"upstream_addr"`
	UpstreamStatus string          `json:"upstream_status"`
	CacheStatus    string          `json:"cache_status"`
}

type groupKey struct {
	ingressHost    string
	upstreamAddr   string
	upstreamStatus string
	cacheStatus    string
}

type aggregate struct {
	requests        uint64
	status2xx       uint64
	status3xx       uint64
	status4xx       uint64
	status5xx       uint64
	statusOther     uint64
	responseTimeSum float64
	upstreamTimeSum float64
}

func main() {
	outputPath := flag.String("output", "-", "TSV output path, or - for stdout")
	groupByIngress := flag.Bool("group-by-ingress", false, "include an ingress_host group derived from each access-log filename")
	flag.Parse()
	paths := flag.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "at least one access log path is required")
		os.Exit(2)
	}

	groups := make(map[groupKey]*aggregate)
	malformed := 0
	for _, path := range paths {
		reader, closeReader, err := openAccessLog(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open access log %s: %v\n", path, err)
			os.Exit(1)
		}
		ingressHost := ""
		if *groupByIngress {
			ingressHost, err = ingressHostFromPath(path)
			if err != nil {
				_ = closeReader()
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		count, err := scanWithIngress(reader, ingressHost, groups)
		closeErr := closeReader()
		if err != nil {
			fmt.Fprintf(os.Stderr, "read access log %s: %v\n", path, err)
			os.Exit(1)
		}
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close access log %s: %v\n", path, closeErr)
			os.Exit(1)
		}
		malformed += count
	}
	if malformed > 0 {
		fmt.Fprintf(os.Stderr, "ignored malformed access log lines: %d\n", malformed)
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
	if err := writeReportWithIngress(output, groups, *groupByIngress); err != nil {
		fmt.Fprintf(os.Stderr, "write upstream report: %v\n", err)
		os.Exit(1)
	}
}

func scan(reader io.Reader, groups map[groupKey]*aggregate) (int, error) {
	return scanWithIngress(reader, "", groups)
}

func scanWithIngress(reader io.Reader, ingressHost string, groups map[groupKey]*aggregate) (int, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	malformed := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record accessLog
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			malformed++
			continue
		}
		key := groupKey{
			ingressHost:    ingressHost,
			upstreamAddr:   normalizeField(record.UpstreamAddr, "NONE"),
			upstreamStatus: normalizeField(record.UpstreamStatus, "NONE"),
			cacheStatus:    normalizeField(record.CacheStatus, "NONE"),
		}
		value := groups[key]
		if value == nil {
			value = &aggregate{}
			groups[key] = value
		}
		value.requests++
		value.responseTimeSum += record.ResponseTime
		value.upstreamTimeSum += parseDurationSum(record.UpstreamTime)
		switch parseStatus(record.Status) / 100 {
		case 2:
			value.status2xx++
		case 3:
			value.status3xx++
		case 4:
			value.status4xx++
		case 5:
			value.status5xx++
		default:
			value.statusOther++
		}
	}
	return malformed, scanner.Err()
}

type commandReadCloser struct {
	reader io.ReadCloser
	cmd    *exec.Cmd
}

func (r *commandReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }

func (r *commandReadCloser) Close() error {
	if err := r.reader.Close(); err != nil {
		return err
	}
	return r.cmd.Wait()
}

func openAccessLog(path string) (io.Reader, func() error, error) {
	if path == "-" {
		return os.Stdin, func() error { return nil }, nil
	}
	if strings.HasSuffix(path, ".zst") {
		cmd := exec.Command("zstdcat", path)
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		reader := &commandReadCloser{reader: pipe, cmd: cmd}
		return reader, reader.Close, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

func ingressHostFromPath(path string) (string, error) {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".zst")
	if !strings.HasPrefix(base, "access-") || !strings.HasSuffix(base, ".log") {
		return "", fmt.Errorf("cannot derive ingress host from access log path %q", path)
	}
	host := strings.TrimSuffix(strings.TrimPrefix(base, "access-"), ".log")
	if host == "" {
		return "", fmt.Errorf("cannot derive ingress host from access log path %q", path)
	}
	return host, nil
}

func parseStatus(raw json.RawMessage) int {
	var numeric int
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return numeric
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		value, err := strconv.Atoi(text)
		if err == nil {
			return value
		}
	}
	return 0
}

func normalizeField(value, empty string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return empty
	}
	return value
}

func parseDurationSum(value string) float64 {
	var total float64
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "-" {
			continue
		}
		seconds, err := strconv.ParseFloat(part, 64)
		if err == nil && seconds >= 0 {
			total += seconds
		}
	}
	return total
}

func writeReport(output io.Writer, groups map[groupKey]*aggregate) error {
	return writeReportWithIngress(output, groups, false)
}

func writeReportWithIngress(output io.Writer, groups map[groupKey]*aggregate, includeIngress bool) error {
	keys := make([]groupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ingressHost != keys[j].ingressHost {
			return keys[i].ingressHost < keys[j].ingressHost
		}
		if keys[i].upstreamAddr != keys[j].upstreamAddr {
			return keys[i].upstreamAddr < keys[j].upstreamAddr
		}
		if keys[i].upstreamStatus != keys[j].upstreamStatus {
			return keys[i].upstreamStatus < keys[j].upstreamStatus
		}
		return keys[i].cacheStatus < keys[j].cacheStatus
	})

	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	writer.UseCRLF = false
	header := []string{
		"upstream_addr",
		"upstream_status",
		"cache_status",
		"requests",
		"status_2xx",
		"status_3xx",
		"status_4xx",
		"status_5xx",
		"status_other",
		"response_time_sum_ms",
		"response_time_avg_ms",
		"upstream_time_sum_ms",
		"upstream_time_avg_ms",
	}
	if includeIngress {
		header = append([]string{"ingress_host"}, header...)
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, key := range keys {
		value := groups[key]
		responseSumMs := value.responseTimeSum * 1_000
		upstreamSumMs := value.upstreamTimeSum * 1_000
		responseAvgMs := 0.0
		upstreamAvgMs := 0.0
		if value.requests > 0 {
			responseAvgMs = responseSumMs / float64(value.requests)
			upstreamAvgMs = upstreamSumMs / float64(value.requests)
		}
		row := []string{
			key.upstreamAddr,
			key.upstreamStatus,
			key.cacheStatus,
			strconv.FormatUint(value.requests, 10),
			strconv.FormatUint(value.status2xx, 10),
			strconv.FormatUint(value.status3xx, 10),
			strconv.FormatUint(value.status4xx, 10),
			strconv.FormatUint(value.status5xx, 10),
			strconv.FormatUint(value.statusOther, 10),
			formatFloat(responseSumMs),
			formatFloat(responseAvgMs),
			formatFloat(upstreamSumMs),
			formatFloat(upstreamAvgMs),
		}
		if includeIngress {
			row = append([]string{key.ingressHost}, row...)
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
