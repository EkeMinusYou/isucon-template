// Command user-transition-metrics aggregates consecutive API calls by an
// identity stored in a structured access-log field. Identity values are hashed
// in memory and are never written to the report.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultMaxEvents      = 5_000_000
	defaultMaxScenarios   = 256
	defaultMaxOutputBytes = 64 << 20
	maxRoutes             = 256
	maxLogLineBytes       = 4 << 20
)

type routeConfig struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	re      *regexp.Regexp
}

type config struct {
	CookieField string        `json:"cookie_field"`
	APIPrefix   string        `json:"api_prefix"`
	Routes      []routeConfig `json:"routes"`
}

type accessLog struct {
	Msec         json.RawMessage `json:"msec"`
	Method       string          `json:"method"`
	URI          string          `json:"uri"`
	Status       json.RawMessage `json:"status"`
	ResponseTime float64         `json:"response_time"`
}

type event struct {
	startUS int64
	endUS   int64
	method  string
	route   string
	status  int
	ordinal uint64
}

type edgeKey struct {
	fromMethod string
	fromRoute  string
	toMethod   string
	toRoute    string
}

type edgeAggregate struct {
	count          uint64
	sessions       uint64
	overlapCount   uint64
	ambiguousCount uint64
	fromStatuses   [6]uint64
	toStatuses     [6]uint64
	startGapsMS    []float64
	idleGapSumMS   float64
}

type summary struct {
	InputFiles              int    `json:"input_files"`
	InputLines              uint64 `json:"input_lines"`
	MalformedLines          uint64 `json:"malformed_lines"`
	InvalidTimeLines        uint64 `json:"invalid_time_lines"`
	APIRequests             uint64 `json:"api_requests"`
	ClassifiedRequests      uint64 `json:"classified_requests"`
	UnmatchedAPIRequests    uint64 `json:"unmatched_api_requests"`
	RequestsWithIdentity    uint64 `json:"requests_with_identity"`
	MissingIdentityRequests uint64 `json:"missing_identity_requests"`
	Sessions                uint64 `json:"sessions"`
	Transitions             uint64 `json:"transitions"`
	OverlappingTransitions  uint64 `json:"overlapping_transitions"`
	AmbiguousTransitions    uint64 `json:"ambiguous_order_transitions"`
	WindowStartUnixMS       int64  `json:"window_start_unix_ms"`
	WindowEndUnixMS         int64  `json:"window_end_unix_ms"`
	ScenarioGroups          uint64 `json:"scenario_groups"`
	ScenariosEmitted        uint64 `json:"scenarios_emitted"`
	ScenarioSessionsOmitted uint64 `json:"scenario_sessions_omitted"`
}

type edgeReport struct {
	FromMethod           string  `json:"from_method"`
	FromRoute            string  `json:"from_route"`
	ToMethod             string  `json:"to_method"`
	ToRoute              string  `json:"to_route"`
	Transitions          uint64  `json:"transitions"`
	Sessions             uint64  `json:"sessions"`
	OverlapTransitions   uint64  `json:"overlap_transitions"`
	AmbiguousTransitions uint64  `json:"ambiguous_order_transitions"`
	StartGapAvgMS        float64 `json:"start_gap_avg_ms"`
	StartGapP50MS        float64 `json:"start_gap_p50_ms"`
	StartGapP95MS        float64 `json:"start_gap_p95_ms"`
	IdleGapAvgMS         float64 `json:"idle_gap_avg_ms"`
	FromStatus2xx        uint64  `json:"from_status_2xx"`
	FromStatus3xx        uint64  `json:"from_status_3xx"`
	FromStatus4xx        uint64  `json:"from_status_4xx"`
	FromStatus5xx        uint64  `json:"from_status_5xx"`
	FromStatusOther      uint64  `json:"from_status_other"`
	ToStatus2xx          uint64  `json:"to_status_2xx"`
	ToStatus3xx          uint64  `json:"to_status_3xx"`
	ToStatus4xx          uint64  `json:"to_status_4xx"`
	ToStatus5xx          uint64  `json:"to_status_5xx"`
	ToStatusOther        uint64  `json:"to_status_other"`
}

type scenarioNodeAggregate struct {
	method      string
	route       string
	requests    uint64
	sessions    uint64
	first       uint64
	last        uint64
	positionSum float64
	offsetSumMS float64
}

