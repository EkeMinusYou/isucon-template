package main

import "testing"

func TestInitialObjectivesAndRelations(t *testing.T) {
	store := testStore(t)
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
	targetID, err := store.addTarget(NewTarget{Axis: "response time", Goal: "reduce response time below 5 ms", Evaluation: "compare saved results at equal load", ID: "A-1000", ObjectiveID: "O-001", Title: "four digit target", Fingerprint: "target:v1:four-digit", Scope: "loop time", Evidence: "1 response-s / load-window", Resolution: "less than 1 response-s / load-window"}, mutation{Actor: "human:test", Operation: "target.add"}, "verify four digit support")
	if err != nil || targetID != "A-1000" {
		t.Fatalf("four digit target = %q, %v", targetID, err)
	}
	objectiveID, err := store.addObjective(NewObjective{ID: "O-1000", Status: "ACTIVE", Mode: "MAXIMIZE", Title: "four digit objective", MetricOrPredicate: "score", Verification: "benchmark score"}, mutation{Actor: "human:test", Operation: "objective.add"}, "verify four digit support")
	if err != nil || objectiveID != "O-1000" {
		t.Fatalf("four digit objective = %q, %v", objectiveID, err)
	}
	for key, want := range map[string]string{"next_target_id": "A-1001", "next_objective_id": "O-1001"} {
		got, err := store.metadata(key)
		if err != nil || got != want {
			t.Fatalf("%s = %q, %v; want %q", key, got, err, want)
		}
	}
}

func TestObjectivePriorityCanBeCreatedUpdatedAndFiltered(t *testing.T) {
	store := testStore(t)
	objectiveID, err := store.addObjective(NewObjective{
		ID:                "O-010",
		Status:            "ACTIVE",
		Mode:              "MAXIMIZE",
		Title:             "priority objective",
		Priority:          "P0",
		MetricOrPredicate: "score contribution",
		Verification:      "compare valid scores",
	}, mutation{Actor: "human:test", Operation: "objective.add"}, "record objective priority")
	if err != nil {
		t.Fatal(err)
	}
	objective, err := store.getObjective(objectiveID)
	if err != nil {
		t.Fatal(err)
	}
	if objective.Priority != "P0" {
		t.Fatalf("created objective priority = %q", objective.Priority)
	}

	p1 := "P1"
	if err := store.updateObjective(objectiveID, ObjectivePatch{Priority: &p1}, objective.Version, mutation{Actor: "human:test", Operation: "objective.update"}, "lower objective priority after reassessment"); err != nil {
		t.Fatal(err)
	}
	updated, err := store.getObjective(objectiveID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Priority != "P1" || updated.Version != objective.Version+1 {
		t.Fatalf("updated objective = %#v", updated)
	}

	objectives, err := store.listObjectives(ObjectiveFilter{Priority: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range objectives {
		if item.ID == objectiveID {
			found = true
		}
	}
	if !found {
		t.Fatalf("priority-filtered objectives omit %s: %#v", objectiveID, objectives)
	}
}
