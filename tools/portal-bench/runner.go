package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 64 << 20

var errBenchmarkFailed = errors.New("benchmark failed")

type retryableError struct{ message string }

func (e retryableError) Error() string { return e.message }

type runner struct {
	config         config
	client         *http.Client
	stdout, stderr io.Writer
	outputDir      string
}

func newRunner(c config, out, diagnostics io.Writer, outputDir string) *runner {
	jar, _ := cookiejar.New(nil)
	return &runner{config: c, stdout: out, stderr: diagnostics, outputDir: outputDir,
		client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func makeRequest(ctx context.Context, h httpAction, headers map[string]string, vars map[string]string) (*http.Request, error) {
	address, err := render(h.URL, vars)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("invalid request URL")
	}
	if !u.IsAbs() {
		base, err := url.Parse(vars["base_url"])
		if err != nil {
			return nil, fmt.Errorf("invalid base URL")
		}
		u = base.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("request URL must use HTTP(S) without userinfo")
	}
	text, err := render(h.Body, vars)
	if err != nil {
		return nil, err
	}
	body := []byte(text)
	if h.BodyBase64 != "" {
		encoded, err := render(h.BodyBase64, vars)
		if err != nil {
			return nil, err
		}
		body, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid body_base64")
		}
	}
	req, err := http.NewRequestWithContext(ctx, h.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP request")
	}
	merged := make(map[string]string)
	for name, value := range headers {
		merged[http.CanonicalHeaderKey(name)] = value
	}
	for name, value := range h.Headers {
		merged[http.CanonicalHeaderKey(name)] = value
	}
	for name, value := range merged {
		value, err = render(value, vars)
		if err != nil {
			return nil, err
		}
		if strings.ContainsAny(value, "\r\n") || strings.ContainsAny(name, "\r\n :\t") {
			return nil, fmt.Errorf("invalid HTTP header")
		}
		req.Header.Set(name, value)
	}
	return req, nil
}

type boundedBuffer struct{ buffer bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > maxResponseBytes {
		return 0, fmt.Errorf("command response exceeds size limit")
	}
	return b.buffer.Write(p)
}

func (r *runner) execute(ctx context.Context, a action, vars map[string]string) (map[string]string, error) {
	d, _ := positiveDuration(r.config.RequestTimeout)
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var body []byte
	if a.HTTP != nil {
		req, err := makeRequest(ctx, *a.HTTP, r.config.Headers, vars)
		if err != nil {
			return nil, err
		}
		res, err := r.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, retryableError{"request timed out or was cancelled"}
			}
			return nil, retryableError{"HTTP request failed"}
		}
		defer res.Body.Close()
		if res.StatusCode == 429 || res.StatusCode >= 500 {
			return nil, retryableError{fmt.Sprintf("HTTP status %d", res.StatusCode)}
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP status %d", res.StatusCode)
		}
		body, err = io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
		if err != nil {
			return nil, retryableError{"HTTP response read failed"}
		}
		if len(body) > maxResponseBytes {
			return nil, fmt.Errorf("HTTP response exceeds size limit")
		}
	} else {
		args := make([]string, len(a.Command))
		for i, arg := range a.Command {
			resolved, err := render(arg, vars)
			if err != nil {
				return nil, err
			}
			args[i] = resolved
		}
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Env = os.Environ()
		for name, value := range a.Env {
			resolved, err := render(value, vars)
			if err != nil {
				return nil, err
			}
			cmd.Env = append(cmd.Env, name+"="+resolved)
		}
		var buffer boundedBuffer
		cmd.Stdout = &buffer
		// A custom command owns its diagnostic output; it must not print secrets.
		cmd.Stderr = r.stderr
		cmd.WaitDelay = time.Second
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("configured command failed")
		}
		body = buffer.buffer.Bytes()
	}
	return extract(body, a.Response)
}

func waitInterval(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *runner) saveLog(log string) error {
	if r.outputDir == "" {
		return nil
	}
	path := filepath.Join(r.outputDir, "benchmark-result.txt")
	return os.WriteFile(path, []byte(log), 0o600)
}

