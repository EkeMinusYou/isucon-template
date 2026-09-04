package main

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// hostPoint mirrors a useful subset of a `<host>-proc-metrics.tsv` row.
// mem_used_pct is derived (mem_used_bytes / mem_total_bytes), the rest are
// passed through as-is.
type hostPoint struct {
	ElapsedMs               int64   `json:"elapsed_ms"`
	CPUBusyPct              float64 `json:"cpu_busy_pct"`
	CPUUserPct              float64 `json:"cpu_user_pct"`
	CPUSystemPct            float64 `json:"cpu_system_pct"`
	CPUIowaitPct            float64 `json:"cpu_iowait_pct"`
	CPUStealPct             float64 `json:"cpu_steal_pct"`
	MemUsedPct              float64 `json:"mem_used_pct"`
	MemAvailableBytes       float64 `json:"mem_available_bytes"`
	BuffersBytes            float64 `json:"buffers_bytes"`
	CachedBytes             float64 `json:"cached_bytes"`
	SwapUsedBytes           float64 `json:"swap_used_bytes"`
	SwapInBytesPerSec       float64 `json:"swap_in_bytes_per_sec"`
	SwapOutBytesPerSec      float64 `json:"swap_out_bytes_per_sec"`
	Load1                   float64 `json:"load1"`
	Load5                   float64 `json:"load5"`
	Load15                  float64 `json:"load15"`
	ProcsRunning            float64 `json:"procs_running"`
	ProcsBlocked            float64 `json:"procs_blocked"`
	ContextSwitchesPerSec   float64 `json:"context_switches_per_sec"`
	InterruptsPerSec        float64 `json:"interrupts_per_sec"`
	DiskReadBytesPerSec     float64 `json:"disk_read_bytes_per_sec"`
	DiskWriteBytesPerSec    float64 `json:"disk_write_bytes_per_sec"`
	DiskIOInProgress        float64 `json:"disk_io_in_progress"`
	DiskIOTimeMillisPerSec  float64 `json:"disk_io_time_millis_per_sec"`
	NetRxBytesPerSec        float64 `json:"net_rx_bytes_per_sec"`
	NetTxBytesPerSec        float64 `json:"net_tx_bytes_per_sec"`
	NetRxPacketsPerSec      float64 `json:"net_rx_packets_per_sec"`
	NetTxPacketsPerSec      float64 `json:"net_tx_packets_per_sec"`
	NetRxErrorsPerSec       float64 `json:"net_rx_errors_per_sec"`
	NetTxErrorsPerSec       float64 `json:"net_tx_errors_per_sec"`
	NetRxDropsPerSec        float64 `json:"net_rx_drops_per_sec"`
	NetTxDropsPerSec        float64 `json:"net_tx_drops_per_sec"`
	CPUPressureSomeAvg10    float64 `json:"cpu_pressure_some_avg10"`
	MemoryPressureSomeAvg10 float64 `json:"memory_pressure_some_avg10"`
	MemoryPressureFullAvg10 float64 `json:"memory_pressure_full_avg10"`
	IOPressureSomeAvg10     float64 `json:"io_pressure_some_avg10"`
	IOPressureFullAvg10     float64 `json:"io_pressure_full_avg10"`
}

// servicePoint mirrors a `<host>-service-metrics.tsv` row for one systemd
// service.
type servicePoint struct {
	ElapsedMs          int64   `json:"elapsed_ms"`
	Available          bool    `json:"available"`
	CPUPct             float64 `json:"cpu_pct"`
	CPUHostPct         float64 `json:"cpu_host_pct"`
	CPUUserPct         float64 `json:"cpu_user_pct"`
	CPUSystemPct       float64 `json:"cpu_system_pct"`
	MemoryCurrentBytes float64 `json:"memory_current_bytes"`
	MemoryPctOfHost    float64 `json:"memory_pct_of_host"`
	MemoryPeakBytes    float64 `json:"memory_peak_bytes"`
	IOReadBytesPerSec  float64 `json:"io_read_bytes_per_sec"`
	IOWriteBytesPerSec float64 `json:"io_write_bytes_per_sec"`
	TasksCurrent       float64 `json:"tasks_current"`
}

