package main

import "fmt"

// Legacy RUNs stay immutable. A migration ID identifies the current application
// in future snapshots; it does not establish agreement with an older RUN.
func (s *Store) migrateApplicationIDs() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"cards", "adoption_event_cards"} {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'application_id'`, table).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN application_id TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
			if table == "cards" {
				if _, err := tx.Exec(`UPDATE cards SET application_id = 'legacy:' || id || '@' || card_version WHERE status = 'APPLIED'`); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func checkExpectedOwner(card Card, expected *string) error {
	if expected != nil && card.Owner != *expected {
		return fmt.Errorf("card owner conflict: %s expected %q, current %q", card.ID, *expected, card.Owner)
	}
	return nil
}

func validateApplicationID(card Card, snapshot AppliedSnapshotCard) error {
	if card.ApplicationID == "" || snapshot.ApplicationID == "" {
		return fmt.Errorf("card %s application ID is missing from the card or RUN snapshot; historical evidence cannot establish the current application", card.ID)
	}
	if card.ApplicationID != snapshot.ApplicationID {
		return fmt.Errorf("card %s application differs from the evidence RUN snapshot: current=%s measured=%s", card.ID, card.ApplicationID, snapshot.ApplicationID)
	}
	return nil
}
