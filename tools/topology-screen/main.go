// topology-screen replays saved access logs through alternative shard
// assignments. Everything problem-specific -- where the shard key lives, how it
// is hashed, which routes are in scope, and what the log columns are called --
// is a flag. Defaults accept the complete vhost as a generic shard key; use
// -key-regex when a tenant or user identifier is embedded in that value.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type aggregate struct {
	Requests     int64
	ResponseTime float64
	UpstreamTime float64
	BodyBytes    int64
}

type aggregateKey struct {
	Modulus     uint32
	Method      string
	Route       string
	StatusClass string
	Bucket      uint32
}

// fieldNames maps the tool's model of a request onto the log's column names.
// Metric names may be empty, which drops that column from the output instead of
// failing the run; identity names may not.
type fieldNames struct {
	Key          string
	Method       string
	URI          string
	Status       string
	ResponseTime string
	UpstreamTime string
	BodyBytes    string
}

type rewriteRule struct {
	Pattern     *regexp.Regexp
	Replacement string
}

type screener struct {
	fields       fieldNames
	keyRE        *regexp.Regexp
	keyExcludeRE *regexp.Regexp
	keyLower     bool
	keyStripPort bool
	routeRE      *regexp.Regexp
	rules        []rewriteRule
	moduli       []uint32
	hash         func(string) uint32
	shard        map[uint32]bool
}

type counters struct {
	Files        int
	Lines        int64
	Matched      int64
	SkippedRoute int64
	SkippedKey   int64
	Keys         map[string]struct{}
}

type result struct {
	Aggregates map[aggregateKey]aggregate
	Totals     map[uint32]aggregate
	// BucketKeys counts distinct shard keys per placement bucket, which is what
	// separates "bucket 0 carries 20% of the load across 200 keys" from "bucket
	// 0 carries 20% because one key is a whale".
	BucketKeys map[aggregateKey]map[string]struct{}
}

type lineParser func([]byte) (map[string]string, error)

const (
	allLabel  = "ALL"
	allStatus = "all"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "topology-screen:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer, diagnostics io.Writer) error {
	fs := flag.NewFlagSet("topology-screen", flag.ContinueOnError)
	runDir := fs.String("run-dir", "", "RUN directory searched with -input-glob")
	inputGlob := fs.String("input-glob", "raw/access-*.log.zst", "glob applied inside -run-dir")
	inputs := fs.String("input", "", "comma-separated log paths or globs; overrides -run-dir")
	format := fs.String("format", "json", "access log format: json or ltsv")

	keyField := fs.String("key-field", "vhost", "log field holding the shard key")
	keyPattern := fs.String("key-regex", `^(.+)$`, "regexp with one capture group extracting the shard key; non-matching records are skipped")
	keyExclude := fs.String("key-exclude", ``, "regexp of extracted keys to drop; empty keeps every key")
	keyLower := fs.Bool("key-lower", true, "lowercase the key field before matching and hashing")
	keyStripPort := fs.Bool("key-strip-port", true, "strip a trailing :port from the key field")

	methodField := fs.String("field-method", "method", "log field holding the HTTP method")
	uriField := fs.String("field-uri", "uri", "log field holding the request URI")
	statusField := fs.String("field-status", "status", "log field holding the HTTP status")
	responseField := fs.String("field-response-time", "response_time", "log field holding total request seconds; empty disables the column")
	upstreamField := fs.String("field-upstream-time", "upstream_time", "log field holding upstream seconds; empty disables the column")
	bytesField := fs.String("field-body-bytes", "body_bytes", "log field holding response body bytes; empty disables the column")

	moduliRaw := fs.String("moduli", "2,3,4,5,10", "comma-separated shard counts to screen")
	hashName := fs.String("hash", "fnv1a", "shard hash: fnv1a, fnv1, crc32, or sha256")
	shardRaw := fs.String("shard-buckets", "0", "comma-separated buckets counted as the proposed shard")
	routePattern := fs.String("route-regex", `^/api/`, "request URI regexp included in screening")
	var normalizers ruleFlag
	fs.Var(&normalizers, "normalize", "route rewrite as '<regexp>=><replacement>'; repeatable, replaces the defaults")
	quiet := fs.Bool("quiet", false, "suppress the stderr coverage summary")
	if err := fs.Parse(args); err != nil {
		return err
	}

	paths, err := resolveInputs(*runDir, *inputGlob, *inputs)
	if err != nil {
		return err
	}
	parse, err := lineParserFor(*format)
	if err != nil {
		return err
	}
	instance, err := newScreener(screenerOptions{
		fields: fieldNames{
			Key:          *keyField,
			Method:       *methodField,
			URI:          *uriField,
			Status:       *statusField,
			ResponseTime: *responseField,
			UpstreamTime: *upstreamField,
			BodyBytes:    *bytesField,
		},
		keyPattern:   *keyPattern,
		keyExclude:   *keyExclude,
		keyLower:     *keyLower,
		keyStripPort: *keyStripPort,
		routePattern: *routePattern,
		rules:        []string(normalizers),
		moduli:       *moduliRaw,
		hashName:     *hashName,
		shard:        *shardRaw,
	})
	if err != nil {
		return err
	}

	screened := newResult()
	stats := &counters{Keys: map[string]struct{}{}}
	for _, path := range paths {
		if err := instance.scanFile(path, parse, screened, stats); err != nil {
			return err
		}
		stats.Files++
	}
	if stats.Matched == 0 {
		return fmt.Errorf("no records matched -route-regex and -key-regex across %d file(s); check -format and the -field-* names", len(paths))
	}
	if !*quiet {
		instance.writeSummary(diagnostics, stats, *hashName)
	}
	return instance.writeTSV(output, screened)
}

