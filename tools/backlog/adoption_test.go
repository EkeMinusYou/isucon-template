package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdoptCardsMatchingRecordsOneAtomicEvent(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 7, "B-003",
		Card{ID: "B-001", Status: "APPLIED", Title: "first", Fingerprint: "first:v1", History: []HistoryEntry{{Actor: "agent:first", Body: "created"}}},
		Card{ID: "B-002", Status: "APPLIED", Title: "second", Fingerprint: "second:v1", History: []HistoryEntry{{Actor: "agent:second", Body: "created"}}})
	first, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.getCard("B-002")
	if err != nil {
		t.Fatal(err)
	}
	score := int64(0)
	passed := false
	event := AdoptionEvent{
		RunID: "20260901-120000", AdoptedAt: "2026-09-01T12:10:00Z", Actor: "task:pass",
		Forced: true, Score: &score, Passed: &passed, ComparisonStatus: "none",
		ManifestSHA256: "sha256:" + strings.Repeat("0", 64), SnapshotRevision: 7,
	}
	cards, err := store.adoptCardsMatching([]string{"B-001", "B-002"}, map[string]string{
		"B-001": cardDefinitionHash(first), "B-002": cardDefinitionHash(second),
	}, event, "forced adoption")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("adopted cards = %d, want 2", len(cards))
	}
	for _, id := range []string{"B-001", "B-002"} {
		card, err := store.getCard(id)
		if err != nil {
			t.Fatal(err)
		}
		if card.Status != "VALIDATED" || !strings.Contains(card.History[len(card.History)-1].Body, "forced adoption") {
			t.Fatalf("card %s after adoption = %#v", id, card)
		}
	}
	var events, eventCards, forced int
	var storedScore int64
	if err := store.db.QueryRow(`SELECT COUNT(*), forced, score FROM adoption_events`).Scan(&events, &forced, &storedScore); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM adoption_event_cards`).Scan(&eventCards); err != nil {
		t.Fatal(err)
	}
	if events != 1 || eventCards != 2 || forced != 1 || storedScore != 0 {
		t.Fatalf("event counts/values = %d/%d forced=%d score=%d", events, eventCards, forced, storedScore)
	}
	if revision, err := store.revision(); err != nil || revision != 8 {
		t.Fatalf("backlog revision = %d, %v; want one transaction at revision 8", revision, err)
	}

	outcomes := filepath.Join(t.TempDir(), "outcomes.tsv")
	if err := store.writeOutcomes(outcomes); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outcomes)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "comparison_status\tforced\n") || !strings.Contains(text, "\t20260901-120000\t0\tnone\tunknown\tunknown\t2\tnone\ttrue\n") {
		t.Fatalf("outcomes.tsv =\n%s", text)
	}
}

func TestAdoptCardsMatchingRollsBackEveryCardOnMismatch(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 3, "B-003",
		Card{ID: "B-001", Status: "APPLIED", Title: "first"},
		Card{ID: "B-002", Status: "APPLIED", Title: "second"})
	first, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	event := AdoptionEvent{
		RunID: "20260901-120000", AdoptedAt: "2026-09-01T12:10:00Z", Actor: "task:pass",
		ComparisonStatus: "none", ManifestSHA256: "sha256:" + strings.Repeat("0", 64), SnapshotRevision: 3,
	}
	_, err = store.adoptCardsMatching([]string{"B-001", "B-002"}, map[string]string{
		"B-001": cardDefinitionHash(first), "B-002": "sha256:stale",
	}, event, "adoption")
	if err == nil || !strings.Contains(err.Error(), "definition differs") {
		t.Fatalf("adoption mismatch error = %v", err)
	}
	for _, id := range []string{"B-001", "B-002"} {
		card, getErr := store.getCard(id)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if card.Status != "APPLIED" {
			t.Fatalf("card %s status = %s, want APPLIED", id, card.Status)
		}
	}
	var events int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM adoption_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatalf("adoption events = %d, want 0", events)
	}
}
