package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// runMigrateEvaluation rewrites one legacy section without changing lifecycle or ownership.
func runMigrateEvaluation(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, cardValueFlags())
	fs := flag.NewFlagSet("migrate-evaluation", flag.ExitOnError)
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "migration reason")
	expected := fs.Int("expect-card-version", -1, "expected target card version")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *expected < 0 {
		fatal(errors.New("migrate-evaluation requires one card ID and --expect-card-version"))
	}
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal(err)
	}
	store := openCLIStore(config)
	defer store.Close()
	if err := store.migrateEvaluation(normalizeID(fs.Arg(0)), string(body), mutation{Actor: *actor, ExpectedCardVersion: expected, Operation: "migrate-evaluation"}, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("migrated Evaluation")
}

func (s *Store) migrateEvaluation(id, body string, options mutation, reason string) error {
	if err := ensureReason(options.Actor, reason); err != nil {
		return err
	}
	if options.ExpectedCardVersion == nil || strings.TrimSpace(body) == "" {
		return errors.New("card version and non-empty Evaluation are required")
	}
	tx, revision, err := s.beginMutation(options)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	card, err := getCardFrom(tx, id)
	if err != nil {
		return err
	}
	old := sectionBody(card.Sections, "Verification")
	if old == "" || sectionBody(card.Sections, sectionEvaluation) != "" {
		return errors.New("migration requires Verification and no Evaluation")
	}
	if err := claimCardVersionTx(tx, id, options.ExpectedCardVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE card_sections SET name = ?, body = ? WHERE card_id = ? AND name = 'Verification'`, sectionEvaluation, strings.TrimSpace(body), id); err != nil {
		return err
	}
	if requiresReadyContract(card.Status) {
		if err := validateReadyContract(tx, id); err != nil {
			return err
		}
	}
	stamp := now()
	if _, err := tx.Exec(`UPDATE cards SET updated = ?, updated_by = ? WHERE id = ?`, stamp, options.Actor, id); err != nil {
		return err
	}
	if err := addHistoryTx(tx, id, stamp, options.Actor, reason+"\nPrevious Verification:\n"+old); err != nil {
		return err
	}
	options.CardID, options.Summary = id, reason
	return finishMutation(tx, revision, options)
}
