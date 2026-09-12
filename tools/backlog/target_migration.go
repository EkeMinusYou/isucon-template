package main

import "fmt"

// migrateLegacyTargets preserves historical assessments and judgments without
// interpreting a legacy resolution as a newly achieved improvement goal.
func (s *Store) migrateLegacyTargets() error {
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='constraints'`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	var done int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM metadata WHERE key='target_migration_v1'`).Scan(&done); err != nil {
		return err
	}
	if done != 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`INSERT INTO legacy_target_exemptions(card_id,reason) SELECT id,'Terminal intervention predates Target chain; historical decision preserved without inventing an improvement target' FROM cards WHERE status IN ('VALIDATED','REJECTED')`,
		`INSERT INTO targets(id,target_version,status,title,priority,fingerprint,scope,source_runs,observed_runs,evidence,resolution,goal,merged_into_target_id,updated,updated_by)
 SELECT id,constraint_version,CASE status WHEN 'INVALIDATED' THEN 'RETIRED' ELSE status END,title,priority,fingerprint,scope,source_runs,observed_runs,evidence,resolution,resolution,merged_into_constraint_id,updated,updated_by FROM constraints`,
		`INSERT INTO target_history SELECT * FROM constraint_history`,
		`INSERT INTO objective_targets(objective_id,target_id,is_primary) SELECT objective_id,constraint_id,CASE WHEN objective_id=(SELECT MIN(b.objective_id) FROM objective_constraints b WHERE b.constraint_id=a.constraint_id) THEN 1 ELSE 0 END FROM objective_constraints a`,
		`INSERT INTO target_interventions(card_id,target_id,role,legacy_role,rationale,is_primary) SELECT card_id,constraint_id,'IMPROVES',role,rationale,CASE WHEN constraint_id=(SELECT MIN(b.constraint_id) FROM constraint_interventions b WHERE b.card_id=a.card_id) THEN 1 ELSE 0 END FROM constraint_interventions a`,
		`INSERT INTO target_intervention_assessments SELECT * FROM constraint_intervention_assessments`,
		`UPDATE metadata SET value=(SELECT value FROM metadata WHERE key='next_constraint_id') WHERE key='next_target_id'`,
		`INSERT INTO metadata(key,value) VALUES('target_migration_v1','legacy identity, evidence, goal, status and assessment preserved; active target axis/evaluation require review')`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("migrate targets: %w", err)
		}
	}
	return tx.Commit()
}
