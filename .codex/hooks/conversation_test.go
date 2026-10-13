package hooks

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	t                            *testing.T
	temp, root, hook, transcript string
	env                          []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	temp := t.TempDir()
	f := &fixture{t: t, temp: temp, root: filepath.Join(temp, "project with spaces"), transcript: filepath.Join(temp, "transcript.jsonl")}
	f.hook = filepath.Join(f.root, ".codex/hooks/conversation.sh")
	if err := os.MkdirAll(filepath.Dir(f.hook), 0755); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("conversation.sh")
	if err != nil {
		t.Fatal(err)
	}
	f.write(f.hook, string(script))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "TMPDIR=") {
			f.env = append(f.env, value)
		}
	}
	f.env = append(f.env, "TMPDIR="+temp)
	f.git("init", "-q")
	f.git("config", "user.name", "Hook Test")
	f.git("config", "user.email", "hook@example.invalid")
	f.git("config", "commit.gpgsign", "false")
	f.git("add", ".")
	f.git("commit", "-qm", "Initial fixture")
	return f
}

func (f *fixture) write(path, text string) {
	f.t.Helper()
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", f.root}, args...)...)
	cmd.Env = f.env
	output, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (f *fixture) run(provider, action, eventName, cwd string) {
	f.t.Helper()
	if cwd == "" {
		cwd = f.root
	}
	event, err := json.Marshal(map[string]string{
		"session_id": "ordinary-session", "cwd": cwd,
		"transcript_path": f.transcript, "hook_event_name": eventName,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	timeout := 30 * time.Second
	if provider == "codex" && eventName == "SessionEnd" {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", f.hook, action, provider)
	cmd.Env, cmd.Stdin = f.env, strings.NewReader(string(event))
	output, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("hook: %v (deadline: %v)\n%s", err, ctx.Err(), output)
	}
	if eventName != "invalid-transcript" && len(output) != 0 {
		f.t.Fatalf("unexpected hook output: %s", output)
	}
}

func (f *fixture) output() (string, string) {
	f.t.Helper()
	paths, err := filepath.Glob(filepath.Join(f.root, "docs/conversation/*.md"))
	if err != nil || len(paths) != 1 {
		f.t.Fatalf("expected one conversation, got %v: %v", paths, err)
	}
	text, err := os.ReadFile(paths[0])
	if err != nil {
		f.t.Fatal(err)
	}
	return paths[0], string(text)
}

func assertContains(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(text, value) {
			t.Errorf("missing %q", value)
		}
	}
	if strings.Contains(text, "OMITTED_") {
		t.Error("tool results or reasoning leaked into the conversation")
	}
}

const codexTranscript = `{"timestamp":"2026-10-14T00:00:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Ordinary request"}]}}
{"timestamp":"2026-10-14T00:00:01Z","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}"}}
{"timestamp":"2026-10-14T00:00:02Z","payload":{"type":"function_call_output","output":"OMITTED_TOOL_RESULT"}}
{"timestamp":"2026-10-14T00:00:03Z","payload":{"type":"reasoning","content":"OMITTED_REASONING"}}
{"timestamp":"2026-10-14T00:00:05Z","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Finished answer"}]}}
`

func TestCodexExportPreservesUnrelatedStagedChanges(t *testing.T) {
	f := newFixture(t)
	f.write(f.transcript, codexTranscript)
	f.write(filepath.Join(f.root, "unrelated.txt"), "Keep staged")
	f.git("add", "unrelated.txt")
	f.run("codex", "export", "Stop", "")
	path, text := f.output()
	assertContains(t, text, "# Codex conversation", "Ordinary request", "Finished answer", "```sh\npwd", "Worked for 5s")
	if got := f.git("diff", "--cached", "--name-only"); got != "unrelated.txt" {
		t.Errorf("staged changes: %q", got)
	}
	relative, err := filepath.Rel(f.root, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.git("show", "--pretty=format:", "--name-only", "HEAD"); got != relative {
		t.Errorf("committed files: %q", got)
	}
}

func TestClaudeExportIncludesToolsAndModel(t *testing.T) {
	f := newFixture(t)
	f.write(f.transcript, `{"type":"user","timestamp":"2026-10-14T00:00:00Z","message":{"role":"user","content":"Ordinary Claude request"}}
{"type":"assistant","timestamp":"2026-10-14T00:00:01Z","message":{"role":"assistant","model":"test-model","content":[{"type":"thinking","thinking":"OMITTED_REASONING"},{"type":"text","text":"Checking now"},{"type":"tool_use","id":"tool-1","name":"Bash","input":{"command":"pwd"}}]}}
{"type":"user","timestamp":"2026-10-14T00:00:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"OMITTED_TOOL_RESULT"}]}}
{"type":"assistant","timestamp":"2026-10-14T00:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Claude finished"}]}}
`)
	f.run("claude", "mark", "SessionStart", "")
	f.run("claude", "export", "Stop", "")
	_, text := f.output()
	assertContains(t, text, "# Claude Code conversation", "Ordinary Claude request", "Checking now", "```sh\npwd", "Claude finished", "- Model: test-model", "Worked for 5s")
}

func TestResumeAndSessionEndReuseFileWithinDeadline(t *testing.T) {
	f := newFixture(t)
	f.write(f.transcript, codexTranscript)
	f.run("codex", "mark", "SessionStart", "")
	f.run("codex", "export", "Stop", "")
	path, _ := f.output()
	head := f.git("rev-parse", "HEAD")
	f.run("codex", "export", "SessionEnd", "")
	if got := f.git("rev-parse", "HEAD"); got != head {
		t.Error("unchanged conversation created another commit")
	}
	if err := os.RemoveAll(filepath.Join(f.temp, "conversation-hook-sessions")); err != nil {
		t.Fatal(err)
	}
	f.write(f.transcript, codexTranscript+"{\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":\"Resumed request\"}}\n")
	f.run("codex", "export", "SessionEnd", "")
	resumedPath, text := f.output()
	if resumedPath != path {
		t.Error("resumed session created a different file")
	}
	assertContains(t, text, "Resumed request")
}

func TestOtherRepositoryIsRejected(t *testing.T) {
	f := newFixture(t)
	other := filepath.Join(f.temp, "other")
	if err := os.Mkdir(other, 0755); err != nil {
		t.Fatal(err)
	}
	f.git("-C", other, "init", "-q")
	f.write(f.transcript, codexTranscript)
	f.run("codex", "export", "Stop", other)
	for _, root := range []string{other, f.root} {
		if _, err := os.Stat(filepath.Join(root, "docs")); !os.IsNotExist(err) {
			t.Errorf("unexpected docs directory in %s: %v", root, err)
		}
	}
}

func TestInvalidTranscriptPreservesSavedConversation(t *testing.T) {
	f := newFixture(t)
	f.write(f.transcript, codexTranscript)
	f.run("codex", "export", "Stop", "")
	_, original := f.output()
	f.write(f.transcript, "{incomplete")
	f.run("codex", "export", "invalid-transcript", "")
	_, text := f.output()
	if text != original {
		t.Error("invalid transcript overwrote the saved conversation")
	}
}
