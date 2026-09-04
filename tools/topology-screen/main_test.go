package main

import (
	"bytes"
	"strings"
	"testing"
)

func newTestScreener(t *testing.T, override func(*screenerOptions)) *screener {
	t.Helper()
	options := screenerOptions{
		fields: fieldNames{
			Key: "vhost", Method: "method", URI: "uri", Status: "status",
			ResponseTime: "response_time", UpstreamTime: "upstream_time", BodyBytes: "body_bytes",
		},
		keyPattern:   `^([^.]+)\.example\.com$`,
		keyExclude:   `^root$`,
		keyLower:     true,
		keyStripPort: true,
		routePattern: `^/api/`,
		moduli:       "4",
		hashName:     "fnv1a",
		shard:        "0",
	}
	if override != nil {
		override(&options)
	}
	instance, err := newScreener(options)
	if err != nil {
		t.Fatal(err)
	}
	return instance
}

func screen(t *testing.T, instance *screener, parse lineParser, lines ...string) (*result, *counters) {
	t.Helper()
	screened, stats := newResult(), &counters{Keys: map[string]struct{}{}}
	if err := instance.scan(strings.NewReader(strings.Join(lines, "\n")), parse, screened, stats); err != nil {
		t.Fatal(err)
	}
	return screened, stats
}

func TestScanBucketsKeyedTrafficAndSkipsTheRest(t *testing.T) {
	instance := newTestScreener(t, nil)
	screened, stats := screen(t, instance, parseJSONLine,
		`{"method":"POST","uri":"/api/items/10/events","status":"201","vhost":"Alice.example.com:443","response_time":0.010,"upstream_time":"0.008, 0.002","body_bytes":100}`,
		`{"method":"GET","uri":"/api/items/20/details?limit=10","status":"200","vhost":"bob.example.com","response_time":0.020,"upstream_time":"0.015","body_bytes":200}`,
		`{"method":"GET","uri":"/api/items/20/details","status":"200","vhost":"root.example.com","response_time":1.0,"upstream_time":"1.0","body_bytes":999}`,
		`{"method":"GET","uri":"/health","status":"200","vhost":"bob.example.com","response_time":1.0,"upstream_time":"1.0","body_bytes":1}`,
	)
	if stats.Matched != 2 || stats.SkippedKey != 1 || stats.SkippedRoute != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	if len(stats.Keys) != 2 {
		t.Fatalf("distinct keys = %d", len(stats.Keys))
	}
	if got := screened.Totals[4]; got.Requests != 2 || got.UpstreamTime != 0.025 || got.BodyBytes != 300 {
		t.Fatalf("total = %+v", got)
	}
	var placementRequests int64
	for key, value := range screened.Aggregates {
		if key.Method == allLabel && key.Route == allLabel {
			placementRequests += value.Requests
		}
	}
	if placementRequests != 2 {
		t.Fatalf("placement requests = %d", placementRequests)
	}
}

func TestScanReadsLTSVAndCustomFieldNames(t *testing.T) {
	instance := newTestScreener(t, func(options *screenerOptions) {
		options.fields = fieldNames{Key: "tenant", Method: "req_method", URI: "req_uri", Status: "code", ResponseTime: "took"}
		options.keyPattern = `^(.+)$`
		options.keyExclude = ""
		options.routePattern = `^/`
	})
	screened, stats := screen(t, instance, parseLTSVLine,
		"tenant:acme\treq_method:get\treq_uri:/api/players/12\tcode:200\ttook:0.5",
		"tenant:globex\treq_method:GET\treq_uri:/api/players/13\tcode:503\ttook:1.5",
	)
	if stats.Matched != 2 {
		t.Fatalf("matched = %d", stats.Matched)
	}
	var routes, classes []string
	for key := range screened.Aggregates {
		if key.Route != allLabel {
			routes = append(routes, key.Route)
			classes = append(classes, key.StatusClass)
		}
	}
	for _, route := range routes {
		if route != "/api/players/:id" {
			t.Fatalf("route = %q", route)
		}
	}
	if len(classes) != 2 || classes[0] == classes[1] {
		t.Fatalf("status classes were merged: %v", classes)
	}
	if screened.Totals[4].BodyBytes != 0 {
		t.Fatalf("disabled body_bytes column should stay zero, got %d", screened.Totals[4].BodyBytes)
	}
}

