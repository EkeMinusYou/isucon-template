package main

import (
	"fmt"
	"strconv"
	"strings"
)

func (s *Store) migrateImplementationEstimate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cards') WHERE name = 'implementation_estimate_minutes'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := tx.Exec(`ALTER TABLE cards ADD COLUMN implementation_estimate_minutes INTEGER CHECK (implementation_estimate_minutes IS NULL OR (typeof(implementation_estimate_minutes) = 'integer' AND implementation_estimate_minutes > 0))`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func cardPatchValue(key, raw string) (any, error) {
	if key != "implementation-estimate-minutes" {
		return raw, nil
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	minutes, err := strconv.Atoi(value)
	if err != nil || minutes <= 0 {
		return nil, fmt.Errorf("--implementation-estimate-minutes requires a positive whole number of minutes, or empty to clear; got %q", raw)
	}
	return minutes, nil
}

func implementationEstimateText(card Card) string {
	if card.ImplementationEstimateMinutes == nil {
		return "未見積り"
	}
	return fmt.Sprintf("%dm", *card.ImplementationEstimateMinutes)
}

func implementationEstimateLabel(card Card) string {
	if card.ImplementationEstimateMinutes == nil {
		return "-"
	}
	return implementationEstimateText(card)
}
