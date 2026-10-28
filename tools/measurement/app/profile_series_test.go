package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/EkeMinusYou/isucon-template/tools/measurement/profileseries"
	"github.com/google/pprof/profile"
)

// Keep the blocking stack identifiable in real runtime-generated profiles.
//
//go:noinline
func seriesTestBlocked(entered chan<- struct{}, release <-chan struct{}, done chan<- struct{}) {
	close(entered)
	<-release
	close(done)
}

func TestSnapshotSeriesTracksMemoryAndChangingGoroutines(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go seriesTestBlocked(entered, release, done)
	<-entered
	runtime.GC()
	var data bytes.Buffer
	stop, err := startProfileSeriesInterval(&data, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	held := make([]byte, 32<<20)
	held[0] = 1
	runtime.GC()
	time.Sleep(70 * time.Millisecond)
	close(release)
	<-done
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(held)
	path := filepath.Join(t.TempDir(), "host-go-snapshots.tar")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var heap []int64
	var blocked []bool
	index, err := profileseries.Read(path, func(e profileseries.Entry, p *profile.Profile) error {
		if profileseries.Kind(e.Name) == "go-heap" {
			i := -1
			for n, k := range p.SampleType {
				if k.Type == "inuse_space" {
					i = n
				}
			}
			var total int64
			for _, s := range p.Sample {
				total += s.Value[i]
			}
			heap = append(heap, total)
		} else {
			found := false
			for _, f := range p.Function {
				if strings.Contains(f.Name, "seriesTestBlocked") {
					found = true
				}
			}
			blocked = append(blocked, found)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heap) < 3 || len(heap) != len(blocked) || index.Status != "complete" {
		t.Fatalf("captures=%d/%d index=%+v", len(heap), len(blocked), index)
	}
	if !blocked[0] || blocked[len(blocked)-1] {
		t.Fatalf("changing blocking stack was not observed: %v", blocked)
	}
	max := heap[0]
	for _, v := range heap {
		if v > max {
			max = v
		}
	}
	if max-heap[0] < 16<<20 {
		t.Fatalf("retained heap growth was not observed: %v", heap)
	}
	if err := os.WriteFile(path, data.Bytes()[:len(data.Bytes())/2], 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := profileseries.Read(path, nil); err == nil {
		t.Fatal("truncated recording accepted")
	}
}

func TestRuntimeMetricsAreAvailableAndCounterChanges(t *testing.T) {
	before := runtimeMetrics()
	held := make([]byte, 8<<20)
	held[0] = 1
	after := runtimeMetrics()
	runtime.KeepAlive(held)
	if after["go_goroutines"].(uint64) < 1 {
		t.Fatal("missing goroutine gauge")
	}
	if after["go_alloc_bytes_total"].(uint64) <= before["go_alloc_bytes_total"].(uint64) {
		t.Fatal("allocation counter did not observe an allocation")
	}
	if _, ok := after["go_gc_cycles_total"]; !ok {
		t.Fatal("GC counter unavailable")
	}
}

func BenchmarkRuntimeMetrics(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		runtimeMetrics()
	}
}

func TestSnapshotHTTPLifecycle(t *testing.T) {
	h, err := NewHandler(Options{})
	if err != nil {
		t.Fatal(err)
	}
	capture := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(capture, httptest.NewRequest("GET", "/debug/profiles/snapshots?seconds=10&run_id=test", nil))
	}()
	defer func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/debug/profiles/stop?kind=snapshots&run_id=test", nil))
		<-done
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		status := httptest.NewRecorder()
		h.ServeHTTP(status, httptest.NewRequest("GET", "/debug/profiles/status", nil))
		var active map[string]string
		if err := json.Unmarshal(status.Body.Bytes(), &active); err != nil {
			t.Fatal(err)
		}
		if active["snapshots"] == "test" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("snapshot capture never became ready")
		}
		time.Sleep(time.Millisecond)
	}
	for _, req := range []struct{ method, path string }{
		{"GET", "/debug/profiles/snapshots?seconds=1&run_id=other"},
		{"POST", "/debug/profiles/stop?kind=snapshots&run_id=other"},
	} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(req.method, req.path, nil))
		if response.Code != http.StatusConflict {
			t.Fatalf("%s returned %d", req.path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("POST", "/debug/profiles/stop?kind=snapshots&run_id=test", nil))
	if response.Code != http.StatusNoContent {
		t.Fatal(response.Code)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("capture did not stop")
	}
	if capture.Code != http.StatusOK {
		t.Fatal(capture.Body.String())
	}
	path := filepath.Join(t.TempDir(), "capture.tar")
	if err := os.WriteFile(path, capture.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := profileseries.Read(path, nil); err != nil {
		t.Fatal(err)
	}
}
