package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "backlog.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureObjectives(t, store)
	t.Cleanup(func() { store.Close() })
	return store
}

func fixtureTarget(t *testing.T, store *Store) string {
	t.Helper()
	id := "A-001"
	for _, query := range []string{
		`INSERT OR IGNORE INTO targets(id,status,title,fingerprint,scope,axis,goal,evaluation,evidence,resolution) VALUES ('A-001','ACTIVE','request latency','target:test:fixture','request','response time','under 5 ms','equal load saved results','test sequential calls','under 5 ms')`,
		`INSERT OR IGNORE INTO objective_targets(objective_id,target_id,is_primary) VALUES ('O-001','A-001',1)`,
		`UPDATE metadata SET value='A-002' WHERE key='next_target_id' AND value='A-001'`,
	} {
		if _, err := store.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func linkFixtureTarget(t *testing.T, store *Store, cardID string) {
	t.Helper()
	id := fixtureTarget(t, store)
	if _, err := store.db.Exec(`INSERT OR IGNORE INTO target_interventions(card_id,target_id,role,rationale,is_primary) VALUES (?,?,'IMPROVES','test contribution',1)`, cardID, id); err != nil {
		t.Fatal(err)
	}
}

func seedBacklog(t *testing.T, store *Store, revision int, nextID string, cards ...Card) {
	t.Helper()
	for index := range cards {
		card := &cards[index]
		if card.Status == "" {
			card.Status = "INVESTIGATE"
		}
		if requiresReadyContract(card.Status) {
			present := map[string]bool{}
			for _, section := range card.Sections {
				present[section.Name] = true
			}
			for _, name := range []string{sectionHypothesis, sectionChangeBoundary, sectionEvaluation} {
				if !present[name] {
					card.Sections = append(card.Sections, Section{Name: name, Position: len(card.Sections), Body: "test " + strings.ToLower(name)})
				}
			}
		}
		if card.Status == "APPLIED" && card.ApplicationID == "" {
			card.ApplicationID = fmt.Sprintf("%s@%d", card.ID, card.Version)
		}
		_, err := store.db.Exec(`INSERT INTO cards(`+cardColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			card.ID, card.Version, card.Status, card.Title, card.Priority, card.Owner, card.Area,
			card.Updated, card.UpdatedBy, card.ApplicationID, card.ImplementationEstimateMinutes)
		if err != nil {
			t.Fatal(err)
		}
		for relation, raw := range map[string]string{"SOURCE": card.SourceRuns, "COMPARE": card.CompareRun, "OBSERVED": card.ObservedRuns} {
			runIDs, err := parseRunIDsStrict(raw)
			if err != nil {
				t.Fatal(err)
			}
			for position, runID := range runIDs {
				if _, err := store.db.Exec(`INSERT INTO card_runs(card_id, run_id, relation, position) VALUES (?, ?, ?, ?)`, card.ID, runID, relation, position); err != nil {
					t.Fatal(err)
				}
			}
		}
		for _, section := range card.Sections {
			if _, err := store.db.Exec(`INSERT INTO card_sections(card_id, name, position, body) VALUES (?, ?, ?, ?)`, card.ID, section.Name, section.Position, section.Body); err != nil {
				t.Fatal(err)
			}
		}
		for _, entry := range card.History {
			if _, err := store.db.Exec(`INSERT INTO card_history(card_id, position, occurred_at, actor, body, raw) VALUES (?, ?, ?, ?, ?, ?)`, card.ID, entry.Position, entry.OccurredAt, entry.Actor, entry.Body, entry.Raw); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, card := range cards {
		linkFixtureTarget(t, store, card.ID)
	}
	for _, card := range cards {
		for _, dependency := range card.Dependencies {
			if _, err := store.db.Exec(`INSERT INTO card_dependencies(card_id, depends_on_card_id, required_status, mode, reason) VALUES (?, ?, ?, ?, ?)`,
				card.ID, dependency.DependsOnCardID, dependency.RequiredStatus, dependency.Mode, dependency.Reason); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := store.db.Exec(`UPDATE metadata SET value = ? WHERE key = 'backlog_revision'`, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE metadata SET value = ? WHERE key = 'next_id'`, nextID); err != nil {
		t.Fatal(err)
	}
}

func prepareReadyContract(t *testing.T, store *Store, cardID string) {
	t.Helper()
	for position, name := range []string{sectionHypothesis, sectionChangeBoundary, sectionEvaluation} {
		if _, err := store.db.Exec(`INSERT INTO card_sections(card_id, name, position, body) VALUES (?, ?, ?, ?)
			ON CONFLICT(card_id, name) DO UPDATE SET body=excluded.body`, cardID, name, position, "test "+strings.ToLower(name)); err != nil {
			t.Fatal(err)
		}
	}
	linkFixtureTarget(t, store, cardID)
}

func TestMutationUsesCardVersionAndTransitionClosesTerminalCard(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "APPLIED", Title: "test"})
	expected := 0
	if err := store.updateCard("B-001", CardPatch{Values: map[string]string{"owner": "agent:test"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: &expected}, "take ownership"); err != nil {
		t.Fatal(err)
	}
	if err := store.updateCard("B-001", CardPatch{Values: map[string]string{"owner": "stale"}}, mutation{Actor: "agent:stale", Operation: "update", ExpectedCardVersion: &expected}, "stale update"); err == nil || !strings.Contains(err.Error(), "card version conflict") {
		t.Fatalf("stale update error = %v", err)
	}
	if err := store.transitionCard("B-001", "VALIDATED", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(1)}, "benchmark passed"); err == nil {
		t.Fatal("transition bypassed adoption checks")
	}
	if err := store.transitionCard("B-001", "DOING", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(1)}, "correction required"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "DOING" || card.Version != 2 || card.Closed || card.Owner != "agent:test" || len(card.History) != 2 {
		t.Fatalf("transition result = %#v", card)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCardRunRelationsNormalizeAndRejectInvalidInput(t *testing.T) {
	store := testStore(t)
	id, err := store.addCard(NewCard{TargetID: fixtureTarget(t, store),
		Title: "normalized runs", Actor: "human:test", Reason: "record run evidence",
		CardPatch: CardPatch{Values: map[string]string{
			"source-runs":   "runs/20260901-120000; 20260901-120100,20260901-120000",
			"compare-run":   "20260901-115900",
			"observed-runs": "20260901-120100",
		}},
	}, mutation{Actor: "human:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard(id)
	if err != nil {
		t.Fatal(err)
	}
	if card.SourceRuns != "20260901-120000,20260901-120100" || card.CompareRun != "20260901-115900" || card.ObservedRuns != "20260901-120100" {
		t.Fatalf("normalized RUN relations = %#v", card)
	}
	if err := store.updateCard(id, CardPatch{Values: map[string]string{"source-runs": "20260901-120200,bad-run"}}, mutation{Actor: "human:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "replace source runs"); err == nil || !strings.Contains(err.Error(), "invalid RUN ID") {
		t.Fatalf("invalid mixed RUN input error = %v", err)
	}
	if err := store.updateCard(id, CardPatch{Values: map[string]string{"compare-run": "20260901-120200,20260901-120300"}}, mutation{Actor: "human:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "replace comparison runs"); err != nil {
		t.Fatal(err)
	}
	card, err = store.getCard(id)
	if err != nil {
		t.Fatal(err)
	}
	if card.CompareRun != "20260901-120200,20260901-120300" {
		t.Fatalf("multiple comparison RUNs = %q", card.CompareRun)
	}
}

func TestExplicitCardIDAdvancesNextIDAndInitRepairsStalePointer(t *testing.T) {
	store := testStore(t)
	if _, err := store.addCard(NewCard{TargetID: fixtureTarget(t, store), ID: "B-1000", Title: "explicit", Actor: "human:test", Reason: "import explicit card"}, mutation{Actor: "human:test", Operation: "add"}); err != nil {
		t.Fatal(err)
	}
	next, err := store.metadata("next_id")
	if err != nil {
		t.Fatal(err)
	}
	if next != "B-1001" {
		t.Fatalf("next_id after explicit add = %s, want B-1001", next)
	}
	if _, err := store.db.Exec(`UPDATE metadata SET value = 'B-004' WHERE key = 'next_id'`); err != nil {
		t.Fatal(err)
	}
	if err := store.validate(); err == nil || !strings.Contains(err.Error(), "must be greater") {
		t.Fatalf("stale next_id validation error = %v", err)
	}
	if err := store.reconcileNextCardID(); err != nil {
		t.Fatal(err)
	}
	next, err = store.metadata("next_id")
	if err != nil {
		t.Fatal(err)
	}
	if next != "B-1001" {
		t.Fatalf("reconciled next_id = %s, want B-1001", next)
	}
}

func TestValidateRejectsInvalidNormalizedRunRows(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "invalid run row"})
	if _, err := store.db.Exec(`INSERT INTO card_runs(card_id, run_id, relation, position) VALUES ('B-001', 'not-a-run', 'SOURCE', 0)`); err != nil {
		t.Fatal(err)
	}
	if err := store.validate(); err == nil || !strings.Contains(err.Error(), "invalid SOURCE RUN ID") {
		t.Fatalf("invalid normalized RUN validation error = %v", err)
	}
}

func TestReadyOwnerInvariant(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Owner: "skill:isucon-investigate", Title: "test"})
	prepareReadyContract(t, store, "B-001")

	if err := store.transitionCard("B-001", "READY", mutation{Actor: "skill:isucon-investigate", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "promote card to the ready queue"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "READY" || card.Owner != "" {
		t.Fatalf("ready card retained owner: %#v", card)
	}

	if err := store.updateCard("B-001", CardPatch{Values: map[string]string{"owner": "agent:stale"}}, mutation{Actor: "agent:stale", Operation: "update", ExpectedCardVersion: intPtr(1)}, "attempt to claim a ready card without changing status"); err == nil || !strings.Contains(err.Error(), "READY cards must have an empty owner") {
		t.Fatalf("ready owner update error = %v", err)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestReadyGateRequiresMinimalContract(t *testing.T) {
	tests := []struct {
		name    string
		remove  string
		wantErr string
	}{
		{"hypothesis", `DELETE FROM card_sections WHERE card_id='B-001' AND name='Hypothesis'`, "Hypothesis"},
		{"change boundary", `DELETE FROM card_sections WHERE card_id='B-001' AND name='Change boundary'`, "Change boundary"},
		{"verification", `DELETE FROM card_sections WHERE card_id='B-001' AND name='Evaluation'`, "Evaluation"},
		{"active objective", `UPDATE objectives SET status='RETIRED' WHERE id='O-001'`, "ACTIVE Target"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := testStore(t)
			seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "candidate"})
			prepareReadyContract(t, store, "B-001")
			if _, err := store.db.Exec(test.remove); err != nil {
				t.Fatal(err)
			}
			err := store.transitionCard("B-001", "READY", mutation{Actor: "skill:isucon-investigate", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "check READY contract")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("READY gate error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestReadyGateIgnoresOrderingDependencyButBlocksBlockingDependency(t *testing.T) {
	for _, mode := range []string{"ORDERING", "BLOCKING"} {
		t.Run(mode, func(t *testing.T) {
			store := testStore(t)
			seedBacklog(t, store, 0, "B-003",
				Card{ID: "B-001", Status: "INVESTIGATE", Title: "candidate", Dependencies: []CardDependency{{DependsOnCardID: "B-002", RequiredStatus: "VERIFY", Mode: mode, Reason: "implementation sequence"}}},
				Card{ID: "B-002", Status: "INVESTIGATE", Title: "prerequisite"})
			prepareReadyContract(t, store, "B-001")
			err := store.transitionCard("B-001", "READY", mutation{Actor: "skill:isucon-investigate", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "check dependency gate")
			if mode == "ORDERING" && err != nil {
				t.Fatalf("ORDERING dependency blocked READY: %v", err)
			}
			if mode == "BLOCKING" && (err == nil || !strings.Contains(err.Error(), "requires VERIFY")) {
				t.Fatalf("BLOCKING dependency error = %v", err)
			}
		})
	}
}

func TestDependencyRegressionCannotInvalidateReadyCard(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "READY", Title: "dependent", Dependencies: []CardDependency{{DependsOnCardID: "B-002", RequiredStatus: "APPLIED", Mode: "BLOCKING", Reason: "requires deployed prerequisite"}}},
		Card{ID: "B-002", Status: "APPLIED", Title: "prerequisite"})
	err := store.transitionCard("B-002", "DOING", mutation{Actor: "human:test", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "revise prerequisite")
	if err == nil || !strings.Contains(err.Error(), "would invalidate dependent READY contract") {
		t.Fatalf("dependency regression error = %v", err)
	}
	card, getErr := store.getCard("B-002")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if card.Status != "APPLIED" || card.Version != 0 {
		t.Fatalf("failed dependency regression was not rolled back: %#v", card)
	}
}

func TestReadyContractProtectsLastActiveObjectiveRelation(t *testing.T) {
	t.Run("retire", func(t *testing.T) {
		store := testStore(t)
		seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Title: "candidate"})
		retired := "RETIRED"
		err := store.updateObjective("O-001", ObjectivePatch{Status: &retired}, 0, mutation{Actor: "human:test", Operation: "objective.update"}, "retire score objective")
		if err == nil || !strings.Contains(err.Error(), "ACTIVE targets are not connected") {
			t.Fatalf("retire error = %v", err)
		}
	})
}

func TestValidateRejectsReadyWithoutContract(t *testing.T) {
	store := testStore(t)
	if _, err := store.db.Exec(`INSERT INTO cards(id, status, title) VALUES ('B-001', 'READY', 'malformed card')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE metadata SET value='B-002' WHERE key='next_id'`); err != nil {
		t.Fatal(err)
	}
	linkFixtureTarget(t, store, "B-001")
	if err := store.validate(); err == nil || !strings.Contains(err.Error(), "Hypothesis") {
		t.Fatalf("validate malformed READY error = %v", err)
	}
}

func TestResolveWithThreeSectionsUpdatesAndTransitionsInOneMutation(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 7, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Owner: "skill:isucon-investigate", Title: "candidate"})

	woke, err := store.resolveCardAndWake("B-001", "READY", CardPatch{
		Values: map[string]string{"status": "READY", "title": "bounded candidate"},
		Sections: map[string]string{
			sectionHypothesis:     "remove repeated query work to increase throughput",
			sectionChangeBoundary: "replace the query and roll it back as one unit",
			sectionEvaluation:     `{"version":1,"checks":["inspect saved correctness results"],"note":"confirm score magnitude after implementation"}`,
			sectionUnknowns:       "- Decision-blocking: none\n- Post-implementation: confirm score magnitude",
		},
	}, mutation{Actor: "skill:isucon-investigate", Operation: "resolve", ExpectedCardVersion: intPtr(0)}, "fast READY gate passed")
	if err != nil {
		t.Fatal(err)
	}
	if len(woke) != 0 {
		t.Fatalf("woke = %v, want none", woke)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "READY" || card.Owner != "" || card.Title != "bounded candidate" || card.Version != 1 || len(card.History) != 1 {
		t.Fatalf("resolved card = %#v", card)
	}
	if err := store.validate(); err != nil {
		t.Fatalf("three-section READY card failed validation: %v", err)
	}
	revision, err := store.revision()
	if err != nil {
		t.Fatal(err)
	}
	if revision != 8 {
		t.Fatalf("revision = %d, want 8", revision)
	}
}

func TestResolveRejectsRemovedSafetySection(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Owner: "skill:isucon-investigate", Title: "candidate"})
	prepareReadyContract(t, store, "B-001")
	_, err := store.resolveCardAndWake("B-001", "READY", CardPatch{
		Sections: map[string]string{"Safety": "require disaster recovery"},
	}, mutation{Actor: "skill:isucon-investigate", Operation: "resolve", ExpectedCardVersion: intPtr(0)}, "reject removed section")
	if err == nil || !strings.Contains(err.Error(), `unsupported section "Safety"`) {
		t.Fatalf("removed section error = %v", err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "INVESTIGATE" || card.Version != 0 || len(card.History) != 0 {
		t.Fatalf("rejected section changed the card: %#v", card)
	}
}

func TestResolveFailureRollsBackContentAndTransition(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 3, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Owner: "skill:isucon-investigate", Title: "original"})

	_, err := store.resolveCardAndWake("B-001", "READY", CardPatch{
		Values:   map[string]string{"status": "READY", "title": "must not persist", "removed-field": "invalid"},
		Sections: map[string]string{sectionEvaluation: "free-form verification"},
	}, mutation{Actor: "skill:isucon-investigate", Operation: "resolve", ExpectedCardVersion: intPtr(0)}, "invalid READY resolution")
	if err == nil || !strings.Contains(err.Error(), "unsupported card field") {
		t.Fatalf("resolve error = %v", err)
	}
	card, getErr := store.getCard("B-001")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if card.Status != "INVESTIGATE" || card.Title != "original" || card.Version != 0 || len(card.History) != 0 {
		t.Fatalf("failed resolve changed card = %#v", card)
	}
	revision, revisionErr := store.revision()
	if revisionErr != nil {
		t.Fatal(revisionErr)
	}
	if revision != 3 {
		t.Fatalf("revision = %d, want 3", revision)
	}
}

func TestStatusTransitionGraphAndAtomicClaim(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-004",
		Card{ID: "B-001", Status: "READY", Title: "ready"},
		Card{ID: "B-002", Status: "INVESTIGATE", Title: "investigate"},
		Card{ID: "B-003", Status: "VALIDATED", Owner: "agent:history", Title: "closed"})

	if err := store.transitionCard("B-001", "DOING", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "non-atomic claim"); err == nil || !strings.Contains(err.Error(), "atomic claim") {
		t.Fatalf("READY -> DOING transition error = %v", err)
	}
	if err := store.updateCard("B-001", CardPatch{Values: map[string]string{"status": "DOING", "owner": "agent:test"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "atomic claim"); err != nil {
		t.Fatal(err)
	}
	if err := store.transitionCard("B-001", "INVESTIGATE", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(1)}, "return for redefinition"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "INVESTIGATE" || card.Owner != "" {
		t.Fatalf("returned investigation card retained implementation owner: %#v", card)
	}
	if err := store.updateCard("B-002", CardPatch{Values: map[string]string{"status": "BLOCKED"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "wrong command"); err == nil || !strings.Contains(err.Error(), "use transition") {
		t.Fatalf("status-changing update error = %v", err)
	}
	if err := store.transitionCard("B-002", "APPLIED", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "skip workflow states"); err == nil || !strings.Contains(err.Error(), "invalid status transition") {
		t.Fatalf("INVESTIGATE -> APPLIED error = %v", err)
	}
	if err := store.transitionCard("B-003", "INVESTIGATE", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "reopen closed history"); err == nil || !strings.Contains(err.Error(), "invalid status transition") {
		t.Fatalf("VALIDATED -> INVESTIGATE error = %v", err)
	}
}

func TestAddCardCannotStartReady(t *testing.T) {
	store := testStore(t)
	_, err := store.addCard(NewCard{TargetID: fixtureTarget(t, store),
		Title:  "ready card",
		Actor:  "agent:test",
		Reason: "test ready owner invariant",
		CardPatch: CardPatch{Values: map[string]string{
			"status": "READY",
			"owner":  "agent:test",
		}},
	}, mutation{Actor: "agent:test", Operation: "add"})
	if err == nil || !strings.Contains(err.Error(), "must start as INVESTIGATE") {
		t.Fatalf("add READY error = %v", err)
	}
}

func TestListCardsFiltersByOwner(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-004",
		Card{ID: "B-001", Status: "INVESTIGATE", Title: "unclaimed"},
		Card{ID: "B-002", Status: "INVESTIGATE", Owner: "agent:isucon-investigate", Title: "ours"},
		Card{ID: "B-003", Status: "INVESTIGATE", Owner: "agent:other", Title: "theirs"})

	unowned, err := store.listCards(ListFilter{Status: "INVESTIGATE", Unowned: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(unowned) != 1 || unowned[0].ID != "B-001" {
		t.Fatalf("unowned list = %#v", unowned)
	}

	owned, err := store.listCards(ListFilter{Status: "INVESTIGATE", Owner: "agent:isucon-investigate"})
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || owned[0].ID != "B-002" {
		t.Fatalf("owner list = %#v", owned)
	}
}

func validTargetAssessmentJSON() string {
	return `{"version":1,"axis":{"unit":"response-s","denominator":"load-window"},"current":{"value":10,"snapshot":"RUN test"},"reduction":{"value":9,"basis":"replay"},"added_cost":{"value":0,"basis":"none"},"threshold":{"value":2,"basis":"next contributor"}}`
}

func TestPerformanceAssessmentStoresInputsAndDerivesResult(t *testing.T) {
	assessment, canonical, err := parsePerformanceResidualAssessment(validTargetAssessmentJSON())
	if err != nil {
		t.Fatal(err)
	}
	if residual := assessment.predictedResidual(); residual != 1 {
		t.Fatalf("predicted residual = %v, want 1", residual)
	}
	if !assessment.resolves() {
		t.Fatal("assessment should satisfy its threshold")
	}
	for _, derived := range []string{"classification", "predicted_residual", "resolution_satisfies", "component_demands", "rank"} {
		if strings.Contains(canonical, derived) {
			t.Fatalf("canonical assessment stores derived field %q: %s", derived, canonical)
		}
	}
}

func TestTargetLifecycleAndCardLink(t *testing.T) {
	store := testStore(t)
	cardID, err := store.addCard(NewCard{TargetID: fixtureTarget(t, store), Title: "candidate", Actor: "skill:isucon-analyze", Reason: "add candidate"}, mutation{Actor: "skill:isucon-analyze", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	targetID, err := store.addTarget(NewTarget{Axis: "response time", Goal: "reduce response time below 5 ms", Evaluation: "compare saved results at equal load", ObjectiveID: "O-001", Title: "CPU queue", Priority: "P0", Fingerprint: "target:v1:cpu-queue", Scope: "app host CPU queue", Evidence: "90 core-s / 120 core-s", Resolution: "normalized queue wait no longer limits the load window"}, mutation{Actor: "skill:isucon-analyze", Operation: "target.add"}, "identified current limiting axis")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setTargetLink(targetID, cardID, true, "IMPROVES", "", 0, mutation{Actor: "skill:isucon-analyze", Operation: "target.link"}, "candidate addresses target"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if card.TargetRoles[targetID] != "IMPROVES" {
		t.Fatalf("target roles = %#v", card.TargetRoles)
	}
	if len(card.ActiveTargetIDs) != 2 {
		t.Fatalf("active target IDs = %#v", card.ActiveTargetIDs)
	}
	listed, err := store.listCards(ListFilter{All: true, TargetID: targetID})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != cardID {
		t.Fatalf("targeted list = %#v", listed)
	}
	targets, err := store.listTargets(TargetFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %#v", targets)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitDependencyWakesBlockedCard(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "INVESTIGATE", Title: "waiting for evidence"},
		Card{ID: "B-002", Status: "APPLIED", Owner: "verifier:test", Title: "prerequisite intervention"})

	if err := store.transitionCard("B-001", "BLOCKED", mutation{Actor: "human:test", ExpectedCardVersion: intPtr(0)}, "reconsider when prerequisite is validated"); err != nil {
		t.Fatal(err)
	}
	blocked, err := store.getCard("B-001")
	if err != nil || len(blocked.History) != 1 || !strings.Contains(blocked.History[0].Body, "reconsider when prerequisite is validated") {
		t.Fatalf("BLOCKED audit = %#v, %v", blocked, err)
	}
	woke, err := store.addDependency(DependencyInput{CardID: "B-001", DependsOnCardID: "B-002", RequiredStatus: "VALIDATED", Mode: "BLOCKING", Reason: "validated prerequisite is required"},
		mutation{Actor: "skill:isucon-investigate", Operation: "dependency.add", ExpectedCardVersion: intPtr(1)})
	if err != nil {
		t.Fatal(err)
	}
	if woke {
		t.Fatal("APPLIED prerequisite unexpectedly woke dependent")
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Dependencies) != 1 || card.Dependencies[0].RequiredStatus != "VALIDATED" || card.Dependencies[0].Mode != "BLOCKING" {
		t.Fatalf("dependency = %#v", card.Dependencies)
	}
	listed, err := store.listCards(ListFilter{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || len(listed[0].Dependencies) != 1 || len(listed[1].Unblocks) != 1 {
		t.Fatalf("listed dependency views = %#v", listed)
	}
	adoptFixtureCard(t, store, "B-002", "prerequisite validated")
	card, err = store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "INVESTIGATE" || card.Version != 3 {
		t.Fatalf("dependent after wake = %#v", card)
	}
}

func TestBlockingDependenciesWakeOnlyAfterAllAreSatisfied(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-004",
		Card{ID: "B-001", Status: "BLOCKED", Title: "waiting"},
		Card{ID: "B-002", Status: "DOING", Title: "local prerequisite"},
		Card{ID: "B-003", Status: "APPLIED", Title: "production prerequisite"})
	if _, err := store.addDependency(DependencyInput{CardID: "B-001", DependsOnCardID: "B-002", RequiredStatus: "VERIFY", Mode: "BLOCKING", Reason: "needs local implementation"}, mutation{Actor: "human:test", Operation: "dependency.add", ExpectedCardVersion: intPtr(0)}); err != nil {
		t.Fatal(err)
	}
	woke, err := store.addDependency(DependencyInput{CardID: "B-001", DependsOnCardID: "B-003", RequiredStatus: "APPLIED", Mode: "BLOCKING", Reason: "needs production state"}, mutation{Actor: "human:test", Operation: "dependency.add", ExpectedCardVersion: intPtr(1)})
	if err != nil {
		t.Fatal(err)
	}
	if woke {
		t.Fatal("dependent woke before all dependencies were satisfied")
	}
	woken, err := store.transitionCardAndWake("B-002", "VERIFY", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "implementation complete")
	if err != nil {
		t.Fatal(err)
	}
	if len(woken) != 1 || woken[0] != "B-001" {
		t.Fatalf("woken = %#v", woken)
	}
}

func TestRejectedBlockingDependencyWakesForRedesign(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "BLOCKED", Title: "waiting", Dependencies: []CardDependency{{DependsOnCardID: "B-002", RequiredStatus: "APPLIED", Mode: "BLOCKING", Reason: "requires production state"}}},
		Card{ID: "B-002", Status: "INVESTIGATE", Title: "prerequisite"})
	woken, err := store.transitionCardAndWake("B-002", "REJECTED", mutation{Actor: "skill:isucon-investigate", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "prerequisite has no valid path")
	if err != nil {
		t.Fatal(err)
	}
	if len(woken) != 1 || woken[0] != "B-001" {
		t.Fatalf("woken = %#v", woken)
	}
}

func TestRemovingBlockingDependencyWakesForReevaluation(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "BLOCKED", Title: "waiting", Dependencies: []CardDependency{{DependsOnCardID: "B-002", RequiredStatus: "APPLIED", Mode: "BLOCKING", Reason: "requires production state"}}},
		Card{ID: "B-002", Status: "DOING", Title: "prerequisite"})
	woke, err := store.removeDependency("B-001", "B-002", mutation{Actor: "human:test", Operation: "dependency.remove", ExpectedCardVersion: intPtr(0)}, "the prerequisite is no longer required")
	if err != nil {
		t.Fatal(err)
	}
	if !woke {
		t.Fatal("removing a blocking dependency did not wake the card")
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "INVESTIGATE" || len(card.Dependencies) != 0 {
		t.Fatalf("dependent after removal = %#v", card)
	}
}

func TestReadyClaimRequiresStructuredDependencies(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "READY", Title: "dependent", Dependencies: []CardDependency{{DependsOnCardID: "B-002", RequiredStatus: "VERIFY", Mode: "ORDERING", Reason: "implementation order"}}},
		Card{ID: "B-002", Status: "DOING", Title: "prerequisite"})
	err := store.updateCard("B-001", CardPatch{Values: map[string]string{"status": "DOING", "owner": "agent:test"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "claim dependent")
	if err == nil || !strings.Contains(err.Error(), "requires VERIFY") {
		t.Fatalf("claim dependency error = %v", err)
	}
	if err := store.transitionCard("B-002", "VERIFY", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "prerequisite complete"); err != nil {
		t.Fatal(err)
	}
	if err := store.updateCard("B-001", CardPatch{Values: map[string]string{"status": "DOING", "owner": "agent:test"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "claim dependent after prerequisite"); err != nil {
		t.Fatal(err)
	}
}

func TestDependencyCycleIsRejected(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "INVESTIGATE", Title: "one"},
		Card{ID: "B-002", Status: "INVESTIGATE", Title: "two"})
	if _, err := store.addDependency(DependencyInput{CardID: "B-001", DependsOnCardID: "B-002", RequiredStatus: "VERIFY", Mode: "ORDERING", Reason: "first edge"}, mutation{Actor: "human:test", Operation: "dependency.add", ExpectedCardVersion: intPtr(0)}); err != nil {
		t.Fatal(err)
	}
	_, err := store.addDependency(DependencyInput{CardID: "B-002", DependsOnCardID: "B-001", RequiredStatus: "VERIFY", Mode: "ORDERING", Reason: "cycle"}, mutation{Actor: "human:test", Operation: "dependency.add", ExpectedCardVersion: intPtr(0)})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestReadyStatusIsGatedForSkillActors(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "candidate"})
	prepareReadyContract(t, store, "B-001")

	for _, actor := range []string{"skill:isucon-analyze-alp", "skill:isucon-dashboard-triage", "agent:isucon-strategy"} {
		err := store.transitionCard("B-001", "READY", mutation{Actor: actor, Operation: "transition", ExpectedCardVersion: intPtr(0)}, "attempt direct promotion")
		if err == nil || !strings.Contains(err.Error(), "only as skill:isucon-investigate") {
			t.Fatalf("transition by %s error = %v", actor, err)
		}
	}

	err := store.updateCard("B-001", CardPatch{Values: map[string]string{"status": "READY"}}, mutation{Actor: "skill:isucon-role-balance", Operation: "update", ExpectedCardVersion: intPtr(0)}, "attempt atomic promotion")
	if err == nil || !strings.Contains(err.Error(), "use transition") {
		t.Fatalf("update READY by another skill error = %v", err)
	}

	if err := store.transitionCard("B-001", "READY", mutation{Actor: "skill:isucon-investigate", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "READY gate passed"); err != nil {
		t.Fatal(err)
	}

}

func TestAddCardKeepsDecisionTextInSections(t *testing.T) {
	store := testStore(t)
	id, err := store.addCard(NewCard{TargetID: fixtureTarget(t, store),
		Title:  "score hypothesis",
		Actor:  "agent:test",
		Reason: "record expected score direction",
		CardPatch: CardPatch{Sections: map[string]string{
			sectionHypothesis: "reduce DB wait on the score path",
			sectionResult:     "- Status: pending",
		}},
	}, mutation{Actor: "agent:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard(id)
	if err != nil {
		t.Fatal(err)
	}
	if hypothesis := sectionBody(card.Sections, sectionHypothesis); hypothesis != "reduce DB wait on the score path" {
		t.Fatalf("Hypothesis section = %q", hypothesis)
	}
	result := sectionBody(card.Sections, sectionResult)
	if result != "- Status: pending" {
		t.Fatalf("Result section = %q", result)
	}
}

func TestInvestigationMayOmitChangeBoundaryUntilReady(t *testing.T) {
	store := testStore(t)
	id, err := store.addCard(NewCard{TargetID: fixtureTarget(t, store),
		Title:  "boundary to investigate",
		Actor:  "agent:test",
		Reason: "record a hypothesis before the implementation scope is known",
		CardPatch: CardPatch{Sections: map[string]string{
			sectionHypothesis: "reduce work on the score path",
			sectionEvaluation: "compare correctness and saved performance evidence",
		}},
	}, mutation{Actor: "agent:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard(id)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "INVESTIGATE" || sectionBody(card.Sections, sectionChangeBoundary) != "" {
		t.Fatalf("investigation card = %#v", card)
	}
	if err := store.validate(); err != nil {
		t.Fatalf("investigation without Change boundary failed validation: %v", err)
	}
	err = store.transitionCard(id, "READY", mutation{Actor: "skill:isucon-investigate", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "require boundary before READY")
	if err == nil || !strings.Contains(err.Error(), "Change boundary") {
		t.Fatalf("READY without Change boundary error = %v", err)
	}
}

func TestValidateRejectsReadyCardWithOwner(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Owner: "agent:stale", Title: "invalid ready card"})
	if err := store.validate(); err == nil || !strings.Contains(err.Error(), "is READY but has non-empty owner") {
		t.Fatalf("validate ready owner error = %v", err)
	}
}

func TestValidateRejectsMalformedOwner(t *testing.T) {
	for _, owner := range []string{"none", " agent:test "} {
		t.Run(owner, func(t *testing.T) {
			store := testStore(t)
			seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Owner: owner, Title: "invalid owner"})
			if err := store.validate(); err == nil || !strings.Contains(err.Error(), "invalid owner") {
				t.Fatalf("validate malformed owner %q error = %v", owner, err)
			}
		})
	}
}

func TestEvaluationAcceptsFreeTextAndStructuredJSON(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "candidate"})

	err := store.updateCard("B-001", CardPatch{Sections: map[string]string{
		sectionEvaluation: "inspect saved correctness results",
	}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "write free-text verification")
	if err != nil {
		t.Fatalf("free-text Evaluation update error = %v", err)
	}

	if err := store.updateCard("B-001", CardPatch{Sections: map[string]string{
		sectionEvaluation: `{"version":1,"checks":["inspect saved correctness results"],"note":"correctness-only"}`,
	}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(1)}, "write structured verification"); err != nil {
		t.Fatal(err)
	}
}

func TestAddCardRejectsInvalidStructuredEvaluation(t *testing.T) {
	store := testStore(t)
	_, err := store.addCard(NewCard{
		Title:  "invalid Evaluation",
		Actor:  "agent:test",
		Reason: "reject unknown Evaluation field",
		CardPatch: CardPatch{Sections: map[string]string{
			sectionEvaluation: `{"version":1,"baseline_run":"20260101-000001"}`,
		}},
	}, mutation{Actor: "agent:test", Operation: "add"})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("invalid Evaluation add error = %v", err)
	}
	if _, err := store.getCard("B-001"); err == nil {
		t.Fatal("invalid Evaluation created a card")
	}
}

func TestUpdateRejectsInvalidStructuredEvaluation(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "candidate"})
	err := store.updateCard("B-001", CardPatch{Sections: map[string]string{
		sectionEvaluation: `{"version":1,"baseline_run":"20260101-000001"}`,
	}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "reject unknown Evaluation field")
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("invalid Evaluation update error = %v", err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Version != 0 || sectionBody(card.Sections, sectionEvaluation) != "" {
		t.Fatalf("invalid Evaluation changed card = %#v", card)
	}
}

func TestCardVersionIgnoresUnrelatedCardUpdates(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "READY", Title: "target"},
		Card{ID: "B-002", Status: "DOING", Title: "other"})
	if err := store.updateCard("B-002", CardPatch{Values: map[string]string{"owner": "agent:other"}}, mutation{Actor: "agent:other", Operation: "update", ExpectedCardVersion: intPtr(0)}, "take other card"); err != nil {
		t.Fatal(err)
	}
	if err := store.updateCard("B-001", CardPatch{Values: map[string]string{"status": "DOING", "owner": "agent:test"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "take target card"); err != nil {
		t.Fatalf("unrelated card update caused conflict: %v", err)
	}
}

func TestHistoryAppendDoesNotChangeCardVersion(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Title: "target"})
	if err := store.addHistory("B-001", "agent:test", "added a note", "", mutation{Actor: "agent:test", Operation: "history.add"}); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Version != 0 || len(card.History) != 1 {
		t.Fatalf("history append changed card version or history: version=%d history=%d", card.Version, len(card.History))
	}
}

func TestNewWritesRejectUnsupportedSections(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "test"})

	for _, name := range []string{"Invalid section"} {
		err := store.updateCard("B-001", CardPatch{Sections: map[string]string{name: "body"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "reject unsupported section")
		if err == nil || !strings.Contains(err.Error(), "unsupported section") {
			t.Fatalf("section %q error = %v", name, err)
		}
	}
}

func TestParseCardIDList(t *testing.T) {
	ids, err := parseCardIDList("B-002, b1, 1, B1, b-12, B-003, B-002")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ids, ","), "B-002,B-001,B-012,B-003"; got != want {
		t.Fatalf("parsed IDs = %s, want %s", got, want)
	}
	if _, err := parseCardIDList("B-001,,B-002"); err == nil {
		t.Fatal("empty ID was accepted")
	}
	if _, err := idNumber("B-nope"); err == nil {
		t.Error("idNumber accepted an invalid ID")
	}
}

func TestEmptySectionRemovesSection(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Title: "test"})

	if err := store.updateCard("B-001", CardPatch{Sections: map[string]string{sectionObservation: "- measured: 1ms"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "add observation"); err != nil {
		t.Fatal(err)
	}
	if err := store.updateCard("B-001", CardPatch{Sections: map[string]string{sectionObservation: ""}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(1)}, "remove obsolete section"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if got := sectionBody(card.Sections, sectionObservation); got != "" {
		t.Fatalf("empty section body = %q", got)
	}
}

func intPtr(value int) *int { return &value }

func fixtureObjectives(t *testing.T, store *Store) {
	t.Helper()
	for _, id := range []string{"O-001", "O-002", "O-003"} {
		_, err := store.db.Exec(`INSERT OR IGNORE INTO objectives(id,status,mode,title,metric_or_predicate,verification) VALUES (?,'ACTIVE','MINIMIZE','Test contribution hypothesis','Test metric','Compare fixture results')`, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`UPDATE metadata SET value='O-004' WHERE key='next_objective_id' AND value='O-001'`); err != nil {
		t.Fatal(err)
	}
}
