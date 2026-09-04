package main

import (
	"database/sql"
	"errors"
	"net/http"

	_ "modernc.org/sqlite"
)

type backlogCard struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Closed    bool   `json:"closed"`
	Priority  string `json:"priority"`
	Owner     string `json:"owner"`
	Area      string `json:"area"`
	Updated   string `json:"updated"`
	UpdatedBy string `json:"updated_by"`
}

type backlogResponse struct {
	Counts map[string]int `json:"counts"`
	Cards  []backlogCard  `json:"cards"`
}

type backlogSection struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	Body     string `json:"body"`
}

type backlogHistoryEntry struct {
	Position   int    `json:"position"`
	OccurredAt string `json:"occurred_at"`
	Actor      string `json:"actor"`
	Body       string `json:"body"`
}

type backlogDependency struct {
	CardID          string `json:"card_id"`
	DependsOnCardID string `json:"depends_on_card_id"`
	RequiredStatus  string `json:"required_status"`
	Mode            string `json:"mode"`
	Reason          string `json:"reason"`
	Status          string `json:"status"`
}

type backlogObjectiveRelation struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Mode      string `json:"mode"`
	Title     string `json:"title"`
	Rationale string `json:"rationale"`
}

type backlogConstraintRelation struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Role      string `json:"role"`
	Rationale string `json:"rationale"`
}

type backlogCardDetail struct {
	ID           string                      `json:"id"`
	Status       string                      `json:"status"`
	Title        string                      `json:"title"`
	Closed       bool                        `json:"closed"`
	Priority     string                      `json:"priority"`
	Owner        string                      `json:"owner"`
	Area         string                      `json:"area"`
	SourceRuns   string                      `json:"source_runs"`
	CompareRun   string                      `json:"compare_run"`
	ObservedRuns string                      `json:"observed_runs"`
	Fingerprint  string                      `json:"fingerprint"`
	Updated      string                      `json:"updated"`
	UpdatedBy    string                      `json:"updated_by"`
	Sections     []backlogSection            `json:"sections"`
	History      []backlogHistoryEntry       `json:"history"`
	Dependencies []backlogDependency         `json:"dependencies"`
	Unblocks     []backlogDependency         `json:"unblocks"`
	Objectives   []backlogObjectiveRelation  `json:"objectives"`
	Constraints  []backlogConstraintRelation `json:"constraints"`
}

const backlogCardColumns = `
		c.id, c.status, c.title, c.priority, c.owner, c.area,
	COALESCE((SELECT group_concat(run_id, ',') FROM (SELECT run_id FROM card_runs WHERE card_id=c.id AND relation='SOURCE' ORDER BY position)), ''),
	COALESCE((SELECT group_concat(run_id, ',') FROM (SELECT run_id FROM card_runs WHERE card_id=c.id AND relation='COMPARE' ORDER BY position)), ''),
	COALESCE((SELECT group_concat(run_id, ',') FROM (SELECT run_id FROM card_runs WHERE card_id=c.id AND relation='OBSERVED' ORDER BY position)), ''),
		c.fingerprint, c.updated, c.updated_by`

