package main

import (
	"encoding/csv"
	"errors"
	"sort"
	"strconv"
	"time"
)

func diskHeader() []string {
	return []string{
		"sample", "timestamp", "elapsed_ms", "device",
		"read_iops", "write_iops", "discard_iops", "flush_iops",
		"read_merged_per_sec", "write_merged_per_sec", "discard_merged_per_sec",
		"read_bytes_per_sec", "write_bytes_per_sec", "discard_bytes_per_sec",
		"read_await_ms", "write_await_ms", "discard_await_ms", "flush_await_ms",
		"avg_read_request_bytes", "avg_write_request_bytes", "avg_discard_request_bytes",
		"io_in_progress", "io_util_pct", "avg_queue_size",
	}
}

func writeDiskRows(writer *csv.Writer, sampleIndex uint64, start time.Time, current snapshot, previous *snapshot) error {
	devices := make([]string, 0, len(current.disks))
	for device := range current.disks {
		devices = append(devices, device)
	}
	sort.Strings(devices)

	seconds := 0.0
	if previous != nil {
		seconds = current.at.Sub(previous.at).Seconds()
		if seconds < 0 {
			return errTimestampMovedBackwards
		}
	}
	for _, device := range devices {
		cur := current.disks[device]
		var prev *diskCounters
		if previous != nil {
			if value, ok := previous.disks[device]; ok {
				prev = &value
			}
		}
		metrics := diskDeviceMetrics(cur, prev, seconds)
		values := []string{
			strconv.FormatUint(sampleIndex, 10), current.at.UTC().Format(time.RFC3339Nano), strconv.FormatInt(current.at.Sub(start).Milliseconds(), 10), device,
			formatFloat(metrics.readIOPS), formatFloat(metrics.writeIOPS), formatFloat(metrics.discardIOPS), formatFloat(metrics.flushIOPS),
			formatFloat(metrics.readMergedPerSec), formatFloat(metrics.writeMergedPerSec), formatFloat(metrics.discardMergedPerSec),
			formatFloat(metrics.readBytesPerSec), formatFloat(metrics.writeBytesPerSec), formatFloat(metrics.discardBytesPerSec),
			formatFloat(metrics.readAwaitMillis), formatFloat(metrics.writeAwaitMillis), formatFloat(metrics.discardAwaitMillis), formatFloat(metrics.flushAwaitMillis),
			formatFloat(metrics.avgReadRequestBytes), formatFloat(metrics.avgWriteRequestBytes), formatFloat(metrics.avgDiscardRequestBytes),
			strconv.FormatUint(cur.ioInProgress, 10), formatFloat(metrics.ioUtilPct), formatFloat(metrics.avgQueueSize),
		}
		if err := writer.Write(values); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

var errTimestampMovedBackwards = errors.New("snapshot timestamp moved backwards")

type diskMetrics struct {
	readIOPS, writeIOPS, discardIOPS, flushIOPS                             float64
	readMergedPerSec, writeMergedPerSec, discardMergedPerSec                float64
	readBytesPerSec, writeBytesPerSec, discardBytesPerSec                   float64
	readAwaitMillis, writeAwaitMillis, discardAwaitMillis, flushAwaitMillis float64
	avgReadRequestBytes, avgWriteRequestBytes, avgDiscardRequestBytes       float64
	ioUtilPct, avgQueueSize                                                 float64
}

func diskDeviceMetrics(current diskCounters, previous *diskCounters, seconds float64) diskMetrics {
	if previous == nil || seconds <= 0 {
		return diskMetrics{}
	}
	delta := func(cur, prev uint64) uint64 {
		if cur < prev {
			return 0
		}
		return cur - prev
	}
	reads := delta(current.readsCompleted, previous.readsCompleted)
	writes := delta(current.writesCompleted, previous.writesCompleted)
	discards := delta(current.discardsCompleted, previous.discardsCompleted)
	flushes := delta(current.flushesCompleted, previous.flushesCompleted)
	readBytes := delta(current.readBytes, previous.readBytes)
	writeBytes := delta(current.writeBytes, previous.writeBytes)
	discardBytes := delta(current.discardBytes, previous.discardBytes)
	perSecond := func(value uint64) float64 { return float64(value) / seconds }
	perOperation := func(value, operations uint64) float64 {
		if operations == 0 {
			return 0
		}
		return float64(value) / float64(operations)
	}
	elapsedMillis := seconds * 1000
	return diskMetrics{
		readIOPS: perSecond(reads), writeIOPS: perSecond(writes), discardIOPS: perSecond(discards), flushIOPS: perSecond(flushes),
		readMergedPerSec: perSecond(delta(current.readsMerged, previous.readsMerged)), writeMergedPerSec: perSecond(delta(current.writesMerged, previous.writesMerged)), discardMergedPerSec: perSecond(delta(current.discardsMerged, previous.discardsMerged)),
		readBytesPerSec: perSecond(readBytes), writeBytesPerSec: perSecond(writeBytes), discardBytesPerSec: perSecond(discardBytes),
		readAwaitMillis: perOperation(delta(current.readTimeMillis, previous.readTimeMillis), reads), writeAwaitMillis: perOperation(delta(current.writeTimeMillis, previous.writeTimeMillis), writes), discardAwaitMillis: perOperation(delta(current.discardTimeMillis, previous.discardTimeMillis), discards), flushAwaitMillis: perOperation(delta(current.flushTimeMillis, previous.flushTimeMillis), flushes),
		avgReadRequestBytes: perOperation(readBytes, reads), avgWriteRequestBytes: perOperation(writeBytes, writes), avgDiscardRequestBytes: perOperation(discardBytes, discards),
		ioUtilPct:    float64(delta(current.ioTimeMillis, previous.ioTimeMillis)) * 100 / elapsedMillis,
		avgQueueSize: float64(delta(current.weightedIOTimeMillis, previous.weightedIOTimeMillis)) / elapsedMillis,
	}
}

func addDiskCounters(total, value diskCounters) diskCounters {
	total.readsCompleted += value.readsCompleted
	total.readsMerged += value.readsMerged
	total.readBytes += value.readBytes
	total.readTimeMillis += value.readTimeMillis
	total.writesCompleted += value.writesCompleted
	total.writesMerged += value.writesMerged
	total.writeBytes += value.writeBytes
	total.writeTimeMillis += value.writeTimeMillis
	total.ioInProgress += value.ioInProgress
	total.ioTimeMillis += value.ioTimeMillis
	total.weightedIOTimeMillis += value.weightedIOTimeMillis
	total.discardsCompleted += value.discardsCompleted
	total.discardsMerged += value.discardsMerged
	total.discardBytes += value.discardBytes
	total.discardTimeMillis += value.discardTimeMillis
	total.flushesCompleted += value.flushesCompleted
	total.flushTimeMillis += value.flushTimeMillis
	return total
}
