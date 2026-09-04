package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskfileDryDeployTasksInvokeDeployctlDryRun(t *testing.T) {
	if _, err := exec.LookPath("task"); err != nil {
		t.Skip("task is not installed")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		task string
		want string
	}{
		{task: "deploy-app-dry", want: "deployctl apply app -dry-run"},
		{task: "deploy-all-dry", want: "deployctl apply-plan deploy-all -dry-run"},
	} {
		cmd := exec.Command("task", "--dry", test.task)
		cmd.Dir = root
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			t.Fatalf("task --dry %s: %v\n%s", test.task, runErr, output)
		}
		if !strings.Contains(string(output), test.want) {
			t.Fatalf("task %s does not invoke deployctl dry-run:\n%s", test.task, output)
		}
	}
}

func TestTaskfileSystemdSetupDoesNotCreateRemoteFiles(t *testing.T) {
	if _, err := exec.LookPath("task"); err != nil {
		t.Skip("task is not installed")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("task", "--dry", "setup-systemd")
	cmd.Dir = root
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("task --dry setup-systemd: %v\n%s", runErr, output)
	}
	text := string(output)
	if strings.Contains(text, "sudo mkdir") || strings.Contains(text, "sudo touch") {
		t.Fatalf("setup-systemd mutates the remote host:\n%s", output)
	}
	if !strings.Contains(text, "sudo test -f") {
		t.Fatalf("setup-systemd does not check the optional remote drop-in:\n%s", output)
	}
}
