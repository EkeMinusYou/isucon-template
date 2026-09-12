package main

import (
	"io"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestTargetListHidesStatusInMainQueueOnly(t *testing.T) {
	targets := []Target{{ID: "A-001", Status: "ACTIVE", Priority: "P1", Title: "current bottleneck"}}
	mainQueue := captureOutput(t, func() { printTargetList(targets, 100, false) })
	explicitList := captureOutput(t, func() { printTargetList(targets, 100, true) })
	if strings.Contains(mainQueue, "[ACTIVE]") {
		t.Fatalf("main backlog repeats ACTIVE status: %q", mainQueue)
	}
	if !strings.Contains(explicitList, "[ACTIVE]") {
		t.Fatalf("explicit target list omits status: %q", explicitList)
	}
	if !strings.Contains(mainQueue, "A-001 P1     current bottleneck") {
		t.Fatalf("main target list does not use the shared title column: %q", mainQueue)
	}
}

func TestPrintListUsesSingularCountLabels(t *testing.T) {
	body := captureOutput(t, func() {
		printList(
			[]Card{{ID: "B-001", Status: "READY", Title: "title"}},
			[]Target{{ID: "A-001", Status: "ACTIVE", Title: "target"}},
			[]Objective{{ID: "O-001", Status: "ACTIVE", Mode: "MAXIMIZE", Title: "objective"}},
			1, "B-002", "A-002", "O-002", 100, false,
		)
	})

	for _, expected := range []string{"next B-002/A-002/O-002", "1 objective · 1 intervention · 1 target", "OBJECTIVES (1)", "O-001        objective", "1 intervention · active"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("list output missing %q: %q", expected, body)
		}
	}
	if strings.Contains(body, "1 objectives") || strings.Contains(body, "1 interventions") || strings.Contains(body, "1 targets") {
		t.Fatalf("list output uses plural nouns for singular counts: %q", body)
	}
}

func TestCardLineShowsSkillOwner(t *testing.T) {
	owner := "agent:isucon-analyze-alp"
	line := cardLine(Card{
		ID:       "B-145",
		Priority: "P1",
		Area:     "mixed",
		Owner:    owner,
		Title:    "title",
	}, 110)

	if !strings.Contains(line, "agent:isucon-analyze-alp") {
		t.Fatalf("card line = %q, want the skill name to remain visible", line)
	}
	if strings.Contains(line, "agent:isu…") {
		t.Fatalf("card line truncates owner before the skill name: %q", line)
	}
}

func TestTruncatedAndUntruncatedTitlesAlignSeparator(t *testing.T) {
	cards := []Card{
		{ID: "B-001", Status: "APPLIED", Priority: "P1", Area: "mixed", Owner: "agent:test", Title: "This title is long enough to be truncated at the shared list width"},
		{ID: "B-002", Status: "APPLIED", Priority: "P1", Area: "mixed", Owner: "agent:test", Title: "short title"},
	}
	layout := makeListLayout(cards, nil, 80, false)
	longLine := cardLineWithLayout(cards[0], 80, layout)
	shortLine := cardLineWithLayout(cards[1], 80, layout)

	longSeparator := strings.Index(longLine, " │ ")
	shortSeparator := strings.Index(shortLine, " │ ")
	if longSeparator < 0 || shortSeparator < 0 {
		t.Fatalf("separator missing: long=%q short=%q", longLine, shortLine)
	}
	if utf8.RuneCountInString(longLine[:longSeparator]) != utf8.RuneCountInString(shortLine[:shortSeparator]) {
		t.Fatalf("title separators do not align: long=%q short=%q", longLine, shortLine)
	}
}