// queryBacklogCard mirrors tools/backlog's getCardFrom/loadCardContent query
// shape (core columns + card_sections + card_history) but is
// implemented independently and read-only, for the same reason as
// queryBacklog below.
func queryBacklogCard(dbPath, id string) (*backlogCardDetail, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var d backlogCardDetail
	row := db.QueryRow(`SELECT `+backlogCardColumns+` FROM cards c WHERE c.id = ?`, id)
	err = row.Scan(
		&d.ID, &d.Status, &d.Title, &d.Priority, &d.Owner, &d.Area,
		&d.SourceRuns, &d.CompareRun, &d.ObservedRuns, &d.Fingerprint,
		&d.Updated, &d.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Closed = isDashboardClosedStatus(d.Status)

	d.Sections = []backlogSection{}
	sectionRows, err := db.Query(`SELECT name, position, body FROM card_sections WHERE card_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	for sectionRows.Next() {
		var s backlogSection
		if err := sectionRows.Scan(&s.Name, &s.Position, &s.Body); err != nil {
			sectionRows.Close()
			return nil, err
		}
		d.Sections = append(d.Sections, s)
	}
	if err := sectionRows.Err(); err != nil {
		sectionRows.Close()
		return nil, err
	}
	sectionRows.Close()

	d.History = []backlogHistoryEntry{}
	historyRows, err := db.Query(`SELECT position, occurred_at, actor, body FROM card_history WHERE card_id = ? ORDER BY position DESC`, id)
	if err != nil {
		return nil, err
	}
	for historyRows.Next() {
		var h backlogHistoryEntry
		if err := historyRows.Scan(&h.Position, &h.OccurredAt, &h.Actor, &h.Body); err != nil {
			historyRows.Close()
			return nil, err
		}
		d.History = append(d.History, h)
	}
	if err := historyRows.Err(); err != nil {
		historyRows.Close()
		return nil, err
	}
	historyRows.Close()

	d.Dependencies = []backlogDependency{}
	dependencyRows, err := db.Query(`SELECT dep.card_id, dep.depends_on_card_id, dep.required_status, dep.mode, dep.reason,
	        target.status
        FROM card_dependencies dep JOIN cards target ON target.id = dep.depends_on_card_id
        WHERE dep.card_id = ? ORDER BY dep.depends_on_card_id`, id)
	if err != nil {
		return nil, err
	}
	for dependencyRows.Next() {
		var dependency backlogDependency
		if err := dependencyRows.Scan(&dependency.CardID, &dependency.DependsOnCardID, &dependency.RequiredStatus,
			&dependency.Mode, &dependency.Reason, &dependency.Status); err != nil {
			dependencyRows.Close()
			return nil, err
		}
		d.Dependencies = append(d.Dependencies, dependency)
	}
	if err := dependencyRows.Err(); err != nil {
		dependencyRows.Close()
		return nil, err
	}
	dependencyRows.Close()

	d.Unblocks = []backlogDependency{}
	unblockRows, err := db.Query(`SELECT dep.card_id, dep.depends_on_card_id, dep.required_status, dep.mode, dep.reason,
	        dependent.status
        FROM card_dependencies dep JOIN cards dependent ON dependent.id = dep.card_id
        WHERE dep.depends_on_card_id = ? ORDER BY dep.card_id`, id)
	if err != nil {
		return nil, err
	}
	for unblockRows.Next() {
		var dependency backlogDependency
		if err := unblockRows.Scan(&dependency.CardID, &dependency.DependsOnCardID, &dependency.RequiredStatus,
			&dependency.Mode, &dependency.Reason, &dependency.Status); err != nil {
			unblockRows.Close()
			return nil, err
		}
		d.Unblocks = append(d.Unblocks, dependency)
	}
	if err := unblockRows.Err(); err != nil {
		unblockRows.Close()
		return nil, err
	}
	unblockRows.Close()

	d.Objectives = []backlogObjectiveRelation{}
	objectiveRows, err := db.Query(`SELECT o.id, o.status, o.mode, o.title, oi.rationale
		FROM objective_interventions oi JOIN objectives o ON o.id = oi.objective_id
		WHERE oi.card_id = ? ORDER BY o.id`, id)
	if err != nil {
		return nil, err
	}
	for objectiveRows.Next() {
		var relation backlogObjectiveRelation
		if err := objectiveRows.Scan(&relation.ID, &relation.Status, &relation.Mode, &relation.Title, &relation.Rationale); err != nil {
			objectiveRows.Close()
			return nil, err
		}
		d.Objectives = append(d.Objectives, relation)
	}
	if err := objectiveRows.Err(); err != nil {
		objectiveRows.Close()
		return nil, err
	}
	if err := objectiveRows.Close(); err != nil {
		return nil, err
	}

	d.Constraints = []backlogConstraintRelation{}
	constraintRows, err := db.Query(`SELECT c.id, c.status, c.title, ci.role, ci.rationale
		FROM constraint_interventions ci JOIN constraints c ON c.id = ci.constraint_id
		WHERE ci.card_id = ? ORDER BY c.id`, id)
	if err != nil {
		return nil, err
	}
	for constraintRows.Next() {
		var relation backlogConstraintRelation
		if err := constraintRows.Scan(&relation.ID, &relation.Status, &relation.Title, &relation.Role, &relation.Rationale); err != nil {
			constraintRows.Close()
			return nil, err
		}
		d.Constraints = append(d.Constraints, relation)
	}
	if err := constraintRows.Err(); err != nil {
		constraintRows.Close()
		return nil, err
	}
	if err := constraintRows.Close(); err != nil {
		return nil, err
	}

	return &d, nil
}

// queryBacklog opens the backlog SQLite database read-only and lists cards.
// By default it matches tools/backlog's own CLI default (`list` without
// `--all`): terminal cards (VALIDATED/REJECTED) are excluded. Pass
// includeClosed=true to include them too. It is intentionally independent of
// tools/backlog's own code (which is `package main` and not importable) so
// this dashboard never risks writing to the store of record.
func queryBacklog(dbPath string, includeClosed bool) (*backlogResponse, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
			SELECT id, status, title, priority, owner, area, updated, updated_by
		FROM cards
	`
	if !includeClosed {
		query += " WHERE status NOT IN ('VALIDATED', 'REJECTED')"
	}
	query += " ORDER BY status, updated DESC"

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resp := &backlogResponse{Counts: map[string]int{}, Cards: []backlogCard{}}
	for rows.Next() {
		var c backlogCard
		if err := rows.Scan(&c.ID, &c.Status, &c.Title, &c.Priority, &c.Owner, &c.Area, &c.Updated, &c.UpdatedBy); err != nil {
			return nil, err
		}
		c.Closed = isDashboardClosedStatus(c.Status)
		resp.Cards = append(resp.Cards, c)
		resp.Counts[c.Status]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

func isDashboardClosedStatus(status string) bool {
	return status == "VALIDATED" || status == "REJECTED"
}

func (a *app) handleBacklog(w http.ResponseWriter, r *http.Request) {
	includeClosed := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	resp, err := queryBacklog(a.dbPath, includeClosed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

func (a *app) handleBacklogDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detail, err := queryBacklogCard(a.dbPath, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	writeJSON(w, detail)
}
