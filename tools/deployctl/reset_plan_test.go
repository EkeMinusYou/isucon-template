package main

import (
	"strings"
	"testing"
)

func TestInitializeUsesGoAPIAndPropagatesFailure(t *testing.T) {
	cfg, err := loadConfig(genericConfig)
	if err != nil {
		t.Fatal(err)
	}
	r := deployRunner{sshUser: "ubuntu", parallel: 1, roles: map[string][]string{"app_init": {"entry"}}, vars: map[string]string{"listen_port": "80", "initialize_path": "/initialize"}}
	for _, fail := range []string{"", "/initialize"} {
		fake := &planExecutor{failScriptPattern: fail}
		r.exec = fake
		err := r.applyPlan(cfg, cfg.Plans["initialize"])
		if (err != nil) != (fail != "") {
			t.Fatalf("err=%v fail=%q", err, fail)
		}
		if len(fake.calls) != 1 || !strings.Contains(fake.calls[0].stdin, "http://127.0.0.1:80/initialize") {
			t.Fatalf("unexpected initialization: %v", fake.calls)
		}
	}
}