type scenarioAggregate struct {
	signature   []string
	sessions    uint64
	requests    uint64
	transitions uint64
	overlaps    uint64
	ambiguous   uint64
	durationsMS []float64
	nodes       map[string]*scenarioNodeAggregate
	edges       map[edgeKey]*edgeAggregate
}

// FirstPositionAvg / FirstOffsetAvgMS place the node on the session timeline:
// where its first occurrence sits between session start (0) and end (1), and
// how long after the session start that first occurrence began. Readers use
// them to lay transitions out in observed chronological order.
type scenarioNodeReport struct {
	Method           string  `json:"method"`
	Route            string  `json:"route"`
	Requests         uint64  `json:"requests"`
	Sessions         uint64  `json:"sessions"`
	FirstSessions    uint64  `json:"first_sessions"`
	LastSessions     uint64  `json:"last_sessions"`
	FirstPositionAvg float64 `json:"first_position_avg"`
	FirstOffsetAvgMS float64 `json:"first_offset_avg_ms"`
}

type scenarioReport struct {
	ID                   string               `json:"id"`
	Signature            []string             `json:"signature"`
	Sessions             uint64               `json:"sessions"`
	Requests             uint64               `json:"requests"`
	Transitions          uint64               `json:"transitions"`
	OverlapTransitions   uint64               `json:"overlap_transitions"`
	AmbiguousTransitions uint64               `json:"ambiguous_order_transitions"`
	RequestsPerSession   float64              `json:"requests_per_session_avg"`
	DurationP50MS        float64              `json:"duration_p50_ms"`
	DurationP95MS        float64              `json:"duration_p95_ms"`
	Nodes                []scenarioNodeReport `json:"nodes"`
	Edges                []edgeReport         `json:"edges"`
}

type report struct {
	SchemaVersion    int              `json:"schema_version"`
	IdentityField    string           `json:"identity_field"`
	Ordering         string           `json:"ordering"`
	ScenarioGrouping string           `json:"scenario_grouping"`
	Summary          summary          `json:"summary"`
	Edges            []edgeReport     `json:"edges"`
	Scenarios        []scenarioReport `json:"scenarios"`
}

type aggregator struct {
	config    config
	maxEvents int
	summary   summary
	sessions  map[[sha256.Size]byte][]event
	ordinal   uint64
}

func main() {
	configPath := flag.String("config", "tools/user-transition-metrics/routes.json", "route normalization config")
	outputPath := flag.String("output", "-", "JSON output path, or - for stdout")
	maxEvents := flag.Int("max-events", defaultMaxEvents, "maximum classified events retained in memory")
	maxScenarios := flag.Int("max-scenarios", defaultMaxScenarios, "maximum scenario groups written to the report")
	maxOutputBytes := flag.Int("max-output-bytes", defaultMaxOutputBytes, "maximum report size")
	flag.Parse()

	if flag.NArg() == 0 {
		fatal(errors.New("at least one access log path is required"))
	}
	if *maxEvents <= 0 || *maxScenarios <= 0 || *maxOutputBytes <= 0 {
		fatal(errors.New("limits must be positive"))
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fatal(err)
	}
	agg := newAggregator(cfg, *maxEvents)
	for _, path := range flag.Args() {
		if err := agg.scanPath(path); err != nil {
			fatal(err)
		}
	}
	rep := agg.buildReport(*maxScenarios)
	body, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fatal(fmt.Errorf("encode report: %w", err))
	}
	body = append(body, '\n')
	if len(body) > *maxOutputBytes {
		fatal(fmt.Errorf("report is %d bytes, limit is %d", len(body), *maxOutputBytes))
	}
	if *outputPath == "-" {
		_, err = os.Stdout.Write(body)
	} else {
		err = os.WriteFile(*outputPath, body, 0o644)
	}
	if err != nil {
		fatal(fmt.Errorf("write report: %w", err))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "user-transition-metrics:", err)
	os.Exit(1)
}

func loadConfig(path string) (config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.CookieField == "" || cfg.APIPrefix == "" {
		return config{}, errors.New("cookie_field and api_prefix are required")
	}
	if len(cfg.Routes) == 0 || len(cfg.Routes) > maxRoutes {
		return config{}, fmt.Errorf("routes must contain 1..%d entries", maxRoutes)
	}
	seen := make(map[string]bool, len(cfg.Routes))
	for i := range cfg.Routes {
		route := &cfg.Routes[i]
		if route.Name == "" || route.Pattern == "" {
			return config{}, fmt.Errorf("route %d requires name and pattern", i)
		}
		if seen[route.Name] {
			return config{}, fmt.Errorf("duplicate route name %q", route.Name)
		}
		seen[route.Name] = true
		route.re, err = regexp.Compile(route.Pattern)
		if err != nil {
			return config{}, fmt.Errorf("route %q: %w", route.Name, err)
		}
	}
	return cfg, nil
}

