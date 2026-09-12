package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Run the real CLI in a child process so fatal errors and stdout are exercised.
func TestBacklogCLIProcess(t *testing.T) {
	if os.Getenv("ISUCON_BACKLOG_CLI_TEST") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"backlog"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	t.Fatal("missing CLI arguments")
}

type backlogCLIResult struct {
	stdout string
	stderr string
	err    error
}

func runBacklogCLI(t *testing.T, dbPath, stdin string, args ...string) backlogCLIResult {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cliArgs := []string{"-test.run=^TestBacklogCLIProcess$", "--", "-root", filepath.Dir(dbPath), "-db", dbPath}
	cmd := exec.Command(executable, append(cliArgs, args...)...)
	cmd.Env = append(os.Environ(), "ISUCON_BACKLOG_CLI_TEST=1", "GORACE=atexit_sleep_ms=0")
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return backlogCLIResult{stdout.String(), stderr.String(), err}
}

func requireCLIJSON(t *testing.T, result backlogCLIResult, value any) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("CLI failed: %v\nstdout: %s\nstderr: %s", result.err, result.stdout, result.stderr)
	}
	if err := json.Unmarshal([]byte(result.stdout), value); err != nil {
		t.Fatalf("invalid JSON output %q: %v", result.stdout, err)
	}
}

func TestCLIJSONCardWorkflow(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	dbPath := filepath.Join(t.TempDir(), "backlog.sqlite3")
	var receipt cardMutationReceipt
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "add", "--title", "CLI workflow", "--actor", "human:test", "--reason", "create", "--format", "json"), &receipt)
	if receipt.ID != "B-001" || receipt.Version != 0 || receipt.Woke == nil {
		t.Fatalf("creation receipt = %#v", receipt)
	}
	contract := `{"Hypothesis":"reduce repeated work","Change boundary":"batch reads","Evaluation":"compare saved RUNs"}`
	requireCLIJSON(t, runBacklogCLI(t, dbPath, contract, "resolve", "B-001", "--status", "READY", "--expect-card-version", "0", "--actor", "skill:isucon-investigate", "--reason", "investigated", "--section-stdin", "--result", "Ready for implementation", "--format", "json"), &receipt)
	if receipt.Version != 1 {
		t.Fatalf("resolution receipt = %#v", receipt)
	}
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "update", "B-001", "--status", "DOING", "--owner", "agent:test", "--expect-card-version", "1", "--actor", "agent:test", "--reason", "claim", "--format=json"), &receipt)
	if receipt.Version != 2 {
		t.Fatalf("claim receipt = %#v", receipt)
	}
	body := "SQL and API checks passed.\nQuotes: \"value\", 日本語, $HOME, `literal`"
	requireCLIJSON(t, runBacklogCLI(t, dbPath, body+"\n", "transition", "B-001", "--status", "VERIFY", "--result-file", "-", "--expect-card-version", "2", "--actor", "agent:test", "--reason", "local verification", "--format", "json"), &receipt)
	if receipt.Version != 3 {
		t.Fatalf("transition receipt = %#v", receipt)
	}
	var card Card
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "intervention", "show", "B-001", "--format", "json"), &card)
	if card.Version != receipt.Version || card.Status != "VERIFY" || card.Owner != "agent:test" || sectionBody(card.Sections, "Result") != body {
		t.Fatalf("verified card = %#v", card)
	}
	if len(card.History) != 4 || card.History[3].Body != "local verification" {
		t.Fatalf("history = %#v", card.History)
	}
	// A stale writer must not change either the status or the Result section.
	failed := runBacklogCLI(t, dbPath, "", "transition", "B-001", "--status", "APPLIED", "--result", "stale result", "--expect-card-version", "2", "--actor", "agent:other", "--reason", "stale", "--format", "json")
	if failed.err == nil || failed.stdout != "" || !strings.Contains(failed.stderr, "card version conflict") {
		t.Fatalf("stale update = %#v", failed)
	}
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "show", "--format=json", "B-001"), &card)
	if card.Version != 3 || card.Status != "VERIFY" || sectionBody(card.Sections, "Result") != body || len(card.History) != 4 {
		t.Fatalf("failed update changed card = %#v", card)
	}
	// A successful direct Result update keeps the legacy text response.
	text := runBacklogCLI(t, dbPath, "", "update", "B-001", "--result", "updated result", "--expect-card-version", "3", "--actor", "agent:test", "--reason", "record result")
	if text.err != nil || text.stdout != "updated\n" {
		t.Fatalf("text update = %#v", text)
	}
	validated := runBacklogCLI(t, dbPath, "", "validate")
	if validated.err != nil {
		t.Fatalf("validation failed: %#v", validated)
	}
}

