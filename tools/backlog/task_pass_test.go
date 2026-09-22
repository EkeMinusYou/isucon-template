package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskPassForwardsExplicitContext(t *testing.T) {
	task, err := exec.LookPath("task")
	if err != nil {
		t.Skip("task is not installed")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	db := filepath.Join(root, "backlog.sqlite3")
	store, err := openStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fixtureObjectives(t, store)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "APPLIED", Owner: "verifier:task", Title: "task integration"})
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	run := "20260901-120000"
	writeAdoptionRun(t, root, run, card)
	reason := "Saved evidence; quotes ' and \"; literal $HOME and `value`"
	args := []string{"--taskfile", filepath.Join(repo, "Taskfile.yml"), "pass", "ROOT_DIR=" + root, "BACKLOG_DB=backlog.sqlite3", "RESULT_DIR=runs", "OWNER=verifier:task", "VERSIONS=B-001=0", "REASON=" + reason}
	// A global latest-RUN default must not become implicit authorization.
	missing := exec.Command(task, append(append([]string{}, args...), "--", "B-001")...)
	missing.Dir = repo
	out, err := missing.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "explicit --evidence-run") {
		t.Fatalf("missing RUN: err=%v output=%s", err, out)
	}
	cmd := exec.Command(task, append(args, "RUN="+run, "--", "B-001")...)
	cmd.Dir = repo
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("task pass: %v\n%s", err, out)
	}
	card, err = store.getCard(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "VALIDATED" || card.Owner != "" || !strings.Contains(card.History[len(card.History)-1].Body, reason) {
		t.Fatalf("task adoption=%#v", card)
	}
}
