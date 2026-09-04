package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	sectorSizeBytes = 512
)

type cpuCounters struct {
	user   uint64
	system uint64
	idle   uint64
	iowait uint64
	steal  uint64
}

func (c cpuCounters) total() uint64 {
	return c.user + c.system + c.idle + c.iowait + c.steal
}

type netCounters struct {
	rxBytes   uint64
	txBytes   uint64
	rxPackets uint64
	txPackets uint64
	rxErrors  uint64
	txErrors  uint64
	rxDrops   uint64
	txDrops   uint64
}

type diskCounters struct {
	readsCompleted       uint64
	readsMerged          uint64
	readBytes            uint64
	readTimeMillis       uint64
	writesCompleted      uint64
	writesMerged         uint64
	writeBytes           uint64
	writeTimeMillis      uint64
	ioInProgress         uint64
	ioTimeMillis         uint64
	weightedIOTimeMillis uint64
	discardsCompleted    uint64
	discardsMerged       uint64
	discardBytes         uint64
	discardTimeMillis    uint64
	flushesCompleted     uint64
	flushTimeMillis      uint64
}

type pressure struct {
	someAvg10 float64
	fullAvg10 float64
}

type snapshot struct {
	at              time.Time
	cpu             cpuCounters
	contextSwitches uint64
	interrupts      uint64
	procsRunning    uint64
	procsBlocked    uint64

	memTotal     uint64
	memAvailable uint64
	memFree      uint64
	buffers      uint64
	cached       uint64
	swapTotal    uint64
	swapFree     uint64
	swapInBytes  uint64
	swapOutBytes uint64

	load1  float64
	load5  float64
	load15 float64

	disk  diskCounters
	disks map[string]diskCounters
	net   netCounters

	cpuPressure    pressure
	memoryPressure pressure
	ioPressure     pressure
}

type rates struct {
	contextSwitches float64
	interrupts      float64
	swapInBytes     float64
	swapOutBytes    float64
	diskReadBytes   float64
	diskWriteBytes  float64
	diskIOMillis    float64
	netRxBytes      float64
	netTxBytes      float64
	netRxPackets    float64
	netTxPackets    float64
	netRxErrors     float64
	netTxErrors     float64
	netRxDrops      float64
	netTxDrops      float64
}

func main() {
	interval := flag.Duration("interval", time.Second, "sampling interval")
	outputPath := flag.String("output", "-", "TSV output path, or - for stdout")
	serviceOutputPath := flag.String("service-output", "", "service metrics TSV output path, or empty to disable")
	diskOutputPath := flag.String("disk-output", "", "per-device disk metrics TSV output path, or empty to disable")
	taskOutputPath := flag.String("task-output", "", "blocked task TSV output path, or empty to disable")
	taskInterval := flag.Duration("task-interval", 100*time.Millisecond, "blocked task sampling interval")
	services := flag.String("services", strings.Join(defaultServiceNames, ","), "comma-separated systemd service names")
	cgroupRoot := flag.String("cgroup-root", "/sys/fs/cgroup", "cgroup v2 mount path")
	flag.Parse()

	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "interval must be greater than zero")
		os.Exit(2)
	}
	if *taskInterval <= 0 {
		fmt.Fprintln(os.Stderr, "task-interval must be greater than zero")
		os.Exit(2)
	}

	output := io.Writer(os.Stdout)
	var outputFile *os.File
	if *outputPath != "-" {
		var err error
		outputFile, err = os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open output: %v\n", err)
			os.Exit(1)
		}
		defer outputFile.Close()
		output = outputFile
	}

	var serviceOutput io.Writer
	var serviceOutputFile *os.File
	if *serviceOutputPath != "" {
		var err error
		serviceOutputFile, err = os.Create(*serviceOutputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open service output: %v\n", err)
			os.Exit(1)
		}
		defer serviceOutputFile.Close()
		serviceOutput = serviceOutputFile
	}

	var diskOutput io.Writer
	var diskOutputFile *os.File
	if *diskOutputPath != "" {
		var err error
		diskOutputFile, err = os.Create(*diskOutputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open disk output: %v\n", err)
			os.Exit(1)
		}
		defer diskOutputFile.Close()
		diskOutput = diskOutputFile
	}

	var taskOutput io.Writer
	var taskOutputFile *os.File
	if *taskOutputPath != "" {
		var err error
		taskOutputFile, err = os.Create(*taskOutputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open task output: %v\n", err)
			os.Exit(1)
		}
		defer taskOutputFile.Close()
		taskOutput = taskOutputFile
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var taskWG sync.WaitGroup
	taskErr := make(chan error, 1)
	if taskOutput != nil {
		taskWG.Add(1)
		go func() {
			defer taskWG.Done()
			taskErr <- collectBlockedTasks(ctx, taskOutput, "/proc", *taskInterval)
		}()
	}
	if err := collect(ctx, output, serviceOutput, diskOutput, parseServiceNames(*services), *cgroupRoot, *interval); err != nil {
		stop()
		taskWG.Wait()
		fmt.Fprintf(os.Stderr, "collect proc metrics: %v\n", err)
		os.Exit(1)
	}
	taskWG.Wait()
	if taskOutput != nil {
		if err := <-taskErr; err != nil {
			fmt.Fprintf(os.Stderr, "collect blocked tasks: %v\n", err)
			os.Exit(1)
		}
	}
}

