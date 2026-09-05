package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

type commandExecutor interface {
	Run(name string, args []string, stdin string) ([]byte, error)
}

type osExecutor struct{}

func (osExecutor) Run(name string, args []string, stdin string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}

type deployRunner struct {
	sshUser  string
	sshOpts  []string
	roles    map[string][]string
	vars     map[string]string
	dryRun   bool
	parallel int
	exec     commandExecutor
}

type uploadJob struct {
	u      upload
	host   string
	local  string
	remote string
}

type activationJob struct {
	a      activation
	host   string
	script string
}

type jobResult struct {
	label    string
	host     string
	duration time.Duration
	output   string
	err      error
}

func (r *deployRunner) apply(d deployment) error {
	uploads, err := r.uploadJobs(d.Uploads)
	if err != nil {
		return err
	}
	activations, err := r.activationJobs(d.Activations)
	if err != nil {
		return err
	}

	if r.dryRun {
		for _, j := range uploads {
			fmt.Printf("[dry-run] %s\n", formatCommand("rsync", r.rsyncArgs(j)...))
		}
		for _, j := range activations {
			args := append(r.sshBase(), r.sshTarget(j.host), "sh -s")
			fmt.Printf("[dry-run] %s <<'EOF'\n%sEOF\n", formatCommand("ssh", args...), j.script)
		}
		return nil
	}

	fmt.Printf("upload phase: %d jobs\n", len(uploads))
	results := runParallel(uploads, r.parallel, r.runUpload)
	if err := reportResults(results); err != nil {
		return fmt.Errorf("upload phase failed; activation was not started: %w", err)
	}

	if len(activations) != 0 {
		fmt.Printf("activation phase: %d jobs\n", len(activations))
		results = runParallel(activations, r.parallel, r.runActivation)
		if err := reportResults(results); err != nil {
			return fmt.Errorf("activation phase failed: %w", err)
		}
	}
	return nil
}

func (r *deployRunner) uploadJobs(uploads []upload) ([]uploadJob, error) {
	var jobs []uploadJob
	for _, u := range uploads {
		hosts, ok := r.roles[u.Role]
		if !ok || len(hosts) == 0 {
			return nil, fmt.Errorf("upload %q が指す role %q のホストが指定されていません", u.Label, u.Role)
		}
		for _, host := range hosts {
			local, err := (expander{host: host, vars: r.vars}).expand(u.Local)
			if err != nil {
				return nil, fmt.Errorf("upload %q local: %w", u.Label, err)
			}
			if err := validateLocalPath(local); err != nil {
				return nil, fmt.Errorf("upload %q: %w", u.Label, err)
			}
			if _, err := os.Stat(strings.TrimSuffix(local, "/")); err != nil {
				return nil, fmt.Errorf("upload %q の local path を確認できません: %w", u.Label, err)
			}
			remote, err := (expander{host: host, vars: r.vars}).expand(u.Remote)
			if err != nil {
				return nil, fmt.Errorf("upload %q remote: %w", u.Label, err)
			}
			if err := validateRemotePath(remote, u.Delete); err != nil {
				return nil, fmt.Errorf("upload %q: %w", u.Label, err)
			}
			jobs = append(jobs, uploadJob{u: u, host: host, local: local, remote: remote})
		}
	}
	return jobs, nil
}

func (r *deployRunner) activationJobs(activations []activation) ([]activationJob, error) {
	var jobs []activationJob
	for _, a := range activations {
		hosts, ok := r.roles[a.Role]
		if !ok || len(hosts) == 0 {
			return nil, fmt.Errorf("activation %q が指す role %q のホストが指定されていません", a.Label, a.Role)
		}
		for _, host := range hosts {
			script, err := (expander{host: host, vars: r.vars}).expand(a.Script)
			if err != nil {
				return nil, fmt.Errorf("activation %q: %w", a.Label, err)
			}
			jobs = append(jobs, activationJob{a: a, host: host, script: script})
		}
	}
	return jobs, nil
}

