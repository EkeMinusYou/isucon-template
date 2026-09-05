package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMySQLRoleConvergenceKeepsEveryAssignedHost(t *testing.T) {
	cfg, err := loadConfig("deployments.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	sudo := filepath.Join(tmp, "sudo")
	if err := os.WriteFile(sudo, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$ISUCON_TEST_CALLS\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"isucon-1", "isucon-2", "isucon-3"} {
		for _, phase := range []string{"roles-off", "roles-on"} {
			t.Run(host+"/"+phase, func(t *testing.T) {
				log := filepath.Join(tmp, host+"-"+phase)
				script := cfg.Deployments[phase].Activations[0].Script
				script = strings.NewReplacer("{host}", host, "{var:mysql_hosts}", "isucon-1,isucon-2", "{var:app_hosts}", "isucon-3", "{var:nginx_hosts}", "isucon-3", "{var:service}", "app").Replace(script)
				cmd := exec.Command("sh", "-c", script)
				cmd.Env = append(os.Environ(), "PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"), "ISUCON_TEST_CALLS="+log)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %s", err, output)
				}
				body, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				disabled := strings.Contains(string(body), "disable --now mysql")
				enabled := strings.Contains(string(body), "enable --now mysql")
				if disabled != (phase == "roles-off" && host == "isucon-3") || enabled != (phase == "roles-on" && host != "isucon-3") {
					t.Fatalf("unexpected MySQL actions: %s", body)
				}
			})
		}
	}
}

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

func TestTaskfileSeparatesMySQLDeploymentAndMeasurementRoles(t *testing.T) {
	cfg := readSetupTaskfile(t)
	cfg["tasks"] = map[string]interface{}{"show": map[string]interface{}{"silent": true, "cmds": []string{"echo 'DEPLOY {{.DEPLOYCTL_COMMON}}'", "echo 'MEASURE {{.MEASURECTL_ROLES}}'"}}}
	output := runSetupFixture(t, cfg, t.TempDir(), nil, "show", "MYSQL_HOSTS=isucon-1 isucon-2", "MYSQL_HOST=isucon-2")
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "DEPLOY ") && (!strings.Contains(line, "-role mysql=isucon-1,isucon-2 ") || !strings.Contains(line, "-var mysql_hosts=isucon-1,isucon-2")) {
			t.Fatalf("deployment role: %s", line)
		}
		if strings.HasPrefix(line, "MEASURE ") && (!strings.Contains(line, "-role mysql=isucon-2 ") || !strings.Contains(line, "-role mysql_all=isucon-1,isucon-2")) {
			t.Fatalf("measurement role: %s", line)
		}
	}
	if !strings.Contains(output, "DEPLOY ") || !strings.Contains(output, "MEASURE ") {
		t.Fatal(output)
	}
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
	runSetupFixture(t, cfg, tmp, []string{"ISUCON_TEST_SOURCE=" + source}, "setup-webapp", "RSYNC="+transport, "SETUP_WEBAPP_EXCLUDES="+excludes)
	for _, name := range []string{"node_modules/package/index.js", "go/app"} {
		if _, err := os.Stat(filepath.Join(tmp, "webapp", name)); !os.IsNotExist(err) {
			t.Fatalf("excluded file was acquired: %s (%v)", name, err)
		}
	}
	for _, name := range []string{"go/vendor/module/source.go", "sql/schema.sql"} {
		if _, err := os.Stat(filepath.Join(tmp, "webapp", name)); err != nil {
			t.Fatalf("required input was excluded: %s (%v)", name, err)
		}
	}
}

func TestConfigCheckExampleUsesGuardedLiveUploads(t *testing.T) {
	cfg, err := loadConfig("../setup/config-check.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	d := cfg.Deployments["config-check"]
	if len(d.Activations) != 0 || len(d.Uploads) != 2 {
		t.Fatalf("unexpected config-only deployment: %#v", d)
	}
	for _, u := range d.Uploads {
		if u.Validate == "" || u.Remote != "/etc/"+u.Role+"/" || u.Local != u.Role+"/" {
			t.Fatalf("unexpected configuration upload: %#v", u)
		}
	}
}
