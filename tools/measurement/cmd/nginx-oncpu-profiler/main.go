package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type options struct {
	delay          time.Duration
	duration       time.Duration
	frequency      int
	maxWorkers     int
	maxSamples     int
	maxOutputBytes int64
	callGraph      string
	foldedOutput   string
	metadataOutput string
	rawOutput      string
}

type workerMetadata struct {
	PID       int    `json:"pid"`
	StartTime string `json:"start_time,omitempty"`
	Exe       string `json:"exe,omitempty"`
	BuildID   string `json:"build_id,omitempty"`
}

type metadata struct {
	Valid              bool             `json:"valid"`
	Error              string           `json:"error,omitempty"`
	StartedAt          time.Time        `json:"started_at"`
	EndedAt            time.Time        `json:"ended_at"`
	ElapsedSeconds     float64          `json:"elapsed_seconds"`
	Delay              string           `json:"delay"`
	Duration           string           `json:"duration"`
	Frequency          int              `json:"frequency_hz"`
	MaxWorkers         int              `json:"max_workers"`
	MaxSamples         int              `json:"max_samples"`
	MaxOutputBytes     int64            `json:"max_output_bytes"`
	CallGraph          string           `json:"call_graph"`
	Workers            []workerMetadata `json:"workers"`
	SampleCount        int              `json:"sample_count"`
	UnknownSampleCount int              `json:"unknown_sample_count"`
	LostSamples        int64            `json:"lost_samples"`
	RawBytes           int64            `json:"raw_bytes"`
	FoldedBytes        int64            `json:"folded_bytes"`
}

type foldedProfile struct {
	stacks         map[string]int
	samples        int
	unknownSamples int
}

var lostSamplesPattern = regexp.MustCompile(`(?i)lost\s+(\d+)\s+samples?`)

