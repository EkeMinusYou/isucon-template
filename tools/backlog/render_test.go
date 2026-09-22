package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureOutput(t *testing.T, render func()) string {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = oldStdout }()
	render()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldStdout
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestListShowsEntitiesAndRelations(t *testing.T) {
	body := captureOutput(t, func() {
		printList(
			[]Card{
				{ID: "B-001", Status: "READY", Priority: "P1", Title: "linked change", Owner: "agent:isucon-analyze-alp", ActiveTargetIDs: []string{"A-001"}, Dependencies: []CardDependency{{DependsOnCardID: "B-010"}}, Unblocks: []CardDependency{{CardID: "B-011"}}},
				{ID: "B-002", Status: "INVESTIGATE", Title: "standalone change"},
			},
			[]Target{{ID: "A-001", Status: "ACTIVE", Title: "target"}},
			[]Objective{{ID: "O-001", Status: "ACTIVE", Mode: "MAXIMIZE", Priority: "P0", Title: "objective"}},
			1, "B-003", "A-002", "O-002", defaultListWidth, false,
		)
	})
	for _, expected := range []string{"B-001", "linked change", "agent:isucon-analyze-alp", "[A-001]", "dep:B-010", "unblocks:B-011", "B-002", "standalone change", "O-001", "P0", "objective", "target", "next B-003/A-002/O-002"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("list missing %q: %q", expected, body)
		}
	}
	explicit := captureOutput(t, func() { printTargetList([]Target{{ID: "A-001", Status: "ACTIVE", Title: "target"}}, 100, true) })
	if !strings.Contains(explicit, "[ACTIVE]") {
		t.Fatalf("explicit target list omits status: %q", explicit)
	}
}

func TestListLayoutPreservesReadableTitles(t *testing.T) {
	cards := []Card{
		{ID: "B-1000", Priority: "P1", Title: "hot owner同居nginx→appをUnix socket transportへ切り替える", Owner: "agent:test"},
		{ID: "B-002", Priority: "P1", Title: "short title", Owner: "agent:test"},
		{ID: "B-003", Priority: "P1", Title: strings.Repeat("long title ", 30), Owner: "agent:test"},
	}
	target := Target{ID: "A-1000", Priority: "P0", Title: "target title"}
	layout := makeListLayout(cards, []Target{target}, defaultListWidth, false)
	targetLine := targetLineWithLayout(target, false, layout)
	boundary := strings.Index(targetLine, "│")
	if boundary < 0 {
		t.Fatalf("target boundary missing: %q", targetLine)
	}
	for i, card := range cards {
		line := cardLineWithLayout(card, defaultListWidth, layout)
		separator := strings.Index(line, "│")
		if separator < 0 || width(line[:separator]) != width(targetLine[:boundary]) || width(line) > defaultListWidth {
			t.Fatalf("unaligned or oversized line: %q; target: %q", line, targetLine)
		}
		if !strings.Contains(line, card.ID) || !strings.Contains(line, card.Owner) || (i < 2 && !strings.Contains(line, card.Title)) {
			t.Fatalf("unreadable card: %q", line)
		}
	}
	if width("→") != 1 {
		t.Fatalf("arrow width = %d, want 1 terminal cell", width("→"))
	}
}

func TestPrintCardShowsMutationMetadata(t *testing.T) {
	output := captureOutput(t, func() {
		printCard(Card{ID: "B-001", Version: 2, Status: "INVESTIGATE", Title: "title", Priority: "P1"})
	})
	if !strings.Contains(output, "Card version:") || !strings.Contains(output, "Priority:") {
		t.Fatalf("required metadata missing: %q", output)
	}
}
