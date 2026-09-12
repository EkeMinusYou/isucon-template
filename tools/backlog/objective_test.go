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
	objective, err := store.getObjective("O-001")
	if err != nil {
		t.Fatal(err)
	}
	if objective.Mode != "MINIMIZE" || objective.ParentObjectiveID != "" {
		t.Fatalf("latency direction objective = %#v", objective)
	}
	cardID, err := store.addCard(NewCard{TargetID: fixtureTarget(t, store), Title: "selection change", Actor: "skill:test", Reason: "add intervention"}, mutation{Actor: "skill:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}

	card, err := store.getCard(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.ActiveTargetIDs) != 1 {
		t.Fatalf("card targets = %#v", card.ActiveTargetIDs)
	}
	target, err := store.getTarget(card.ActiveTargetIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if target.PrimaryObjectiveID != "O-001" {
		t.Fatalf("target objective = %q", target.PrimaryObjectiveID)
	}
}

func TestTargetRequiresActiveObjectiveRelation(t *testing.T) {
	store := testStore(t)
	if _, err := store.addTarget(NewTarget{Axis: "response time", Goal: "reduce response time below 5 ms", Evaluation: "compare saved results at equal load", Title: "unscoped", Fingerprint: "target:v1:unscoped", Scope: "hot request response time", Evidence: "10 response-s / load-window", Resolution: "less than 5 response-s / load-window"}, mutation{Actor: "skill:test", Operation: "target.add"}, "reject unscoped target"); err == nil {
		t.Fatal("target without objective unexpectedly accepted")
	}
	targetID, err := store.addTarget(NewTarget{Axis: "response time", Goal: "reduce response time below 5 ms", Evaluation: "compare saved results at equal load", ObjectiveID: "O-001", Title: "current wait", Fingerprint: "target:v1:test", Scope: "hot request response time", Evidence: "10 response-s / load-window", Resolution: "less than 5 response-s / load-window"}, mutation{Actor: "skill:test", Operation: "target.add"}, "record observed target")
	if err != nil {
		t.Fatal(err)
	}
	objective, err := store.getObjective("O-001")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setObjectiveRelation("O-001", targetID, "target", "", false, objective.Version, mutation{Actor: "skill:test", Operation: "objective.unlink"}, "attempt to orphan active target"); err == nil {
		t.Fatal("orphaned ACTIVE target unexpectedly accepted")
	}
}

func TestFourDigitExplicitIDsAdvanceSequences(t *testing.T) {
	store := testStore(t)
	cardID, err := store.addCard(NewCard{TargetID: fixtureTarget(t, store), ID: "B-1000", Title: "four digit intervention", Actor: "human:test", Reason: "verify four digit support"}, mutation{Actor: "human:test", Operation: "add"})
	if err != nil || cardID != "B-1000" {
		t.Fatalf("four digit intervention = %q, %v", cardID, err)
	}
	targetID, err := store.addTarget(NewTarget{Axis: "response time", Goal: "reduce response time below 5 ms", Evaluation: "compare saved results at equal load", ID: "A-1000", ObjectiveID: "O-001", Title: "four digit target", Fingerprint: "target:v1:four-digit", Scope: "loop time", Evidence: "1 response-s / load-window", Resolution: "less than 1 response-s / load-window"}, mutation{Actor: "human:test", Operation: "target.add"}, "verify four digit support")
	if err != nil || targetID != "A-1000" {
		t.Fatalf("four digit target = %q, %v", targetID, err)
	}
	objectiveID, err := store.addObjective(NewObjective{ID: "O-1000", Status: "ACTIVE", Mode: "MAXIMIZE", Title: "four digit objective", MetricOrPredicate: "score", Verification: "benchmark score"}, mutation{Actor: "human:test", Operation: "objective.add"}, "verify four digit support")
	if err != nil || objectiveID != "O-1000" {
		t.Fatalf("four digit objective = %q, %v", objectiveID, err)
	}
	for key, want := range map[string]string{"next_id": "B-1001", "next_target_id": "A-1001", "next_objective_id": "O-1001"} {
		got, err := store.metadata(key)
		if err != nil || got != want {
			t.Fatalf("%s = %q, %v; want %q", key, got, err, want)
		}
	}
}
