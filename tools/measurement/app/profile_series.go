package app

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/EkeMinusYou/isucon-template/tools/measurement/profileseries"
	pprofprofile "github.com/google/pprof/profile"
)

func startProfileSeries(w io.Writer) (func() error, error) {
	return startProfileSeriesInterval(w, profileseries.DefaultInterval)
}

// Snapshot recording runs for the same lifecycle as CPU collection. Memory
// profiles contain both live-heap and cumulative-allocation sample types.
func startProfileSeriesInterval(w io.Writer, interval time.Duration) (func() error, error) {
	stop, done := make(chan struct{}), make(chan error, 1)
	ready := make(chan error, 1)
	go func() { done <- recordProfileSeries(w, interval, stop, ready) }()
	if err := <-ready; err != nil {
		return nil, err
	}
	var once sync.Once
	var result error
	return func() error { once.Do(func() { close(stop); result = <-done }); return result }, nil
}

func recordProfileSeries(w io.Writer, interval time.Duration, stop <-chan struct{}, ready chan<- error) error {
	tw := tar.NewWriter(w)
	index := profileseries.Index{Version: 1, StartedAt: time.Now().UnixNano(), IntervalNS: int64(interval), Status: "complete"}
	var used int64
	sequence := 0
	capture := func() error {
		for _, kind := range []string{"heap", "goroutine"} {
			started := time.Now()
			var data bytes.Buffer
			if err := pprof.Lookup(kind).WriteTo(&data, 0); err != nil {
				return err
			}
			if data.Len() > profileseries.MaxProfileBytes || used+int64(data.Len())+(1<<20) > profileseries.MaxBundleBytes || len(index.Entries) >= profileseries.MaxEntries {
				return fmt.Errorf("snapshot recording limit exceeded")
			}
			p, err := pprofprofile.ParseData(data.Bytes())
			if err != nil {
				return err
			}
			name := fmt.Sprintf("%06d-go-%s.pprof", sequence, kind)
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(data.Len())}); err != nil {
				return err
			}
			if _, err := tw.Write(data.Bytes()); err != nil {
				return err
			}
			used += (int64(data.Len())+511)/512*512 + 512
			index.Entries = append(index.Entries, profileseries.Entry{Name: name, CapturedAt: p.TimeNanos, DurationNS: int64(time.Since(started))})
		}
		sequence++
		return nil
	}
	err := capture()
	ready <- err
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for err == nil {
		select {
		case <-stop:
			err = capture()
			goto finished
		case <-ticker.C:
			err = capture()
		}
	}
finished:
	if err != nil {
		index.Status = "failed"
		index.Error = err.Error()
	}
	index.EndedAt = time.Now().UnixNano()
	body, jsonErr := json.Marshal(index)
	if jsonErr != nil {
		return jsonErr
	}
	if e := tw.WriteHeader(&tar.Header{Name: "index.json", Mode: 0600, Size: int64(len(body))}); e != nil {
		return e
	}
	if _, e := tw.Write(body); e != nil {
		return e
	}
	if e := tw.Close(); e != nil {
		return e
	}
	return err
}
