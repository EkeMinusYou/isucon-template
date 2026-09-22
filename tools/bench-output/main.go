package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"
)

// The log format is contest specific and declared outside this program. The
// markers written downstream (SCORE, BENCHMARK_PASS, BENCHMARK_FAIL) are not.
type patternDeclaration struct {
	Score string `json:"score"`
	Pass  string `json:"pass"`
	Fail  string `json:"fail"`
}

type patterns struct {
	score *regexp.Regexp
	pass  *regexp.Regexp
	fail  *regexp.Regexp
}

func compilePattern(name, expression string, captures int) (*regexp.Regexp, error) {
	if expression == "" {
		return nil, nil
	}
	compiled, err := regexp.Compile(expression)
	if err != nil {
		return nil, fmt.Errorf("%s pattern: %w", name, err)
	}
	if compiled.NumSubexp() < captures {
		return nil, fmt.Errorf("%s pattern requires %d capture group(s)", name, captures)
	}
	return compiled, nil
}

func loadPatterns(filename string) (patterns, error) {
	body, err := os.ReadFile(filename)
	if err != nil {
		return patterns{}, fmt.Errorf("cannot read benchmark log format: %w", err)
	}
	var declaration patternDeclaration
	if err := json.Unmarshal(body, &declaration); err != nil {
		return patterns{}, fmt.Errorf("cannot parse benchmark log format: %w", err)
	}
	var result patterns
	if result.score, err = compilePattern("score", declaration.Score, 1); err != nil {
		return patterns{}, err
	}
	if result.pass, err = compilePattern("pass", declaration.Pass, 0); err != nil {
		return patterns{}, err
	}
	if result.fail, err = compilePattern("fail", declaration.Fail, 0); err != nil {
		return patterns{}, err
	}
	if result.score == nil && result.pass == nil && result.fail == nil {
		return patterns{}, fmt.Errorf("%s declares no pattern", filename)
	}
	return result, nil
}

// Forward every byte immediately while retaining only bounded lines for parsing.
type resultWriter struct {
	out       io.Writer
	patterns  patterns
	line      []byte
	oversized bool
	score     string
	passed    bool
	failed    bool
}

func (w *resultWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	for _, b := range p[:n] {
		if b == '\n' {
			w.finishLine()
		} else if !w.oversized {
			if len(w.line) == 64*1024 {
				w.oversized = true
				w.line = w.line[:0]
			} else {
				w.line = append(w.line, b)
			}
		}
	}
	return n, err
}

func (w *resultWriter) finishLine() {
	if !w.oversized {
		if w.patterns.score != nil {
			if match := w.patterns.score.FindSubmatch(w.line); match != nil {
				w.score = string(match[1])
			}
		}
		// A failure anywhere in the log outranks a later success marker.
		if w.patterns.fail != nil && w.patterns.fail.Match(w.line) {
			w.failed = true
		} else if w.patterns.pass != nil && w.patterns.pass.Match(w.line) {
			w.passed = true
		}
	}
	w.line = w.line[:0]
	w.oversized = false
}

func (w *resultWriter) markers() []byte {
	w.finishLine()
	var output bytes.Buffer
	if w.score != "" {
		fmt.Fprintln(&output, "SCORE: "+w.score)
	}
	switch {
	case w.failed:
		fmt.Fprintln(&output, "BENCHMARK_FAIL")
	case w.passed:
		fmt.Fprintln(&output, "BENCHMARK_PASS")
	}
	return output.Bytes()
}

func run(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("bench-output", flag.ContinueOnError)
	fs.SetOutput(errOut)
	patternsPath := fs.String("patterns", "tools/contest/bench-patterns.json", "benchmark log format declaration")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	command := fs.Args()
	if len(command) == 0 {
		fmt.Fprintln(errOut, "usage: bench-output [-patterns file] command [arguments...]")
		return 2
	}
	declared, err := loadPatterns(*patternsPath)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	cmd := exec.Command(command[0], command[1:]...)
	writer := &resultWriter{out: out, patterns: declared}
	cmd.Stdout, cmd.Stderr = writer, writer
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(errOut, err)
		return 127
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case sig := <-signals:
				_ = cmd.Process.Signal(sig)
			case <-done:
				return
			}
		}
	}()
	err = cmd.Wait()
	if markers := writer.markers(); len(markers) > 0 {
		if _, writeErr := fmt.Fprintf(out, "\n%s", markers); writeErr != nil {
			fmt.Fprintln(errOut, writeErr)
			return 1
		}
	}
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exit.ExitCode()
	}
	fmt.Fprintln(errOut, err)
	return 1
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
