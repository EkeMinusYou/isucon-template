package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func portalConfig(t *testing.T, path string) config {
	t.Helper()
	t.Setenv("PORTAL_URL", "http://portal.invalid")
	t.Setenv("PORTAL_COOKIE", "session=test")
	t.Setenv("PORTAL_CSRF_TOKEN", "test-csrf")
	t.Setenv("PORTAL_TARGET_ID", "129")
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	c.PollInterval = "1ms"
	return c
}

// These fields follow the portal's contestant API .proto files, not benchmark code.
func portalJob(state uint64, passed bool) []byte {
	job := pbUint(1, 42)
	if state != 0 {
		job = append(job, pbUint(4, state)...)
	}
	if state == 4 {
		job = append(job, pbUint(9, 12345)...)
		result := pbUint(1, 1)
		if passed {
			result = append(result, pbUint(2, 1)...)
		}
		execution := append(pbBytes(2, []byte("benchmark output\n")), pbBytes(3, []byte("benchmark stderr\n"))...)
		result = append(result, pbBytes(5, execution)...)
		job = append(job, pbBytes(18, result)...)
	}
	return pbBytes(1, job)
}

func TestISUCON12ConfigWorkflow(t *testing.T) {
	c := portalConfig(t, "../contest/portal/isucon12.json")
	starts, polls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Cookie") != "session=test" || req.Header.Get("X-CSRF-Token") != "test-csrf" {
			t.Errorf("missing auth headers")
		}
		if req.Method == "POST" {
			starts++
			body, _ := io.ReadAll(req.Body)
			if !bytes.Equal(body, []byte{0x08, 0x81, 0x01}) {
				t.Errorf("target encoding = %x", body)
			}
			if req.Header.Get("Content-Type") != "application/vnd.google.protobuf" || req.URL.Path != "/api/contestant/benchmark_jobs" {
				t.Errorf("incorrect enqueue request")
			}
			w.Write(portalJob(0, false))
			return
		}
		if req.URL.Path != "/api/contestant/benchmark_jobs/42" {
			t.Errorf("poll path=%s", req.URL.Path)
		}
		polls++
		if polls == 1 {
			w.Write(portalJob(1, false))
		} else {
			w.Write(portalJob(4, true))
		}
	}))
	defer server.Close()
	c.BaseURL = server.URL
	var out bytes.Buffer
	if err := newRunner(c, &out, io.Discard, "").run(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || polls != 2 || out.String() != "benchmark output\nbenchmark stderr\nSCORE: 12345\nBENCHMARK_PASS\n" {
		t.Fatalf("starts=%d polls=%d output=%s", starts, polls, out.String())
	}
	// The passed=false field is omitted by proto3, even on a completed job.
	fields, err := extract(portalJob(4, false), c.Poll.Response)
	if err != nil || fields["passed"] != "false" {
		t.Fatalf("fields=%v err=%v", fields, err)
	}
}

// Markup follows the participant job_detail_part.html template in isucon13-portal.
func portalHTML(status, result, score string) string {
	return fmt.Sprintf(`<table><tr><th width="10%%">JobId</th><td>42</td></tr>
<tr><th width="10%%">Total Score</th><td> %s </td></tr>
<tr><th width="10%%">Status</th><td><span class="tag is-info">%s</span></td></tr>
<tr><th width="10%%">Result</th><td>%s</td></tr>
<tr><th width="10%%">Message</th><td><pre style="overflow-x: scroll;">message &lt;test&gt;<br>next line</pre></td></tr></table>`, score, status, result)
}

func TestISUCON13ConfigWorkflow(t *testing.T) {
	c := portalConfig(t, "../contest/portal/isucon13.json")
	starts, polls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Requested-With") != "XMLHttpRequest" || req.Header.Get("X-CSRFToken") != "test-csrf" {
			t.Errorf("missing Django AJAX/CSRF headers")
		}
		if req.Method == "POST" {
			starts++
			if req.URL.Path != "/contest/jobs/enqueue/" {
				t.Errorf("enqueue path=%s", req.URL.Path)
			}
			io.WriteString(w, `{"id":42}`)
			return
		}
		if req.URL.Path != "/contest/jobs/42/" {
			t.Errorf("poll path=%s", req.URL.Path)
		}
		polls++
		if polls == 1 {
			io.WriteString(w, portalHTML("Running", "", ""))
		} else {
			io.WriteString(w, portalHTML("Done", `<span class="tag is-success">Passed</span>`, "12,345"))
		}
	}))
	defer server.Close()
	c.BaseURL = server.URL
	var out bytes.Buffer
	if err := newRunner(c, &out, io.Discard, "").run(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || polls != 2 || out.String() != "message <test>\nnext line\nSCORE: 12345\nBENCHMARK_PASS\n" {
		t.Fatalf("starts=%d polls=%d output=%s", starts, polls, out.String())
	}
	for _, state := range []string{"Aborted", "Canceled"} {
		fields, err := extract([]byte(portalHTML(state, `<span>Failed</span>`, "0")), c.Poll.Response)
		if err != nil || !contains(c.Outcome.FailedValues, fields["state"]) {
			t.Fatalf("fields=%v err=%v", fields, err)
		}
	}
}