func collect(ctx context.Context, output io.Writer, serviceOutput io.Writer, diskOutput io.Writer, serviceNames []string, cgroupRoot string, interval time.Duration) error {
	diskDevices := listBlockDevices()
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	writer.UseCRLF = false
	if err := writer.Write(header()); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	var serviceWriter *csv.Writer
	var services *serviceCollector
	if serviceOutput != nil {
		serviceWriter = csv.NewWriter(serviceOutput)
		serviceWriter.Comma = '\t'
		serviceWriter.UseCRLF = false
		if err := serviceWriter.Write(serviceHeader()); err != nil {
			return err
		}
		serviceWriter.Flush()
		if err := serviceWriter.Error(); err != nil {
			return err
		}
		services = newServiceCollector(cgroupRoot, serviceNames)
	}

	var diskWriter *csv.Writer
	if diskOutput != nil {
		diskWriter = csv.NewWriter(diskOutput)
		diskWriter.Comma = '\t'
		diskWriter.UseCRLF = false
		if err := diskWriter.Write(diskHeader()); err != nil {
			return err
		}
		diskWriter.Flush()
		if err := diskWriter.Error(); err != nil {
			return err
		}
	}

	start := time.Now()
	previous, err := readSnapshot(start, diskDevices)
	if err != nil {
		return err
	}
	if err := writeRow(writer, 0, start, previous, nil); err != nil {
		return err
	}
	if diskWriter != nil {
		if err := writeDiskRows(diskWriter, 0, start, previous, nil); err != nil {
			return err
		}
	}
	previousServices := map[string]serviceSnapshot{}
	if services != nil {
		previousServices = services.read()
		if err := writeServiceRows(serviceWriter, 0, start, previous, previousServices, nil, nil, services.names()); err != nil {
			return err
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var sampleIndex uint64
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			current, err := readSnapshot(time.Now(), diskDevices)
			if err != nil {
				return err
			}
			sampleIndex++
			if err := writeRow(writer, sampleIndex, start, current, &previous); err != nil {
				return err
			}
			if diskWriter != nil {
				if err := writeDiskRows(diskWriter, sampleIndex, start, current, &previous); err != nil {
					return err
				}
			}
			currentServices := map[string]serviceSnapshot{}
			if services != nil {
				currentServices = services.read()
				if err := writeServiceRows(serviceWriter, sampleIndex, start, current, currentServices, previousServices, &previous, services.names()); err != nil {
					return err
				}
			}
			previous = current
			previousServices = currentServices
		}
	}
}