// ruleFlag collects repeated -normalize values.
type ruleFlag []string

func (r *ruleFlag) String() string { return strings.Join(*r, " ") }

func (r *ruleFlag) Set(value string) error {
	*r = append(*r, value)
	return nil
}

func resolveInputs(runDir, inputGlob, inputs string) ([]string, error) {
	var patterns []string
	switch {
	case strings.TrimSpace(inputs) != "":
		for _, value := range strings.Split(inputs, ",") {
			if value = strings.TrimSpace(value); value != "" {
				patterns = append(patterns, value)
			}
		}
	case strings.TrimSpace(runDir) != "":
		patterns = append(patterns, filepath.Join(runDir, inputGlob))
	default:
		return nil, errors.New("-run-dir or -input is required")
	}

	seen := map[string]bool{}
	var paths []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid input pattern %q: %w", pattern, err)
		}
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				paths = append(paths, match)
			}
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no access logs matched %s", strings.Join(patterns, ", "))
	}
	sort.Strings(paths)
	return paths, nil
}

func lineParserFor(format string) (lineParser, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return parseJSONLine, nil
	case "ltsv":
		return parseLTSVLine, nil
	default:
		return nil, fmt.Errorf("unsupported -format %q; use json or ltsv", format)
	}
}

func parseJSONLine(line []byte) (map[string]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	record := make(map[string]string, len(raw))
	for name, value := range raw {
		switch typed := value.(type) {
		case nil:
			record[name] = ""
		case string:
			record[name] = typed
		case json.Number:
			record[name] = typed.String()
		case bool:
			record[name] = strconv.FormatBool(typed)
		default:
			// Nested objects and arrays cannot be a scalar field.
			continue
		}
	}
	return record, nil
}

func parseLTSVLine(line []byte) (map[string]string, error) {
	columns := strings.Split(string(line), "\t")
	record := make(map[string]string, len(columns))
	for _, column := range columns {
		if column == "" {
			continue
		}
		name, value, ok := strings.Cut(column, ":")
		if !ok {
			return nil, fmt.Errorf("LTSV column %q has no label separator", column)
		}
		record[name] = value
	}
	if len(record) == 0 {
		return nil, errors.New("no labeled columns")
	}
	return record, nil
}