func TestCLIJSONReadsAndFilters(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backlog.sqlite3")
	store, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	fixtureObjectives(t, store)
	t.Cleanup(func() { store.Close() })
	seedBacklog(t, store, 7, "B-003",
		Card{ID: "B-001", Version: 2, Status: "READY", Title: "Ready card"},
		Card{ID: "B-002", Version: 5, Status: "DOING", Owner: "agent:test", Title: "Claimed card"},
	)
	var list struct {
		Revision   int         `json:"backlog_revision"`
		Cards      []Card      `json:"cards"`
		Targets    []Target    `json:"targets"`
		Objectives []Objective `json:"objectives"`
	}
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "list", "--status", "READY", "--unowned", "--format", "json"), &list)
	if list.Revision != 7 || len(list.Cards) != 1 || list.Cards[0].ID != "B-001" || list.Cards[0].Version != 2 || len(list.Targets) != 1 || len(list.Objectives) == 0 {
		t.Fatalf("filtered list = %#v", list)
	}
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "list", "--owner", "unknown", "--format=json"), &list)
	if list.Cards == nil || len(list.Cards) != 0 {
		t.Fatalf("empty cards must be [], got %#v", list.Cards)
	}
	for _, args := range [][]string{
		{"show", "B-001", "--format", "json"},
		{"target", "show", "A-001", "--format=json"},
		{"objective", "show", "--format", "json", "O-001"},
	} {
		var fields map[string]json.RawMessage
		requireCLIJSON(t, runBacklogCLI(t, dbPath, "", args...), &fields)
		if fields["id"] == nil || fields["version"] == nil || fields["status"] == nil {
			t.Fatalf("missing stable JSON keys: %v", fields)
		}
		if fields["ID"] != nil {
			t.Fatal("JSON exposed untagged Go field names")
		}
	}
	for _, entity := range []string{"target", "objective"} {
		var rows []json.RawMessage
		requireCLIJSON(t, runBacklogCLI(t, dbPath, "", entity, "list", "--format", "json"), &rows)
		if len(rows) == 0 {
			t.Fatalf("empty %s list", entity)
		}
		requireCLIJSON(t, runBacklogCLI(t, dbPath, "", entity, "list", "--status", "RETIRED", "--format=json"), &rows)
		if rows == nil || len(rows) != 0 {
			t.Fatalf("empty %s list must be []: %s", entity, rows)
		}
	}
	for _, args := range [][]string{
		{"list", "--format", "yaml"},
		{"list", "--watch", "--format", "json"},
		{"watch", "--format", "json"},
	} {
		failed := runBacklogCLI(t, dbPath, "", args...)
		if failed.err == nil || failed.stdout != "" {
			t.Fatalf("invalid flags succeeded: %v: %#v", args, failed)
		}
	}
}

func TestResultInputSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.txt")
	if err := os.WriteFile(path, []byte("file result\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		args  []string
		stdin string
		want  map[string]string
	}{
		{"unchanged", nil, "", nil},
		{"literal", []string{"--result", "a\nb"}, "", map[string]string{"Result": "a\nb"}},
		{"clear", []string{"--result="}, "", map[string]string{"Result": ""}},
		{"file", []string{"--result-file", path}, "", map[string]string{"Result": "file result\nsecond line"}},
		{"stdin", []string{"--result-file", "-"}, "stdin result\n", map[string]string{"Result": "stdin result"}},
		{"sections and result", []string{"--section-stdin", "--result", "done"}, `{"Observation":"observed"}`, map[string]string{"Observation": "observed", "Result": "done"}},
		{"sections and file", []string{"--section-stdin", "--result-file", path}, `{"Observation":"observed"}`, map[string]string{"Observation": "observed", "Result": "file result\nsecond line"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			input := sectionInputFlags(fs)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			got, err := input.read(fs, strings.NewReader(tc.stdin))
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("sections = %#v, error = %v, want %#v", got, err, tc.want)
			}
		})
	}
	for _, args := range [][]string{
		{"--result", "a", "--result-file", path},
		{"--section-stdin", "--result-file", "-"},
		{"--section-stdin", "--result", "conflict"},
		{"--result-file", path + ".missing"},
	} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		input := sectionInputFlags(fs)
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		if _, err := input.read(fs, strings.NewReader(`{"Result":"existing"}`)); err == nil {
			t.Fatalf("conflicting or missing input accepted: %v", args)
		}
	}
}