func header() []string {
	return []string{
		"sample",
		"timestamp",
		"elapsed_ms",
		"cpu_count",
		"cpu_busy_pct",
		"cpu_nonidle_pct",
		"cpu_user_pct",
		"cpu_system_pct",
		"cpu_iowait_pct",
		"cpu_steal_pct",
		"cpu_idle_pct",
		"load1",
		"load5",
		"load15",
		"procs_running",
		"procs_blocked",
		"context_switches_per_sec",
		"interrupts_per_sec",
		"mem_total_bytes",
		"mem_used_bytes",
		"mem_available_bytes",
		"mem_free_bytes",
		"buffers_bytes",
		"cached_bytes",
		"swap_total_bytes",
		"swap_used_bytes",
		"swap_in_bytes_per_sec",
		"swap_out_bytes_per_sec",
		"disk_read_bytes_per_sec",
		"disk_write_bytes_per_sec",
		"disk_io_in_progress",
		"disk_io_time_millis_per_sec",
		"net_rx_bytes_per_sec",
		"net_tx_bytes_per_sec",
		"net_rx_packets_per_sec",
		"net_tx_packets_per_sec",
		"net_rx_errors_per_sec",
		"net_tx_errors_per_sec",
		"net_rx_drops_per_sec",
		"net_tx_drops_per_sec",
		"cpu_pressure_some_avg10",
		"memory_pressure_some_avg10",
		"memory_pressure_full_avg10",
		"io_pressure_some_avg10",
		"io_pressure_full_avg10",
	}
}