type screenerOptions struct {
	fields       fieldNames
	keyPattern   string
	keyExclude   string
	keyLower     bool
	keyStripPort bool
	routePattern string
	rules        []string
	moduli       string
	hashName     string
	shard        string
}

func newScreener(options screenerOptions) (*screener, error) {
	for name, value := range map[string]string{
		"-key-field":    options.fields.Key,
		"-field-method": options.fields.Method,
		"-field-uri":    options.fields.URI,
		"-field-status": options.fields.Status,
	} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s must name a log field", name)
		}
	}
	keyRE, err := regexp.Compile(options.keyPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid -key-regex: %w", err)
	}
	if keyRE.NumSubexp() != 1 {
		return nil, fmt.Errorf("-key-regex must have exactly one capture group, got %d", keyRE.NumSubexp())
	}
	var keyExcludeRE *regexp.Regexp
	if strings.TrimSpace(options.keyExclude) != "" {
		if keyExcludeRE, err = regexp.Compile(options.keyExclude); err != nil {
			return nil, fmt.Errorf("invalid -key-exclude: %w", err)
		}
	}
	routeRE, err := regexp.Compile(options.routePattern)
	if err != nil {
		return nil, fmt.Errorf("invalid -route-regex: %w", err)
	}
	rules, err := parseRules(options.rules)
	if err != nil {
		return nil, err
	}
	moduli, err := parseModuli(options.moduli)
	if err != nil {
		return nil, err
	}
	hash, err := hasherFor(options.hashName)
	if err != nil {
		return nil, err
	}
	shard, err := parseBuckets(options.shard, moduli)
	if err != nil {
		return nil, err
	}
	return &screener{
		fields:       options.fields,
		keyRE:        keyRE,
		keyExcludeRE: keyExcludeRE,
		keyLower:     options.keyLower,
		keyStripPort: options.keyStripPort,
		routeRE:      routeRE,
		rules:        rules,
		moduli:       moduli,
		hash:         hash,
		shard:        shard,
	}, nil
}

// defaultRules collapse the identifier shapes that show up in ISUCON-style
// APIs. -normalize replaces the whole set.
var defaultRules = []rewriteRule{
	{regexp.MustCompile(`/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(/|$)`), `/:uuid$1`},
	{regexp.MustCompile(`/[0-9a-fA-F]{32}(/|$)`), `/:hash$1`},
	{regexp.MustCompile(`/[0-9]+(/|$)`), `/:id$1`},
}

func parseRules(raw []string) ([]rewriteRule, error) {
	if len(raw) == 0 {
		return defaultRules, nil
	}
	var rules []rewriteRule
	for _, value := range raw {
		pattern, replacement, ok := strings.Cut(value, "=>")
		if !ok {
			return nil, fmt.Errorf("invalid -normalize %q; expected '<regexp>=><replacement>'", value)
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid -normalize pattern %q: %w", pattern, err)
		}
		rules = append(rules, rewriteRule{Pattern: compiled, Replacement: replacement})
	}
	return rules, nil
}

func parseModuli(raw string) ([]uint32, error) {
	seen := map[uint32]bool{}
	var moduli []uint32
	for _, value := range strings.Split(raw, ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
		if err != nil || n < 2 {
			return nil, fmt.Errorf("invalid modulus %q; each value must be at least 2", value)
		}
		if !seen[uint32(n)] {
			seen[uint32(n)] = true
			moduli = append(moduli, uint32(n))
		}
	}
	if len(moduli) == 0 {
		return nil, errors.New("at least one modulus is required")
	}
	sort.Slice(moduli, func(i, j int) bool { return moduli[i] < moduli[j] })
	return moduli, nil
}

