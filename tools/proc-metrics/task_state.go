package main

import (
	"context"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type blockedTask struct {
	pid    int
	tid    int
	comm   string
	state  string
	wchan  string
	cgroup string
}

func collectBlockedTasks(ctx context.Context, output io.Writer, procRoot string, interval time.Duration) error {
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	writer.UseCRLF = false
	if err := writer.Write([]string{"sample", "timestamp", "elapsed_ms", "blocked_tasks", "pid", "tid", "comm", "state", "wchan", "cgroup"}); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	startedAt := time.Now()
	var sample uint64
	poll := func() error {
		at := time.Now()
		tasks, err := readBlockedTasks(procRoot)
		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			tasks = []blockedTask{{state: "-"}}
		}
		for _, task := range tasks {
			row := []string{
				strconv.FormatUint(sample, 10), at.UTC().Format(time.RFC3339Nano),
				strconv.FormatInt(at.Sub(startedAt).Milliseconds(), 10), strconv.Itoa(lenNonSentinelTasks(tasks)),
				strconv.Itoa(task.pid), strconv.Itoa(task.tid), task.comm, task.state, task.wchan, task.cgroup,
			}
			if err := writer.Write(row); err != nil {
				return err
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return err
		}
		sample++
		return nil
	}
	if err := poll(); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := poll(); err != nil {
				return err
			}
		}
	}
}

func lenNonSentinelTasks(tasks []blockedTask) int {
	if len(tasks) == 1 && tasks[0].pid == 0 {
		return 0
	}
	return len(tasks)
}

func readBlockedTasks(procRoot string) ([]blockedTask, error) {
	paths, err := filepath.Glob(filepath.Join(procRoot, "[0-9]*", "task", "[0-9]*"))
	if err != nil {
		return nil, err
	}
	tasks := make([]blockedTask, 0)
	for _, taskPath := range paths {
		pid, err := strconv.Atoi(filepath.Base(filepath.Dir(filepath.Dir(taskPath))))
		if err != nil {
			continue
		}
		tid, err := strconv.Atoi(filepath.Base(taskPath))
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join(taskPath, "stat"))
		if err != nil {
			continue
		}
		comm, state, ok := parseTaskStat(string(stat))
		if !ok || state != "D" {
			continue
		}
		wchan, _ := os.ReadFile(filepath.Join(taskPath, "wchan"))
		cgroup, _ := os.ReadFile(filepath.Join(taskPath, "cgroup"))
		tasks = append(tasks, blockedTask{
			pid: pid, tid: tid, comm: sanitizeTaskField(comm), state: state,
			wchan: sanitizeTaskField(string(wchan)), cgroup: parseUnifiedCgroup(string(cgroup)),
		})
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].pid != tasks[j].pid {
			return tasks[i].pid < tasks[j].pid
		}
		return tasks[i].tid < tasks[j].tid
	})
	return tasks, nil
}

func parseTaskStat(raw string) (string, string, bool) {
	open := strings.IndexByte(raw, '(')
	close := strings.LastIndex(raw, ") ")
	if open < 0 || close <= open || close+2 >= len(raw) {
		return "", "", false
	}
	comm := raw[open+1 : close]
	rest := raw[close+2:]
	stateEnd := strings.IndexByte(rest, ' ')
	if stateEnd <= 0 {
		return "", "", false
	}
	return comm, rest[:stateEnd], true
}

func parseUnifiedCgroup(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "0::") {
			return sanitizeTaskField(strings.TrimPrefix(line, "0::"))
		}
	}
	return sanitizeTaskField(raw)
}

func sanitizeTaskField(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if value == "" {
		return "-"
	}
	return value
}
