package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadinessRetriesAndPreservesActivationFailure(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	body := `#!/bin/sh
count=0
[ ! -f "$READY_COUNT_FILE" ] || count=$(cat "$READY_COUNT_FILE")
count=$((count+1))
printf '%s' "$count" > "$READY_COUNT_FILE"
if [ "$count" -ge 3 ]; then printf 204; else printf 503; fi
`
	curl := filepath.Join(dir, "curl")
	if err := os.WriteFile(curl, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	runner := deployRunner{roles: map[string][]string{"app": {"host"}}, vars: map[string]string{"listen_port": "8080"}}
	ready := &readiness{URL: "http://127.0.0.1:{var:listen_port}/ready", Timeout: "3s", Status: 204}
	run := func(script string) ([]byte, error) {
		t.Helper()
		jobs, err := runner.activationJobs([]activation{{Label: "test", Role: "app", Script: script, Ready: ready}})
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", "-s")
		cmd.Stdin = strings.NewReader(jobs[0].script)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "READY_COUNT_FILE="+count)
		return cmd.CombinedOutput()
	}
	if output, err := run("printf activated"); err != nil || !strings.Contains(string(output), "activated") {
		t.Fatalf("readiness failed: %v: %s", err, output)
	}
	attempts, _ := os.ReadFile(count)
	if string(attempts) != "3" {
		t.Fatalf("unexpected probe attempts: %s", attempts)
	}
	os.Remove(count)
	if _, err := run("exit 7"); err == nil {
		t.Fatal("a successful probe hid activation failure")
	}
	if _, err := os.Stat(count); !os.IsNotExist(err) {
		t.Fatal("probe ran after activation failure")
	}
	os.WriteFile(curl, []byte("#!/bin/sh\nprintf 503\n"), 0755)
	ready.Timeout = "1s"
	if output, err := run("true"); err == nil || !strings.Contains(string(output), "readiness timed out") {
		t.Fatalf("missing timeout: %v: %s", err, output)
	}
}

func TestReadinessRejectsInvalidConditions(t *testing.T) {
	for _, ready := range []readiness{{URL: "file:///tmp/a"}, {URL: "http://127.0.0.1", Timeout: "-1s"}, {URL: "http://127.0.0.1", Status: 999}, {URL: "http://{var:missing}"}} {
		if _, err := readinessScript(ready, expander{}); err == nil {
			t.Fatalf("invalid condition accepted: %+v", ready)
		}
	}
}

func TestNginxDeploymentPreservesOSModuleConfiguration(t *testing.T) {
	cfg, err := loadConfig("deployments.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var excluded []string
	for _, upload := range cfg.Deployments["nginx"].Uploads {
		if upload.Remote == "/etc/nginx/" {
			excluded = upload.Excludes
		}
	}
	// OS-provided files do not affect the generated configuration fingerprint.
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "modules-enabled"), 0755)
	os.WriteFile(filepath.Join(root, "nginx.conf"), []byte("configuration"), 0644)
	before, err := localFingerprint(root+"/", excluded)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "modules-enabled/50-module.conf"), []byte("package-managed"), 0644)
	after, err := localFingerprint(root+"/", excluded)
	if err != nil || before != after {
		t.Fatalf("OS module config is not preserved: %v", err)
	}
}