func parseBuckets(raw string, moduli []uint32) (map[uint32]bool, error) {
	buckets := map[uint32]bool{}
	for _, value := range strings.Split(raw, ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid -shard-buckets entry %q", value)
		}
		buckets[uint32(n)] = true
	}
	if len(buckets) == 0 {
		return nil, errors.New("-shard-buckets requires at least one bucket")
	}
	for _, modulus := range moduli {
		if uint32(len(buckets)) >= modulus {
			return nil, fmt.Errorf("-shard-buckets holds %d bucket(s), which leaves no primary placement for modulus %d", len(buckets), modulus)
		}
		for bucket := range buckets {
			if bucket >= modulus {
				return nil, fmt.Errorf("shard bucket %d does not exist for modulus %d", bucket, modulus)
			}
		}
	}
	return buckets, nil
}

func hasherFor(name string) (func(string) uint32, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "fnv1a":
		return func(key string) uint32 {
			hash := fnv.New32a()
			_, _ = hash.Write([]byte(key))
			return hash.Sum32()
		}, nil
	case "fnv1":
		return func(key string) uint32 {
			hash := fnv.New32()
			_, _ = hash.Write([]byte(key))
			return hash.Sum32()
		}, nil
	case "crc32":
		return func(key string) uint32 { return crc32.ChecksumIEEE([]byte(key)) }, nil
	case "sha256":
		return func(key string) uint32 {
			sum := sha256.Sum256([]byte(key))
			return binary.BigEndian.Uint32(sum[:4])
		}, nil
	default:
		return nil, fmt.Errorf("unsupported -hash %q; use fnv1a, fnv1, crc32, or sha256", name)
	}
}

func newResult() *result {
	return &result{
		Aggregates: map[aggregateKey]aggregate{},
		Totals:     map[uint32]aggregate{},
		BucketKeys: map[aggregateKey]map[string]struct{}{},
	}
}

func (s *screener) scanFile(path string, parse lineParser, screened *result, stats *counters) error {
	reader, err := openLog(path)
	if err != nil {
		return err
	}
	if scanErr := s.scan(reader, parse, screened, stats); scanErr != nil {
		_ = reader.Close()
		return fmt.Errorf("%s: %w", path, scanErr)
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func (s *screener) scan(reader io.Reader, parse lineParser, screened *result, stats *counters) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		stats.Lines++
		record, err := parse(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", stats.Lines, err)
		}
		if err := s.observe(record, screened, stats); err != nil {
			return fmt.Errorf("line %d: %w", stats.Lines, err)
		}
	}
	return scanner.Err()
}