func newAggregator(cfg config, maxEvents int) *aggregator {
	return &aggregator{
		config:    cfg,
		maxEvents: maxEvents,
		sessions:  make(map[[sha256.Size]byte][]event),
	}
}

func (a *aggregator) scanPath(path string) error {
	r, closeReader, err := openAccessLog(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	a.summary.InputFiles++
	scanErr := a.scan(r)
	closeErr := closeReader()
	return errors.Join(scanErr, closeErr)
}

func (a *aggregator) scan(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxLogLineBytes)
	for scanner.Scan() {
		a.summary.InputLines++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var base accessLog
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &base); err != nil || json.Unmarshal(line, &fields) != nil {
			a.summary.MalformedLines++
			continue
		}
		path := normalizePath(base.URI)
		if !strings.HasPrefix(path, a.config.APIPrefix) {
			continue
		}
		a.summary.APIRequests++
		route, ok := a.classify(path)
		if !ok {
			a.summary.UnmatchedAPIRequests++
			continue
		}
		a.summary.ClassifiedRequests++

		identity := rawString(fields[a.config.CookieField])
		if identity == "" || identity == "-" {
			a.summary.MissingIdentityRequests++
			continue
		}
		endUS, ok := rawSecondsUS(base.Msec)
		if !ok || base.ResponseTime < 0 || math.IsNaN(base.ResponseTime) || math.IsInf(base.ResponseTime, 0) {
			a.summary.InvalidTimeLines++
			continue
		}
		if int(a.summary.RequestsWithIdentity) >= a.maxEvents {
			return fmt.Errorf("classified events exceed -max-events=%d", a.maxEvents)
		}
		responseUS := int64(math.Round(base.ResponseTime * 1_000_000))
		startUS := endUS - responseUS
		status := rawInt(base.Status)
		key := sha256.Sum256([]byte(identity))
		a.ordinal++
		a.sessions[key] = append(a.sessions[key], event{
			startUS: startUS,
			endUS:   endUS,
			method:  normalizeMethod(base.Method),
			route:   route,
			status:  status,
			ordinal: a.ordinal,
		})
		a.summary.RequestsWithIdentity++
		if a.summary.WindowStartUnixMS == 0 || startUS/1000 < a.summary.WindowStartUnixMS {
			a.summary.WindowStartUnixMS = startUS / 1000
		}
		if endUS/1000 > a.summary.WindowEndUnixMS {
			a.summary.WindowEndUnixMS = endUS / 1000
		}
	}
	return scanner.Err()
}

func (a *aggregator) classify(path string) (string, bool) {
	for _, route := range a.config.Routes {
		if route.re.MatchString(path) {
			return route.Name, true
		}
	}
	return "", false
}

func (a *aggregator) buildReport(maxScenarios int) report {
	edges := make(map[edgeKey]*edgeAggregate)
	scenarioGroups := make(map[string]*scenarioAggregate)
	a.summary.Sessions = uint64(len(a.sessions))
	for _, events := range a.sessions {
		sortEvents(events)
		overlaps, ambiguous := addSequence(edges, events)
		a.summary.Transitions += uint64(max(0, len(events)-1))
		a.summary.OverlappingTransitions += overlaps
		a.summary.AmbiguousTransitions += ambiguous

		signature := scenarioSignature(events)
		key := strings.Join(signature, "\x00")
		group := scenarioGroups[key]
		if group == nil {
			group = &scenarioAggregate{
				signature: signature,
				nodes:     make(map[string]*scenarioNodeAggregate),
				edges:     make(map[edgeKey]*edgeAggregate),
			}
			scenarioGroups[key] = group
		}
		group.addSession(events)
	}

	scenarios := make([]scenarioReport, 0, len(scenarioGroups))
	for _, group := range scenarioGroups {
		scenarios = append(scenarios, group.report())
	}
	sort.Slice(scenarios, func(i, j int) bool {
		if scenarios[i].Sessions != scenarios[j].Sessions {
			return scenarios[i].Sessions > scenarios[j].Sessions
		}
		if scenarios[i].Requests != scenarios[j].Requests {
			return scenarios[i].Requests > scenarios[j].Requests
		}
		return scenarios[i].ID < scenarios[j].ID
	})
	a.summary.ScenarioGroups = uint64(len(scenarios))
	if len(scenarios) > maxScenarios {
		for _, scenario := range scenarios[maxScenarios:] {
			a.summary.ScenarioSessionsOmitted += scenario.Sessions
		}
		scenarios = scenarios[:maxScenarios]
	}
	a.summary.ScenariosEmitted = uint64(len(scenarios))

	return report{
		SchemaVersion:    3,
		IdentityField:    a.config.CookieField,
		Ordering:         "request_start_time; ties by response_end_time then input order",
		ScenarioGrouping: "exact set of normalized method-route nodes observed in one Cookie session",
		Summary:          a.summary,
		Edges:            edgeReports(edges),
		Scenarios:        scenarios,
	}
}