func validateRemotePath(remote string, delete bool) error {
	if !remotePathRE.MatchString(remote) {
		return fmt.Errorf("remote path が許可された絶対パスではありません: %q", remote)
	}
	clean := path.Clean(remote)
	if (strings.HasSuffix(remote, "/") && clean+"/" != remote) || (!strings.HasSuffix(remote, "/") && clean != remote) {
		return fmt.Errorf("remote path に正規化されていない要素があります: %q", remote)
	}
	allowed := remote == "/etc/sysctl.conf" ||
		strings.HasPrefix(remote, "/home/isucon/") ||
		strings.HasPrefix(remote, "/etc/systemd/system/") ||
		strings.HasPrefix(remote, "/etc/nginx/") ||
		strings.HasPrefix(remote, "/etc/mysql/") ||
		strings.HasPrefix(remote, "/usr/local/bin/")
	if !allowed {
		return fmt.Errorf("remote path はデプロイ対象の許可済みパスに限定されます: %q", remote)
	}
	if delete && !strings.HasSuffix(remote, "/") {
		return fmt.Errorf("--delete の remote path は / で終わる必要があります: %q", remote)
	}
	return nil
}

func (r *deployRunner) sshBase() []string {
	return append([]string{"-o", "RequestTTY=no", "-o", "LogLevel=ERROR"}, r.sshOpts...)
}

func (r *deployRunner) sshTarget(host string) string {
	return r.sshUser + "@" + host
}

func (r *deployRunner) rsyncArgs(j uploadJob) []string {
	ssh := append([]string{"ssh"}, r.sshBase()...)
	args := []string{"-az", "--rsync-path=sudo rsync", "-e", strings.Join(ssh, " ")}
	if j.u.Delete {
		args = append(args, "--delete")
	}
	for _, exclude := range j.u.Excludes {
		args = append(args, "--exclude="+exclude)
	}
	return append(args, j.local, r.sshTarget(j.host)+":"+j.remote)
}

func (r *deployRunner) runUpload(j uploadJob) jobResult {
	started := time.Now()
	out, err := r.exec.Run("rsync", r.rsyncArgs(j), "")
	if err != nil {
		err = fmt.Errorf("rsync: %w", err)
	}
	return jobResult{label: j.u.Label, host: j.host, duration: time.Since(started), output: strings.TrimSpace(string(out)), err: err}
}

func (r *deployRunner) runActivation(j activationJob) jobResult {
	started := time.Now()
	args := append(r.sshBase(), r.sshTarget(j.host), "sh -s")
	out, err := r.exec.Run("ssh", args, j.script)
	if err != nil {
		err = fmt.Errorf("ssh: %w", err)
	}
	return jobResult{label: j.a.Label, host: j.host, duration: time.Since(started), output: strings.TrimSpace(string(out)), err: err}
}

func runParallel[T any](jobs []T, limit int, fn func(T) jobResult) []jobResult {
	if limit < 1 {
		limit = 1
	}
	results := make([]jobResult, len(jobs))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job T) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = fn(job)
		}(i, job)
	}
	wg.Wait()
	return results
}

func reportResults(results []jobResult) error {
	var errs []error
	for _, result := range results {
		status := "ok"
		if result.err != nil {
			status = "failed"
			errs = append(errs, fmt.Errorf("[%s] %s: %w", result.host, result.label, result.err))
		}
		fmt.Printf("[%s] %s: %s (%s)\n", result.host, result.label, status, result.duration.Round(time.Millisecond))
		if result.output != "" {
			for _, line := range strings.Split(result.output, "\n") {
				fmt.Printf("[%s] %s\n", result.host, line)
			}
		}
	}
	return errors.Join(errs...)
}

func formatCommand(name string, args ...string) string {
	parts := append([]string{name}, args...)
	for i, part := range parts {
		if strings.ContainsAny(part, " \t\n'\"*?[]$;&|()<>\\") {
			parts[i] = fmt.Sprintf("%q", part)
		}
	}
	return strings.Join(parts, " ")
}

func sortedDeploymentNames(cfg *config) []string {
	names := make([]string, 0, len(cfg.Deployments))
	for name := range cfg.Deployments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedPlanNames(cfg *config) []string {
	names := make([]string, 0, len(cfg.Plans))
	for name := range cfg.Plans {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