func main() {
	var opts options
	flag.DurationVar(&opts.delay, "delay", 0, "delay before capture")
	flag.DurationVar(&opts.duration, "duration", 60*time.Second, "capture duration")
	flag.IntVar(&opts.frequency, "frequency", 49, "sampling frequency in Hz")
	flag.IntVar(&opts.maxWorkers, "max-workers", 2, "maximum nginx workers to profile")
	flag.IntVar(&opts.maxSamples, "max-samples", 5880, "maximum folded samples")
	flag.Int64Var(&opts.maxOutputBytes, "max-output-bytes", 64<<20, "maximum raw and folded output size")
	flag.StringVar(&opts.callGraph, "call-graph", "dwarf,8192", "perf call graph mode and stack size")
	flag.StringVar(&opts.foldedOutput, "folded-output", "nginx-oncpu.folded", "folded stack output")
	flag.StringVar(&opts.metadataOutput, "metadata-output", "nginx-oncpu-metadata.json", "metadata output")
	flag.StringVar(&opts.rawOutput, "raw-output", "nginx-oncpu.data", "perf.data output")
	flag.Parse()

	result, err := run(opts)
	if err != nil {
		result.Error = err.Error()
		fmt.Fprintln(os.Stderr, err)
	}
	result.Valid = err == nil
	if writeErr := writeMetadata(opts.metadataOutput, result); writeErr != nil {
		fmt.Fprintln(os.Stderr, writeErr)
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
}

func run(opts options) (result metadata, err error) {
	result = metadata{
		StartedAt:      time.Now().UTC(),
		Delay:          opts.delay.String(),
		Duration:       opts.duration.String(),
		Frequency:      opts.frequency,
		MaxWorkers:     opts.maxWorkers,
		MaxSamples:     opts.maxSamples,
		MaxOutputBytes: opts.maxOutputBytes,
		CallGraph:      opts.callGraph,
	}
	defer func() {
		result.EndedAt = time.Now().UTC()
		result.ElapsedSeconds = result.EndedAt.Sub(result.StartedAt).Seconds()
	}()

	if err := validateOptions(opts); err != nil {
		return result, err
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	if opts.delay > 0 {
		timer := time.NewTimer(opts.delay)
		select {
		case sig := <-stop:
			timer.Stop()
			return result, fmt.Errorf("capture canceled during delay: %s", sig)
		case <-timer.C:
		}
	}

	workers, err := findNginxWorkers(opts.maxWorkers)
	if err != nil {
		return result, err
	}
	result.Workers = workers
	pids := make([]string, 0, len(workers))
	for _, worker := range workers {
		pids = append(pids, strconv.Itoa(worker.PID))
	}

	perfStderr, err := capturePerf(opts, strings.Join(pids, ","), stop)
	result.LostSamples = parseLostSamples(perfStderr)
	if err != nil {
		return result, err
	}
	result.RawBytes, err = boundedFileSize(opts.rawOutput, opts.maxOutputBytes)
	if err != nil {
		return result, err
	}

	profile, err := readPerfScript(opts.rawOutput, opts.maxSamples)
	if err != nil {
		return result, err
	}
	result.SampleCount = profile.samples
	result.UnknownSampleCount = profile.unknownSamples
	if profile.samples == 0 {
		return result, errors.New("perf captured no stack samples")
	}
	result.FoldedBytes, err = writeFolded(opts.foldedOutput, profile.stacks, opts.maxOutputBytes)
	if err != nil {
		return result, err
	}
	fmt.Fprintf(os.Stdout, "captured %d samples from %d nginx workers\n", profile.samples, len(workers))
	return result, nil
}

func validateOptions(opts options) error {
	if opts.delay < 0 || opts.duration <= 0 {
		return errors.New("delay must be non-negative and duration must be positive")
	}
	if opts.frequency <= 0 || opts.maxWorkers <= 0 || opts.maxSamples <= 0 || opts.maxOutputBytes <= 0 {
		return errors.New("frequency and safety limits must be positive")
	}
	if opts.callGraph == "" || opts.foldedOutput == "" || opts.metadataOutput == "" || opts.rawOutput == "" {
		return errors.New("all output paths are required")
	}
	return nil
}

func findNginxWorkers(limit int) ([]workerMetadata, error) {
	out, err := exec.Command("ps", "-eo", "pid=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	var workers []workerMetadata
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.Contains(strings.Join(fields[1:], " "), "nginx: worker process") {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		workers = append(workers, inspectWorker(pid))
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].PID < workers[j].PID })
	if len(workers) == 0 {
		return nil, errors.New("no nginx worker processes found")
	}
	if len(workers) > limit {
		workers = workers[:limit]
	}
	return workers, nil
}

func inspectWorker(pid int) workerMetadata {
	worker := workerMetadata{PID: pid}
	if stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		if closeParen := bytes.LastIndexByte(stat, ')'); closeParen >= 0 {
			fields := strings.Fields(string(stat[closeParen+1:]))
			if len(fields) > 19 {
				worker.StartTime = fields[19]
			}
		}
	}
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		worker.Exe = exe
		worker.BuildID = readBuildID(exe)
	}
	return worker
}

