package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteCoverageUsesCapturedDefinitionsAndReconcilesTraffic(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb not installed")
	}
	root := filepath.Join(t.TempDir(), "runs")
	for _, id := range []string{"20261001-120000", "20261001-130000", "20261001-140000"} {
		writeTestFile(t, filepath.Join(root, id), "raw/access-entry.log", `{"msec":"1001","response_time":0.1,"method":"GET","uri":"/api/search?mode=fast","status":200}
{"msec":"1002","response_time":0.1,"method":"GET","uri":"/api/search?other=2","status":200}
{"msec":"1003","response_time":0.1,"method":"GET","uri":"/api/unknown/secret?x=1","status":200}
`)
	}
	schema, err := os.ReadFile("../analysis/schema/access-log.sql")
	if err != nil {
		t.Fatal(err)
	}
	defs := `[{"run_id":"20261001-120000","config":{"api_prefix":"/api/","routes":[{"name":"fast-search","pattern":"^/api/search[?]mode=fast$","match_query":true},{"name":"search","pattern":"^/api/search$"}]}},{"run_id":"20261001-130000","config":{"api_prefix":"/api/","routes":[{"name":"old-search","pattern":"^/api/search$"}]}}]`
	query := "set variable run_glob=" + sqlString(filepath.ToSlash(root)+"/*") + ";set variable operation_definitions=" + sqlString(defs) + ";" + string(schema) + `
select run_id,route,sum(requests) from http_traffic_buckets group by all order by run_id,route;
select sum(requests)- (select sum(requests) from http_traffic_buckets) from http_route_coverage;
select requests from http_route_coverage where run_id='20261001-120000' and status='unclassified-api';
select requests from http_route_coverage where status='definitions-missing';`
	db := filepath.Join(t.TempDir(), "quality.duckdb")
	query += "create table route_coverage as select * from http_route_coverage; create table http_traffic as select * from http_traffic_buckets;"
	cmd := exec.Command(duckdb, db, "-noheader", "-list")
	cmd.Stdin = strings.NewReader(query)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("route coverage: %v: %s", err, output)
	}
	want := []string{"20261001-120000|<unclassified-api>|1", "20261001-120000|fast-search|1", "20261001-120000|search|1", "20261001-130000|old-search|2", "0\n1\n3"}
	for _, line := range want {
		if !strings.Contains(string(output), line) {
			t.Fatalf("missing %q in %s", line, output)
		}
	}
	report, _ := filepath.Abs("../analysis/queries/route-quality.sql")
	var stdout bytes.Buffer
	r := runner{options: options{db: db, duckdb: duckdb, stdout: &stdout, stderr: &bytes.Buffer{}}, config: config{Reports: reportConfig{RouteQuality: report}}}
	if err := r.routeQuality("20261001-120000", "20261001-140000"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "unclassified-api") || !strings.Contains(stdout.String(), "definitions-missing") {
		t.Fatalf("quality report hid missing classifications: %s", stdout.String())
	}

}
