package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"
)

func runCheckRoles(args []string) error {
	fs := flag.NewFlagSet("check-roles", flag.ContinueOnError)
	sshUser := fs.String("ssh-user", "ubuntu", "SSH user")
	sshOpts := fs.String("ssh-opts", "", "additional SSH options")
	_ = fs.String("run-state-file", "raw/current-run-id", "accepted for Taskfile common flags")
	parallel := fs.Int("parallel", 8, "maximum number of concurrent jobs")
	dryRun := fs.Bool("dry-run", false, "print checks without executing them")
	roles := roleValues{}
	vars := variableValues{}
	fs.Var(roles, "role", "role=host1,host2 (repeatable)")
	fs.Var(vars, "var", "name=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *parallel < 1 {
		return errors.New("-parallel must be at least 1")
	}
	if !nameRE.MatchString(*sshUser) {
		return fmt.Errorf("invalid SSH user: %q", *sshUser)
	}
	service := vars["service"]
	if service == "" || !nameRE.MatchString(service) {
		return errors.New("-var service=<systemd-unit> is required")
	}
	for _, role := range []string{"all", "app", "nginx", "mysql"} {
		if len(roles[role]) == 0 {
			return fmt.Errorf("role %q has no hosts", role)
		}
	}
	runner := newDeployRunner(*sshUser, *sshOpts, roles, vars, *dryRun, *parallel)
	return runner.checkRoles(service)
}

type roleCheckJob struct {
	host   string
	script string
}

func (r *deployRunner) checkRoles(appService string) error {
	jobs := make([]roleCheckJob, 0, len(r.roles["all"]))
	for _, host := range r.roles["all"] {
		script := roleCheckScript(appService,
			containsRoleHost(r.roles["app"], host),
			containsRoleHost(r.roles["nginx"], host),
			containsRoleHost(r.roles["mysql"], host))
		jobs = append(jobs, roleCheckJob{host: host, script: script})
	}
	if r.dryRun {
		for _, job := range jobs {
			args := append(r.sshBase(), r.sshTarget(job.host), "sh -s")
			fmt.Printf("[dry-run] %s <<'EOF'\n%sEOF\n", formatCommand("ssh", args...), job.script)
		}
		return nil
	}
	results := runParallel(jobs, r.parallel, func(job roleCheckJob) jobResult {
		started := time.Now()
		args := append(r.sshBase(), r.sshTarget(job.host), "sh -s")
		out, err := r.exec.Run("ssh", args, job.script)
		if err != nil {
			err = fmt.Errorf("ssh: %w", err)
		}
		return jobResult{label: "roles", host: job.host, duration: time.Since(started), output: strings.TrimSpace(string(out)), err: err}
	})
	return reportResults(results)
}

func containsRoleHost(hosts []string, target string) bool {
	for _, host := range hosts {
		if host == target {
			return true
		}
	}
	return false
}

func roleCheckScript(appService string, app, nginx, mysql bool) string {
	expected := func(active bool) string {
		if active {
			return "active"
		}
		return "inactive"
	}
	return fmt.Sprintf(`set -eu
check_unit() {
  unit=$1
  expected=$2
  actual=$(systemctl is-active "$unit" 2>/dev/null || true)
  if [ "$expected" = active ]; then
    [ "$actual" = active ] || { echo "$unit: expected active, got ${actual:-missing}" >&2; exit 1; }
  else
    [ "$actual" != active ] || { echo "$unit: expected inactive, got active" >&2; exit 1; }
  fi
}
check_unit '%s' %s
check_unit nginx %s
check_unit mysql %s
echo 'roles OK'
`, appService, expected(app), expected(nginx), expected(mysql))
}
