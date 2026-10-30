package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func readSetupTaskfile(t *testing.T) map[string]interface{} {
	t.Helper()
	body, err := os.ReadFile("../../Taskfile.yml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	// The fixture runs from a temporary directory without the contest file.
	// These tests exercise generic vars and tasks, which never call into it.
	delete(cfg, "includes")
	// Task resolves global variables in declaration order. Preserve that order
	// when rendering an isolated fixture with replacement tasks.
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	mapping := document.Content[0].Content
	for i := 0; i < len(mapping); i += 2 {
		if mapping[i].Value == "vars" {
			cfg["vars"] = mapping[i+1]
		}
	}
	return cfg
}

func runSetupFixture(t *testing.T, cfg map[string]interface{}, dir string, env []string, args ...string) string {
	t.Helper()
	task, err := exec.LookPath("task")
	if err != nil {
		t.Skip("task is not installed")
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Taskfile.yml")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(task, append([]string{"--taskfile", path}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("task: %v\n%s", err, output)
	}
	return string(output)
}

func TestSetupWebappUsesCustomExcludesWithoutDroppingVendor(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync is not installed")
	}
	cfg := readSetupTaskfile(t)
	tmp := t.TempDir()
	source := filepath.Join(tmp, "source")
	for _, name := range []string{"node_modules/package/index.js", "go/app", "go/vendor/module/source.go", "sql/schema.sql"} {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	excludes := filepath.Join(tmp, "custom excludes.txt")
	defaultExcludes, err := os.ReadFile("../setup/webapp-excludes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludes, append(defaultExcludes, []byte("\n/go/app\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	// Exercise the real setup task with a local-only rsync transport.
	transport := filepath.Join(tmp, "local-rsync")
	script := "#!/bin/sh\nset -eu\n[ \"$#\" -eq 3 ]\nexec rsync -a \"$1\" \"$ISUCON_TEST_SOURCE/\" \"$3\"\n"
	if err := os.WriteFile(transport, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	runSetupFixture(t, cfg, tmp, []string{"ISUCON_TEST_SOURCE=" + source}, "setup-webapp", "RSYNC="+transport, "SETUP_WEBAPP_EXCLUDES="+excludes, "REFERENCE_DIR="+filepath.Join(tmp, "..", filepath.Base(tmp)+"-reference"))
	for _, name := range []string{"node_modules/package/index.js", "go/app"} {
		if _, err := os.Stat(filepath.Join(tmp, "..", filepath.Base(tmp)+"-reference", "webapp", name)); !os.IsNotExist(err) {
			t.Fatalf("excluded file was acquired: %s (%v)", name, err)
		}
	}
	for _, name := range []string{"go/vendor/module/source.go", "sql/schema.sql"} {
		if _, err := os.Stat(filepath.Join(tmp, "..", filepath.Base(tmp)+"-reference", "webapp", name)); err != nil {
			t.Fatalf("required input was excluded: %s (%v)", name, err)
		}
	}
}

func TestGoRolesOnlyManageAssignedServices(t *testing.T) {
	cfg, err := loadConfig("deployments.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"app-host", "edge-host", "unused-host"} {
		t.Run(host, func(t *testing.T) {
			tmp := t.TempDir()
			log := filepath.Join(tmp, "calls")
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$ISUCON_TEST_CALLS\"\nif [ \"$2\" = is-active ]; then echo inactive; fi\n"
			if err := os.WriteFile(filepath.Join(tmp, "sudo"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			r := deployRunner{roles: map[string][]string{"all": {host}}, vars: map[string]string{"app_hosts": "app-host", "nginx_hosts": "edge-host", "service": "app", "disabled_services": "nginx,mysql,redis-server"}}
			for _, phase := range []string{"roles-off", "roles-on"} {
				jobs, err := r.activationJobs(cfg.Deployments[phase].Activations)
				if err != nil || len(jobs) != 1 {
					t.Fatalf("jobs=%v err=%v", jobs, err)
				}
				cmd := exec.Command("sh", "-c", jobs[0].script)
				cmd.Env = append(os.Environ(), "PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"), "ISUCON_TEST_CALLS="+log)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %s", err, out)
				}
			}
			body, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			calls := string(body)
			for _, unit := range []string{"mysql", "redis-server", "app", "nginx"} {
				assigned := (unit == "app" && host == "app-host")
				if strings.Contains(calls, "enable --now "+unit) != assigned || strings.Contains(calls, "disable --now "+unit) == assigned {
					t.Fatalf("wrong role for %s: %s", unit, calls)
				}
			}
		})
	}
}

func TestDefaultConfigCheckResolvesApplicationDirectory(t *testing.T) {
	cfg := readSetupTaskfile(t)
	cfg["tasks"] = map[string]interface{}{"show": map[string]interface{}{"silent": true, "cmds": []string{"echo '{{.CONFIG_CHECK_COMMAND}}'"}}}
	for _, tc := range []struct{ override, directory string }{{"", "webapp/go"}, {"APP_DIR=custom-go", "custom-go"}} {
		args := []string{"show"}
		if tc.override != "" {
			args = append(args, tc.override)
		}
		got := runSetupFixture(t, cfg, t.TempDir(), nil, args...)
		if strings.TrimSpace(got) != "go -C "+tc.directory+" vet ./..." {
			t.Fatalf("config check has an unresolved directory: %q", got)
		}
	}
}

func TestTaskfileDefaultsToDirectGoRuntime(t *testing.T) {
	cfg := readSetupTaskfile(t)
	cfg["tasks"] = map[string]interface{}{"show": map[string]interface{}{"silent": true, "cmds": []string{"echo '{{.MANAGED_SERVICES}}'", "echo '{{.DEPLOYCTL_COMMON}}'", "echo '{{.MEASURECTL_ROLES}}'"}}}
	out := runSetupFixture(t, cfg, t.TempDir(), nil, "show", "INGRESS_HOSTS=isucon-2", "SERVICE=app", "APP_HOSTS=isucon-1,isucon-2", "APP_TRAFFIC_HOSTS=isucon-1", "LISTEN_PORT=80", "STATIC_REMOTE_DIR=/home/isucon/public", "CONTEST_SERVICE_ROLES=")
	for _, unwanted := range []string{"-role mysql=", "-role nginx=", "-var nginx_hosts=", "nginx:nginx"} {
		if strings.Contains(out, unwanted) {
			t.Fatal(out)
		}
	}
	for _, want := range []string{"app\n", "-role app_traffic=isucon-1", "-role ingress=isucon-2", "-role entry=isucon-2", "-var listen_port=80", "-var static_remote_dir=/home/isucon/public", "-var disabled_services=nginx,"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s: %s", want, out)
		}
	}
}