func TestCardLineShowsCompactForwardAndReverseRelations(t *testing.T) {
	line := cardLine(Card{
		ID:           "B-001",
		Title:        "dependent card",
		Dependencies: []CardDependency{{DependsOnCardID: "B-010", Mode: "BLOCKING", RequiredStatus: "VALIDATED", TargetStatus: "INVESTIGATE"}},
		Unblocks:     []CardDependency{{CardID: "B-011", Mode: "ORDERING", RequiredStatus: "VERIFY", TargetStatus: "READY"}},
	}, 140)
	for _, expected := range []string{"dep:B-010", "unblocks:B-011"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("dependency list missing %q: %q", expected, line)
		}
	}
	for _, omitted := range []string{"VALIDATED", "INVESTIGATE", "ORDERING", "VERIFY", "READY"} {
		if strings.Contains(line, omitted) {
			t.Fatalf("compact dependency list contains detail %q: %q", omitted, line)
		}
	}
}

func TestCardLineShowsLongTitleAtDefaultWidth(t *testing.T) {
	title := "write POSTのレスポンス構築をコミット後へ移してロック保持時間を短縮する"
	line := cardLine(Card{
		ID:       "B-146",
		Priority: "P2",
		Area:     "mixed",
		Owner:    "agent:isucon-optimize:B-146",
		Title:    title,
	}, defaultListWidth)

	if !strings.Contains(line, title) {
		t.Fatalf("card line = %q, want the full title %q", line, title)
	}
}

func TestCardLinePrioritizesTitleOverLongAssessment(t *testing.T) {
	title := "event INSERTをDB-scoped prepared statementで再利用する"
	line := cardLine(Card{
		ID:       "B-403",
		Priority: "P1",
		Area:     "app",
		Title:    title,
	}, defaultListWidth)

	if !strings.Contains(line, title) {
		t.Fatalf("card line = %q, want the full title %q", line, title)
	}
	if width(line) > defaultListWidth {
		t.Fatalf("card line width = %d, want at most %d: %q", width(line), defaultListWidth, line)
	}
}

func TestCardLineShowsActiveTargetMarker(t *testing.T) {
	line := cardLine(Card{
		ID: "B-501", Priority: "P1", Area: "app", Title: "dominant request path", ActiveTargetIDs: []string{"A-001"},
	}, defaultListWidth)
	if !strings.Contains(line, "B-501 P1  [A-001]  dominant request path") {
		t.Fatalf("card line = %q, want active target marker immediately after priority", line)
	}
}

func TestCardLineShowsUnlinkedMarkerAfterPriority(t *testing.T) {
	line := cardLine(Card{
		ID: "B-502", Priority: "P1", Area: "app", Title: "ordinary optimization",
	}, defaultListWidth)
	if !strings.Contains(line, "B-502 P1  -  ordinary optimization") {
		t.Fatalf("card line = %q, want unlinked marker immediately after priority", line)
	}
}

func TestCardLinePlacesCardOnlyMetadataAfterTitle(t *testing.T) {
	line := cardLine(Card{
		ID: "B-001", Priority: "P1", Area: "mixed", Owner: "agent:test", Title: "title",
	}, 110)

	if !strings.Contains(line, "B-001 P1  -  title │ agent:test") {
		t.Fatalf("card line does not place card-only metadata after the title: %q", line)
	}
}

func TestCardLineOmitsEmptyRelationColumn(t *testing.T) {
	card := Card{ID: "B-001", Priority: "P1", Area: "mixed", Owner: "agent:test", Title: "title"}
	target := Target{ID: "A-001", Priority: "P1", Title: "target", CardIDs: []string{"B-001"}}
	layout := makeListLayout([]Card{card}, []Target{target}, 110, false)
	line := cardLineWithLayout(card, 110, layout)

	if strings.Count(line, "│") != 1 {
		t.Fatalf("card line renders an empty relation column: %q", line)
	}
}

func TestCardLinePlacesRelationAfterMetadata(t *testing.T) {
	line := cardLine(Card{
		ID: "B-001", Priority: "P1", Area: "mixed", Owner: "agent:test", Title: "title",
		Dependencies: []CardDependency{{DependsOnCardID: "B-010"}},
	}, 110)

	metadataPosition := strings.Index(line, "agent:test")
	relationPosition := strings.Index(line, "dep:B-010")
	if metadataPosition == -1 || relationPosition == -1 || metadataPosition > relationPosition {
		t.Fatalf("card relation is not placed after metadata: %q", line)
	}
}

