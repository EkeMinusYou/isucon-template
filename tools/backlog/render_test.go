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

func TestConstraintListHidesStatusInMainQueueOnly(t *testing.T) {
	constraints := []Constraint{{ID: "A-001", Status: "ACTIVE", Priority: "P1", Title: "current bottleneck"}}
	mainQueue := captureOutput(t, func() { printConstraintList(constraints, 100, false) })
	explicitList := captureOutput(t, func() { printConstraintList(constraints, 100, true) })
	if strings.Contains(mainQueue, "[ACTIVE]") {
		t.Fatalf("main backlog repeats ACTIVE status: %q", mainQueue)
	}
	if !strings.Contains(explicitList, "[ACTIVE]") {
		t.Fatalf("explicit constraint list omits status: %q", explicitList)
	}
	if !strings.Contains(mainQueue, "A-001 P1     current bottleneck") {
		t.Fatalf("main constraint list does not use the shared title column: %q", mainQueue)
	}
}

func TestPrintListUsesSingularCountLabels(t *testing.T) {
	body := captureOutput(t, func() {
		printList(
			[]Card{{ID: "B-001", Status: "READY", Title: "title"}},
			[]Constraint{{ID: "A-001", Status: "ACTIVE", Title: "constraint"}},
			[]Objective{{ID: "O-001", Status: "ACTIVE", Mode: "MAXIMIZE", Title: "objective"}},
			1, "B-002", "A-002", "O-002", 100, false,
		)
	})

	for _, expected := range []string{"next B-002/A-002/O-002", "1 objective · 1 intervention · 1 constraint", "OBJECTIVES (1)", "O-001 [ACTIVE] MAXIMIZE", "1 intervention · active"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("list output missing %q: %q", expected, body)
		}
	}
	if strings.Contains(body, "1 objectives") || strings.Contains(body, "1 interventions") || strings.Contains(body, "1 constraints") {
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

func TestCardLineShowsActiveConstraintMarker(t *testing.T) {
	line := cardLine(Card{
		ID: "B-501", Priority: "P1", Area: "app", Title: "dominant request path", ActiveConstraintIDs: []string{"A-001"},
	}, defaultListWidth)
	if !strings.Contains(line, "B-501 P1  [A-001]  dominant request path") {
		t.Fatalf("card line = %q, want active constraint marker immediately after priority", line)
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

	if !strings.Contains(line, "B-001 P1  -  title │ mixed  agent:test") {
		t.Fatalf("card line does not place card-only metadata after the title: %q", line)
	}
}

func TestCardLineOmitsEmptyRelationColumn(t *testing.T) {
	card := Card{ID: "B-001", Priority: "P1", Area: "mixed", Owner: "agent:test", Title: "title"}
	constraint := Constraint{ID: "A-001", Priority: "P1", Title: "constraint", CardIDs: []string{"B-001"}}
	layout := makeListLayout([]Card{card}, []Constraint{constraint}, 110, false)
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

func TestCardAndConstraintColumnsAlign(t *testing.T) {
	card := Card{ID: "B-001", Priority: "P1", Title: "card title", Area: "app"}
	constraint := Constraint{ID: "A-001", Priority: "P1", Title: "constraint title", CardIDs: []string{"B-001"}}
	layout := makeListLayout([]Card{card}, []Constraint{constraint}, 100, false)
	cardLine := cardLineWithLayout(card, 100, layout)
	constraintLine := constraintLineWithLayout(constraint, false, layout)

	if strings.Index(cardLine, card.Title) != strings.Index(constraintLine, constraint.Title) {
		t.Fatalf("title columns differ: card=%q constraint=%q", cardLine, constraintLine)
	}
	cardBoundary := strings.Index(cardLine, "│")
	constraintBoundary := strings.Index(constraintLine, "│")
	if width(cardLine[:cardBoundary]) != width(constraintLine[:constraintBoundary]) {
		t.Fatalf("title boundaries differ: card=%q constraint=%q", cardLine, constraintLine)
	}
}

func TestCardAndConstraintColumnsAlignWithAmbiguousWidthArrow(t *testing.T) {
	card := Card{ID: "B-605", Priority: "P1", Title: "hot owner同居nginx→appをUnix socket transportへ切り替える", Area: "infra"}
	constraint := Constraint{ID: "A-006", Priority: "P0", Title: "Hot request serial response floor"}
	layout := makeListLayout([]Card{card}, []Constraint{constraint}, defaultListWidth, false)
	cardLine := cardLineWithLayout(card, defaultListWidth, layout)
	constraintLine := constraintLineWithLayout(constraint, false, layout)

	cardBoundary := strings.Index(cardLine, "│")
	constraintBoundary := strings.Index(constraintLine, "│")
	if width(cardLine[:cardBoundary]) != width(constraintLine[:constraintBoundary]) {
		t.Fatalf("title boundaries differ with arrow: card=%q constraint=%q", cardLine, constraintLine)
	}
	if width("→") != 1 {
		t.Fatalf("arrow width = %d, want 1 terminal cell", width("→"))
	}
}

func TestFourDigitIDsKeepColumnsAligned(t *testing.T) {
	card := Card{ID: "B-1000", Priority: "P1", Title: "card title", Area: "app"}
	constraint := Constraint{ID: "A-1000", Priority: "P0", Title: "constraint title"}
	layout := makeListLayout([]Card{card}, []Constraint{constraint}, 100, false)
	cardLine := cardLineWithLayout(card, 100, layout)
	constraintLine := constraintLineWithLayout(constraint, false, layout)
	if strings.Index(cardLine, card.Title) != strings.Index(constraintLine, constraint.Title) {
		t.Fatalf("four-digit title columns differ: card=%q constraint=%q", cardLine, constraintLine)
	}
	if got := formatID(1000); got != "B-1000" {
		t.Fatalf("formatID(1000) = %q", got)
	}
	if got := formatConstraintID(1000); got != "A-1000" {
		t.Fatalf("formatConstraintID(1000) = %q", got)
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
