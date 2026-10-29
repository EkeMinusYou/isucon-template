package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeUsesGoAPIAndPropagatesFailure(t *testing.T) {
	cfg, err := loadConfig(genericConfig)
	if err != nil {
		t.Fatal(err)
	}
	r := deployRunner{sshUser: "ubuntu", parallel: 1, roles: map[string][]string{"app_init": {"entry"}}, vars: map[string]string{"listen_port": "80", "initialize_path": "/initialize", "remote_home": "/home/isucon"}}
	for _, fail := range []string{"", "/initialize"} {
		fake := &planExecutor{failScriptPattern: fail}
		r.exec = fake
		err := r.applyPlan(cfg, cfg.Plans["initialize"])
		if (err != nil) != (fail != "") {
			t.Fatalf("err=%v fail=%q", err, fail)
		}
		if len(fake.calls) != 1 || !strings.Contains(fake.calls[0].stdin, "http.json") || !strings.Contains(fake.calls[0].stdin, "\"$url/initialize\"") {
			t.Fatalf("unexpected initialization: %v", fake.calls)
		}
	}
}

func TestInitializeSelectsHTTPOrTLSFromDeployedConfig(t *testing.T) {
	cfg, err := loadConfig(genericConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ address, cert, want string }{{":8080", "", "http://127.0.0.1:8080/initialize"}, {":443", "/cert.pem", "https://127.0.0.1:443/initialize"}, {"[::1]:8443", "/cert.pem", "https://[::1]:8443/initialize"}} {
		dir := t.TempDir()
		body := fmt.Sprintf(`{"address":%q,"tls_cert_file":%q}`, tc.address, tc.cert)
		if err := os.WriteFile(filepath.Join(dir, "http.json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "curl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		r := deployRunner{roles: map[string][]string{"app_init": {"local"}}, vars: map[string]string{"remote_home": dir, "initialize_path": "/initialize"}}
		jobs, err := r.activationJobs(cfg.Deployments["app-initialize"].Activations)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command("sh", "-c", jobs[0].script)
		command.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := command.CombinedOutput()
		if err != nil || !strings.Contains(string(out), tc.want) {
			t.Fatalf("%s: %s %v", tc.address, out, err)
		}
	}
}
