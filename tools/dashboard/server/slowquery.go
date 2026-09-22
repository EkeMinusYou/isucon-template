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

const slowQueryTopN = 30

// slowQueryClass is one query group as reported by slp (SQL-parser-based
// abstraction), read from a host-specific runs/<RUN_ID>/*-slp.tsv artifact.
type slowQueryClass struct {
	Host       string `json:"host"`
	Query      string `json:"query"`
	QueryCount int64  `json:"query_count"`

	QueryTimeSum float64 `json:"query_time_sum"`
	QueryTimeAvg float64 `json:"query_time_avg"`
	QueryTimeMax float64 `json:"query_time_max"`
	QueryTimeMin float64 `json:"query_time_min"`
	QueryTimeP95 float64 `json:"query_time_p95"`
	QueryTimePct float64 `json:"query_time_pct"`

	LockTimeSum float64 `json:"lock_time_sum"`
	LockTimeAvg float64 `json:"lock_time_avg"`
	LockTimeMax float64 `json:"lock_time_max"`

	RowsExaminedSum float64 `json:"rows_examined_sum"`
	RowsExaminedAvg float64 `json:"rows_examined_avg"`
	RowsExaminedMax float64 `json:"rows_examined_max"`

	RowsSentSum float64 `json:"rows_sent_sum"`
	RowsSentAvg float64 `json:"rows_sent_avg"`
}

type slowQueryResponse struct {
	RunID             string           `json:"run_id"`
	Available         bool             `json:"available"`
	TotalQueryCount   int64            `json:"total_query_count"`
	UniqueQueryCount  int64            `json:"unique_query_count"`
	TotalQueryTimeSum float64          `json:"total_query_time_sum"`
	TotalLockTimeSum  float64          `json:"total_lock_time_sum"`
	TotalRowsExamined float64          `json:"total_rows_examined_sum"`
	TotalRowsSent     float64          `json:"total_rows_sent_sum"`
	Classes           []slowQueryClass `json:"classes"`
	TotalClasses      int              `json:"total_classes"`
	Truncated         bool             `json:"truncated"`
}

// slpColumnSetters maps slp's `--format tsv` header text (fixed per field,
// independent of the -o order requested in Taskfile's collect-slp) to a
// setter on the row being built. Unknown headers are ignored, so adding or
// removing -o fields in Taskfile.yml doesn't break parsing.
var slpColumnSetters = map[string]func(c *slowQueryClass, v string){
	"Count":             func(c *slowQueryClass, v string) { c.QueryCount = parseSlpInt(v) },
	"Query":             func(c *slowQueryClass, v string) { c.Query = v },
	"Sum(QueryTime)":    func(c *slowQueryClass, v string) { c.QueryTimeSum = parseSlpFloat(v) },
	"Avg(QueryTime)":    func(c *slowQueryClass, v string) { c.QueryTimeAvg = parseSlpFloat(v) },
	"Max(QueryTime)":    func(c *slowQueryClass, v string) { c.QueryTimeMax = parseSlpFloat(v) },
	"Min(QueryTime)":    func(c *slowQueryClass, v string) { c.QueryTimeMin = parseSlpFloat(v) },
	"P95(QueryTime)":    func(c *slowQueryClass, v string) { c.QueryTimeP95 = parseSlpFloat(v) },
	"Sum(LockTime)":     func(c *slowQueryClass, v string) { c.LockTimeSum = parseSlpFloat(v) },
	"Avg(LockTime)":     func(c *slowQueryClass, v string) { c.LockTimeAvg = parseSlpFloat(v) },
	"Max(LockTime)":     func(c *slowQueryClass, v string) { c.LockTimeMax = parseSlpFloat(v) },
	"Sum(RowsExamined)": func(c *slowQueryClass, v string) { c.RowsExaminedSum = parseSlpFloat(v) },
	"Avg(RowsExamined)": func(c *slowQueryClass, v string) { c.RowsExaminedAvg = parseSlpFloat(v) },
	"Max(RowsExamined)": func(c *slowQueryClass, v string) { c.RowsExaminedMax = parseSlpFloat(v) },
	"Sum(RowsSent)":     func(c *slowQueryClass, v string) { c.RowsSentSum = parseSlpFloat(v) },
	"Avg(RowsSent)":     func(c *slowQueryClass, v string) { c.RowsSentAvg = parseSlpFloat(v) },
}

func parseSlpFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func parseSlpInt(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseSlpTSV reads one slp artifact. A header-only file (no data rows) means
// slp skipped or found nothing, not zero load; it is reported as unavailable
// rather than an empty-but-real result.
func parseSlpTSV(path string) (*slowQueryResponse, error) {
	classes, err := readSlpClasses(path)
	if err != nil {
		return nil, err
	}
	if classes == nil {
		return nil, nil
	}
	return summarizeSlowQueries(classes), nil
}

func readSlpClasses(path string) ([]slowQueryClass, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// slp's Query field can be a very large single line (multi-KB seed
	// INSERT/CREATE statements observed up to ~380KB); default 64KB token
	// size is too small.
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	var header []string
	classes := make([]slowQueryClass, 0, 64)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if header == nil {
			header = fields
			continue
		}
		c := slowQueryClass{}
		for i, v := range fields {
			if i >= len(header) {
				break
			}
			if setter, ok := slpColumnSetters[header[i]]; ok {
				setter(&c, v)
			}
		}
		classes = append(classes, c)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return classes, nil
}

func summarizeSlowQueries(classes []slowQueryClass) *slowQueryResponse {
	if len(classes) == 0 {
		return &slowQueryResponse{Available: false, Classes: []slowQueryClass{}}
	}

	var totalQueryTime, totalLockTime, totalRowsExamined, totalRowsSent float64
	var totalQueryCount int64
	for _, c := range classes {
		totalQueryTime += c.QueryTimeSum
		totalLockTime += c.LockTimeSum
		totalRowsExamined += c.RowsExaminedSum
		totalRowsSent += c.RowsSentSum
		totalQueryCount += c.QueryCount
	}
	if totalQueryTime > 0 {
		for i := range classes {
			classes[i].QueryTimePct = classes[i].QueryTimeSum / totalQueryTime * 100
		}
	}

	sort.Slice(classes, func(i, j int) bool {
		if classes[i].QueryTimeSum != classes[j].QueryTimeSum {
			return classes[i].QueryTimeSum > classes[j].QueryTimeSum
		}
		if classes[i].Host != classes[j].Host {
			return classes[i].Host < classes[j].Host
		}
		return classes[i].Query < classes[j].Query
	})

	total := len(classes)
	truncated := total > slowQueryTopN
	if truncated {
		classes = classes[:slowQueryTopN]
	}

	return &slowQueryResponse{
		Available:         true,
		TotalQueryCount:   totalQueryCount,
		UniqueQueryCount:  int64(total),
		TotalQueryTimeSum: totalQueryTime,
		TotalLockTimeSum:  totalLockTime,
		TotalRowsExamined: totalRowsExamined,
		TotalRowsSent:     totalRowsSent,
		Classes:           classes,
		TotalClasses:      total,
		Truncated:         truncated,
	}
}

func slpHost(path, fallback string) string {
	base := filepath.Base(path)
	if base == "slp.tsv" {
		if fallback != "" {
			return fallback
		}
		return "mysql"
	}
	return strings.TrimSuffix(base, "-slp.tsv")
}

func slpPaths(dir string) []string {
	paths := mustGlob(filepath.Join(dir, "*-slp.tsv"))
	legacy := filepath.Join(dir, "slp.tsv")
	if fileExists(legacy) {
		paths = append(paths, legacy)
	}
	sort.Strings(paths)
	return paths
}

func parseSlpTSVFiles(paths []string, fallbackHost string) (*slowQueryResponse, error) {
	classes := make([]slowQueryClass, 0)
	for _, path := range paths {
		rows, err := readSlpClasses(path)
		if err != nil {
			return nil, err
		}
		if rows == nil {
			continue
		}
		host := slpHost(path, fallbackHost)
		for i := range rows {
			rows[i].Host = host
		}
		classes = append(classes, rows...)
	}
	return summarizeSlowQueries(classes), nil
}

func (a *app) handleSlowQuery(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	dir, ok := a.resolveRunDir(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run_id")
		return
	}
	fallbackHost := ""
	if roles := readRunRoles(dir); roles != nil {
		fallbackHost = roles.Mysql
	}
	resp, err := parseSlpTSVFiles(slpPaths(dir), fallbackHost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resp == nil {
		resp = &slowQueryResponse{Available: false, Classes: []slowQueryClass{}}
	}
	resp.RunID = runID
	writeJSON(w, resp)
}