func TestCardAndTargetColumnsAlign(t *testing.T) {
	card := Card{ID: "B-001", Priority: "P1", Title: "card title", Area: "app"}
	target := Target{ID: "A-001", Priority: "P1", Title: "target title", CardIDs: []string{"B-001"}}
	layout := makeListLayout([]Card{card}, []Target{target}, 100, false)
	cardLine := cardLineWithLayout(card, 100, layout)
	targetLine := targetLineWithLayout(target, false, layout)

	if strings.Index(cardLine, card.Title) != strings.Index(targetLine, target.Title) {
		t.Fatalf("title columns differ: card=%q target=%q", cardLine, targetLine)
	}
	cardBoundary := strings.Index(cardLine, "│")
	targetBoundary := strings.Index(targetLine, "│")
	if width(cardLine[:cardBoundary]) != width(targetLine[:targetBoundary]) {
		t.Fatalf("title boundaries differ: card=%q target=%q", cardLine, targetLine)
	}
}

func TestCardAndTargetColumnsAlignWithAmbiguousWidthArrow(t *testing.T) {
	card := Card{ID: "B-605", Priority: "P1", Title: "hot owner同居nginx→appをUnix socket transportへ切り替える", Area: "infra"}
	target := Target{ID: "A-006", Priority: "P0", Title: "Hot request serial response floor"}
	layout := makeListLayout([]Card{card}, []Target{target}, defaultListWidth, false)
	cardLine := cardLineWithLayout(card, defaultListWidth, layout)
	targetLine := targetLineWithLayout(target, false, layout)

	cardBoundary := strings.Index(cardLine, "│")
	targetBoundary := strings.Index(targetLine, "│")
	if width(cardLine[:cardBoundary]) != width(targetLine[:targetBoundary]) {
		t.Fatalf("title boundaries differ with arrow: card=%q target=%q", cardLine, targetLine)
	}
	if width("→") != 1 {
		t.Fatalf("arrow width = %d, want 1 terminal cell", width("→"))
	}
}

func TestFourDigitIDsKeepColumnsAligned(t *testing.T) {
	card := Card{ID: "B-1000", Priority: "P1", Title: "card title", Area: "app"}
	target := Target{ID: "A-1000", Priority: "P0", Title: "target title"}
	layout := makeListLayout([]Card{card}, []Target{target}, 100, false)
	cardLine := cardLineWithLayout(card, 100, layout)
	targetLine := targetLineWithLayout(target, false, layout)
	if strings.Index(cardLine, card.Title) != strings.Index(targetLine, target.Title) {
		t.Fatalf("four-digit title columns differ: card=%q target=%q", cardLine, targetLine)
	}
	if got := formatID(1000); got != "B-1000" {
		t.Fatalf("formatID(1000) = %q", got)
	}
	if got := formatTargetID(1000); got != "A-1000" {
		t.Fatalf("formatTargetID(1000) = %q", got)
	}
	if got := formatObjectiveID(1000); got != "O-1000" {
		t.Fatalf("formatObjectiveID(1000) = %q", got)
	}
}

func TestPrintCardHidesEmptyOptionalMetadata(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = oldStdout }()
	printCard(Card{ID: "B-001", Version: 2, Status: "INVESTIGATE", Title: "title", Priority: "P1"})
	closeErr := writer.Close()
	os.Stdout = oldStdout
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	output := string(body)
	if !strings.Contains(output, "Card version:") || !strings.Contains(output, "Priority:") {
		t.Fatalf("required metadata missing: %q", output)
	}
	for _, emptyField := range []string{"Owner:", "Compare RUNs:", "Depends on:", "BLOCKED contract:"} {
		if strings.Contains(output, emptyField) {
			t.Fatalf("empty field %q was rendered: %q", emptyField, output)
		}
	}
}
