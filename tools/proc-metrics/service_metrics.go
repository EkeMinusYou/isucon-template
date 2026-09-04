package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var defaultServiceNames = []string{"nginx.service", "mysql.service"}

type serviceTarget struct {
	name string
	path string
}

type serviceCollector struct {
	root    string
	targets []serviceTarget
}

type serviceCPUCounters struct {
	usageUsec  uint64
	userUsec   uint64
	systemUsec uint64
}

type serviceIOCounters struct {
	readBytes  uint64
	writeBytes uint64
}

type serviceSnapshot struct {
	available     bool
	cpu           serviceCPUCounters
	memoryCurrent uint64
	memoryPeak    uint64
	io            serviceIOCounters
	tasksCurrent  uint64
}

type serviceRates struct {
	cpuPct       float64
	cpuHostPct   float64
	cpuUserPct   float64
	cpuSystemPct float64
	ioReadBytes  float64
	ioWriteBytes float64
}

func parseServiceNames(value string) []string {
	seen := make(map[string]struct{})
	var names []string
	for _, field := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
		name := normalizeServiceName(field)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func normalizeServiceName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if !strings.HasSuffix(name, ".service") {
		return name + ".service"
	}
	return name
}

func newServiceCollector(root string, names []string) *serviceCollector {
	targets := make([]serviceTarget, 0, len(names))
	for _, name := range names {
		name = normalizeServiceName(name)
		if name == "" {
			continue
		}
		alreadyAdded := false
		for _, target := range targets {
			if target.name == name {
				alreadyAdded = true
				break
			}
		}
		if !alreadyAdded {
			targets = append(targets, serviceTarget{name: name})
		}
	}
	return &serviceCollector{root: root, targets: targets}
}

func (c *serviceCollector) names() []string {
	names := make([]string, len(c.targets))
	for index, target := range c.targets {
		names[index] = target.name
	}
	return names
}

func (c *serviceCollector) read() map[string]serviceSnapshot {
	values := make(map[string]serviceSnapshot, len(c.targets))
	for index := range c.targets {
		target := &c.targets[index]
		if target.path == "" || !isDirectory(target.path) {
			target.path = findCgroupPath(c.root, target.name)
		}

		value := serviceSnapshot{}
		if target.path != "" {
			var err error
			value, err = readServiceCgroup(target.path)
			if err != nil {
				// A service can be restarted while the sampler is running. Drop the
				// cached path so that the next sample resolves the new cgroup again.
				target.path = ""
				value = serviceSnapshot{}
			}
		}
		values[target.name] = value
	}
	return values
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func findCgroupPath(root, name string) string {
	if !isDirectory(root) {
		return ""
	}

	var candidates []string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() && path != root && entry.Name() == name {
			candidates = append(candidates, path)
			return fs.SkipDir
		}
		return nil
	})

	for _, candidate := range candidates {
		relative, err := filepath.Rel(root, candidate)
		if err == nil && strings.HasPrefix(relative, "system.slice"+string(os.PathSeparator)) {
			return candidate
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func readServiceCgroup(path string) (serviceSnapshot, error) {
	cpu, err := readServiceCPUStat(filepath.Join(path, "cpu.stat"))
	if err != nil {
		return serviceSnapshot{}, err
	}
	memoryCurrent, err := readUintFile(filepath.Join(path, "memory.current"))
	if err != nil {
		return serviceSnapshot{}, err
	}

	return serviceSnapshot{
		available:     true,
		cpu:           cpu,
		memoryCurrent: memoryCurrent,
		memoryPeak:    readOptionalUintFile(filepath.Join(path, "memory.peak")),
		io:            readOptionalServiceIO(filepath.Join(path, "io.stat")),
		tasksCurrent:  readOptionalUintFile(filepath.Join(path, "pids.current")),
	}, nil
}

func readServiceCPUStat(path string) (serviceCPUCounters, error) {
	file, err := os.Open(path)
	if err != nil {
		return serviceCPUCounters{}, err
	}
	defer file.Close()
	return parseServiceCPUStat(file)
}

func parseServiceCPUStat(reader io.Reader) (serviceCPUCounters, error) {
	var counters serviceCPUCounters
	seenUsage := false
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return serviceCPUCounters{}, err
		}
		switch fields[0] {
		case "usage_usec":
			counters.usageUsec = value
			seenUsage = true
		case "user_usec":
			counters.userUsec = value
		case "system_usec":
			counters.systemUsec = value
		}
	}
	if err := scanner.Err(); err != nil {
		return serviceCPUCounters{}, err
	}
	if !seenUsage {
		return serviceCPUCounters{}, errors.New("usage_usec not found in cpu.stat")
	}
	return counters, nil
}

func readUintFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

func readOptionalUintFile(path string) uint64 {
	value, err := readUintFile(path)
	if err != nil {
		return 0
	}
	return value
}

func readOptionalServiceIO(path string) serviceIOCounters {
	file, err := os.Open(path)
	if err != nil {
		return serviceIOCounters{}
	}
	defer file.Close()
	values, err := parseServiceIOStat(file)
	if err != nil {
		return serviceIOCounters{}
	}
	return values
}

func parseServiceIOStat(reader io.Reader) (serviceIOCounters, error) {
	var counters serviceIOCounters
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		for _, field := range fields[1:] {
			key, value, ok := strings.Cut(field, "=")
			if !ok || (key != "rbytes" && key != "wbytes") {
				continue
			}
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return serviceIOCounters{}, err
			}
			switch key {
			case "rbytes":
				counters.readBytes += parsed
			case "wbytes":
				counters.writeBytes += parsed
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return serviceIOCounters{}, err
	}
	return counters, nil
}

func serviceCounterRates(current serviceSnapshot, previous *serviceSnapshot, seconds float64) serviceRates {
	if previous == nil || !current.available || !previous.available || seconds <= 0 {
		return serviceRates{}
	}

	cpuPct := counterRate(current.cpu.usageUsec, previous.cpu.usageUsec, seconds) / 10000
	cpuUserPct := counterRate(current.cpu.userUsec, previous.cpu.userUsec, seconds) / 10000
	cpuSystemPct := counterRate(current.cpu.systemUsec, previous.cpu.systemUsec, seconds) / 10000
	cpuHostPct := cpuPct / float64(runtime.NumCPU())
	return serviceRates{
		cpuPct:       cpuPct,
		cpuHostPct:   cpuHostPct,
		cpuUserPct:   cpuUserPct,
		cpuSystemPct: cpuSystemPct,
		ioReadBytes:  counterRate(current.io.readBytes, previous.io.readBytes, seconds),
		ioWriteBytes: counterRate(current.io.writeBytes, previous.io.writeBytes, seconds),
	}
}

func serviceHeader() []string {
	return []string{
		"sample",
		"timestamp",
		"elapsed_ms",
		"service",
		"available",
		"cpu_pct",
		"cpu_host_pct",
		"cpu_user_pct",
		"cpu_system_pct",
		"memory_current_bytes",
		"memory_pct_of_host",
		"memory_peak_bytes",
		"io_read_bytes_per_sec",
		"io_write_bytes_per_sec",
		"tasks_current",
	}
}

func writeServiceRows(
	writer *csv.Writer,
	sampleIndex uint64,
	start time.Time,
	current snapshot,
	currentServices map[string]serviceSnapshot,
	previousServices map[string]serviceSnapshot,
	previous *snapshot,
	serviceNames []string,
) error {
	intervalSeconds := 0.0
	if previous != nil {
		intervalSeconds = current.at.Sub(previous.at).Seconds()
		if intervalSeconds < 0 {
			return errors.New("snapshot timestamp moved backwards")
		}
	}

	for _, name := range serviceNames {
		service := currentServices[name]
		var previousService *serviceSnapshot
		if previous != nil {
			value := previousServices[name]
			previousService = &value
		}
		rates := serviceCounterRates(service, previousService, intervalSeconds)

		memoryPct := 0.0
		if current.memTotal > 0 {
			memoryPct = float64(service.memoryCurrent) * 100 / float64(current.memTotal)
		}
		available := "0"
		if service.available {
			available = "1"
		}

		values := []string{
			strconv.FormatUint(sampleIndex, 10),
			current.at.UTC().Format(time.RFC3339Nano),
			strconv.FormatInt(current.at.Sub(start).Milliseconds(), 10),
			name,
			available,
			formatFloat(rates.cpuPct),
			formatFloat(rates.cpuHostPct),
			formatFloat(rates.cpuUserPct),
			formatFloat(rates.cpuSystemPct),
			strconv.FormatUint(service.memoryCurrent, 10),
			formatFloat(memoryPct),
			strconv.FormatUint(service.memoryPeak, 10),
			formatFloat(rates.ioReadBytes),
			formatFloat(rates.ioWriteBytes),
			strconv.FormatUint(service.tasksCurrent, 10),
		}
		if err := writer.Write(values); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
