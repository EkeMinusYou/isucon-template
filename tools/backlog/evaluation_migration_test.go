package main

import (
	"strings"
	"testing"
)

func TestMigrateEvaluationPreservesCardAndRejectsStaleVersion(t *testing.T) {
	for _, status := range []string{"READY", "APPLIED", "REJECTED"} {
		t.Run(status, func(t *testing.T) {
			store := testStore(t)
			seedBacklog(t, store, 0, "B-002", Card{ID: "B-001", Status: status, Title: "existing card", Owner: "skill:worker"})
			if _, err := store.db.Exec(`DELETE FROM card_sections WHERE card_id='B-001' AND name='Evaluation'`); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.Exec(`INSERT INTO card_sections VALUES ('B-001', 'Verification', 7, 'old implementation checks')`); err != nil {
				t.Fatal(err)
			}
			stale := 9
			options := mutation{Actor: "human:test", ExpectedCardVersion: &stale, Operation: "migrate-evaluation"}
			if err := store.migrateEvaluation("B-001", "inspect saved results", options, "scope evaluation"); err == nil {
				t.Fatal("stale migration succeeded")
			}
			current, _ := store.getCard("B-001")
			if current.Version != 0 || sectionBody(current.Sections, "Verification") == "" {
				t.Fatal("failed migration changed card")
			}
			version := current.Version
			options.ExpectedCardVersion = &version
			if err := store.migrateEvaluation("B-001", "inspect saved results", options, "scope evaluation"); err != nil {
				t.Fatal(err)
			}
			card, err := store.getCard("B-001")
			if err != nil {
				t.Fatal(err)
			}
			if card.Status != status || card.Owner != "skill:worker" || card.Version != 1 || sectionBody(card.Sections, "Verification") != "" || sectionBody(card.Sections, sectionEvaluation) != "inspect saved results" {
				t.Fatalf("migrated card = %#v", card)
			}
			found := false
			for _, section := range card.Sections {
				if section.Name == sectionEvaluation && section.Position == 7 {
					found = true
				}
			}
			if !found {
				t.Fatal("section order changed")
			}
			var history string
			if err := store.db.QueryRow(`SELECT body FROM card_history WHERE card_id='B-001' ORDER BY position DESC LIMIT 1`).Scan(&history); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(history, "old implementation checks") {
				t.Fatal("old body missing from history")
			}
			version = card.Version
			if err := store.migrateEvaluation("B-001", "another body", options, "repeat"); err == nil {
				t.Fatal("repeat migration succeeded")
			}
		})
	}
}

func TestNormalWritesRejectVerification(t *testing.T) {
	if err := validateSectionName("Verification"); err == nil {
		t.Fatal("legacy section accepted")
	}
	if err := validateSectionName(sectionEvaluation); err != nil {
		t.Fatal(err)
	}
}
