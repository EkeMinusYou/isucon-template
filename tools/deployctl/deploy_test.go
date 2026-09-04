package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeExecutor struct {
	mu       sync.Mutex
	calls    []string
	failName string
}

type recordedCall struct {
	name  string
	stdin string
}

type planExecutor struct {
	mu                sync.Mutex
	calls             []recordedCall
	failScriptPattern string
}

func (f *planExecutor) Run(name string, _ []string, stdin string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedCall{name: name, stdin: stdin})
	if f.failScriptPattern != "" && strings.Contains(stdin, f.failScriptPattern) {
		return []byte("planned failure"), errors.New("failed")
	}
	return []byte("ok"), nil
}

func (f *fakeExecutor) Run(name string, args []string, stdin string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	if name == f.failName {
		return []byte("planned failure"), errors.New("failed")
	}
	return []byte("ok"), nil
}

func TestApplyDoesNotActivateAfterUploadFailure(t *testing.T) {
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDirectory) })
	if err := os.WriteFile("app", []byte("binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeExecutor{failName: "rsync"}
	runner := deployRunner{
		sshUser:  "ubuntu",
		roles:    map[string][]string{"app": {"isucon-1"}},
		vars:     map[string]string{},
		parallel: 2,
		exec:     fake,
	}
	d := deployment{
		Uploads:     []upload{{Label: "app", Role: "app", Local: "app", Remote: "/home/isucon/app"}},
		Activations: []activation{{Label: "restart", Role: "app", Script: "sudo systemctl restart app"}},
	}
	if err := runner.apply(d); err == nil {
		t.Fatal("apply succeeded, want upload failure")
	}
	for _, call := range fake.calls {
		if call == "ssh" {
			t.Fatal("activation SSH ran after an upload failure")
		}
	}
}

func TestApplyPlanUploadsEverythingBeforeActivationAndSkipsDependent(t *testing.T) {
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDirectory) })
	for _, filename := range []string{"mysql", "app"} {
		if err := os.WriteFile(filename, []byte("artifact"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fake := &planExecutor{failScriptPattern: "restart mysql"}
	runner := deployRunner{
		sshUser:  "ubuntu",
		roles:    map[string][]string{"mysql": {"isucon-1"}, "app": {"isucon-2"}},
		parallel: 2,
		exec:     fake,
	}
	cfg := &config{Deployments: map[string]deployment{
		"mysql": {
			Uploads:     []upload{{Label: "mysql", Role: "mysql", Local: "mysql", Remote: "/etc/mysql/mysql.cnf"}},
			Activations: []activation{{Label: "restart", Role: "mysql", Script: "sudo systemctl restart mysql"}},
		},
		"app": {
			Uploads:     []upload{{Label: "app", Role: "app", Local: "app", Remote: "/home/isucon/app"}},
			Activations: []activation{{Label: "restart", Role: "app", Script: "sudo systemctl restart app"}},
		},
	}}
	p := plan{Deployments: map[string]planDeployment{
		"mysql": {},
		"app":   {Needs: []string{"mysql"}},
	}}
	if err := runner.applyPlan(cfg, p); err == nil {
		t.Fatal("applyPlan succeeded, want MySQL activation failure")
	}
	firstSSH := len(fake.calls)
	for i, call := range fake.calls {
		if call.name == "ssh" && firstSSH == len(fake.calls) {
			firstSSH = i
		}
		if strings.Contains(call.stdin, "restart app") {
			t.Fatal("dependent app activation ran after MySQL activation failure")
		}
	}
	if firstSSH != 2 {
		t.Fatalf("first SSH call index = %d, want 2 uploads first; calls = %#v", firstSSH, fake.calls)
	}
}

func TestLoadConfigRejectsPlanCycle(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "deployments.yaml")
	body := `deployments:
  first:
    activations:
      - {label: first, role: all, script: "true"}
  second:
    activations:
      - {label: second, role: all, script: "true"}
plans:
  cycle:
    deployments:
      first: {needs: [second]}
      second: {needs: [first]}
`
	if err := os.WriteFile(filename, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(filename)
	if err == nil || !strings.Contains(err.Error(), "循環") {
		t.Fatalf("loadConfig error = %v, want cycle rejection", err)
	}
}

func TestLoadConfigRejectsNodeDeployment(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "deployments.yaml")
	body := `deployments:
  app:
    uploads:
      - {label: node, role: app, local: webapp/node/, remote: /home/isucon/webapp/node/}
    activations:
      - {label: restart, role: app, script: "true"}
`
	if err := os.WriteFile(filename, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(filename)
	if err == nil || !strings.Contains(err.Error(), "webapp/node") {
		t.Fatalf("loadConfig error = %v, want webapp/node rejection", err)
	}
}

func TestValidateRemotePath(t *testing.T) {
	for _, remote := range []string{
		"/",
		"/tmp/app",
		"/etc/",
		"/usr/local/",
		"/etc/nginx/../shadow",
		"/home/isucon/../../etc/passwd",
	} {
		if err := validateRemotePath(remote, false); err == nil {
			t.Errorf("validateRemotePath(%q) succeeded", remote)
		}
	}
	for _, remote := range []string{
		"/home/isucon/webapp/go/",
		"/etc/systemd/system/app.service",
		"/etc/nginx/",
		"/etc/mysql/",
		"/etc/sysctl.conf",
	} {
		if err := validateRemotePath(remote, false); err != nil {
			t.Errorf("validateRemotePath(%q) = %v", remote, err)
		}
	}
}

func TestRepositoryConfigContainsEveryDeployTarget(t *testing.T) {
	cfg, err := loadConfig("deployments.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app", "nginx", "mysql", "sysctl"} {
		if _, ok := cfg.Deployments[name]; !ok {
			t.Errorf("deployment %q is missing", name)
		}
	}
	for _, name := range []string{"deploy-all", "roles"} {
		if _, ok := cfg.Plans[name]; !ok {
			t.Errorf("plan %q is missing", name)
		}
	}
	if got := len(cfg.Deployments["roles-off"].Uploads); got != 0 {
		t.Errorf("roles-off upload count = %d, want activation-only deployment", got)
	}
}

func TestRunRejectsActiveBenchmark(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "current-run-id")
	if err := os.WriteFile(marker, []byte("20260829-120000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"apply", "app", "-run-state-file", marker})
	if err == nil || !strings.Contains(err.Error(), "active benchmark run exists") {
		t.Fatalf("run error = %v, want active benchmark rejection", err)
	}
}

func TestCheckRolesBuildsExpectedServiceStates(t *testing.T) {
	fake := &planExecutor{}
	runner := deployRunner{
		sshUser: "ubuntu",
		roles: map[string][]string{
			"all": {"isucon-1", "isucon-2"}, "app": {"isucon-1"},
			"nginx": {"isucon-1"}, "mysql": {"isucon-2"},
		},
		parallel: 2,
		exec:     fake,
	}
	if err := runner.checkRoles("app"); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("role check calls = %d, want 2", len(fake.calls))
	}
	joined := fake.calls[0].stdin + "\n" + fake.calls[1].stdin
	for _, expected := range []string{
		"check_unit 'app' active", "check_unit 'app' inactive",
		"check_unit nginx active", "check_unit nginx inactive",
		"check_unit mysql active", "check_unit mysql inactive",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("role scripts do not contain %q:\n%s", expected, joined)
		}
	}
}

func TestRunCheckRolesDoesNotRejectActiveBenchmark(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "current-run-id")
	if err := os.WriteFile(marker, []byte("20260829-120000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCheckRoles([]string{
		"-run-state-file", marker, "-dry-run", "-ssh-user", "ubuntu",
		"-role", "all=isucon-1", "-role", "app=isucon-1", "-role", "nginx=isucon-1", "-role", "mysql=isucon-1",
		"-var", "service=app",
	})
	if err != nil {
		t.Fatalf("read-only role check rejected active benchmark: %v", err)
	}
}