func writeRow(writer *csv.Writer, sampleIndex uint64, start time.Time, current snapshot, previous *snapshot) error {
	cpu := cpuPercentages(current.cpu, previous)
	r := counterRates(current, previous)
	intervalSeconds := 0.0
	if previous != nil {
		intervalSeconds = current.at.Sub(previous.at).Seconds()
	}

	memUsed := uint64(0)
	if current.memTotal > current.memAvailable {
		memUsed = current.memTotal - current.memAvailable
	}
	swapUsed := uint64(0)
	if current.swapTotal > current.swapFree {
		swapUsed = current.swapTotal - current.swapFree
	}

	values := []string{
		strconv.FormatUint(sampleIndex, 10),
		current.at.UTC().Format(time.RFC3339Nano),
		strconv.FormatInt(current.at.Sub(start).Milliseconds(), 10),
		strconv.Itoa(runtime.NumCPU()),
		formatFloat(cpu.busy),
		formatFloat(cpu.nonIdle),
		formatFloat(cpu.user),
		formatFloat(cpu.system),
		formatFloat(cpu.iowait),
		formatFloat(cpu.steal),
		formatFloat(cpu.idle),
		formatFloat(current.load1),
		formatFloat(current.load5),
		formatFloat(current.load15),
		strconv.FormatUint(current.procsRunning, 10),
		strconv.FormatUint(current.procsBlocked, 10),
		formatFloat(r.contextSwitches),
		formatFloat(r.interrupts),
		strconv.FormatUint(current.memTotal, 10),
		strconv.FormatUint(memUsed, 10),
		strconv.FormatUint(current.memAvailable, 10),
		strconv.FormatUint(current.memFree, 10),
		strconv.FormatUint(current.buffers, 10),
		strconv.FormatUint(current.cached, 10),
		strconv.FormatUint(current.swapTotal, 10),
		strconv.FormatUint(swapUsed, 10),
		formatFloat(r.swapInBytes),
		formatFloat(r.swapOutBytes),
		formatFloat(r.diskReadBytes),
		formatFloat(r.diskWriteBytes),
		strconv.FormatUint(current.disk.ioInProgress, 10),
		formatFloat(r.diskIOMillis),
		formatFloat(r.netRxBytes),
		formatFloat(r.netTxBytes),
		formatFloat(r.netRxPackets),
		formatFloat(r.netTxPackets),
		formatFloat(r.netRxErrors),
		formatFloat(r.netTxErrors),
		formatFloat(r.netRxDrops),
		formatFloat(r.netTxDrops),
		formatFloat(current.cpuPressure.someAvg10),
		formatFloat(current.memoryPressure.someAvg10),
		formatFloat(current.memoryPressure.fullAvg10),
		formatFloat(current.ioPressure.someAvg10),
		formatFloat(current.ioPressure.fullAvg10),
	}

	if intervalSeconds < 0 {
		return errTimestampMovedBackwards
	}
	if err := writer.Write(values); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

type cpuPercent struct {
	busy    float64
	nonIdle float64
	user    float64
	system  float64
	iowait  float64
	steal   float64
	idle    float64
}

func cpuPercentages(current cpuCounters, previous *snapshot) cpuPercent {
	if previous == nil {
		return cpuPercent{}
	}
	previousCPU := previous.cpu
	totalDelta := current.total() - previousCPU.total()
	if current.total() < previousCPU.total() || totalDelta == 0 {
		return cpuPercent{}
	}
	percent := func(value uint64) float64 {
		return float64(value) * 100 / float64(totalDelta)
	}
	userDelta := (current.user - previousCPU.user)
	systemDelta := (current.system - previousCPU.system)
	iowaitDelta := (current.iowait - previousCPU.iowait)
	stealDelta := (current.steal - previousCPU.steal)
	idleDelta := (current.idle - previousCPU.idle)
	return cpuPercent{
		busy:    percent(userDelta + systemDelta + stealDelta),
		nonIdle: percent(userDelta + systemDelta + iowaitDelta + stealDelta),
		user:    percent(userDelta),
		system:  percent(systemDelta),
		iowait:  percent(iowaitDelta),
		steal:   percent(stealDelta),
		idle:    percent(idleDelta),
	}
}

func counterRates(current snapshot, previous *snapshot) rates {
	if previous == nil {
		return rates{}
	}
	seconds := current.at.Sub(previous.at).Seconds()
	if seconds <= 0 {
		return rates{}
	}
	return rates{
		contextSwitches: counterRate(current.contextSwitches, previous.contextSwitches, seconds),
		interrupts:      counterRate(current.interrupts, previous.interrupts, seconds),
		swapInBytes:     counterRate(current.swapInBytes, previous.swapInBytes, seconds),
		swapOutBytes:    counterRate(current.swapOutBytes, previous.swapOutBytes, seconds),
		diskReadBytes:   counterRate(current.disk.readBytes, previous.disk.readBytes, seconds),
		diskWriteBytes:  counterRate(current.disk.writeBytes, previous.disk.writeBytes, seconds),
		diskIOMillis:    counterRate(current.disk.ioTimeMillis, previous.disk.ioTimeMillis, seconds),
		netRxBytes:      counterRate(current.net.rxBytes, previous.net.rxBytes, seconds),
		netTxBytes:      counterRate(current.net.txBytes, previous.net.txBytes, seconds),
		netRxPackets:    counterRate(current.net.rxPackets, previous.net.rxPackets, seconds),
		netTxPackets:    counterRate(current.net.txPackets, previous.net.txPackets, seconds),
		netRxErrors:     counterRate(current.net.rxErrors, previous.net.rxErrors, seconds),
		netTxErrors:     counterRate(current.net.txErrors, previous.net.txErrors, seconds),
		netRxDrops:      counterRate(current.net.rxDrops, previous.net.rxDrops, seconds),
		netTxDrops:      counterRate(current.net.txDrops, previous.net.txDrops, seconds),
	}
}

func counterRate(current, previous uint64, seconds float64) float64 {
	if current < previous || seconds <= 0 {
		return 0
	}
	return float64(current-previous) / seconds
}

func readSnapshot(at time.Time, diskDevices map[string]struct{}) (snapshot, error) {
	cpu, contextSwitches, interrupts, _, procsBlocked, err := readProcStat()
	if err != nil {
		return snapshot{}, err
	}

	meminfo, err := readMeminfo()
	if err != nil {
		return snapshot{}, err
	}
	vmstat, err := readVMStat()
	if err != nil {
		return snapshot{}, err
	}
	load1, load5, load15, loadRunning, err := readLoadavg()
	if err != nil {
		return snapshot{}, err
	}
	net, err := readNetDev()
	if err != nil {
		return snapshot{}, err
	}
	disk, disks, err := readDiskstats(diskDevices)
	if err != nil {
		return snapshot{}, err
	}

	cpuPressure := readPressure("/proc/pressure/cpu")
	memoryPressure := readPressure("/proc/pressure/memory")
	ioPressure := readPressure("/proc/pressure/io")

	pageSize := uint64(os.Getpagesize())
	return snapshot{
		at:              at,
		cpu:             cpu,
		contextSwitches: contextSwitches,
		interrupts:      interrupts,
		procsRunning:    loadRunning,
		procsBlocked:    procsBlocked,
		memTotal:        meminfo["MemTotal"],
		memAvailable:    meminfo["MemAvailable"],
		memFree:         meminfo["MemFree"],
		buffers:         meminfo["Buffers"],
		cached:          meminfo["Cached"],
		swapTotal:       meminfo["SwapTotal"],
		swapFree:        meminfo["SwapFree"],
		swapInBytes:     vmstat["pswpin"] * pageSize,
		swapOutBytes:    vmstat["pswpout"] * pageSize,
		load1:           load1,
		load5:           load5,
		load15:          load15,
		disk:            disk,
		disks:           disks,
		net:             net,
		cpuPressure:     cpuPressure,
		memoryPressure:  memoryPressure,
		ioPressure:      ioPressure,
	}, nil
}

func readProcStat() (cpuCounters, uint64, uint64, uint64, uint64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuCounters{}, 0, 0, 0, 0, err
	}
	defer file.Close()
	return parseProcStat(file)
}

