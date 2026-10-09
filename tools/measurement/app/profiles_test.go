package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/pprof/profile"
)

func awaitProfile(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("profile did not complete")
	}
}

func profileRequest(h http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestProfileStopOwnsRunAndPreservesExport(t *testing.T) {
	var cpuStops, fgStops atomic.Int32
	cpuStarted, fgStarted := make(chan struct{}), make(chan struct{})
	starter := func(started chan struct{}, stops *atomic.Int32) samplingStarter {
		return func(w io.Writer) (func() error, error) {
			close(started)
			return func() error {
				stops.Add(1)
				_, err := io.WriteString(w, "complete profile")
				return err
			}, nil
		}
	}
	h := profileHandlerWithStarters(180, starter(cpuStarted, &cpuStops), starter(fgStarted, &fgStops))
	cpuDone, fgDone := make(chan struct{}), make(chan struct{})
	cpu, fg := httptest.NewRecorder(), httptest.NewRecorder()
	go func() {
		h.ServeHTTP(cpu, httptest.NewRequest("GET", "/debug/pprof/profile?seconds=180&run_id=run-a", nil))
		close(cpuDone)
	}()
	go func() {
		h.ServeHTTP(fg, httptest.NewRequest("GET", "/debug/fgprof?seconds=180&run_id=run-a", nil))
		close(fgDone)
	}()
	awaitProfile(t, cpuStarted)
	awaitProfile(t, fgStarted)
	// Wait for ownership publication after the starter has returned.
	deadline := time.Now().Add(3 * time.Second)
	for profileRequest(h, "GET", "/debug/profiles/status").Body.String() != "{\"cpu\":\"run-a\",\"fgprof\":\"run-a\"}\n" {
		if time.Now().After(deadline) {
			t.Fatal("RUN ownership was not published")
		}
		time.Sleep(time.Millisecond)
	}
	for _, tc := range []struct {
		method, query string
		status        int
	}{
		{"GET", "kind=cpu&run_id=run-a", http.StatusMethodNotAllowed},
		{"POST", "kind=cpu", http.StatusBadRequest},
		{"POST", "kind=heap&run_id=run-a", http.StatusBadRequest},
		{"POST", "kind=cpu&run_id=run-b", http.StatusConflict},
	} {
		if w := profileRequest(h, tc.method, "/debug/profiles/stop?"+tc.query); w.Code != tc.status {
			t.Fatalf("%s %s: status=%d", tc.method, tc.query, w.Code)
		}
	}
	if w := profileRequest(h, "GET", "/debug/pprof/profile?seconds=180&run_id=run-a"); w.Code != http.StatusConflict {
		t.Fatalf("duplicate capture status=%d", w.Code)
	}
	for i := 0; i < 2; i++ {
		if w := profileRequest(h, "POST", "/debug/profiles/stop?kind=cpu&run_id=run-a"); w.Code != http.StatusNoContent {
			t.Fatalf("CPU stop status=%d", w.Code)
		}
	}
	awaitProfile(t, cpuDone)
	if cpuStops.Load() != 1 || cpu.Body.String() != "complete profile" || fgStops.Load() != 0 {
		t.Fatalf("cpu stops=%d output=%q fg stops=%d", cpuStops.Load(), cpu.Body.String(), fgStops.Load())
	}
	if w := profileRequest(h, "POST", "/debug/profiles/stop?kind=fgprof&run_id=run-a"); w.Code != http.StatusNoContent {
		t.Fatalf("fgprof stop status=%d", w.Code)
	}
	awaitProfile(t, fgDone)
	if fgStops.Load() != 1 || fg.Body.String() != "complete profile" {
		t.Fatalf("fgprof stops=%d output=%q", fgStops.Load(), fg.Body.String())
	}
	if status := profileRequest(h, "GET", "/debug/profiles/status").Body.String(); status != "{\"cpu\":\"\",\"fgprof\":\"\"}\n" {
		t.Fatalf("status after export=%s", status)
	}
}

func TestProfileStopProducesParseableProfiles(t *testing.T) {
	h := profileHandler(180)
	for _, tc := range []struct{ kind, path, sampleType string }{
		{"cpu", "/debug/pprof/profile", "cpu"},
		{"fgprof", "/debug/fgprof", "time"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			w, done := httptest.NewRecorder(), make(chan struct{})
			go func() {
				h.ServeHTTP(w, httptest.NewRequest("GET", tc.path+"?seconds=180&run_id=run-a", nil))
				close(done)
			}()
			deadline := time.Now().Add(3 * time.Second)
			for profileRequest(h, "GET", "/debug/profiles/status").Body.String() == "{\"cpu\":\"\",\"fgprof\":\"\"}\n" {
				if time.Now().After(deadline) {
					t.Fatal("profiler did not start")
				}
				time.Sleep(time.Millisecond)
			}
			// Allow real samplers to collect data before the end marker.
			time.Sleep(50 * time.Millisecond)
			ended := time.Now()
			if stop := profileRequest(h, "POST", "/debug/profiles/stop?kind="+tc.kind+"&run_id=run-a"); stop.Code != http.StatusNoContent {
				t.Fatalf("stop status=%d", stop.Code)
			}
			awaitProfile(t, done)
			p, err := profile.ParseData(w.Body.Bytes())
			if err != nil {
				t.Fatalf("incomplete profile: %v", err)
			}
			if p.TimeNanos <= 0 || p.DurationNanos <= 0 || p.DurationNanos >= int64(3*time.Second) {
				t.Fatalf("capture time=%d duration=%d", p.TimeNanos, p.DurationNanos)
			}
			if p.TimeNanos > ended.UnixNano() || p.TimeNanos+p.DurationNanos < ended.UnixNano() {
				t.Fatal("profile does not cover the end marker before stop notification")
			}
			found := false
			for _, sampleType := range p.SampleType {
				found = found || sampleType.Type == tc.sampleType
			}
			if !found {
				t.Fatalf("missing sample type %s", tc.sampleType)
			}
		})
	}
}

func TestProfileUpperBoundStillExports(t *testing.T) {
	var stops atomic.Int32
	start := func(w io.Writer) (func() error, error) {
		return func() error { stops.Add(1); _, err := io.WriteString(w, "bounded profile"); return err }, nil
	}
	h := profileHandlerWithStarters(1, start, start)
	w := profileRequest(h, "GET", "/debug/fgprof?seconds=1&run_id=run-a")
	if w.Code != http.StatusOK || stops.Load() != 1 || w.Body.String() != "bounded profile" {
		t.Fatalf("upper bound: status=%d stops=%d output=%s", w.Code, stops.Load(), w.Body.String())
	}
}
