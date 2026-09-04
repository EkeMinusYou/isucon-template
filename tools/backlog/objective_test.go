package main

import "testing"

func TestInitialObjectivesAndRelations(t *testing.T) {
	store := testStore(t)
	objectives, err := store.listObjectives(ObjectiveFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(objectives) != 3 {
		t.Fatalf("initial objectives = %d, want 3", len(objectives))
	}
	objective, err := store.getObjective("O-003")
	if err != nil {
		t.Fatal(err)
	}
	if objective.Mode != "MAXIMIZE" || objective.ParentObjectiveID != "" {
		t.Fatalf("score objective = %#v", objective)
	}
	cardID, err := store.addCard(NewCard{Title: "selection change", Actor: "skill:test", Reason: "add intervention"}, mutation{Actor: "skill:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	objective, _ = store.getObjective("O-003")
	if err := store.setObjectiveRelation("O-003", cardID, "intervention", "increase valid score", true, objective.Version, mutation{Actor: "skill:test", Operation: "objective.link"}, "link intervention to score objective"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.ObjectiveIDs) != 1 || card.ObjectiveIDs[0] != "O-003" {
		t.Fatalf("card objectives = %#v", card.ObjectiveIDs)
	}
}

func TestConstraintRequiresActiveObjectiveRelation(t *testing.T) {
	store := testStore(t)
	if _, err := store.addConstraint(NewConstraint{Title: "unscoped", Fingerprint: "constraint:v1:unscoped", Scope: "hot request response time", Evidence: "10 response-s / load-window", Resolution: "less than 5 response-s / load-window"}, mutation{Actor: "skill:test", Operation: "constraint.add"}, "reject unscoped constraint"); err == nil {
		t.Fatal("constraint without objective unexpectedly accepted")
	}
	constraintID, err := store.addConstraint(NewConstraint{ObjectiveID: "O-003", Title: "current wait", Fingerprint: "constraint:v1:test", Scope: "hot request response time", Evidence: "10 response-s / load-window", Resolution: "less than 5 response-s / load-window"}, mutation{Actor: "skill:test", Operation: "constraint.add"}, "record observed constraint")
	if err != nil {
		t.Fatal(err)
	}
	objective, err := store.getObjective("O-003")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setObjectiveRelation("O-003", constraintID, "constraint", "", false, objective.Version, mutation{Actor: "skill:test", Operation: "objective.unlink"}, "attempt to orphan active constraint"); err == nil {
		t.Fatal("orphaned ACTIVE constraint unexpectedly accepted")
	}
}

func TestFourDigitExplicitIDsAdvanceSequences(t *testing.T) {
	store := testStore(t)
	cardID, err := store.addCard(NewCard{ID: "B-1000", Title: "four digit intervention", Actor: "human:test", Reason: "verify four digit support"}, mutation{Actor: "human:test", Operation: "add"})
	if err != nil || cardID != "B-1000" {
		t.Fatalf("four digit intervention = %q, %v", cardID, err)
	}
	constraintID, err := store.addConstraint(NewConstraint{ID: "A-1000", ObjectiveID: "O-003", Title: "four digit constraint", Fingerprint: "constraint:v1:four-digit", Scope: "loop time", Evidence: "1 response-s / load-window", Resolution: "less than 1 response-s / load-window"}, mutation{Actor: "human:test", Operation: "constraint.add"}, "verify four digit support")
	if err != nil || constraintID != "A-1000" {
		t.Fatalf("four digit constraint = %q, %v", constraintID, err)
	}
	objectiveID, err := store.addObjective(NewObjective{ID: "O-1000", Status: "ACTIVE", Mode: "MAXIMIZE", Title: "four digit objective", MetricOrPredicate: "score", Verification: "benchmark score"}, mutation{Actor: "human:test", Operation: "objective.add"}, "verify four digit support")
	if err != nil || objectiveID != "O-1000" {
		t.Fatalf("four digit objective = %q, %v", objectiveID, err)
	}
	for key, want := range map[string]string{"next_id": "B-1001", "next_constraint_id": "A-1001", "next_objective_id": "O-1001"} {
		got, err := store.metadata(key)
		if err != nil || got != want {
			t.Fatalf("%s = %q, %v; want %q", key, got, err, want)
		}
	}
}
