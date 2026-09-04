package main

import (
	"bufio"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type AdoptionEvent struct {
	RunID            string
	AdoptedAt        string
	Actor            string
	Forced           bool
	Score            *int64
	Passed           *bool
	ComparisonRunID  string
	ComparisonScore  *int64
	ComparisonStatus string
	Delta            *int64
	ManifestSHA256   string
	SnapshotRevision int
}

func (s *Store) adoptCardsMatching(ids []string, changeBoundaryHashes map[string]string, event AdoptionEvent, reason string) ([]Card, error) {
	if err := ensureReason(event.Actor, reason); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("at least one card ID is required")
	}
	if runs, err := parseRunIDsStrict(event.RunID); err != nil || len(runs) != 1 || runs[0] != event.RunID {
		return nil, fmt.Errorf("invalid adoption RUN ID %q", event.RunID)
	}
	if event.ComparisonRunID != "" {
		if runs, err := parseRunIDsStrict(event.ComparisonRunID); err != nil || len(runs) != 1 || runs[0] != event.ComparisonRunID {
			return nil, fmt.Errorf("invalid adoption comparison RUN ID %q", event.ComparisonRunID)
		}
	}
	if event.AdoptedAt == "" || event.ComparisonStatus == "" || event.ManifestSHA256 == "" || event.SnapshotRevision < 0 {
		return nil, errors.New("adoption event is missing timestamp, comparison status, manifest hash, or snapshot revision")
	}
	if !map[string]bool{"none": true, "compatible": true, "incompatible": true}[event.ComparisonStatus] {
		return nil, fmt.Errorf("invalid adoption comparison status %q", event.ComparisonStatus)
	}
	digest, ok := strings.CutPrefix(event.ManifestSHA256, "sha256:")
	if !ok || len(digest) != 64 {
		return nil, fmt.Errorf("invalid adoption manifest hash %q", event.ManifestSHA256)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return nil, fmt.Errorf("invalid adoption manifest hash %q: %w", event.ManifestSHA256, err)
	}

	tx, current, err := s.beginMutation(mutation{Actor: event.Actor})
	if err != nil {
		return nil, err
	}
	rollback := func(err error) ([]Card, error) {
		_ = tx.Rollback()
		return nil, err
	}

	requested := make([]Card, 0, len(ids))
	seen := map[string]bool{}
	for _, rawID := range ids {
		id := normalizeID(rawID)
		if _, err := idNumber(id); err != nil {
			return rollback(err)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		card, err := getCardFrom(tx, id)
		if err != nil {
			return rollback(err)
		}
		if card.Status != "APPLIED" {
			return rollback(fmt.Errorf("card %s has status %s; expected APPLIED", id, card.Status))
		}
		if changeBoundaryHashes[id] != cardChangeBoundaryHash(card) {
			return rollback(fmt.Errorf("card %s change boundary differs from the evidence RUN snapshot", id))
		}
		requested = append(requested, card)
	}

	result, err := tx.Exec(`INSERT INTO adoption_events(
        run_id, adopted_at, actor, forced, score, passed, comparison_run_id,
        comparison_score, comparison_status, delta, manifest_sha256, snapshot_revision
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.RunID, event.AdoptedAt, event.Actor, event.Forced, nullableInt64(event.Score), nullableBool(event.Passed),
		event.ComparisonRunID, nullableInt64(event.ComparisonScore), event.ComparisonStatus, nullableInt64(event.Delta),
		event.ManifestSHA256, event.SnapshotRevision)
	if err != nil {
		return rollback(err)
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return rollback(err)
	}

	var woke []string
	for _, card := range requested {
		expected := card.Version
		if err := claimCardVersionTx(tx, card.ID, &expected); err != nil {
			return rollback(err)
		}
		updated := event.AdoptedAt
		if _, err := tx.Exec(`UPDATE cards SET status = 'VALIDATED', updated = ?, updated_by = ? WHERE id = ?`, updated, event.Actor, card.ID); err != nil {
			return rollback(err)
		}
		if err := addHistoryTx(tx, card.ID, updated, event.Actor, reason); err != nil {
			return rollback(err)
		}
		origin := "unknown"
		if len(card.History) > 0 && strings.TrimSpace(card.History[0].Actor) != "" {
			origin = card.History[0].Actor
		}
		if _, err := tx.Exec(`INSERT INTO adoption_event_cards(
			adoption_event_id, card_id, origin, change_boundary_hash
		) VALUES (?, ?, ?, ?)`, eventID, card.ID, origin, changeBoundaryHashes[card.ID]); err != nil {
			return rollback(err)
		}
		newlyWoke, err := wakeDependentsTx(tx, card.ID, event.Actor)
		if err != nil {
			return rollback(err)
		}
		woke = append(woke, newlyWoke...)
	}

	summary := fmt.Sprintf("adopted %d card(s) from RUN %s", len(requested), event.RunID)
	if event.Forced {
		summary += " with force"
	}
	if len(woke) > 0 {
		summary += "; woke " + strings.Join(woke, ",") + " to INVESTIGATE"
	}
	if err := finishMutation(tx, current, mutation{
		Actor: event.Actor, Operation: "pass", CardID: strings.Join(ids, ","), Summary: summary,
	}); err != nil {
		return nil, err
	}
	return requested, nil
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func (s *Store) writeOutcomes(path string) error {
	rows, err := s.db.Query(`SELECT
		e.adopted_at, c.card_id, c.origin, c.change_boundary_hash, e.run_id, e.score,
        e.comparison_run_id, e.comparison_score, e.delta,
        (SELECT COUNT(*) FROM adoption_event_cards counted WHERE counted.adoption_event_id = e.id),
        e.comparison_status, e.forced
    FROM adoption_events e
    JOIN adoption_event_cards c ON c.adoption_event_id = e.id
    ORDER BY e.id, c.card_id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".outcomes-*.tsv")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	w := bufio.NewWriter(tmp)
	if _, err := fmt.Fprintln(w, "promoted_at\tcard_id\torigin\tchange_boundary_hash\trun_id\tscore\tcomparison_run_id\tcomparison_score\tdelta\tcards_in_pass\tcomparison_status\tforced"); err != nil {
		_ = tmp.Close()
		return err
	}
	for rows.Next() {
		var adoptedAt, cardID, origin, changeBoundaryHash, runID, comparisonRunID, comparisonStatus string
		var score, comparisonScore, delta sql.NullInt64
		var count int
		var forced bool
		if err := rows.Scan(&adoptedAt, &cardID, &origin, &changeBoundaryHash, &runID, &score,
			&comparisonRunID, &comparisonScore, &delta, &count, &comparisonStatus, &forced); err != nil {
			_ = tmp.Close()
			return err
		}
		fields := []string{
			adoptedAt, cardID, origin, changeBoundaryHash, runID, formatNullInt64(score),
			valueOr(comparisonRunID, "none"), formatNullInt64(comparisonScore), formatNullInt64(delta),
			strconv.Itoa(count), comparisonStatus, strconv.FormatBool(forced),
		}
		for i := range fields {
			fields[i] = strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(fields[i])
		}
		if _, err := fmt.Fprintln(w, strings.Join(fields, "\t")); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func formatNullInt64(value sql.NullInt64) string {
	if !value.Valid {
		return "unknown"
	}
	return strconv.FormatInt(value.Int64, 10)
}
