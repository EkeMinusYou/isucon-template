package main

import (
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
	t.Cleanup(func() { store.Close() })
	return store
}

func seedBacklog(t *testing.T, store *Store, revision int, nextID string, cards ...Card) {
	t.Helper()
	for index := range cards {
		card := &cards[index]
		if card.Status == "" {
			card.Status = "INVESTIGATE"
		}
		if requiresReadyContract(card.Status) {
			if strings.TrimSpace(card.Fingerprint) == "" {
				card.Fingerprint = "test:" + card.ID
			}
			present := map[string]bool{}
			for _, section := range card.Sections {
				present[section.Name] = true
			}
			for _, name := range []string{sectionHypothesis, sectionChangeBoundary, sectionVerification, sectionSafety} {
				if !present[name] {
					card.Sections = append(card.Sections, Section{Name: name, Position: len(card.Sections), Body: "test " + strings.ToLower(name)})
				}
			}
		}
		_, err := store.db.Exec(`INSERT INTO cards(`+cardColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			card.ID, card.Version, card.Status, card.Title, card.Priority, card.Owner, card.Area,
			card.Fingerprint, card.Updated, card.UpdatedBy)
		if err != nil {
			t.Fatal(err)
		}
		for relation, raw := range map[string]string{"SOURCE": card.SourceRuns, "COMPARE": card.CompareRun, "OBSERVED": card.ObservedRuns} {
			for position, runID := range parseRunIDs(raw) {
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
		if requiresReadyContract(card.Status) {
			if _, err := store.db.Exec(`INSERT INTO objective_interventions(objective_id, card_id, rationale) VALUES ('O-003', ?, 'test objective relation')`, card.ID); err != nil {
				t.Fatal(err)
			}
		}
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
	if _, err := store.db.Exec(`UPDATE cards SET fingerprint = ? WHERE id = ?`, "test:"+cardID, cardID); err != nil {
		t.Fatal(err)
	}
	for position, name := range []string{sectionHypothesis, sectionChangeBoundary, sectionVerification, sectionSafety} {
		if _, err := store.db.Exec(`INSERT INTO card_sections(card_id, name, position, body) VALUES (?, ?, ?, ?)
			ON CONFLICT(card_id, name) DO UPDATE SET body=excluded.body`, cardID, name, position, "test "+strings.ToLower(name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`INSERT INTO objective_interventions(objective_id, card_id, rationale) VALUES ('O-003', ?, 'test objective relation')
		ON CONFLICT(objective_id, card_id) DO NOTHING`, cardID); err != nil {
		t.Fatal(err)
	}
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
	if err := store.transitionCard("B-001", "VALIDATED", mutation{Actor: "agent:test", Operation: "transition", ExpectedCardVersion: intPtr(1)}, "benchmark passed"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "VALIDATED" || card.Version != 2 || !card.Closed || card.Owner != "agent:test" || len(card.History) != 2 {
		t.Fatalf("transition result = %#v", card)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCardRunRelationsNormalizeAndRejectInvalidInput(t *testing.T) {
	store := testStore(t)
	id, err := store.addCard(NewCard{
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
	if _, err := store.addCard(NewCard{ID: "B-005", Title: "explicit", Actor: "human:test", Reason: "import explicit card"}, mutation{Actor: "human:test", Operation: "add"}); err != nil {
		t.Fatal(err)
	}
	next, err := store.metadata("next_id")
	if err != nil {
		t.Fatal(err)
	}
	if next != "B-006" {
		t.Fatalf("next_id after explicit add = %s, want B-006", next)
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
	if next != "B-006" {
		t.Fatalf("reconciled next_id = %s, want B-006", next)
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

func TestBlockedRecordsReasonInHistory(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "wait"})
	options := mutation{Actor: "human:test", Operation: "transition", ExpectedCardVersion: intPtr(0)}
	if err := store.transitionCard("B-001", "BLOCKED", options, "environment unavailable; reconsider when host access returns"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(card.History) != 1 || !strings.Contains(card.History[0].Body, "reconsider when host access returns") {
		t.Fatalf("BLOCKED audit = %#v", card)
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
		{"fingerprint", `UPDATE cards SET fingerprint='' WHERE id='B-001'`, "fingerprint"},
		{"hypothesis", `DELETE FROM card_sections WHERE card_id='B-001' AND name='Hypothesis'`, "Hypothesis"},
		{"change boundary", `DELETE FROM card_sections WHERE card_id='B-001' AND name='Change boundary'`, "Change boundary"},
		{"verification", `DELETE FROM card_sections WHERE card_id='B-001' AND name='Verification'`, "Verification"},
		{"safety", `DELETE FROM card_sections WHERE card_id='B-001' AND name='Safety'`, "Safety"},
		{"active objective", `DELETE FROM objective_interventions WHERE card_id='B-001'`, "ACTIVE Objective"},
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
	err := store.transitionCard("B-002", "DOING", mutation{Actor: "human:test", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "roll back prerequisite")
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
	t.Run("unlink", func(t *testing.T) {
		store := testStore(t)
		seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Title: "candidate"})
		err := store.setObjectiveRelation("O-003", "B-001", "intervention", "", false, 0, mutation{Actor: "human:test", Operation: "objective.unlink"}, "remove objective relation")
		if err == nil || !strings.Contains(err.Error(), "would invalidate READY contract") {
			t.Fatalf("unlink error = %v", err)
		}
	})

	t.Run("retire", func(t *testing.T) {
		store := testStore(t)
		seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Title: "candidate"})
		retired := "RETIRED"
		err := store.updateObjective("O-003", ObjectivePatch{Status: &retired}, 0, mutation{Actor: "human:test", Operation: "objective.update"}, "retire score objective")
		if err == nil || !strings.Contains(err.Error(), "would invalidate READY contract") {
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
	if err := store.validate(); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("validate malformed READY error = %v", err)
	}
}

func TestResolveUpdatesAndTransitionsInOneMutation(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 7, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Owner: "skill:isucon-investigate", Title: "candidate"})
	if _, err := store.db.Exec(`INSERT INTO objective_interventions(objective_id, card_id, rationale) VALUES ('O-003', 'B-001', 'increase throughput')`); err != nil {
		t.Fatal(err)
	}

	woke, err := store.resolveCardAndWake("B-001", "READY", CardPatch{
		Values: map[string]string{"status": "READY", "title": "bounded candidate", "fingerprint": "query:v1"},
		Sections: map[string]string{
			sectionHypothesis:     "remove repeated query work to increase throughput",
			sectionChangeBoundary: "replace the query and roll it back as one unit",
			sectionVerification:   `{"version":1,"checks":["run focused tests"],"note":"confirm score magnitude after implementation"}`,
			sectionSafety:         "stop on correctness errors and restore the original query",
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
	revision, err := store.revision()
	if err != nil {
		t.Fatal(err)
	}
	if revision != 8 {
		t.Fatalf("revision = %d, want 8", revision)
	}
}

func TestResolveFailureRollsBackContentAndTransition(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 3, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Owner: "skill:isucon-investigate", Title: "original"})

	_, err := store.resolveCardAndWake("B-001", "READY", CardPatch{
		Values:   map[string]string{"status": "READY", "title": "must not persist", "removed-field": "invalid"},
		Sections: map[string]string{sectionVerification: "free-form verification"},
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
	_, err := store.addCard(NewCard{
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

func TestAddCardRejectsNonQueueStatus(t *testing.T) {
	store := testStore(t)
	_, err := store.addCard(NewCard{
		Title:  "invalid initial state",
		Actor:  "agent:test",
		Reason: "attempt to skip the workflow",
		CardPatch: CardPatch{Values: map[string]string{
			"status": "DOING",
		}},
	}, mutation{Actor: "agent:test", Operation: "add"})
	if err == nil || !strings.Contains(err.Error(), "must start as INVESTIGATE") {
		t.Fatalf("add DOING card error = %v", err)
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

func validConstraintAssessmentJSON() string {
	return `{"version":1,"axis":{"unit":"response-s","denominator":"load-window"},"current":{"value":10,"snapshot":"RUN test"},"reduction":{"value":9,"basis":"replay"},"added_cost":{"value":0,"basis":"none"},"threshold":{"value":2,"basis":"next contributor"}}`
}

func TestPerformanceAssessmentStoresInputsAndDerivesResult(t *testing.T) {
	assessment, canonical, err := parsePerformanceResidualAssessment(validConstraintAssessmentJSON())
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

func TestConstraintLinkAcceptsOrdinaryPartialReductionAsMitigation(t *testing.T) {
	store := testStore(t)
	cardID, err := store.addCard(NewCard{Title: "partial reduction", Actor: "skill:test", Reason: "add partial candidate"}, mutation{Actor: "skill:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	constraintID, err := store.addConstraint(NewConstraint{ObjectiveID: "O-003", Title: "CPU queue", Fingerprint: "constraint:v1:partial", Scope: "app CPU", Evidence: "90 core-s / 120 core-s", Resolution: "queue is non-limiting"}, mutation{Actor: "skill:test", Operation: "constraint.add"}, "create constraint")
	if err != nil {
		t.Fatal(err)
	}
	ordinary := strings.Replace(validConstraintAssessmentJSON(), `"threshold":{"value":2`, `"threshold":{"value":0.5`, 1)
	err = store.setConstraintLink(constraintID, cardID, true, "", ordinary, 0, mutation{Actor: "skill:test", Operation: "constraint.link"}, "record partial reduction")
	if err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if card.ConstraintRoles[constraintID] != "MITIGATES" {
		t.Fatalf("constraint role = %q, want MITIGATES", card.ConstraintRoles[constraintID])
	}
}

func TestNonPerformanceConstraintResolvesWithoutPerformanceAssessment(t *testing.T) {
	store := testStore(t)
	cardID, err := store.addCard(NewCard{Title: "restore valid result", Actor: "skill:test", Reason: "add validity fix"}, mutation{Actor: "skill:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	constraintID, err := store.addConstraint(NewConstraint{ObjectiveID: "O-001", Title: "final consistency fails", Fingerprint: "constraint:v1:final-consistency", Scope: "final consistency check", Evidence: "final check failed in RUN test", Resolution: "final check succeeds"}, mutation{Actor: "skill:test", Operation: "constraint.add"}, "create validity constraint")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setConstraintLink(constraintID, cardID, true, "RESOLVES", "", 0, mutation{Actor: "skill:test", Operation: "constraint.link"}, "the fix makes final consistency succeed"); err != nil {
		t.Fatal(err)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFingerprintSuppressesExactDuplicates(t *testing.T) {
	store := testStore(t)
	_, err := store.addCard(NewCard{
		Title: "first candidate", Actor: "skill:test", Reason: "create candidate",
		CardPatch: CardPatch{Values: map[string]string{"fingerprint": "candidate:v1:lookup"}},
	}, mutation{Actor: "skill:test", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.addCard(NewCard{
		Title: "duplicate candidate", Actor: "skill:test", Reason: "test duplicate suppression",
		CardPatch: CardPatch{Values: map[string]string{"fingerprint": "candidate:v1:lookup"}},
	}, mutation{Actor: "skill:test", Operation: "add"})
	if err == nil || !strings.Contains(err.Error(), "duplicate fingerprint") {
		t.Fatalf("duplicate fingerprint error = %v", err)
	}
}

func TestConstraintLifecycleAndCardLink(t *testing.T) {
	store := testStore(t)
	cardID, err := store.addCard(NewCard{Title: "candidate", Actor: "skill:isucon-analyze", Reason: "add candidate"}, mutation{Actor: "skill:isucon-analyze", Operation: "add"})
	if err != nil {
		t.Fatal(err)
	}
	constraintID, err := store.addConstraint(NewConstraint{ObjectiveID: "O-003", Title: "CPU queue", Priority: "P0", Fingerprint: "constraint:v1:cpu-queue", Scope: "app host CPU queue", Evidence: "90 core-s / 120 core-s", Resolution: "normalized queue wait no longer limits the load window"}, mutation{Actor: "skill:isucon-analyze", Operation: "constraint.add"}, "identified current limiting axis")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setConstraintLink(constraintID, cardID, true, "", validConstraintAssessmentJSON(), 0, mutation{Actor: "skill:isucon-analyze", Operation: "constraint.link"}, "candidate addresses constraint"); err != nil {
		t.Fatal(err)
	}
	card, err := store.getCard(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.ActiveConstraintIDs) != 1 || card.ActiveConstraintIDs[0] != constraintID {
		t.Fatalf("active constraint IDs = %#v", card.ActiveConstraintIDs)
	}
	if err := store.updateCard(cardID, CardPatch{Sections: map[string]string{sectionChangeBoundary: "changed after assessment"}}, mutation{Actor: "skill:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "attempt stale boundary update"); err == nil || !strings.Contains(err.Error(), "card fingerprint or change boundary") {
		t.Fatalf("stale card binding error = %v", err)
	}
	changedEvidence := "new limiting-axis snapshot"
	if err := store.updateConstraint(constraintID, ConstraintPatch{Evidence: &changedEvidence}, 1, mutation{Actor: "skill:test", Operation: "constraint.update"}, "attempt stale constraint update"); err == nil || !strings.Contains(err.Error(), "constraint definition changed") {
		t.Fatalf("stale constraint binding error = %v", err)
	}
	ordinary := strings.Replace(validConstraintAssessmentJSON(), `"threshold":{"value":2`, `"threshold":{"value":0.5`, 1)
	if err := store.setConstraintInterventionAssessment(constraintID, cardID, ordinary, 1, mutation{Actor: "skill:test", Operation: "constraint.assess"}, "downgrade linked assessment"); err == nil || !strings.Contains(err.Error(), "must retain") {
		t.Fatalf("linked assessment downgrade error = %v", err)
	}
	listed, err := store.listCards(ListFilter{All: true, ConstraintID: constraintID})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != cardID {
		t.Fatalf("constrainted list = %#v", listed)
	}
	constraints, err := store.listConstraints(ConstraintFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(constraints) != 1 || constraints[0].Status != "ACTIVE" || len(constraints[0].CardIDs) != 1 {
		t.Fatalf("constraints = %#v", constraints)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMergedConstraintRequiresLiveSurvivor(t *testing.T) {
	store := testStore(t)
	survivorID, err := store.addConstraint(NewConstraint{ObjectiveID: "O-003", Title: "survivor", Fingerprint: "constraint:v1:survivor", Scope: "app CPU", Evidence: "80 core-s / 120 core-s", Resolution: "cpu is no longer limiting"}, mutation{Actor: "skill:isucon-investigate", Operation: "constraint.add"}, "create survivor")
	if err != nil {
		t.Fatal(err)
	}
	duplicateID, err := store.addConstraint(NewConstraint{ObjectiveID: "O-003", Title: "duplicate", Fingerprint: "constraint:v1:duplicate", Scope: "app CPU", Evidence: "80 core-s / 120 core-s", Resolution: "cpu is no longer limiting"}, mutation{Actor: "skill:isucon-investigate", Operation: "constraint.add"}, "create duplicate")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.transitionConstraint(duplicateID, "MERGED", "", 0, mutation{Actor: "skill:isucon-investigate", Operation: "constraint.transition"}, "missing survivor"); err == nil || !strings.Contains(err.Error(), "requires --merged-into") {
		t.Fatalf("missing survivor error = %v", err)
	}
	if err := store.transitionConstraint(duplicateID, "MERGED", duplicateID, 0, mutation{Actor: "skill:isucon-investigate", Operation: "constraint.transition"}, "self merge"); err == nil || !strings.Contains(err.Error(), "cannot merge into itself") {
		t.Fatalf("self merge error = %v", err)
	}
	if err := store.transitionConstraint(duplicateID, "MERGED", survivorID, 0, mutation{Actor: "skill:isucon-investigate", Operation: "constraint.transition"}, "same limiting identity"); err != nil {
		t.Fatal(err)
	}
	merged, err := store.getConstraint(duplicateID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != "MERGED" || merged.MergedIntoID != survivorID {
		t.Fatalf("merged constraint = %#v", merged)
	}
	newEvidence := "new evidence"
	if err := store.updateConstraint(duplicateID, ConstraintPatch{Evidence: &newEvidence}, 1, mutation{Actor: "skill:isucon-investigate", Operation: "constraint.update"}, "rewrite terminal constraint"); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("terminal update error = %v", err)
	}
	if err := store.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInvestigatedChangeCanBecomeReadyWithoutConstraint(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "candidate"})
	prepareReadyContract(t, store, "B-001")
	_, err := store.resolveCardAndWake("B-001", "READY", CardPatch{}, mutation{
		Actor: "skill:isucon-investigate", Operation: "resolve", ExpectedCardVersion: intPtr(0),
	}, "safe boundary is ready")
	if err != nil {
		t.Fatalf("resolve error = %v", err)
	}
}

func TestExplicitDependencyWakesBlockedCard(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-003",
		Card{ID: "B-001", Status: "BLOCKED", Title: "waiting for evidence"},
		Card{ID: "B-002", Status: "APPLIED", Title: "prerequisite intervention"})

	woke, err := store.addDependency(DependencyInput{CardID: "B-001", DependsOnCardID: "B-002", RequiredStatus: "VALIDATED", Mode: "BLOCKING", Reason: "validated prerequisite is required"},
		mutation{Actor: "skill:isucon-investigate", Operation: "dependency.add", ExpectedCardVersion: intPtr(0)})
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
	woken, err := store.transitionCardAndWake("B-002", "VALIDATED", mutation{Actor: "agent:worker", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "prerequisite validated")
	if err != nil {
		t.Fatal(err)
	}
	if len(woken) != 1 || woken[0] != "B-001" {
		t.Fatalf("woken = %#v", woken)
	}
	card, err = store.getCard("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "INVESTIGATE" || card.Version != 2 {
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

	if err := store.transitionCard("B-001", "READY", mutation{Actor: "skill:isucon-investigate", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "safety gate passed"); err != nil {
		t.Fatal(err)
	}

	_, err = store.addCard(NewCard{
		Title:  "forbidden ready card",
		Actor:  "skill:isucon-special-sauce",
		Reason: "attempt direct READY creation",
		CardPatch: CardPatch{Values: map[string]string{
			"status": "READY",
		}},
	}, mutation{Actor: "skill:isucon-special-sauce", Operation: "add"})
	if err == nil || !strings.Contains(err.Error(), "must start as INVESTIGATE") {
		t.Fatalf("add READY by another skill error = %v", err)
	}
}

func TestAddCardKeepsDecisionTextInSections(t *testing.T) {
	store := testStore(t)
	id, err := store.addCard(NewCard{
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

func TestValidateRejectsReadyCardWithOwner(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "READY", Owner: "agent:stale", Title: "invalid ready card"})
	if err := store.validate(); err == nil || !strings.Contains(err.Error(), "is READY but has non-empty owner") {
		t.Fatalf("validate ready owner error = %v", err)
	}
}

func TestValidateRejectsMalformedOwner(t *testing.T) {
	for _, owner := range []string{"none", "NONE", "null", "-", " agent:test "} {
		t.Run(owner, func(t *testing.T) {
			store := testStore(t)
			seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Owner: owner, Title: "invalid owner"})
			if err := store.validate(); err == nil || !strings.Contains(err.Error(), "invalid owner") {
				t.Fatalf("validate malformed owner %q error = %v", owner, err)
			}
		})
	}
}

func TestVerificationAcceptsFreeTextAndStructuredJSON(t *testing.T) {
	store := testStore(t)
	seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: "INVESTIGATE", Title: "candidate"})

	err := store.updateCard("B-001", CardPatch{Sections: map[string]string{
		sectionVerification: "run a focused test",
	}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "write free-text verification")
	if err != nil {
		t.Fatalf("free-text Verification update error = %v", err)
	}

	if err := store.updateCard("B-001", CardPatch{Sections: map[string]string{
		sectionVerification: `{"version":1,"checks":["run a focused test"],"note":"correctness-only"}`,
	}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(1)}, "write structured verification"); err != nil {
		t.Fatal(err)
	}
}

func TestReadyGateAcceptsFreeTextVerification(t *testing.T) {
	store := testStore(t)
	verification := Section{Name: sectionVerification, Body: "run a focused test"}
	seedBacklog(t, store, 0, "B-002",
		Card{ID: "B-001", Status: "INVESTIGATE", Title: "candidate", Sections: []Section{verification}})
	prepareReadyContract(t, store, "B-001")

	err := store.transitionCard("B-001", "READY", mutation{Actor: "skill:isucon-investigate", Operation: "transition", ExpectedCardVersion: intPtr(0)}, "promote free-text contract")
	if err != nil {
		t.Fatalf("free-text READY transition error = %v", err)
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

	for _, name := range []string{"Invalid section", "observation", "Touches"} {
		err := store.updateCard("B-001", CardPatch{Sections: map[string]string{name: "body"}}, mutation{Actor: "agent:test", Operation: "update", ExpectedCardVersion: intPtr(0)}, "reject unsupported section")
		if err == nil || !strings.Contains(err.Error(), "unsupported section") {
			t.Fatalf("section %q error = %v", name, err)
		}
	}
}

func TestParseCardIDList(t *testing.T) {
	ids, err := parseCardIDList("B-002, b1, B-002")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ids, ","), "B-002,B-001"; got != want {
		t.Fatalf("parsed IDs = %s, want %s", got, want)
	}
	if _, err := parseCardIDList("B-001,,B-002"); err == nil {
		t.Fatal("empty ID was accepted")
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

func TestNormalizeID(t *testing.T) {
	for input, want := range map[string]string{"1": "B-001", "B1": "B-001", "b-12": "B-012", "B-003": "B-003"} {
		if got := normalizeID(input); got != want {
			t.Errorf("normalizeID(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := idNumber("B-nope"); err == nil {
		t.Error("idNumber accepted an invalid ID")
	}
}

func intPtr(value int) *int { return &value }