func (s *screener) observe(record map[string]string, screened *result, stats *counters) error {
	uri, err := requiredField(record, s.fields.URI)
	if err != nil {
		return err
	}
	if !s.routeRE.MatchString(uri) {
		stats.SkippedRoute++
		return nil
	}
	rawKey, err := requiredField(record, s.fields.Key)
	if err != nil {
		return err
	}
	key, ok := s.shardKey(rawKey)
	if !ok {
		stats.SkippedKey++
		return nil
	}
	rawMethod, err := requiredField(record, s.fields.Method)
	if err != nil {
		return err
	}
	method := strings.ToUpper(strings.TrimSpace(rawMethod))
	if method == "" {
		return fmt.Errorf("record for %s has an empty method", uri)
	}
	rawStatus, err := requiredField(record, s.fields.Status)
	if err != nil {
		return err
	}
	class, err := statusClass(rawStatus)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, uri, err)
	}
	value := aggregate{Requests: 1}
	if value.ResponseTime, err = optionalDuration(record, s.fields.ResponseTime); err != nil {
		return fmt.Errorf("%s %s: response time: %w", method, uri, err)
	}
	if value.UpstreamTime, err = optionalDuration(record, s.fields.UpstreamTime); err != nil {
		return fmt.Errorf("%s %s: upstream time: %w", method, uri, err)
	}
	if value.BodyBytes, err = optionalBytes(record, s.fields.BodyBytes); err != nil {
		return fmt.Errorf("%s %s: body bytes: %w", method, uri, err)
	}

	route := s.normalizeRoute(uri)
	stats.Matched++
	stats.Keys[key] = struct{}{}
	for _, modulus := range s.moduli {
		bucket := s.hash(key) % modulus
		placementKey := aggregateKey{Modulus: modulus, Method: allLabel, Route: allLabel, StatusClass: allStatus, Bucket: bucket}
		addAggregate(screened.Aggregates, placementKey, value)
		addAggregate(screened.Aggregates, aggregateKey{Modulus: modulus, Method: method, Route: route, StatusClass: class, Bucket: bucket}, value)
		if screened.BucketKeys[placementKey] == nil {
			screened.BucketKeys[placementKey] = map[string]struct{}{}
		}
		screened.BucketKeys[placementKey][key] = struct{}{}
		total := screened.Totals[modulus]
		total.Requests += value.Requests
		total.ResponseTime += value.ResponseTime
		total.UpstreamTime += value.UpstreamTime
		total.BodyBytes += value.BodyBytes
		screened.Totals[modulus] = total
	}
	return nil
}

func (s *screener) shardKey(raw string) (string, bool) {
	key := strings.TrimSpace(raw)
	if s.keyStripPort {
		if host, port, ok := strings.Cut(key, ":"); ok && isDigits(port) {
			key = host
		}
	}
	if s.keyLower {
		key = strings.ToLower(key)
	}
	match := s.keyRE.FindStringSubmatch(key)
	if match == nil || match[1] == "" {
		return "", false
	}
	if s.keyExcludeRE != nil && s.keyExcludeRE.MatchString(match[1]) {
		return "", false
	}
	return match[1], true
}

func (s *screener) normalizeRoute(uri string) string {
	path := strings.SplitN(uri, "?", 2)[0]
	for _, rule := range s.rules {
		// Adjacent identifier segments share a slash, so a single pass can miss
		// the second one; iterate to a fixed point with a hard bound.
		for i := 0; i < 10; i++ {
			rewritten := rule.Pattern.ReplaceAllString(path, rule.Replacement)
			if rewritten == path {
				break
			}
			path = rewritten
		}
	}
	return path
}

func addAggregate(aggregates map[aggregateKey]aggregate, key aggregateKey, value aggregate) {
	current := aggregates[key]
	current.Requests += value.Requests
	current.ResponseTime += value.ResponseTime
	current.UpstreamTime += value.UpstreamTime
	current.BodyBytes += value.BodyBytes
	aggregates[key] = current
}

func requiredField(record map[string]string, name string) (string, error) {
	value, ok := record[name]
	if !ok {
		return "", fmt.Errorf("field %q is missing", name)
	}
	return value, nil
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func statusClass(raw string) (string, error) {
	status, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || status < 100 || status > 999 {
		return "", fmt.Errorf("invalid HTTP status %q", raw)
	}
	return fmt.Sprintf("%dxx", status/100), nil
}

func optionalDuration(record map[string]string, name string) (float64, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil
	}
	raw, err := requiredField(record, name)
	if err != nil {
		return 0, err
	}
	return parseDuration(raw)
}

// parseDuration sums the retry/fallback durations nginx emits as a list. A
// malformed value fails the run rather than silently becoming zero, because a
// zero here understates the placement it belongs to.
func parseDuration(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return 0, nil
	}
	var total float64
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ':' || r == ' ' }) {
		if token == "-" || token == "" {
			continue
		}
		value, err := strconv.ParseFloat(token, 64)
		if err != nil || value < 0 {
			return 0, fmt.Errorf("invalid duration %q", token)
		}
		total += value
	}
	return total, nil
}

