package main

import (
	"strings"
	"testing"
)

func TestSharedTargetResolutionIsIndependentOfInterventionValidation(t *testing.T) {
	s := testStore(t)
	seedBacklog(t, s, 0, "B-003",
		Card{ID: "B-001", Status: "APPLIED", Owner: "verifier:test", Title: "batch reads"},
		Card{ID: "B-002", Status: "APPLIED", Owner: "verifier:test", Title: "reuse computation"})
	id := fixtureTarget(t, s)
	for _, cardID := range []string{"B-001", "B-002"} {
		adoptFixtureCard(t, s, cardID, "saved results support adoption")
		target, err := s.getTarget(id)
		if err != nil {
			t.Fatal(err)
		}
		if target.Status != "ACTIVE" || target.CompletionEvidence != "" {
			t.Fatalf("intervention auto-resolved target: %#v", target)
		}
	}
	target, _ := s.getTarget(id)
	evidence := "equal-load saved results show response time below 5 ms"
	if err := s.transitionTarget(id, "RESOLVED", "", target.Version, mutation{Actor: "human:test", Operation: "target.transition", CompletionEvidence: evidence}, "independently confirmed target goal"); err != nil {
		t.Fatal(err)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	resolved, _ := s.getTarget(id)
	if resolved.Status != "RESOLVED" || len(resolved.CardIDs) != 2 || resolved.CompletionEvidence != evidence {
		t.Fatalf("resolved shared target: %#v", resolved)
	}
	for _, cardID := range resolved.CardIDs {
		card, err := s.getCard(cardID)
		if err != nil {
			t.Fatal(err)
		}
		if card.Status != "VALIDATED" || len(card.History) != 1 || !strings.Contains(card.History[0].Body, "saved results support adoption") {
			t.Fatalf("terminal history changed: %#v", card)
		}
	}
	recurringID, err := s.addTarget(NewTarget{ObjectiveID: "O-001", Title: "request latency under increased load", Fingerprint: resolved.Fingerprint, Scope: resolved.Scope, Axis: resolved.Axis, Goal: "under 3 ms at twice the request rate", Evaluation: "compare saved results at doubled request rate", Evidence: "request rate doubled after deployment", PreviousTargetID: id}, mutation{Actor: "human:test", Operation: "target.add"}, "increased load requires a new latency goal")
	if err != nil {
		t.Fatal(err)
	}
	recurring, _ := s.getTarget(recurringID)
	if recurring.Status != "ACTIVE" || recurring.CompletionEvidence != "" || recurring.Goal == resolved.Goal || recurring.PreviousTargetID != id {
		t.Fatalf("recurrence inherited outcome: %#v", recurring)
	}
	if !strings.Contains(recurring.History[0].Body, "increased load") {
		t.Fatalf("recurrence reason missing: %#v", recurring.History)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestResolvedTargetPreservesExistingQueueCardsButRejectsNewReadyAdmission(t *testing.T) {
	s := testStore(t)
	seedBacklog(t, s, 0, "B-003", Card{ID: "B-001", Status: "READY", Title: "queued improvement"}, Card{ID: "B-002", Status: "INVESTIGATE", Title: "unexamined alternative"})
	prepareReadyContract(t, s, "B-002")
	id := fixtureTarget(t, s)
	target, _ := s.getTarget(id)
	if err := s.transitionTarget(id, "RESOLVED", "", target.Version, mutation{Actor: "human:test", CompletionEvidence: "saved results meet the request latency goal"}, "other changes achieved the shared goal"); err != nil {
		t.Fatal(err)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"B-001": "READY", "B-002": "INVESTIGATE"} {
		card, err := s.getCard(id)
		if err != nil {
			t.Fatal(err)
		}
		if card.Status != want {
			t.Fatalf("card %s changed status to %s", id, card.Status)
		}
	}
	if err := s.transitionCard("B-002", "READY", mutation{Actor: "skill:isucon-investigate", ExpectedCardVersion: intPtr(0)}, "attempt admission after target completion"); err == nil {
		t.Fatal("new READY admission accepted a completed target")
	}
}