func readBuildID(exe string) string {
	out, err := exec.Command("readelf", "-n", exe).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if before, after, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && before == "Build ID" {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func capturePerf(opts options, pids string, stop <-chan os.Signal) (string, error) {
	args := []string{"-n", "perf", "record", "-F", strconv.Itoa(opts.frequency), "--call-graph", opts.callGraph, "-p", pids, "-o", opts.rawOutput}
	cmd := exec.Command("sudo", args...)
	// sudo starts perf as a child process. Keep both in one dedicated process
	// group so SIGINT reaches perf and lets it finalize perf.data before parsing.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return stderr.String(), fmt.Errorf("start perf record: %w", err)
	}

	timer := time.NewTimer(opts.duration)
	canceled := false
	select {
	case <-timer.C:
	case <-stop:
		canceled = true
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return stderr.String(), fmt.Errorf("stop perf record: %w", err)
	}
	err := cmd.Wait()
	if canceled {
		return stderr.String(), errors.New("capture canceled")
	}
	if err != nil && !isExpectedPerfInterrupt(err) {
		return stderr.String(), fmt.Errorf("perf record: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stderr.String(), nil
}

func isExpectedPerfInterrupt(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	if exitErr.ExitCode() == 130 {
		return true
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGINT
}

func readPerfScript(rawPath string, maxSamples int) (foldedProfile, error) {
	cmd := exec.Command("sudo", "-n", "perf", "script", "-i", rawPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return foldedProfile{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return foldedProfile{}, fmt.Errorf("start perf script: %w", err)
	}
	profile, parseErr := parsePerfScript(stdout, maxSamples)
	if parseErr != nil {
		_ = stdout.Close()
	}
	waitErr := cmd.Wait()
	if parseErr != nil {
		return foldedProfile{}, parseErr
	}
	if waitErr != nil {
		return foldedProfile{}, fmt.Errorf("perf script: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return profile, nil
}

func parsePerfScript(reader io.Reader, maxSamples int) (foldedProfile, error) {
	profile := foldedProfile{stacks: make(map[string]int)}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	var command string
	var frames []string
	flush := func() error {
		if command == "" {
			frames = frames[:0]
			return nil
		}
		profile.samples++
		if profile.samples > maxSamples {
			return fmt.Errorf("sample limit exceeded: %d", maxSamples)
		}
		unknown := false
		stack := make([]string, 0, len(frames)+1)
		stack = append(stack, sanitizeFrame(command))
		for i := len(frames) - 1; i >= 0; i-- {
			frame := sanitizeFrame(frames[i])
			if frame == "unknown" || strings.Contains(frame, "[unknown]") {
				unknown = true
			}
			stack = append(stack, frame)
		}
		if unknown {
			profile.unknownSamples++
		}
		profile.stacks[strings.Join(stack, ";")]++
		command = ""
		frames = frames[:0]
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return profile, err
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if !looksLikeAddress(fields[0]) {
			if err := flush(); err != nil {
				return profile, err
			}
			command = fields[0]
			continue
		}
		if len(fields) >= 2 {
			frames = append(frames, fields[1])
		} else if len(fields) == 1 {
			frames = append(frames, "unknown")
		}
	}
	if err := scanner.Err(); err != nil {
		return profile, err
	}
	if err := flush(); err != nil {
		return profile, err
	}
	return profile, nil
}

func looksLikeAddress(value string) bool {
	value = strings.TrimPrefix(value, "0x")
	if value == "" {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func sanitizeFrame(frame string) string {
	frame = strings.TrimSpace(frame)
	frame = strings.ReplaceAll(frame, ";", ":")
	if frame == "" {
		return "unknown"
	}
	return frame
}

func writeFolded(path string, stacks map[string]int, maxBytes int64) (int64, error) {
	keys := make([]string, 0, len(stacks))
	for stack := range stacks {
		keys = append(keys, stack)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, stack := range keys {
		fmt.Fprintf(&b, "%s %d\n", stack, stacks[stack])
		if int64(b.Len()) > maxBytes {
			return 0, fmt.Errorf("folded output exceeds %d bytes", maxBytes)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return 0, err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return 0, err
	}
	return int64(b.Len()), nil
}

func boundedFileSize(path string, maxBytes int64) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if info.Size() > maxBytes {
		return 0, fmt.Errorf("raw output is %d bytes, limit is %d", info.Size(), maxBytes)
	}
	return info.Size(), nil
}

func parseLostSamples(stderr string) int64 {
	var total int64
	for _, match := range lostSamplesPattern.FindAllStringSubmatch(stderr, -1) {
		value, err := strconv.ParseInt(match[1], 10, 64)
		if err == nil {
			total += value
		}
	}
	return total
}

func writeMetadata(path string, result metadata) error {
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}
