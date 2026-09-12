package main

import (
	"strings"
	"testing"
)

func TestUnlinkedInterventionLifecycleAndRelinking(t *testing.T) {
	s := testStore(t)
	id, err := s.addCard(NewCard{Title: "Batch related reads", Actor: "skill:isucon-use-solution", Reason: "Remove item-proportional database round trips"}, mutation{Actor: "skill:isucon-use-solution"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	sections := map[string]string{
		sectionHypothesis:     "Reduce database wait to advance scoring actions",
		sectionChangeBoundary: "Batch related reads in the list endpoint",
		sectionEvaluation:     "Verify constant query count and unchanged response semantics",
	}
	for _, missing := range []string{sectionHypothesis, sectionChangeBoundary, sectionEvaluation} {
		incomplete := map[string]string{}
		for key, value := range sections {
			if key != missing {
				incomplete[key] = value
			}
		}
		_, err := s.resolveCardAndWake(id, "READY", CardPatch{Sections: incomplete}, mutation{Actor: "skill:isucon-investigate", ExpectedCardVersion: intPtr(0)}, "check incomplete contract")
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Fatalf("missing %s: %v", missing, err)
		}
	}
	if _, err := s.resolveCardAndWake(id, "READY", CardPatch{Sections: sections}, mutation{Actor: "skill:isucon-investigate", ExpectedCardVersion: intPtr(0)}, "contract verified"); err != nil {
		t.Fatal(err)
	}
	card, err := s.getCard(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.TargetIDs) != 0 || card.Status != "READY" {
		t.Fatalf("card = %#v", card)
	}
	targetID := fixtureTarget(t, s)
	target, _ := s.getTarget(targetID)
	if err := s.setTargetLink(targetID, id, true, "", "", target.Version, mutation{Actor: "human:test", ExpectedCardVersion: intPtr(card.Version)}, "existing target now fits"); err != nil {
		t.Fatal(err)
	}
	card, _ = s.getCard(id)
	if card.PrimaryTargetID != targetID {
		t.Fatalf("primary = %q", card.PrimaryTargetID)
	}
	target, _ = s.getTarget(targetID)
	if err := s.setTargetLink(targetID, id, false, "", "", target.Version, mutation{Actor: "human:test", ExpectedCardVersion: intPtr(card.Version)}, "correct an unrelated target link"); err != nil {
		t.Fatal(err)
	}
	card, _ = s.getCard(id)
	if err := s.updateCard(id, CardPatch{Values: map[string]string{"status": "DOING", "owner": "human:test"}}, mutation{Actor: "human:test", ExpectedCardVersion: intPtr(card.Version)}, "implement standalone change"); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"VERIFY", "APPLIED"} {
		card, _ = s.getCard(id)
		if err := s.transitionCard(id, status, mutation{Actor: "human:test", ExpectedCardVersion: intPtr(card.Version)}, "advance standalone change"); err != nil {
			t.Fatal(err)
		}
		if err := s.validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnlinkedInterventionStillRequiresBlockingDependency(t *testing.T) {
	s := testStore(t)
	seedBacklog(t, s, 0, "B-003",
		Card{ID: "B-001", Status: "INVESTIGATE", Title: "dependent", Dependencies: []CardDependency{{DependsOnCardID: "B-002", RequiredStatus: "APPLIED", Mode: "BLOCKING", Reason: "needs prerequisite"}}},
		Card{ID: "B-002", Status: "INVESTIGATE", Title: "prerequisite"})
	prepareReadyContract(t, s, "B-001")
	if _, err := s.db.Exec(`DELETE FROM target_interventions WHERE card_id='B-001'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveCardAndWake("B-001", "READY", CardPatch{}, mutation{Actor: "skill:isucon-investigate", ExpectedCardVersion: intPtr(0)}, "check dependency"); err == nil || !strings.Contains(err.Error(), "dependency") {
		t.Fatalf("dependency error = %v", err)
	}
}