func sortEvents(events []event) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].startUS != events[j].startUS {
			return events[i].startUS < events[j].startUS
		}
		if events[i].endUS != events[j].endUS {
			return events[i].endUS < events[j].endUS
		}
		return events[i].ordinal < events[j].ordinal
	})
}

func addSequence(edges map[edgeKey]*edgeAggregate, events []event) (uint64, uint64) {
	var overlaps uint64
	var ambiguous uint64
	seenForSession := make(map[edgeKey]bool)
	for i := 1; i < len(events); i++ {
		from, to := events[i-1], events[i]
		key := edgeKey{from.method, from.route, to.method, to.route}
		edge := edges[key]
		if edge == nil {
			edge = &edgeAggregate{}
			edges[key] = edge
		}
		edge.count++
		if !seenForSession[key] {
			edge.sessions++
			seenForSession[key] = true
		}
		edge.startGapsMS = append(edge.startGapsMS, float64(to.startUS-from.startUS)/1000)
		idleUS := to.startUS - from.endUS
		if idleUS < 0 {
			edge.overlapCount++
			overlaps++
			idleUS = 0
		}
		if to.startUS == from.startUS {
			edge.ambiguousCount++
			ambiguous++
		}
		edge.idleGapSumMS += float64(idleUS) / 1000
		edge.fromStatuses[statusBucket(from.status)]++
		edge.toStatuses[statusBucket(to.status)]++
	}
	return overlaps, ambiguous
}

