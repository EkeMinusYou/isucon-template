package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func defaultString(s string) *string { return &s }

func jsonWorkflow() config {
	return config{Timeout: "1s", RequestTimeout: "100ms", PollInterval: "1ms", PollRetries: 1,
		Start:   action{HTTP: &httpAction{Method: "POST", URL: "/start", Body: `{"target": {{json .target}}}`}, Response: response{Format: "json", Fields: map[string]selector{"job_id": {Path: "/id"}}}},
		Poll:    action{HTTP: &httpAction{Method: "GET", URL: "/jobs/{{urlquery .job_id}}"}, Response: response{Format: "json", Fields: map[string]selector{"job_id": {Path: "/id"}, "state": {Path: "/state"}, "passed": {Path: "/passed", Default: defaultString("")}, "score": {Path: "/score", Default: defaultString("")}, "log": {Path: "/log", Default: defaultString("")}}}},
		Vars:    map[string]string{"target": "a\"b"},
		Outcome: outcome{Status: "state", DoneValues: []string{"done"}, FailedValues: []string{"cancelled"}, Pass: "passed", PassValues: []string{"true"}, FailValues: []string{"false"}, Score: "score", Logs: []string{"log"}}}
}

func TestJSONWorkflowRetriesPollAndSavesOriginalLog(t *testing.T) {
	starts, polls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "POST" {
			starts++
			body, _ := io.ReadAll(req.Body)
			if string(body) != `{"target": "a\"b"}` {
				t.Errorf("body = %s", body)
			}
			io.WriteString(w, `{"id":9007199254740993}`)
			return
		}
		polls++
		if req.URL.Path != "/jobs/9007199254740993" {
			t.Errorf("path = %s", req.URL.Path)
		}
		switch polls {
		case 1:
			w.WriteHeader(503)
		case 2:
			io.WriteString(w, `{"id":9007199254740993,"state":"running","log":"partial\n"}`)
		default:
			io.WriteString(w, `{"id":9007199254740993,"state":"done","passed":true,"score":123,"log":"partial\ncomplete\n"}`)
		}
	}))
	defer server.Close()
	c := jsonWorkflow()
	c.BaseURL = server.URL
	var out, diagnostics bytes.Buffer
	dir := t.TempDir()
	if err := newRunner(c, &out, &diagnostics, dir).run(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || polls != 3 {
		t.Fatalf("starts=%d polls=%d", starts, polls)
	}
	if out.String() != "partial\ncomplete\nSCORE: 123\nBENCHMARK_PASS\n" {
		t.Fatal(out.String())
	}
	log, err := os.ReadFile(filepath.Join(dir, "benchmark-result.txt"))
	if err != nil || string(log) != "partial\ncomplete\n" {
		t.Fatalf("saved log=%s err=%v", log, err)
	}
}

func TestStartIsNeverRetriedOrRedirected(t *testing.T) {
	for _, status := range []int{302, 403, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls++
				w.Header().Set("Location", "/login")
				w.WriteHeader(status)
			}))
			defer server.Close()
			c := jsonWorkflow()
			c.BaseURL = server.URL
			err := newRunner(c, io.Discard, io.Discard, "").run(context.Background(), "")
			if err == nil || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestExistingJobAndSeparateResultCommand(t *testing.T) {
	c := jsonWorkflow()
	c.Start = action{Command: []string{"sh", "-c", "exit 99"}, Response: c.Start.Response}
	c.Poll = action{Command: []string{"printf", `%s`, `{"id":"{{.job_id}}","state":"done"}`}, Response: c.Poll.Response}
	c.Result = &action{Command: []string{"printf", "%s", `{"passed":false,"score":0,"log":"validation failed\n"}`}, Response: response{Format: "json", Fields: map[string]selector{"passed": {Path: "/passed"}, "score": {Path: "/score"}, "log": {Path: "/log"}}}}
	var out bytes.Buffer
	err := newRunner(c, &out, io.Discard, "").run(context.Background(), "42")
	if !errors.Is(err, errBenchmarkFailed) {
		t.Fatal(err)
	}
	if out.String() != "validation failed\nSCORE: 0\nBENCHMARK_FAIL\n" {
		t.Fatal(out.String())
	}
}

