package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type plannedActivationJob struct {
	deployment string
	job        activationJob
}

func (r *deployRunner) applyPlan(cfg *config, p plan) error {
	names := sortedPlanDeploymentNames(p)
	var uploads []uploadJob
	activations := make(map[string][]activationJob, len(names))
	for _, name := range names {
		d := cfg.Deployments[name]
		deploymentUploads, err := r.uploadJobs(d.Uploads)
		if err != nil {
			return fmt.Errorf("deployment %q: %w", name, err)
		}
		for i := range deploymentUploads {
			deploymentUploads[i].u.Label = name + "/" + deploymentUploads[i].u.Label
		}
		uploads = append(uploads, deploymentUploads...)

		deploymentActivations, err := r.activationJobs(d.Activations)
		if err != nil {
			return fmt.Errorf("deployment %q: %w", name, err)
		}
		for i := range deploymentActivations {
			deploymentActivations[i].a.Label = name + "/" + deploymentActivations[i].a.Label
		}
		activations[name] = deploymentActivations
	}

	if r.dryRun {
		fmt.Printf("plan upload phase: %d jobs\n", len(uploads))
		for _, j := range uploads {
			fmt.Printf("[dry-run] %s\n", formatCommand("rsync", r.rsyncArgs(j)...))
		}
		fmt.Println("plan activation graph:")
		for _, name := range names {
			needs := strings.Join(p.Deployments[name].Needs, ",")
			if needs == "" {
				needs = "-"
			}
			fmt.Printf("[dry-run] deployment=%s needs=%s jobs=%d\n", name, needs, len(activations[name]))
			for _, j := range activations[name] {
				args := append(r.sshBase(), r.sshTarget(j.host), "sh -s")
				fmt.Printf("[dry-run] %s <<'EOF'\n%sEOF\n", formatCommand("ssh", args...), j.script)
			}
		}
		return nil
	}

	if len(uploads) != 0 {
		fmt.Printf("plan upload phase: %d jobs\n", len(uploads))
		if err := reportResults(runParallel(uploads, r.parallel, r.runUpload)); err != nil {
			return fmt.Errorf("plan upload phase failed; no activation was started: %w", err)
		}
	}

	pending := make(map[string]bool, len(names))
	succeeded := make(map[string]bool, len(names))
	failed := make(map[string]bool, len(names))
	for _, name := range names {
		pending[name] = true
	}
	var planErrors []error
	for len(pending) != 0 {
		progressed := false
		for _, name := range names {
			if !pending[name] {
				continue
			}
			var blockedBy []string
			for _, dependency := range p.Deployments[name].Needs {
				if failed[dependency] {
					blockedBy = append(blockedBy, dependency)
				}
			}
			if len(blockedBy) != 0 {
				delete(pending, name)
				failed[name] = true
				progressed = true
				err := fmt.Errorf("deployment %q skipped because dependencies failed: %s", name, strings.Join(blockedBy, ", "))
				fmt.Printf("[skipped] %v\n", err)
				planErrors = append(planErrors, err)
			}
		}

		var ready []string
		for _, name := range names {
			if !pending[name] {
				continue
			}
			allSucceeded := true
			for _, dependency := range p.Deployments[name].Needs {
				if !succeeded[dependency] {
					allSucceeded = false
					break
				}
			}
			if allSucceeded {
				ready = append(ready, name)
			}
		}
		if len(ready) == 0 {
			if progressed {
				continue
			}
			return errors.Join(append(planErrors, errors.New("plan activation graph made no progress"))...)
		}

		var jobs []plannedActivationJob
		for _, name := range ready {
			for _, job := range activations[name] {
				jobs = append(jobs, plannedActivationJob{deployment: name, job: job})
			}
		}
		fmt.Printf("plan activation phase: deployments=%s jobs=%d\n", strings.Join(ready, ","), len(jobs))
		results := runParallel(jobs, r.parallel, func(j plannedActivationJob) jobResult {
			return r.runActivation(j.job)
		})
		_ = reportResults(results)
		failedReady := make(map[string]bool)
		for i, result := range results {
			if result.err != nil {
				deploymentName := jobs[i].deployment
				failedReady[deploymentName] = true
				planErrors = append(planErrors, fmt.Errorf("deployment %q [%s] %s: %w", deploymentName, result.host, result.label, result.err))
			}
		}
		for _, name := range ready {
			delete(pending, name)
			if failedReady[name] {
				failed[name] = true
			} else {
				succeeded[name] = true
			}
		}
	}
	return errors.Join(planErrors...)
}

func sortedPlanDeploymentNames(p plan) []string {
	names := make([]string, 0, len(p.Deployments))
	for name := range p.Deployments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