func edgeReports(edges map[edgeKey]*edgeAggregate) []edgeReport {
	out := make([]edgeReport, 0, len(edges))
	for key, edge := range edges {
		sort.Float64s(edge.startGapsMS)
		out = append(out, edgeReport{
			FromMethod:           key.fromMethod,
			FromRoute:            key.fromRoute,
			ToMethod:             key.toMethod,
			ToRoute:              key.toRoute,
			Transitions:          edge.count,
			Sessions:             edge.sessions,
			OverlapTransitions:   edge.overlapCount,
			AmbiguousTransitions: edge.ambiguousCount,
			StartGapAvgMS:        average(edge.startGapsMS),
			StartGapP50MS:        percentile(edge.startGapsMS, 0.50),
			StartGapP95MS:        percentile(edge.startGapsMS, 0.95),
			IdleGapAvgMS:         edge.idleGapSumMS / float64(edge.count),
			FromStatus2xx:        edge.fromStatuses[2],
			FromStatus3xx:        edge.fromStatuses[3],
			FromStatus4xx:        edge.fromStatuses[4],
			FromStatus5xx:        edge.fromStatuses[5],
			FromStatusOther:      edge.fromStatuses[0] + edge.fromStatuses[1],
			ToStatus2xx:          edge.toStatuses[2],
			ToStatus3xx:          edge.toStatuses[3],
			ToStatus4xx:          edge.toStatuses[4],
			ToStatus5xx:          edge.toStatuses[5],
			ToStatusOther:        edge.toStatuses[0] + edge.toStatuses[1],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Transitions != out[j].Transitions {
			return out[i].Transitions > out[j].Transitions
		}
		left := out[i].FromMethod + " " + out[i].FromRoute + "\x00" + out[i].ToMethod + " " + out[i].ToRoute
		right := out[j].FromMethod + " " + out[j].FromRoute + "\x00" + out[j].ToMethod + " " + out[j].ToRoute
		return left < right
	})
	return out
}

func scenarioSignature(events []event) []string {
	seen := make(map[string]bool)
	for _, item := range events {
		seen[item.method+" "+item.route] = true
	}
	out := make([]string, 0, len(seen))
	for node := range seen {
		out = append(out, node)
	}
	sort.Strings(out)
	return out
}

func (s *scenarioAggregate) addSession(events []event) {
	if len(events) == 0 {
		return
	}
	s.sessions++
	s.requests += uint64(len(events))
	s.transitions += uint64(max(0, len(events)-1))
	overlaps, ambiguous := addSequence(s.edges, events)
	s.overlaps += overlaps
	s.ambiguous += ambiguous
	s.durationsMS = append(s.durationsMS, float64(events[len(events)-1].endUS-events[0].startUS)/1000)
	seenNodes := make(map[string]bool)
	for index, item := range events {
		id := item.method + " " + item.route
		node := s.nodes[id]
		if node == nil {
			node = &scenarioNodeAggregate{method: item.method, route: item.route}
			s.nodes[id] = node
		}
		node.requests++
		if !seenNodes[id] {
			node.sessions++
			seenNodes[id] = true
			// Anchor the node on the session timeline at its first occurrence.
			// Single-request sessions put that occurrence at the start.
			if len(events) > 1 {
				node.positionSum += float64(index) / float64(len(events)-1)
			}
			node.offsetSumMS += float64(item.startUS-events[0].startUS) / 1000
		}
		if index == 0 {
			node.first++
		}
		if index == len(events)-1 {
			node.last++
		}
	}
}

func (s *scenarioAggregate) report() scenarioReport {
	sort.Float64s(s.durationsMS)
	nodes := make([]scenarioNodeReport, 0, len(s.nodes))
	for _, node := range s.nodes {
		report := scenarioNodeReport{
			Method:        node.method,
			Route:         node.route,
			Requests:      node.requests,
			Sessions:      node.sessions,
			FirstSessions: node.first,
			LastSessions:  node.last,
		}
		if node.sessions > 0 {
			report.FirstPositionAvg = node.positionSum / float64(node.sessions)
			report.FirstOffsetAvgMS = node.offsetSumMS / float64(node.sessions)
		}
		nodes = append(nodes, report)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Requests != nodes[j].Requests {
			return nodes[i].Requests > nodes[j].Requests
		}
		return nodes[i].Method+" "+nodes[i].Route < nodes[j].Method+" "+nodes[j].Route
	})
	digest := sha256.Sum256([]byte(strings.Join(s.signature, "\n")))
	return scenarioReport{
		ID:                   fmt.Sprintf("scenario-%x", digest[:6]),
		Signature:            s.signature,
		Sessions:             s.sessions,
		Requests:             s.requests,
		Transitions:          s.transitions,
		OverlapTransitions:   s.overlaps,
		AmbiguousTransitions: s.ambiguous,
		RequestsPerSession:   float64(s.requests) / float64(s.sessions),
		DurationP50MS:        percentile(s.durationsMS, 0.50),
		DurationP95MS:        percentile(s.durationsMS, 0.95),
		Nodes:                nodes,
		Edges:                edgeReports(s.edges),
	}
}

func normalizePath(raw string) string {
	if parsed, err := url.ParseRequestURI(raw); err == nil {
		return parsed.Path
	}
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i]
	}
	return raw
}

func normalizeMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return method
	default:
		return "OTHER"
	}
}

func rawString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func rawSecondsUS(raw json.RawMessage) (int64, bool) {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		if value, err := strconv.ParseFloat(number.String(), 64); err == nil && value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value) {
			return int64(math.Round(value * 1_000_000)), true
		}
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if value, err := strconv.ParseFloat(text, 64); err == nil && value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value) {
			return int64(math.Round(value * 1_000_000)), true
		}
	}
	return 0, false
}

func rawInt(raw json.RawMessage) int {
	var value int
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		value, _ = strconv.Atoi(text)
	}
	return value
}

func statusBucket(status int) int {
	bucket := status / 100
	if bucket < 1 || bucket > 5 {
		return 0
	}
	return bucket
}

func average(sorted []float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	var sum float64
	for _, value := range sorted {
		sum += value
	}
	return sum / float64(len(sorted))
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(q*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

type commandReadCloser struct {
	reader io.ReadCloser
	cmd    *exec.Cmd
}

func (r *commandReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }

func (r *commandReadCloser) Close() error {
	readErr := r.reader.Close()
	waitErr := r.cmd.Wait()
	return errors.Join(readErr, waitErr)
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
