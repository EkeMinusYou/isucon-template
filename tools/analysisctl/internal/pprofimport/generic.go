package pprofimport

import (
	"encoding/csv"
	"path/filepath"
	"strconv"
)

// Every table includes source and sample_type so different captures and metrics
// from the same host cannot be joined accidentally by sample_id alone.
func writeGenericProfiles(output string, profiles []normalizedProfile) error {
	keyHeader := []string{"run_id", "host", "source", "profile_type", "sample_type", "sample_unit", "value_unit"}
	key := func(m metadataRow) []string {
		return []string{m.runID, m.host, m.source, m.profileType, m.sampleType, m.sampleUnit, m.valueUnit}
	}
	type table struct {
		name   string
		header []string
		rows   func(normalizedProfile) [][]string
	}
	tables := []table{
		{
			name: "metadata",
			header: []string{"profile_sha256", "time_unix_nano", "duration_seconds", "total_value", "period_seconds",
				"sample_count", "incomplete_samples", "embedded_function_names", "period", "period_unit"},
			rows: func(p normalizedProfile) [][]string {
				m := p.metadata
				return [][]string{{m.profileSHA256, strconv.FormatInt(m.timeUnixNano, 10), formatFloat(m.durationSeconds),
					formatFloat(m.totalValue), formatFloat(m.periodSeconds), strconv.Itoa(m.sampleCount),
					strconv.Itoa(m.incompleteSamples), strconv.Itoa(m.embeddedFunctionNames), strconv.FormatInt(m.period, 10), m.periodUnit}}
			},
		},
		{
			name: "samples", header: []string{"sample_id", "value", "raw_value", "stack_depth"},
			rows: func(p normalizedProfile) [][]string {
				var rows [][]string
				for _, r := range p.samples {
					rows = append(rows, []string{strconv.Itoa(r.sampleID), formatFloat(r.value), strconv.FormatInt(r.rawValue, 10), strconv.Itoa(r.stackDepth)})
				}
				return rows
			},
		},
		{
			name: "frames", header: []string{"sample_id", "depth", "function", "file", "line"},
			rows: func(p normalizedProfile) [][]string {
				var rows [][]string
				for _, r := range p.frames {
					rows = append(rows, []string{strconv.Itoa(r.sampleID), strconv.Itoa(r.depth), r.function, r.file, strconv.Itoa(r.line)})
				}
				return rows
			},
		},
		{
			name: "functions", header: []string{"function", "file", "line", "flat_value", "cumulative_value"},
			rows: func(p normalizedProfile) [][]string {
				var rows [][]string
				for _, r := range p.functions {
					rows = append(rows, []string{r.function, r.file, strconv.Itoa(r.line), formatFloat(r.flatValue), formatFloat(r.cumulative)})
				}
				return rows
			},
		},
		{
			name: "edges", header: []string{"caller", "callee", "value", "sample_occurrences"},
			rows: func(p normalizedProfile) [][]string {
				var rows [][]string
				for _, r := range p.edges {
					rows = append(rows, []string{r.caller, r.callee, formatFloat(r.value), strconv.Itoa(r.sampleOccurrences)})
				}
				return rows
			},
		},
	}
	for _, table := range tables {
		header := append(append([]string{}, keyHeader...), table.header...)
		err := writeTSV(filepath.Join(output, "pprof-"+table.name+".rows"), header, func(w *csv.Writer) error {
			for _, p := range profiles {
				for _, row := range table.rows(p) {
					if err := w.Write(append(key(p.metadata), row...)); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
