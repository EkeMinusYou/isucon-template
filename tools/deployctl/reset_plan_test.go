package main

import (
	"strings"
	"testing"
)

func TestDBRecreatePlanStopsWritersAndFailureBoundaries(t *testing.T) {
	cfg, err := loadConfig(genericConfig)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the actual graph through an executor that cannot contact hosts.
	for name, d := range cfg.Deployments {
		d.Uploads = nil
		cfg.Deployments[name] = d
	}
	runner := deployRunner{
		sshUser: "ubuntu", parallel: 1,
		roles: map[string][]string{"all": {"edge", "db", "app-one", "app-two"}, "app": {"app-one", "app-two"}, "app_init": {"app-one"}, "app_db": {"db"}, "mysql": {"db"}, "nginx": {"edge"}},
		vars:  map[string]string{"remote_home": "/home/isucon", "app_dir": "webapp/go", "service": "app-service", "isucon_user": "isucon", "db_name": "app", "listen_port": "8080", "initialize_path": "/initialize", "sql_dir": "webapp/sql", "sql_schema_file": "schema.sql", "app_hosts": "app-one,app-two", "mysql_hosts": "db", "user_db_hosts": "db", "nginx_hosts": "edge"},
	}
	for name, want := range map[string]string{"db-schema": "db", "app-initialize": "app-one"} {
		jobs, err := runner.activationJobs(cfg.Deployments[name].Activations)
		if err != nil || len(jobs) != 1 || jobs[0].host != want {
			t.Fatalf("%s: %+v %v", name, jobs, err)
		}
	}
	for _, failure := range []string{"", "sudo systemctl stop app-service", "DROP DATABASE", "sudo systemctl start 'app-service'", "/initialize"} {
		t.Run(failure, func(t *testing.T) {
			fake := &planExecutor{failScriptPattern: failure}
			runner.exec = fake
			err := runner.applyPlan(cfg, cfg.Plans["db-recreate"])
			if (err != nil) != (failure != "") {
				t.Fatalf("unexpected result: %v", err)
			}
			index := func(pattern string) int {
				for i, c := range fake.calls {
					if strings.Contains(c.stdin, pattern) {
						return i
					}
				}
				return -1
			}
			if failure == "" {
				previous := -1
				for _, pattern := range []string{"sudo systemctl stop app-service", "DROP DATABASE", "sudo systemctl start 'app-service'", "/initialize"} {
					i := index(pattern)
					if i <= previous {
						t.Fatalf("out-of-order or absent %q", pattern)
					}
					previous = i
				}
				stopped, initialized := 0, 0
				for _, c := range fake.calls {
					if strings.Contains(c.stdin, "sudo systemctl stop app-service") {
						stopped++
					}
					if strings.Contains(c.stdin, "DROP DATABASE") && stopped != 2 {
						t.Fatal("reset before every writer stopped")
					}
					if strings.Contains(c.stdin, "/initialize") {
						initialized++
					}
				}
				if initialized != 1 {
					t.Fatalf("initialization count = %d", initialized)
				}
			} else {
				if index("enable_unit()") != -1 {
					t.Fatal("roles-on ran after failure")
				}
				if failure == "sudo systemctl stop app-service" && index("DROP DATABASE") != -1 {
					t.Fatal("reset ran after stop failure")
				}
				if failure == "DROP DATABASE" && index("sudo systemctl start 'app-service'") != -1 {
					t.Fatal("app started after schema failure")
				}
				if failure == "sudo systemctl start 'app-service'" && index("/initialize") != -1 {
					t.Fatal("application initialized after app start failure")
				}
			}
		})
	}
}