func parseProcStat(reader io.Reader) (cpuCounters, uint64, uint64, uint64, uint64, error) {
	var cpu cpuCounters
	var contextSwitches, interrupts, procsRunning, procsBlocked uint64
	var err error
	seenCPU := false
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "cpu":
			if len(fields) < 6 {
				return cpuCounters{}, 0, 0, 0, 0, errors.New("invalid cpu line")
			}
			values, parseErr := parseUintFields(fields[1:])
			if parseErr != nil {
				return cpuCounters{}, 0, 0, 0, 0, parseErr
			}
			cpu.user = values[0] + valueAt(values, 1)
			cpu.system = valueAt(values, 2) + valueAt(values, 5) + valueAt(values, 6)
			cpu.idle = valueAt(values, 3)
			cpu.iowait = valueAt(values, 4)
			cpu.steal = valueAt(values, 7)
			seenCPU = true
		case "ctxt":
			contextSwitches, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return cpuCounters{}, 0, 0, 0, 0, err
			}
		case "intr":
			interrupts, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return cpuCounters{}, 0, 0, 0, 0, err
			}
		case "procs_running":
			procsRunning, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return cpuCounters{}, 0, 0, 0, 0, err
			}
		case "procs_blocked":
			procsBlocked, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return cpuCounters{}, 0, 0, 0, 0, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return cpuCounters{}, 0, 0, 0, 0, err
	}
	if !seenCPU {
		return cpuCounters{}, 0, 0, 0, 0, errors.New("cpu line not found")
	}
	return cpu, contextSwitches, interrupts, procsRunning, procsBlocked, nil
}

func readMeminfo() (map[string]uint64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseMeminfo(file)
}

