package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AppliedSnapshot struct {
	SchemaVersion int                   `json:"schema_version"`
	Status        string                `json:"status"`
	CapturedAt    string                `json:"captured_at"`
	Revision      int                   `json:"revision"`
	Cards         []AppliedSnapshotCard `json:"cards"`
}

type AppliedSnapshotCard struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Version        int    `json:"version"`
	Title          string `json:"title"`
	Fingerprint    string `json:"fingerprint"`
	DefinitionHash string `json:"definition_hash"`
}

type runSnapshotEnvelope struct {
	SchemaVersion   int             `json:"schema_version"`
	Phase           string          `json:"phase"`
	RunID           string          `json:"run_id"`
	BacklogSnapshot AppliedSnapshot `json:"backlog_snapshot"`
}

func (s *Store) appliedSnapshot() (AppliedSnapshot, error) {
	tx, err := s.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return AppliedSnapshot{}, err
	}
	defer tx.Rollback()

	var revisionText string
	if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = 'backlog_revision'`).Scan(&revisionText); err != nil {
		return AppliedSnapshot{}, fmt.Errorf("read backlog revision: %w", err)
	}
	revision, err := strconvAtoiNonNegative(revisionText)
	if err != nil {
		return AppliedSnapshot{}, err
	}

	rows, err := tx.Query(`SELECT id FROM cards WHERE status = 'APPLIED' ORDER BY id`)
	if err != nil {
		return AppliedSnapshot{}, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return AppliedSnapshot{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return AppliedSnapshot{}, err
	}

	cards := make([]AppliedSnapshotCard, 0, len(ids))
	for _, id := range ids {
		card, err := getCardFrom(tx, id)
		if err != nil {
			return AppliedSnapshot{}, err
		}
		cards = append(cards, snapshotCard(card))
	}
	if err := tx.Commit(); err != nil {
		return AppliedSnapshot{}, err
	}
	return AppliedSnapshot{
		SchemaVersion: 1,
		Status:        "ok",
		CapturedAt:    time.Now().Format(time.RFC3339Nano),
		Revision:      revision,
		Cards:         cards,
	}, nil
}

func strconvAtoiNonNegative(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid backlog revision %q", value)
	}
	return n, nil
}

func snapshotCard(card Card) AppliedSnapshotCard {
	return AppliedSnapshotCard{
		ID:             card.ID,
		Status:         card.Status,
		Version:        card.Version,
		Title:          card.Title,
		Fingerprint:    card.Fingerprint,
		DefinitionHash: cardDefinitionHash(card),
	}
}

func cardDefinitionHash(card Card) string {
	definition := struct {
		Fingerprint    string `json:"fingerprint"`
		ChangeBoundary string `json:"change_boundary"`
		Verification   string `json:"verification"`
	}{
		Fingerprint:    strings.TrimSpace(card.Fingerprint),
		ChangeBoundary: strings.TrimSpace(sectionBody(card.Sections, sectionChangeBoundary)),
		Verification:   strings.TrimSpace(sectionBody(card.Sections, sectionVerification)),
	}
	body, _ := json.Marshal(definition)
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeAppliedSnapshot(path string, snapshot AppliedSnapshot) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("--output is required")
	}
	body, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeFileAtomic(path, body, 0o644)
}

func writeFileAtomic(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func loadRunAppliedSnapshot(root, runID string) (runSnapshotEnvelope, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || filepath.Base(runID) != runID || strings.Contains(runID, string(filepath.Separator)) {
		return runSnapshotEnvelope{}, fmt.Errorf("invalid evidence RUN %q", runID)
	}
	path := filepath.Join(root, "runs", runID, "run.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return runSnapshotEnvelope{}, fmt.Errorf("read evidence RUN manifest: %w", err)
	}
	var run runSnapshotEnvelope
	if err := json.Unmarshal(body, &run); err != nil {
		return runSnapshotEnvelope{}, fmt.Errorf("parse evidence RUN manifest: %w", err)
	}
	if run.RunID != runID {
		return runSnapshotEnvelope{}, fmt.Errorf("evidence RUN ID mismatch: manifest=%q requested=%q", run.RunID, runID)
	}
	if run.Phase != "finalized" {
		return runSnapshotEnvelope{}, fmt.Errorf("evidence RUN %s is not finalized", runID)
	}
	if run.BacklogSnapshot.Status != "ok" || run.BacklogSnapshot.SchemaVersion < 1 {
		return runSnapshotEnvelope{}, fmt.Errorf("evidence RUN %s has no usable APPLIED snapshot", runID)
	}
	return run, nil
}

func validateRunSnapshotCard(run runSnapshotEnvelope, card Card) error {
	for _, item := range run.BacklogSnapshot.Cards {
		if item.ID != card.ID {
			continue
		}
		if item.Status != "APPLIED" {
			return fmt.Errorf("evidence RUN %s records %s as %s, expected APPLIED", run.RunID, card.ID, item.Status)
		}
		if item.DefinitionHash != cardDefinitionHash(card) {
			return fmt.Errorf("card %s definition differs from evidence RUN %s snapshot", card.ID, run.RunID)
		}
		return nil
	}
	return fmt.Errorf("card %s was not APPLIED in evidence RUN %s", card.ID, run.RunID)
}

func snapshotCardIDs(run runSnapshotEnvelope) []string {
	var ids []string
	for _, card := range run.BacklogSnapshot.Cards {
		if card.Status == "APPLIED" {
			ids = append(ids, card.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func snapshotDefinitionHashes(run runSnapshotEnvelope) map[string]string {
	hashes := map[string]string{}
	for _, card := range run.BacklogSnapshot.Cards {
		if card.Status == "APPLIED" {
			hashes[card.ID] = card.DefinitionHash
		}
	}
	return hashes
}