func TestScanFailsOnMalformedDurationAndMissingField(t *testing.T) {
	instance := newTestScreener(t, nil)
	screened, stats := newResult(), &counters{Keys: map[string]struct{}{}}
	line := `{"method":"GET","uri":"/api/x","status":"200","vhost":"a.example.com","response_time":0.1,"upstream_time":"oops","body_bytes":1}`
	if err := instance.scan(strings.NewReader(line), parseJSONLine, screened, stats); err == nil {
		t.Fatal("malformed upstream duration must fail the run")
	}
	missing := `{"method":"GET","uri":"/api/x","status":"200","vhost":"a.example.com","response_time":0.1,"body_bytes":1}`
	if err := instance.scan(strings.NewReader(missing), parseJSONLine, newResult(), &counters{Keys: map[string]struct{}{}}); err == nil {
		t.Fatal("missing configured field must fail the run")
	}
}

func TestNormalizeRouteCollapsesIdentifiers(t *testing.T) {
	instance := newTestScreener(t, nil)
	cases := map[string]string{
		"/api/items/20/details?limit=10":                 "/api/items/:id/details",
		"/api/items/20/events/1002/status":               "/api/items/:id/events/:id/status",
		"/api/user/1/2":                                  "/api/user/:id/:id",
		"/api/o/3fa85f64-5717-4562-b3fc-2c963f66afa6/ic": "/api/o/:uuid/ic",
	}
	for uri, want := range cases {
		if got := instance.normalizeRoute(uri); got != want {
			t.Fatalf("normalizeRoute(%q) = %q, want %q", uri, got, want)
		}
	}
}

func TestNormalizeRuleFlagReplacesDefaults(t *testing.T) {
	instance := newTestScreener(t, func(options *screenerOptions) {
		options.rules = []string{`/v[0-9]+/=>/:version/`}
	})
	if got := instance.normalizeRoute("/api/v2/items/7"); got != "/api/:version/items/7" {
		t.Fatalf("route = %q", got)
	}
}

func TestHashAndShardBucketSelection(t *testing.T) {
	for _, name := range []string{"fnv1a", "fnv1", "crc32", "sha256"} {
		if _, err := hasherFor(name); err != nil {
			t.Fatalf("hasher %s: %v", name, err)
		}
	}
	if _, err := hasherFor("md5"); err == nil {
		t.Fatal("unknown hash must be rejected")
	}
	if _, err := parseBuckets("0,1", []uint32{2}); err == nil {
		t.Fatal("a shard covering every bucket must be rejected")
	}
	if _, err := parseBuckets("3", []uint32{2, 4}); err == nil {
		t.Fatal("a bucket outside the smallest modulus must be rejected")
	}
	buckets, err := parseBuckets("0,1", []uint32{4})
	if err != nil || !buckets[0] || !buckets[1] || buckets[2] {
		t.Fatalf("buckets = %v, err = %v", buckets, err)
	}
}

func TestWriteTSVMarksShardBucketsAndCountsKeys(t *testing.T) {
	instance := newTestScreener(t, nil)
	placement := aggregateKey{Modulus: 4, Method: allLabel, Route: allLabel, StatusClass: allStatus, Bucket: 0}
	route := aggregateKey{Modulus: 4, Method: "GET", Route: "/api/x", StatusClass: "2xx", Bucket: 1}
	screened := newResult()
	screened.Aggregates[placement] = aggregate{Requests: 10, ResponseTime: 2}
	screened.Aggregates[route] = aggregate{Requests: 4, ResponseTime: 1}
	screened.BucketKeys[placement] = map[string]struct{}{"a": {}, "b": {}}
	screened.Totals[4] = aggregate{Requests: 20, ResponseTime: 4}
	var output bytes.Buffer
	if err := instance.writeTSV(&output, screened); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "4\tALL\tALL\tall\t0\tshard\t10\t0.500000000\t2.000000\t0.500000000\t0.000000\t0\t2") {
		t.Fatalf("placement row missing: %s", got)
	}
	if !strings.Contains(got, "4\tGET\t/api/x\t2xx\t1\tprimary\t4\t0.200000000") {
		t.Fatalf("route row missing: %s", got)
	}
	if !strings.Contains(got, "/api/x\t2xx\t1\tprimary\t4\t0.200000000\t1.000000\t0.250000000\t0.000000\t0\t-\n") {
		t.Fatalf("non-placement rows must leave distinct_keys empty: %s", got)
	}
}

func TestResolveInputsRejectsEmptyMatch(t *testing.T) {
	if _, err := resolveInputs("", "raw/*.log", ""); err == nil {
		t.Fatal("missing -run-dir and -input must fail")
	}
	if _, err := resolveInputs(t.TempDir(), "raw/access-*.log.zst", ""); err == nil {
		t.Fatal("an empty RUN directory must fail")
	}
}