func optionalBytes(record map[string]string, name string) (int64, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil
	}
	raw, err := requiredField(record, name)
	if err != nil {
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid byte count %q", raw)
	}
	return value, nil
}

func openLog(path string) (io.ReadCloser, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zst", ".zstd":
		command := exec.Command("zstdcat", path)
		command.Stderr = os.Stderr
		pipe, err := command.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := command.Start(); err != nil {
			return nil, fmt.Errorf("start zstdcat: %w", err)
		}
		return &commandReader{ReadCloser: pipe, command: command}, nil
	case ".gz":
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		return &chainCloser{Reader: gzipReader, closers: []io.Closer{gzipReader, file}}, nil
	default:
		return os.Open(path)
	}
}

type commandReader struct {
	io.ReadCloser
	command *exec.Cmd
}

func (r *commandReader) Close() error {
	closeErr := r.ReadCloser.Close()
	if err := r.command.Wait(); err != nil {
		return fmt.Errorf("zstdcat: %w", err)
	}
	return closeErr
}

type chainCloser struct {
	io.Reader
	closers []io.Closer
}

func (c *chainCloser) Close() error {
	var firstErr error
	for _, closer := range c.closers {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *screener) writeSummary(output io.Writer, stats *counters, hashName string) {
	if output == nil {
		return
	}
	buckets := make([]string, 0, len(s.shard))
	for bucket := range s.shard {
		buckets = append(buckets, strconv.FormatUint(uint64(bucket), 10))
	}
	sort.Strings(buckets)
	fmt.Fprintf(output, "topology-screen: files=%d lines=%d matched=%d skipped_route=%d skipped_key=%d distinct_keys=%d hash=%s shard_buckets=%s\n",
		stats.Files, stats.Lines, stats.Matched, stats.SkippedRoute, stats.SkippedKey, len(stats.Keys), hashName, strings.Join(buckets, ","))
}

func (s *screener) writeTSV(output io.Writer, screened *result) error {
	header := "modulus\tmethod\troute\tstatus_class\tbucket\tplacement\trequests\trequest_fraction\tresponse_time_s\tresponse_fraction\tupstream_time_s\tbody_bytes\tdistinct_keys"
	if _, err := fmt.Fprintln(output, header); err != nil {
		return err
	}
	keys := make([]aggregateKey, 0, len(screened.Aggregates))
	for key := range screened.Aggregates {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Modulus != keys[j].Modulus {
			return keys[i].Modulus < keys[j].Modulus
		}
		if keys[i].Route != keys[j].Route {
			return keys[i].Route < keys[j].Route
		}
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		if keys[i].StatusClass != keys[j].StatusClass {
			return keys[i].StatusClass < keys[j].StatusClass
		}
		return keys[i].Bucket < keys[j].Bucket
	})
	for _, key := range keys {
		value, total := screened.Aggregates[key], screened.Totals[key.Modulus]
		placement := "primary"
		if s.shard[key.Bucket] {
			placement = "shard"
		}
		requestFraction, responseFraction := 0.0, 0.0
		if total.Requests > 0 {
			requestFraction = float64(value.Requests) / float64(total.Requests)
		}
		if total.ResponseTime > 0 {
			responseFraction = value.ResponseTime / total.ResponseTime
		}
		distinctKeys := "-"
		if unique, ok := screened.BucketKeys[key]; ok {
			distinctKeys = strconv.Itoa(len(unique))
		}
		if _, err := fmt.Fprintf(output, "%d\t%s\t%s\t%s\t%d\t%s\t%d\t%.9f\t%.6f\t%.9f\t%.6f\t%d\t%s\n",
			key.Modulus, key.Method, key.Route, key.StatusClass, key.Bucket, placement,
			value.Requests, requestFraction, value.ResponseTime, responseFraction, value.UpstreamTime, value.BodyBytes, distinctKeys); err != nil {
			return err
		}
	}
	return nil
}