func joinedLog(o outcome, fields map[string]string) string {
	var b strings.Builder
	for _, name := range o.Logs {
		value := fields[name]
		if value == "" {
			continue
		}
		b.WriteString(value)
		if !strings.HasSuffix(value, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (r *runner) run(ctx context.Context, jobID string) error {
	if err := r.config.validate(); err != nil {
		return err
	}
	vars, err := r.config.variables()
	if err != nil {
		return err
	}
	if r.outputDir != "" {
		if err := os.MkdirAll(r.outputDir, 0o755); err != nil {
			return err
		}
		// Verify writability without truncating a previously retrieved log.
		file, err := os.OpenFile(filepath.Join(r.outputDir, "benchmark-result.txt"), os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	d, _ := positiveDuration(r.config.Timeout)
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	interval, _ := positiveDuration(r.config.PollInterval)
	var log string
	defer func() {
		if log != "" {
			fmt.Fprint(r.stdout, log)
		}
	}()
	if jobID == "" {
		fields, err := r.execute(ctx, r.config.Start, vars)
		if err != nil {
			return fmt.Errorf("start failed (not retried; check portal before submitting again): %w", err)
		}
		for name, value := range fields {
			vars[name] = value
		}
		jobID = vars["job_id"]
	}
	if jobID == "" || strings.ContainsAny(jobID, "\r\n") {
		return fmt.Errorf("empty or invalid job_id")
	}
	vars["job_id"] = jobID
	fmt.Fprintf(r.stderr, "PORTAL_JOB_ID: %s\n", jobID)
	previousStatus := ""
	retries := 0
	for {
		fields, err := r.execute(ctx, r.config.Poll, vars)
		if err != nil {
			var temporary retryableError
			if ctx.Err() == nil && errors.As(err, &temporary) && retries < r.config.PollRetries {
				retries++
				fmt.Fprintf(r.stderr, "portal poll retry %d: %s\n", retries, temporary.message)
				if err := waitInterval(ctx, interval); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("poll job %s: %w", jobID, err)
		}
		retries = 0
		if got, ok := fields["job_id"]; ok && got != jobID {
			return fmt.Errorf("poll returned a different job_id")
		}
		for name, value := range fields {
			vars[name] = value
		}
		status := fields[r.config.Outcome.Status]
		if status != previousStatus {
			fmt.Fprintf(r.stderr, "PORTAL_STATUS: %s\n", strconv.Quote(status))
			previousStatus = status
		}
		log = joinedLog(r.config.Outcome, vars)
		if err := r.saveLog(log); err != nil {
			return err
		}
		o := r.config.Outcome
		failed := contains(o.FailedValues, status)
		if contains(o.DoneValues, status) || failed {
			if r.config.Result != nil {
				result, err := r.execute(ctx, *r.config.Result, vars)
				if err != nil {
					return fmt.Errorf("result: %w", err)
				}
				if got, ok := result["job_id"]; ok && got != jobID {
					return fmt.Errorf("result returned a different job_id")
				}
				for name, value := range result {
					vars[name] = value
				}
				log = joinedLog(o, vars)
				if err := r.saveLog(log); err != nil {
					return err
				}
			}
			// Emit logs before normalized markers, without duplicating snapshots.
			fmt.Fprint(r.stdout, log)
			log = ""
			if score := vars[o.Score]; o.Score != "" && score != "" {
				n, err := strconv.ParseFloat(score, 64)
				if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || strings.ContainsAny(score, "\r\n") {
					return fmt.Errorf("invalid score")
				}
				fmt.Fprintf(r.stdout, "SCORE: %s\n", score)
			}
			if o.Pass != "" {
				failed = failed || contains(o.FailValues, vars[o.Pass])
			}
			if failed {
				fmt.Fprintln(r.stdout, "BENCHMARK_FAIL")
				return errBenchmarkFailed
			}
			if o.Pass != "" && contains(o.PassValues, vars[o.Pass]) {
				fmt.Fprintln(r.stdout, "BENCHMARK_PASS")
			}
			return nil
		}
		if err := waitInterval(ctx, interval); err != nil {
			return fmt.Errorf("waiting for job %s: %w", jobID, err)
		}
	}
}