func parseMeminfo(reader io.Reader) (map[string]uint64, error) {
	values := make(map[string]uint64)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, err
		}
		if len(fields) >= 3 {
			switch fields[2] {
			case "kB":
				value *= 1024
			case "mB":
				value *= 1024 * 1024
			}
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func readVMStat() (map[string]uint64, error) {
	file, err := os.Open("/proc/vmstat")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := make(map[string]uint64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, err
		}
		if fields[0] == "pswpin" || fields[0] == "pswpout" {
			values[fields[0]] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func readLoadavg() (float64, float64, float64, uint64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 4 {
		return 0, 0, 0, 0, errors.New("invalid loadavg")
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	running := strings.Split(fields[3], "/")
	if len(running) != 2 {
		return 0, 0, 0, 0, errors.New("invalid running process count")
	}
	procsRunning, err := strconv.ParseUint(running[0], 10, 64)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return load1, load5, load15, procsRunning, nil
}

func readNetDev() (netCounters, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return netCounters{}, err
	}
	defer file.Close()
	return parseNetDev(file)
}

func parseNetDev(reader io.Reader) (netCounters, error) {
	var counters netCounters
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		_, values, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) < 16 {
			return netCounters{}, errors.New("invalid network statistics")
		}
		parsed, err := parseUintFields(fields)
		if err != nil {
			return netCounters{}, err
		}
		counters.rxBytes += parsed[0]
		counters.rxPackets += parsed[1]
		counters.rxErrors += parsed[2]
		counters.rxDrops += parsed[3]
		counters.txBytes += parsed[8]
		counters.txPackets += parsed[9]
		counters.txErrors += parsed[10]
		counters.txDrops += parsed[11]
	}
	if err := scanner.Err(); err != nil {
		return netCounters{}, err
	}
	return counters, nil
}

func listBlockDevices() map[string]struct{} {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil
	}
	devices := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		devices[entry.Name()] = struct{}{}
	}
	return devices
}

func readDiskstats(devices map[string]struct{}) (diskCounters, map[string]diskCounters, error) {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return diskCounters{}, nil, err
	}
	defer file.Close()
	return parseDiskstats(file, devices)
}

func parseDiskstats(reader io.Reader, devices map[string]struct{}) (diskCounters, map[string]diskCounters, error) {
	var counters diskCounters
	perDevice := make(map[string]diskCounters)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}
		if len(devices) > 0 {
			if _, ok := devices[fields[2]]; !ok {
				continue
			}
		}
		values, err := parseUintFields(fields[3:])
		if err != nil {
			return diskCounters{}, nil, err
		}
		device := diskCounters{
			readsCompleted: values[0], readsMerged: values[1], readBytes: values[2] * sectorSizeBytes, readTimeMillis: values[3],
			writesCompleted: values[4], writesMerged: values[5], writeBytes: values[6] * sectorSizeBytes, writeTimeMillis: values[7],
			ioInProgress: values[8], ioTimeMillis: values[9], weightedIOTimeMillis: values[10],
		}
		if len(values) >= 15 {
			device.discardsCompleted = values[11]
			device.discardsMerged = values[12]
			device.discardBytes = values[13] * sectorSizeBytes
			device.discardTimeMillis = values[14]
		}
		if len(values) >= 17 {
			device.flushesCompleted = values[15]
			device.flushTimeMillis = values[16]
		}
		perDevice[fields[2]] = device
		counters = addDiskCounters(counters, device)
	}
	if err := scanner.Err(); err != nil {
		return diskCounters{}, nil, err
	}
	return counters, perDevice, nil
}

func readPressure(path string) pressure {
	file, err := os.Open(path)
	if err != nil {
		return pressure{}
	}
	defer file.Close()
	return parsePressure(file)
}

func parsePressure(reader io.Reader) pressure {
	var result pressure
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		var target *float64
		switch fields[0] {
		case "some":
			target = &result.someAvg10
		case "full":
			target = &result.fullAvg10
		default:
			continue
		}
		for _, field := range fields[1:] {
			key, value, ok := strings.Cut(field, "=")
			if !ok || key != "avg10" {
				continue
			}
			parsed, err := strconv.ParseFloat(value, 64)
			if err == nil {
				*target = parsed
			}
		}
	}
	return result
}

func parseUintFields(fields []string) ([]uint64, error) {
	values := make([]uint64, len(fields))
	for index, field := range fields {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	return values, nil
}

func valueAt(values []uint64, index int) uint64 {
	if index < 0 || index >= len(values) {
		return 0
	}
	return values[index]
}

func formatFloat(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', 6, 64)
}
