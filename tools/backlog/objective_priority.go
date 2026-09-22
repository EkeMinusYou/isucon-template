package main

func (s *Store) migrateObjectivePriority() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('objectives') WHERE name = 'priority'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := tx.Exec(`ALTER TABLE objectives ADD COLUMN priority TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return tx.Commit()
}
