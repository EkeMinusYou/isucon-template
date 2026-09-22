package main

import (
	"strings"
	"testing"
)

func newDesignTarget(t *testing.T, s *Store, fingerprint, previous string) string {
	t.Helper()
	id, err := s.addTarget(NewTarget{ObjectiveID: "O-001", Title: "Already fast icon endpoint", Fingerprint: fingerprint, Scope: "GET icon", Axis: "response latency", Goal: "p95 <= 2 ms at the same request rate", Evaluation: "compare finalized equal-load RUNs", Evidence: "current p95 3 ms; two sequential reads", PreviousTargetID: previous}, mutation{Actor: "skill:isucon-analyze", Operation: "target.add"}, "reduce request wait to advance the viewing scenario")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTargetGoalHistoryAndResolutionEvidence(t *testing.T) {
	s := testStore(t)
	id := newDesignTarget(t, s, "icon-latency", "")
	target, _ := s.getTarget(id)
	if target.Goal == "" || target.PrimaryObjectiveID != "O-001" {
		t.Fatalf("target = %#v", target)
	}
	goal := "p95 <= 1.5 ms at the same request rate"
	if err := s.updateTarget(id, TargetPatch{Goal: &goal}, target.Version, mutation{Actor: "human:test"}, "tighten target after baseline review"); err != nil {
		t.Fatal(err)
	}
	target, _ = s.getTarget(id)
	history := target.History[len(target.History)-1].Body
	if !strings.Contains(history, "p95 <= 2 ms") || !strings.Contains(history, goal) {
		t.Fatalf("goal history lost: %s", history)
	}
	if err := s.transitionTarget(id, "RESOLVED", "", target.Version, mutation{Actor: "human:test"}, "implementation accepted"); err == nil {
		t.Fatal("resolution without outcome evidence accepted")
	}
	if err := s.transitionTarget(id, "RESOLVED", "", target.Version, mutation{Actor: "human:test", CompletionEvidence: "saved RUN shows p95 1.4 ms at equal request rate"}, "goal met"); err != nil {
		t.Fatal(err)
	}
}

func TestMergedTargetSurvivorCanComplete(t *testing.T) {
	s := testStore(t)
	original := newDesignTarget(t, s, "original", "")
	survivor := newDesignTarget(t, s, "survivor", "")
	target, _ := s.getTarget(original)
	for _, invalid := range []string{"", original} {
		if err := s.transitionTarget(original, "MERGED", invalid, target.Version, mutation{Actor: "human:test"}, "invalid survivor"); err == nil {
			t.Fatalf("accepted survivor %q", invalid)
		}
	}
	if err := s.transitionTarget(original, "MERGED", survivor, target.Version, mutation{Actor: "human:test"}, "same target episode"); err != nil {
		t.Fatal(err)
	}
	target, _ = s.getTarget(survivor)
	if err := s.transitionTarget(survivor, "RESOLVED", "", target.Version, mutation{Actor: "human:test", CompletionEvidence: "saved RUN p95 1.5 ms at matching load"}, "goal met"); err != nil {
		t.Fatal(err)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	merged, _ := s.getTarget(original)
	if merged.Status != "MERGED" || merged.MergedIntoID != survivor {
		t.Fatalf("merge history changed: %#v", merged)
	}
	newEvidence := "new evidence"
	if err := s.updateTarget(original, TargetPatch{Evidence: &newEvidence}, merged.Version, mutation{Actor: "human:test"}, "rewrite terminal target"); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("terminal update error = %v", err)
	}
	if _, err := s.db.Exec(`UPDATE targets SET status='MERGED',merged_into_target_id=? WHERE id=?`, original, survivor); err != nil {
		t.Fatal(err)
	}
	if err := s.validate(); err == nil || !strings.Contains(err.Error(), "cycles") {
		t.Fatalf("merge cycle error=%v", err)
	}
}

func TestPrimarySwitchRejectsStaleOwnerSetVersions(t *testing.T) {
	s := testStore(t)
	first := newDesignTarget(t, s, "first-cas", "")
	second := newDesignTarget(t, s, "second-cas", "")
	third := newDesignTarget(t, s, "third-cas", "")
	cardID, err := s.addCard(NewCard{TargetID: first, Title: "candidate", Actor: "skill:test", Reason: "candidate"}, mutation{Actor: "skill:test"})
	if err != nil {
		t.Fatal(err)
	}
	secondTarget, _ := s.getTarget(second)
	thirdTarget, _ := s.getTarget(third)
	card, _ := s.getCard(cardID)
	if err := s.setTargetLink(second, cardID, true, "", "", secondTarget.Version, mutation{Actor: "writer:one", Primary: true, ExpectedCardVersion: intPtr(card.Version)}, "primary switch"); err != nil {
		t.Fatal(err)
	}
	updatedCard, err := s.getCard(cardID)
	if err != nil || updatedCard.PrimaryTargetID != second {
		t.Fatalf("primary target = %#v, %v", updatedCard, err)
	}
	if err := s.setTargetLink(third, cardID, true, "", "", thirdTarget.Version, mutation{Actor: "writer:two", Primary: true, ExpectedCardVersion: intPtr(card.Version)}, "stale competing switch"); err == nil || !strings.Contains(err.Error(), "card version conflict") {
		t.Fatalf("stale primary switch=%v", err)
	}
	secondTarget, _ = s.getTarget(second)
	objectiveOne, _ := s.getObjective("O-001")
	objectiveTwo, _ := s.getObjective("O-002")
	if err := s.setObjectiveRelation("O-002", second, "target", "secondary outcome", true, objectiveTwo.Version, mutation{Actor: "writer:one", Primary: true, ExpectedTargetVersion: intPtr(secondTarget.Version)}, "primary objective switch"); err != nil {
		t.Fatal(err)
	}
	if err := s.setObjectiveRelation("O-001", second, "target", "primary outcome", true, objectiveOne.Version, mutation{Actor: "writer:two", Primary: true, ExpectedTargetVersion: intPtr(secondTarget.Version)}, "stale competing objective switch"); err == nil || !strings.Contains(err.Error(), "target version conflict") {
		t.Fatalf("stale objective switch=%v", err)
	}
	target, _ := s.getTarget(second)
	if target.PrimaryObjectiveID != "O-002" {
		t.Fatal("failed CAS changed primary")
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
}
