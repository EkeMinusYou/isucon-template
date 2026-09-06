package pprofimport

import (
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	pprofprofile "github.com/google/pprof/profile"
)

var runIDPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}$`)

type metadataRow struct {
	period                                                     int64
	periodUnit                                                 string
	runID, host, source, profileSHA256, sampleType, sampleUnit string
	profileType, valueUnit                                     string
	timeUnixNano                                               int64
	durationSeconds, totalValue, periodSeconds                 float64
	sampleCount, incompleteSamples, embeddedFunctionNames      int
}

type sampleRow struct {
	runID, host          string
	sampleID, stackDepth int
	value                float64
	rawValue             int64
}

type frameRow struct {
	runID, host, function, file string
	sampleID, depth, line       int
}

type functionRow struct {
	runID, host, function, file string
	line                        int
	flatValue, cumulative       float64
}

type edgeRow struct {
	runID, host, caller, callee string
	value                       float64
	sampleOccurrences           int
}

type normalizedProfile struct {
	metadata  metadataRow
	samples   []sampleRow
	frames    []frameRow
	functions []functionRow
	edges     []edgeRow
}

type functionValue struct {
	file             string
	line             int
	flat, cumulative float64
}

type edgeValue struct {
	value             float64
	sampleOccurrences int
}

func selectSampleType(profile *pprofprofile.Profile) (int, *pprofprofile.ValueType, error) {
	for i, sampleType := range profile.SampleType {
		if sampleType != nil && sampleType.Type == "time" {
			return i, sampleType, nil
		}
	}
	if profile.DefaultSampleType != "" {
		for i, sampleType := range profile.SampleType {
			if sampleType != nil && sampleType.Type == profile.DefaultSampleType {
				return i, sampleType, nil
			}
		}
	}
	for i, sampleType := range profile.SampleType {
		if sampleType != nil {
			return i, sampleType, nil
		}
	}
	return -1, nil, fmt.Errorf("profile has no sample type")
}

func valueToSeconds(value int64, unit string) (float64, error) {
	switch strings.ToLower(unit) {
	case "nanoseconds":
		return float64(value) / 1e9, nil
	case "microseconds":
		return float64(value) / 1e6, nil
	case "milliseconds":
		return float64(value) / 1e3, nil
	case "seconds":
		return float64(value), nil
	default:
		return 0, fmt.Errorf("sample unit %q is not a wall-clock unit", unit)
	}
}

func frameName(line pprofprofile.Line) (name, file string, lineNumber int, complete bool) {
	if line.Function == nil || line.Function.Name == "" {
		return "[unknown]", "", int(line.Line), false
	}
	return line.Function.Name, line.Function.Filename, int(line.Line), true
}

func sampleFrames(sample *pprofprofile.Sample) ([]frameRow, bool) {
	frames := make([]frameRow, 0, len(sample.Location))
	complete := true
	for _, location := range sample.Location {
		if location == nil || len(location.Line) == 0 {
			frames = append(frames, frameRow{function: "[unknown]"})
			complete = false
			continue
		}
		for _, line := range location.Line {
			name, file, lineNumber, ok := frameName(line)
			frames = append(frames, frameRow{function: name, file: file, line: lineNumber})
			complete = complete && ok
		}
	}
	if len(frames) == 0 {
		frames = append(frames, frameRow{function: "[unknown]"})
		complete = false
	}
	return frames, complete
}

func profileKind(path string) string {
	for _, kind := range []string{"fgprof", "go-cpu", "go-heap", "go-allocs", "go-goroutine"} {
		if strings.HasSuffix(filepath.Base(path), "-"+kind+".pprof") {
			return kind
		}
	}
	return ""
}

func profileIdentity(path string) (runID, host string, err error) {
	runID = filepath.Base(filepath.Dir(path))
	if !runIDPattern.MatchString(runID) {
		return "", "", fmt.Errorf("profile parent directory %q is not a RUN ID", runID)
	}
	base := filepath.Base(path)
	kind := profileKind(path)
	if kind == "" {
		return "", "", fmt.Errorf("profile %q has an unsupported profile kind", base)
	}
	host = strings.TrimSuffix(base, "-"+kind+".pprof")
	if host == "" {
		return "", "", fmt.Errorf("profile %q has no host", base)
	}
	return runID, host, nil
}

func normalizeProfile(path string) (normalizedProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return normalizedProfile{}, err
	}
	profile, err := pprofprofile.ParseData(data)
	if err != nil {
		return normalizedProfile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	sampleIndex, _, err := selectSampleType(profile)
	if err != nil {
		return normalizedProfile{}, err
	}
	return normalizeMetric(path, data, profile, sampleIndex)
}

// Time values use seconds; bytes and counts keep their original scale.
// Only fgprof time is written to the legacy wall-time tables.
func metricValue(value int64, unit string) (float64, string, error) {
	if unit == "bytes" || unit == "count" {
		return float64(value), unit, nil
	}
	seconds, err := valueToSeconds(value, unit)
	return seconds, "seconds", err
}

func normalizeMetric(path string, data []byte, profile *pprofprofile.Profile, sampleIndex int) (normalizedProfile, error) {
	runID, host, err := profileIdentity(path)
	if err != nil {
		return normalizedProfile{}, err
	}
	if err := profile.CheckValid(); err != nil {
		return normalizedProfile{}, fmt.Errorf("invalid profile %s: %w", path, err)
	}
	sampleType := profile.SampleType[sampleIndex]
	_, valueUnit, err := metricValue(1, sampleType.Unit)
	if err != nil {
		return normalizedProfile{}, err
	}

	result := normalizedProfile{}
	functionValues := make(map[string]*functionValue)
	edgeValues := make(map[string]*edgeValue)
	var total float64
	var incomplete int
	for index, sample := range profile.Sample {
		if sample == nil || sampleIndex >= len(sample.Value) || sample.Value[sampleIndex] == 0 {
			continue
		}
		sampleValue, _, err := metricValue(sample.Value[sampleIndex], sampleType.Unit)
		if err != nil {
			return normalizedProfile{}, err
		}
		frames, complete := sampleFrames(sample)
		if !complete {
			incomplete++
		}
		sampleID := index + 1
		result.samples = append(result.samples, sampleRow{
			runID: runID, host: host, sampleID: sampleID,
			value: sampleValue, stackDepth: len(frames),
			rawValue: sample.Value[sampleIndex],
		})
		total += sampleValue
		seenFunctions := make(map[string]struct{})
		for depth := range frames {
			frame := frames[depth]
			frame.runID, frame.host, frame.sampleID, frame.depth = runID, host, sampleID, depth
			result.frames = append(result.frames, frame)
			value := functionValues[frame.function]
			if value == nil {
				value = &functionValue{file: frame.file, line: frame.line}
				functionValues[frame.function] = value
			}
			if depth == 0 {
				value.flat += sampleValue
			}
			if _, ok := seenFunctions[frame.function]; !ok {
				value.cumulative += sampleValue
				seenFunctions[frame.function] = struct{}{}
			}
			if depth+1 < len(frames) {
				caller, callee := frames[depth+1].function, frame.function
				key := caller + "\x00" + callee
				edge := edgeValues[key]
				if edge == nil {
					edge = &edgeValue{}
					edgeValues[key] = edge
				}
				edge.value += sampleValue
				edge.sampleOccurrences++
			}
		}
	}

	for function, value := range functionValues {
		result.functions = append(result.functions, functionRow{
			runID: runID, host: host, function: function, file: value.file,
			line: value.line, flatValue: value.flat, cumulative: value.cumulative,
		})
	}
	for key, value := range edgeValues {
		parts := strings.SplitN(key, "\x00", 2)
		result.edges = append(result.edges, edgeRow{
			runID: runID, host: host, caller: parts[0], callee: parts[1],
			value: value.value, sampleOccurrences: value.sampleOccurrences,
		})
	}
	sort.Slice(result.functions, func(i, j int) bool {
		if result.functions[i].cumulative != result.functions[j].cumulative {
			return result.functions[i].cumulative > result.functions[j].cumulative
		}
		return result.functions[i].function < result.functions[j].function
	})
	sort.Slice(result.edges, func(i, j int) bool {
		if result.edges[i].value != result.edges[j].value {
			return result.edges[i].value > result.edges[j].value
		}
		if result.edges[i].caller != result.edges[j].caller {
			return result.edges[i].caller < result.edges[j].caller
		}
		return result.edges[i].callee < result.edges[j].callee
	})

	profileHash := sha256.Sum256(data)
	periodSeconds := float64(0)
	if profile.PeriodType != nil && profile.Period > 0 {
		if converted, convertErr := valueToSeconds(profile.Period, profile.PeriodType.Unit); convertErr == nil {
			periodSeconds = converted
		}
	}
	embeddedFunctionNames := 0
	for _, function := range profile.Function {
		if function != nil && function.Name != "" {
			embeddedFunctionNames++
		}
	}
	result.metadata = metadataRow{
		runID: runID, host: host, source: filepath.Base(path),
		profileSHA256: fmt.Sprintf("%x", profileHash),
		sampleType:    sampleType.Type, sampleUnit: sampleType.Unit,
		profileType: profileKind(path), valueUnit: valueUnit, timeUnixNano: profile.TimeNanos,
		durationSeconds: float64(profile.DurationNanos) / 1e9,
		totalValue:      total, periodSeconds: periodSeconds,
		sampleCount: len(result.samples), incompleteSamples: incomplete,
		embeddedFunctionNames: embeddedFunctionNames,
	}
	result.metadata.period = profile.Period
	if profile.PeriodType != nil {
		result.metadata.periodUnit = profile.PeriodType.Unit
	}
	return result, nil
}

func writeTSV(path string, header []string, rows func(*csv.Writer) error) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	writer.Comma = '\t'
	if err = writer.Write(header); err == nil {
		err = rows(writer)
	}
	writer.Flush()
	if err == nil {
		err = writer.Error()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func formatFloat(value float64) string { return strconv.FormatFloat(value, 'g', 17, 64) }

func writeProfiles(output string, profiles []normalizedProfile) error {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	if err := writeTSV(filepath.Join(output, "profile-metadata.rows"), []string{
		"run_id", "host", "source", "profile_sha256", "sample_type", "sample_unit",
		"duration_seconds", "total_wall_seconds", "period_seconds", "sample_count",
		"incomplete_samples", "embedded_function_names", "external_binary_used",
		"binary_path", "binary_sha256", "binary_matches_run",
	}, func(writer *csv.Writer) error {
		for _, profile := range profiles {
			m := profile.metadata
			if err := writer.Write([]string{m.runID, m.host, m.source, m.profileSHA256, m.sampleType, m.sampleUnit,
				formatFloat(m.durationSeconds), formatFloat(m.totalValue), formatFloat(m.periodSeconds),
				strconv.Itoa(m.sampleCount), strconv.Itoa(m.incompleteSamples), strconv.Itoa(m.embeddedFunctionNames),
				"false", "", "", "not-checked"}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := writeTSV(filepath.Join(output, "profile-samples.rows"), []string{"run_id", "host", "sample_id", "wall_seconds", "stack_depth"}, func(writer *csv.Writer) error {
		for _, profile := range profiles {
			for _, row := range profile.samples {
				if err := writer.Write([]string{row.runID, row.host, strconv.Itoa(row.sampleID), formatFloat(row.value), strconv.Itoa(row.stackDepth)}); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := writeTSV(filepath.Join(output, "profile-frames.rows"), []string{"run_id", "host", "sample_id", "depth", "function", "file", "line"}, func(writer *csv.Writer) error {
		for _, profile := range profiles {
			for _, row := range profile.frames {
				if err := writer.Write([]string{row.runID, row.host, strconv.Itoa(row.sampleID), strconv.Itoa(row.depth), row.function, row.file, strconv.Itoa(row.line)}); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := writeTSV(filepath.Join(output, "profile-functions.rows"), []string{"run_id", "host", "function", "file", "line", "flat_wall_seconds", "cumulative_wall_seconds"}, func(writer *csv.Writer) error {
		for _, profile := range profiles {
			for _, row := range profile.functions {
				if err := writer.Write([]string{row.runID, row.host, row.function, row.file, strconv.Itoa(row.line), formatFloat(row.flatValue), formatFloat(row.cumulative)}); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return writeTSV(filepath.Join(output, "profile-edges.rows"), []string{"run_id", "host", "caller", "callee", "wall_seconds", "sample_occurrences"}, func(writer *csv.Writer) error {
		for _, profile := range profiles {
			for _, row := range profile.edges {
				if err := writer.Write([]string{row.runID, row.host, row.caller, row.callee, formatFloat(row.value), strconv.Itoa(row.sampleOccurrences)}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func Run(output string, paths []string, stderr io.Writer) error {
	sort.Strings(paths)
	profiles := make([]normalizedProfile, 0, len(paths))
	var legacy []normalizedProfile
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsed, err := pprofprofile.ParseData(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if err := parsed.CheckValid(); err != nil {
			return fmt.Errorf("invalid profile %s: %w", path, err)
		}
		if len(parsed.SampleType) == 0 {
			return fmt.Errorf("profile %s has no sample types", path)
		}
		seen := make(map[string]bool)
		for index, sampleType := range parsed.SampleType {
			if sampleType == nil || sampleType.Type == "" || seen[sampleType.Type] {
				return fmt.Errorf("profile %s has missing or duplicate sample types", path)
			}
			seen[sampleType.Type] = true
			profile, err := normalizeMetric(path, data, parsed, index)
			if err != nil {
				return fmt.Errorf("normalize %s: %w", path, err)
			}
			profiles = append(profiles, profile)
			if profile.metadata.profileType == "fgprof" && sampleType.Type == "time" {
				legacy = append(legacy, profile)
			}
		}
	}
	if err := writeProfiles(output, legacy); err != nil {
		return err
	}
	if err := writeGenericProfiles(output, profiles); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "normalized %d pprof file(s), %d sample types into %s\n", len(paths), len(profiles), output)
	return nil
}