func TestTransitionResultIsAtomicAndReportsWakeups(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	dbPath := filepath.Join(t.TempDir(), "backlog.sqlite3")
	store, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	fixtureObjectives(t, store)
	t.Cleanup(func() { store.Close() })
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "DOING", Owner: "agent:test", Sections: []Section{{Name: "Result", Body: "before"}}},
		Card{ID: "B-002", Status: "BLOCKED", Dependencies: []CardDependency{{DependsOnCardID: "B-001", RequiredStatus: "VERIFY", Mode: "BLOCKING"}}},
	)
	failed := runBacklogCLI(t, dbPath, "", "transition", "B-001", "--status", "VALIDATED", "--result", "must not be saved", "--expect-card-version", "0", "--actor", "agent:test", "--reason", "invalid transition", "--format", "json")
	if failed.err == nil || failed.stdout != "" {
		t.Fatalf("invalid transition succeeded: %#v", failed)
	}
	card, err := store.getCard("B-001")
	if err != nil || card.Version != 0 || card.Status != "DOING" || sectionBody(card.Sections, "Result") != "before" || len(card.History) != 0 {
		t.Fatalf("invalid transition was not atomic: %#v, %v", card, err)
	}
	// Fail after the status SQL has run to verify that both writes roll back.
	if _, err := store.db.Exec(`CREATE TRIGGER reject_test_result BEFORE INSERT ON card_sections
        WHEN NEW.name = 'Result' AND NEW.body = 'reject'
        BEGIN SELECT RAISE(ABORT, 'test Result failure'); END`); err != nil {
		t.Fatal(err)
	}
	failed = runBacklogCLI(t, dbPath, "", "transition", "B-001", "--status", "VERIFY", "--result", "reject", "--expect-card-version", "0", "--actor", "agent:test", "--reason", "failed storage", "--format", "json")
	if failed.err == nil || failed.stdout != "" || !strings.Contains(failed.stderr, "test Result failure") {
		t.Fatalf("Result storage error = %#v", failed)
	}
	card, err = store.getCard("B-001")
	if err != nil || card.Version != 0 || card.Status != "DOING" || sectionBody(card.Sections, "Result") != "before" || len(card.History) != 0 {
		t.Fatalf("Result storage failure was not atomic: %#v, %v", card, err)
	}
	var receipt cardMutationReceipt
	requireCLIJSON(t, runBacklogCLI(t, dbPath, "", "transition", "B-001", "--status", "VERIFY", "--result", "after", "--expect-card-version", "0", "--actor", "agent:test", "--reason", "verified", "--format", "json"), &receipt)
	if receipt.Version != 1 || !reflect.DeepEqual(receipt.Woke, []string{"B-002"}) {
		t.Fatalf("wake receipt = %#v", receipt)
	}
	card, err = store.getCard("B-001")
	if err != nil || card.Version != 1 || card.Status != "VERIFY" || sectionBody(card.Sections, "Result") != "after" {
		t.Fatalf("transition missing result: %#v, %v", card, err)
	}
	dependent, err := store.getCard("B-002")
	if err != nil || dependent.Status != "INVESTIGATE" || dependent.Version != 1 {
		t.Fatalf("dependent was not woken: %#v, %v", dependent, err)
	}
	revision, err := store.revision()
	if err != nil || revision != 1 {
		t.Fatalf("transition and Result must share one revision, got %d: %v", revision, err)
	}
}
