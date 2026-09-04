package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTaskStatHandlesSpacesAndParentheses(t *testing.T) {
	comm, state, ok := parseTaskStat("42 (mysql worker (io)) D 1 2 3")
	if !ok || comm != "mysql worker (io)" || state != "D" {
		t.Fatalf("parseTaskStat() = (%q, %q, %v)", comm, state, ok)
	}
}

func TestReadBlockedTasksReturnsOnlyDStateThreads(t *testing.T) {
	root := t.TempDir()
	writeTaskFixture(t, root, "10", "11", "11 (mysql io) D 1 2", "io_schedule", "0::/system.slice/mysql.service\n")
	writeTaskFixture(t, root, "20", "21", "21 (nginx) R 1 2", "0", "0::/system.slice/nginx.service\n")

	tasks, err := readBlockedTasks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
	if tasks[0].pid != 10 || tasks[0].tid != 11 || tasks[0].comm != "mysql io" || tasks[0].wchan != "io_schedule" || tasks[0].cgroup != "/system.slice/mysql.service" {
		t.Fatalf("unexpected task: %+v", tasks[0])
	}
}

func TestCollectBlockedTasksWritesEmptySamples(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- collectBlockedTasks(ctx, &output, t.TempDir(), time.Millisecond)
	}()
	time.Sleep(3 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("output has %d lines, want header and samples: %q", len(lines), output.String())
	}
	if !strings.Contains(lines[1], "\t0\t0\t0\t\t-\t") {
		t.Fatalf("empty sample row = %q", lines[1])
	}
}

func writeTaskFixture(t *testing.T, root, pid, tid, stat, wchan, cgroup string) {
	t.Helper()
	dir := filepath.Join(root, pid, "task", tid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"stat": stat, "wchan": wchan, "cgroup": cgroup} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