type hostMetrics struct {
	Host   string      `json:"host"`
	Series []hostPoint `json:"series"`
}

type serviceMetrics struct {
	Host    string         `json:"host"`
	Service string         `json:"service"`
	Series  []servicePoint `json:"series"`
}

type metricsResponse struct {
	RunID    string           `json:"run_id"`
	Hosts    []hostMetrics    `json:"hosts"`
	Services []serviceMetrics `json:"services"`
}

// readTSV reads a header-delimited TSV file and returns the header and rows.
func readTSV(path string) ([]string, [][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var header []string
	var rows [][]string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if header == nil {
			header = cols
			continue
		}
		rows = append(rows, cols)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return header, rows, nil
}

func columnIndex(header []string, name string) int {
	for i, h := range header {
		if h == name {
			return i
		}
	}
	return -1
}

// colIndexer resolves column names to indices once per file and returns a
// getter that reads a float64 from a row (0 when the column is missing or
// unparsable, e.g. cpu_count on the first idle sample).
type colIndexer struct {
	header []string
	row    []string
}

func (c *colIndexer) float(name string) float64 {
	idx := columnIndex(c.header, name)
	if idx < 0 || idx >= len(c.row) {
		return 0
	}
	v, _ := strconv.ParseFloat(c.row[idx], 64)
	return v
}

func (c *colIndexer) int64(name string) int64 {
	idx := columnIndex(c.header, name)
	if idx < 0 || idx >= len(c.row) {
		return 0
	}
	v, _ := strconv.ParseInt(c.row[idx], 10, 64)
	return v
}

// parseHostSeries reads a `<host>-proc-metrics.tsv` file into a time series
// of host-wide resource points (CPU, memory, load, disk I/O, network I/O,
// PSI pressure).
func parseHostSeries(path string) ([]hostPoint, error) {
	header, rows, err := readTSV(path)
	if err != nil {
		return nil, err
	}
	if columnIndex(header, "elapsed_ms") < 0 {
		return nil, nil
	}

	series := make([]hostPoint, 0, len(rows))
	for _, row := range rows {
		c := &colIndexer{header: header, row: row}
		memTotal := c.float("mem_total_bytes")
		memUsed := c.float("mem_used_bytes")
		memUsedPct := 0.0
		if memTotal > 0 {
			memUsedPct = memUsed / memTotal * 100
		}
		series = append(series, hostPoint{
			ElapsedMs:               c.int64("elapsed_ms"),
			CPUBusyPct:              c.float("cpu_busy_pct"),
			CPUUserPct:              c.float("cpu_user_pct"),
			CPUSystemPct:            c.float("cpu_system_pct"),
			CPUIowaitPct:            c.float("cpu_iowait_pct"),
			CPUStealPct:             c.float("cpu_steal_pct"),
			MemUsedPct:              memUsedPct,
			MemAvailableBytes:       c.float("mem_available_bytes"),
			BuffersBytes:            c.float("buffers_bytes"),
			CachedBytes:             c.float("cached_bytes"),
			SwapUsedBytes:           c.float("swap_used_bytes"),
			SwapInBytesPerSec:       c.float("swap_in_bytes_per_sec"),
			SwapOutBytesPerSec:      c.float("swap_out_bytes_per_sec"),
			Load1:                   c.float("load1"),
			Load5:                   c.float("load5"),
			Load15:                  c.float("load15"),
			ProcsRunning:            c.float("procs_running"),
			ProcsBlocked:            c.float("procs_blocked"),
			ContextSwitchesPerSec:   c.float("context_switches_per_sec"),
			InterruptsPerSec:        c.float("interrupts_per_sec"),
			DiskReadBytesPerSec:     c.float("disk_read_bytes_per_sec"),
			DiskWriteBytesPerSec:    c.float("disk_write_bytes_per_sec"),
			DiskIOInProgress:        c.float("disk_io_in_progress"),
			DiskIOTimeMillisPerSec:  c.float("disk_io_time_millis_per_sec"),
			NetRxBytesPerSec:        c.float("net_rx_bytes_per_sec"),
			NetTxBytesPerSec:        c.float("net_tx_bytes_per_sec"),
			NetRxPacketsPerSec:      c.float("net_rx_packets_per_sec"),
			NetTxPacketsPerSec:      c.float("net_tx_packets_per_sec"),
			NetRxErrorsPerSec:       c.float("net_rx_errors_per_sec"),
			NetTxErrorsPerSec:       c.float("net_tx_errors_per_sec"),
			NetRxDropsPerSec:        c.float("net_rx_drops_per_sec"),
			NetTxDropsPerSec:        c.float("net_tx_drops_per_sec"),
			CPUPressureSomeAvg10:    c.float("cpu_pressure_some_avg10"),
			MemoryPressureSomeAvg10: c.float("memory_pressure_some_avg10"),
			MemoryPressureFullAvg10: c.float("memory_pressure_full_avg10"),
			IOPressureSomeAvg10:     c.float("io_pressure_some_avg10"),
			IOPressureFullAvg10:     c.float("io_pressure_full_avg10"),
		})
	}
	return series, nil
}

// parseServiceSeries reads a `<host>-service-metrics.tsv` file into
// per-service time series, grouped by systemd service name.
func parseServiceSeries(path string) (map[string][]servicePoint, error) {
	header, rows, err := readTSV(path)
	if err != nil {
		return nil, err
	}
	serviceIdx := columnIndex(header, "service")
	if columnIndex(header, "elapsed_ms") < 0 || serviceIdx < 0 {
		return nil, nil
	}

	byService := make(map[string][]servicePoint)
	for _, row := range rows {
		if serviceIdx >= len(row) {
			continue
		}
		c := &colIndexer{header: header, row: row}
		service := row[serviceIdx]
		byService[service] = append(byService[service], servicePoint{
			ElapsedMs:          c.int64("elapsed_ms"),
			Available:          c.int64("available") != 0,
			CPUPct:             c.float("cpu_pct"),
			CPUHostPct:         c.float("cpu_host_pct"),
			CPUUserPct:         c.float("cpu_user_pct"),
			CPUSystemPct:       c.float("cpu_system_pct"),
			MemoryCurrentBytes: c.float("memory_current_bytes"),
			MemoryPctOfHost:    c.float("memory_pct_of_host"),
			MemoryPeakBytes:    c.float("memory_peak_bytes"),
			IOReadBytesPerSec:  c.float("io_read_bytes_per_sec"),
			IOWriteBytesPerSec: c.float("io_write_bytes_per_sec"),
			TasksCurrent:       c.float("tasks_current"),
		})
	}
	return byService, nil
}

func (a *app) handleMetrics(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}

	resp := metricsResponse{RunID: runID, Hosts: []hostMetrics{}, Services: []serviceMetrics{}}

	procFiles := mustGlob(filepath.Join(dir, "*-proc-metrics.tsv"))
	for _, f := range procFiles {
		host := strings.TrimSuffix(filepath.Base(f), "-proc-metrics.tsv")
		series, err := parseHostSeries(f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp.Hosts = append(resp.Hosts, hostMetrics{Host: host, Series: series})
	}
	sort.Slice(resp.Hosts, func(i, j int) bool { return resp.Hosts[i].Host < resp.Hosts[j].Host })

	serviceFiles := mustGlob(filepath.Join(dir, "*-service-metrics.tsv"))
	for _, f := range serviceFiles {
		host := strings.TrimSuffix(filepath.Base(f), "-service-metrics.tsv")
		byService, err := parseServiceSeries(f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for service, series := range byService {
			resp.Services = append(resp.Services, serviceMetrics{Host: host, Service: service, Series: series})
		}
	}
	sort.Slice(resp.Services, func(i, j int) bool {
		if resp.Services[i].Host != resp.Services[j].Host {
			return resp.Services[i].Host < resp.Services[j].Host
		}
		return resp.Services[i].Service < resp.Services[j].Service
	})

	writeJSON(w, resp)
}