func TestTimeoutRetainsPartialLogAndDoesNotClaimSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"id":"42","state":"running","log":"partial\n"}`)
	}))
	defer server.Close()
	c := jsonWorkflow()
	c.BaseURL = server.URL
	c.Timeout = "100ms"
	var out bytes.Buffer
	dir := t.TempDir()
	err := newRunner(c, &out, io.Discard, dir).run(context.Background(), "42")
	if err == nil || strings.Contains(out.String(), "BENCHMARK_PASS") {
		t.Fatalf("err=%v out=%s", err, out.String())
	}
	body, err := os.ReadFile(filepath.Join(dir, "benchmark-result.txt"))
	if err != nil || string(body) != "partial\n" {
		t.Fatalf("body=%s err=%v", body, err)
	}
}

func TestWrongJobRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"id":"43","state":"done","passed":true}`)
	}))
	defer server.Close()
	c := jsonWorkflow()
	c.BaseURL = server.URL
	err := newRunner(c, io.Discard, io.Discard, "").run(context.Background(), "42")
	if err == nil || !strings.Contains(err.Error(), "different job_id") {
		t.Fatal(err)
	}
}

func TestInvalidLaterPhaseStopsBeforeSubmission(t *testing.T) {
	c := jsonWorkflow()
	c.BaseURL = "http://127.0.0.1:1"
	c.Poll.HTTP.Headers = map[string]string{"Authorization": `{{env "PORTAL_TEST_MISSING_SECRET"}}`}
	t.Setenv("PORTAL_TEST_MISSING_SECRET", "")
	err := newRunner(c, io.Discard, io.Discard, "").run(context.Background(), "")
	if err == nil || strings.Contains(err.Error(), "start failed") {
		t.Fatal(err)
	}
}

func TestExtractJSONPointerAndHTML(t *testing.T) {
	r := response{Format: "json", Fields: map[string]selector{"value": {Path: "/a~1b/~0/0"}}}
	values, err := extract([]byte(`{"a/b":{"~":["日本語"]}}`), r)
	if err != nil || values["value"] != "日本語" {
		t.Fatalf("values=%v err=%v", values, err)
	}
	r = response{Format: "regex", Fields: map[string]selector{"log": {Path: `(?s)<pre>(.*?)</pre>`, Type: "html_text"}}}
	values, err = extract([]byte("<pre>a &lt;b&gt;<br />c &amp; d</pre>"), r)
	if err != nil || values["log"] != "a <b>\nc & d" {
		t.Fatalf("values=%v err=%v", values, err)
	}
}

func pbUint(field int, value uint64) []byte {
	b := binary.AppendUvarint(nil, uint64(field)<<3)
	return binary.AppendUvarint(b, value)
}

func pbBytes(field int, value []byte) []byte {
	b := binary.AppendUvarint(nil, uint64(field)<<3|2)
	b = binary.AppendUvarint(b, uint64(len(value)))
	return append(b, value...)
}

func TestProtobufRepeatedFieldsAndTruncation(t *testing.T) {
	body := append(pbBytes(1, pbUint(2, 8)), pbBytes(1, pbUint(2, 9))...)
	value, ok, err := protoSelect(body, selector{Path: "1[1].2", Type: "uint"})
	if err != nil || !ok || value != "9" {
		t.Fatalf("value=%s found=%v err=%v", value, ok, err)
	}
	for _, b := range [][]byte{{0x0a, 0x80}, {0x0a, 0xff, 0xff, 0xff, 0xff, 0x7f}, {0x08, 0x80}, {0}, {0x09, 1}} {
		if _, err := protoFields(b); err == nil {
			t.Fatalf("accepted invalid bytes %x", b)
		}
	}
}

func TestCommandOutputCopyEnforcesSizeLimit(t *testing.T) {
	var buffer boundedBuffer
	// Hide WriterTo to exercise the same copy path used for command pipes.
	source := struct{ io.Reader }{strings.NewReader(strings.Repeat("x", maxResponseBytes+1))}
	n, err := io.Copy(&buffer, source)
	if err == nil || n > maxResponseBytes {
		t.Fatalf("copied=%d err=%v", n, err)
	}
}

func TestFailedRetrievalPreservesExistingLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	c := jsonWorkflow()
	c.BaseURL = server.URL
	dir := t.TempDir()
	path := filepath.Join(dir, "benchmark-result.txt")
	if err := os.WriteFile(path, []byte("previous log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := newRunner(c, io.Discard, io.Discard, dir).run(context.Background(), "42"); err == nil {
		t.Fatal("expected retrieval failure")
	}
	log, err := os.ReadFile(path)
	if err != nil || string(log) != "previous log\n" {
		t.Fatalf("log=%q err=%v", log, err)
	}
}
