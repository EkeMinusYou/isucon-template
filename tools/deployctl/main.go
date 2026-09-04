// deployctl performs repository-declared deployments against role-based hosts.
// Taskfile.yml remains the source of truth for roles and the public entrypoint.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "deployctl: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage()
		return nil
	}
	if args[0] == "check-roles" {
		return runCheckRoles(args[1:])
	}
	if len(args) < 2 || (args[0] != "apply" && args[0] != "apply-plan") {
		usage()
		return errors.New("usage: deployctl apply <deployment> [flags] | deployctl apply-plan <plan> [flags] | deployctl check-roles [flags]")
	}
	command := args[0]
	name := args[1]
	fs := flag.NewFlagSet(command+" "+name, flag.ContinueOnError)
	configPath := fs.String("config", "tools/deployctl/deployments.yaml", "deployment declaration")
	sshUser := fs.String("ssh-user", "ubuntu", "SSH user")
	sshOpts := fs.String("ssh-opts", "", "additional SSH options")
	runStateFile := fs.String("run-state-file", "raw/current-run-id", "active benchmark marker")
	parallel := fs.Int("parallel", 8, "maximum number of concurrent jobs")
	dryRun := fs.Bool("dry-run", false, "print the plan without executing it")
	roles := roleValues{}
	vars := variableValues{}
	fs.Var(roles, "role", "role=host1,host2 (repeatable)")
	fs.Var(vars, "var", "name=value for declaration placeholders (repeatable)")
	if err := fs.Parse(args[2:]); err != nil {
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
	if *runStateFile == "" {
		return errors.New("-run-state-file must not be empty")
	}
	if !*dryRun {
		if _, err := os.Stat(*runStateFile); err == nil {
			return fmt.Errorf("active benchmark run exists (%s); run task after-bench or task abort-run first", *runStateFile)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("cannot inspect benchmark state %s: %w", *runStateFile, err)
		}
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	runner := newDeployRunner(*sshUser, *sshOpts, roles, vars, *dryRun, *parallel)
	if command == "apply" {
		d, ok := cfg.Deployments[name]
		if !ok {
			return fmt.Errorf("unknown deployment %q (available: %s)", name, strings.Join(sortedDeploymentNames(cfg), ", "))
		}
		return runner.apply(d)
	}
	p, ok := cfg.Plans[name]
	if !ok {
		return fmt.Errorf("unknown plan %q (available: %s)", name, strings.Join(sortedPlanNames(cfg), ", "))
	}
	return runner.applyPlan(cfg, p)
}

func newDeployRunner(sshUser string, sshOpts string, roles roleValues, vars variableValues, dryRun bool, parallel int) *deployRunner {
	return &deployRunner{
		sshUser:  sshUser,
		sshOpts:  strings.Fields(sshOpts),
		roles:    roles,
		vars:     vars,
		dryRun:   dryRun,
		parallel: parallel,
		exec:     osExecutor{},
	}
}

type roleValues map[string][]string

func (v roleValues) String() string { return "role=host1,host2" }

func (v roleValues) Set(value string) error {
	name, raw, ok := strings.Cut(value, "=")
	if !ok || !nameRE.MatchString(name) || raw == "" {
		return fmt.Errorf("invalid role %q; expected role=host1,host2", value)
	}
	seen := map[string]bool{}
	for _, host := range strings.Split(raw, ",") {
		host = strings.TrimSpace(host)
		if !hostRE.MatchString(host) {
			return fmt.Errorf("invalid host in role %q: %q", name, host)
		}
		if !seen[host] {
			v[name] = append(v[name], host)
			seen[host] = true
		}
	}
	return nil
}

type variableValues map[string]string

func (v variableValues) String() string { return "name=value" }

func (v variableValues) Set(value string) error {
	name, raw, ok := strings.Cut(value, "=")
	if !ok || !nameRE.MatchString(name) || !valueRE.MatchString(raw) {
		return fmt.Errorf("invalid variable %q; values may contain letters, digits, and _./,:-", value)
	}
	v[name] = raw
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `deployctl — role-based ISUCON deployment runner

usage:
  deployctl apply <deployment> [flags]
  deployctl apply-plan <plan> [flags]
  deployctl check-roles [flags]

The Taskfile is the public entrypoint and passes role and variable values.
Use -dry-run to inspect all rsync and SSH operations without changing servers.
`)
}
