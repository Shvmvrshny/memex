package memex

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// KnowledgeGraph stores temporal entity-relationship triples in SQLite (WAL mode).
// NOTE: Fact, KGStats, and RecordFactRequest types are defined in models.go (Phase 1).
type KnowledgeGraph struct {
	db *sql.DB
}

// NewKnowledgeGraph opens (or creates) the SQLite database at path.
// Use ":memory:" for tests.
func NewKnowledgeGraph(path string) (*KnowledgeGraph, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	// Single writer — serialized writes are fine for this workload.
	db.SetMaxOpenConns(1)
	return &KnowledgeGraph{db: db}, nil
}

// Init creates the schema and enables WAL mode.
func (kg *KnowledgeGraph) Init() error {
	_, err := kg.db.Exec(`PRAGMA journal_mode=WAL`)
	if err != nil {
		return fmt.Errorf("WAL pragma: %w", err)
	}
	_, err = kg.db.Exec(`
		CREATE TABLE IF NOT EXISTS facts (
			id          TEXT PRIMARY KEY,
			subject     TEXT NOT NULL,
			predicate   TEXT NOT NULL,
			object      TEXT NOT NULL,
			valid_from  TEXT,
			valid_until TEXT,
			source      TEXT,
			created_at  TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_facts_subject          ON facts(subject);
		CREATE INDEX IF NOT EXISTS idx_facts_object           ON facts(object);
		CREATE INDEX IF NOT EXISTS idx_facts_subject_pred     ON facts(subject, predicate);
		CREATE INDEX IF NOT EXISTS idx_facts_valid_until      ON facts(valid_until);
	`)
	return err
}

// RecordFact inserts a new triple.
//   - validFrom: ISO8601 timestamp. Empty string means "now".
//   - singular: if true, closes any existing active fact for (subject, predicate) first.
//
// Idempotent: if an identical active triple exists (same subject+predicate+object, no valid_until),
// the existing ID is returned without insertion.
func (kg *KnowledgeGraph) RecordFact(subject, predicate, object, validFrom, source string, singular bool) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if validFrom == "" {
		validFrom = now
	}

	// Idempotency check
	var existingID string
	err := kg.db.QueryRow(
		`SELECT id FROM facts WHERE subject=? AND predicate=? AND object=? AND valid_until IS NULL`,
		subject, predicate, object,
	).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}

	// Singular: close the current active fact for this (subject, predicate)
	if singular {
		_, err = kg.db.Exec(
			`UPDATE facts SET valid_until=? WHERE subject=? AND predicate=? AND valid_until IS NULL`,
			now, subject, predicate,
		)
		if err != nil {
			return "", fmt.Errorf("expire old singular fact: %w", err)
		}
	}

	id := uuid.New().String()
	_, err = kg.db.Exec(
		`INSERT INTO facts (id, subject, predicate, object, valid_from, valid_until, source, created_at)
		 VALUES (?, ?, ?, ?, ?, NULL, ?, ?)`,
		id, subject, predicate, object, validFrom, source, now,
	)
	if err != nil {
		return "", fmt.Errorf("insert fact: %w", err)
	}
	return id, nil
}

// ExpireFact sets valid_until on the fact with the given ID.
// The fact is preserved for history — it is never deleted.
func (kg *KnowledgeGraph) ExpireFact(id, validUntil string) error {
	if validUntil == "" {
		validUntil = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := kg.db.Exec(`UPDATE facts SET valid_until=? WHERE id=? AND valid_until IS NULL`, validUntil, id)
	if err != nil {
		return fmt.Errorf("expire fact: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("fact %q not found or already expired", id)
	}
	return nil
}

// QueryEntity returns facts where subject OR object equals entity.
// If asOf is non-empty (ISO8601), returns only facts valid at that point in time.
// If asOf is empty, returns only currently active facts (valid_until IS NULL).
func (kg *KnowledgeGraph) QueryEntity(entity, asOf string) ([]Fact, error) {
	var rows *sql.Rows
	var err error

	if asOf == "" {
		rows, err = kg.db.Query(
			`SELECT id, subject, predicate, object, valid_from, valid_until, source, created_at
			 FROM facts
			 WHERE (subject=? OR object=?) AND valid_until IS NULL
			 ORDER BY created_at DESC`,
			entity, entity,
		)
	} else {
		rows, err = kg.db.Query(
			`SELECT id, subject, predicate, object, valid_from, valid_until, source, created_at
			 FROM facts
			 WHERE (subject=? OR object=?)
			   AND (valid_from IS NULL OR valid_from <= ?)
			   AND (valid_until IS NULL OR valid_until > ?)
			 ORDER BY created_at DESC`,
			entity, entity, asOf, asOf,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query entity: %w", err)
	}
	defer rows.Close()
	return scanFacts(rows)
}

// History returns all facts (active and expired) for an entity, ordered oldest first.
func (kg *KnowledgeGraph) History(entity string) ([]Fact, error) {
	rows, err := kg.db.Query(
		`SELECT id, subject, predicate, object, valid_from, valid_until, source, created_at
		 FROM facts
		 WHERE subject=? OR object=?
		 ORDER BY created_at ASC`,
		entity, entity,
	)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	defer rows.Close()
	return scanFacts(rows)
}

// Stats returns aggregate counts and predicate type counts.
// PredicateTypes matches the KGStats field defined in models.go.
func (kg *KnowledgeGraph) Stats() (KGStats, error) {
	var stats KGStats

	row := kg.db.QueryRow(`SELECT COUNT(*) FROM facts`)
	if err := row.Scan(&stats.TotalFacts); err != nil {
		return KGStats{}, fmt.Errorf("stats total: %w", err)
	}

	row = kg.db.QueryRow(`SELECT COUNT(*) FROM facts WHERE valid_until IS NULL`)
	if err := row.Scan(&stats.ActiveFacts); err != nil {
		return KGStats{}, fmt.Errorf("stats active: %w", err)
	}

	stats.ExpiredFacts = stats.TotalFacts - stats.ActiveFacts

	row = kg.db.QueryRow(`SELECT COUNT(DISTINCT subject) FROM facts`)
	if err := row.Scan(&stats.EntityCount); err != nil {
		return KGStats{}, fmt.Errorf("stats entity count: %w", err)
	}

	rows, err := kg.db.Query(`SELECT predicate, COUNT(*) FROM facts GROUP BY predicate ORDER BY predicate`)
	if err != nil {
		return KGStats{}, fmt.Errorf("stats predicate query: %w", err)
	}
	defer rows.Close()
	stats.PredicateTypes = make(map[string]int)
	for rows.Next() {
		var p string
		var count int
		if err := rows.Scan(&p, &count); err != nil {
			return KGStats{}, fmt.Errorf("stats predicate scan: %w", err)
		}
		stats.PredicateTypes[p] = count
	}

	return stats, nil
}

func scanFacts(rows *sql.Rows) ([]Fact, error) {
	var facts []Fact
	for rows.Next() {
		var f Fact
		var validUntil sql.NullString
		var validFrom sql.NullString
		var source sql.NullString
		if err := rows.Scan(&f.ID, &f.Subject, &f.Predicate, &f.Object,
			&validFrom, &validUntil, &source, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.ValidFrom = validFrom.String
		f.ValidUntil = validUntil.String
		f.Source = source.String
		facts = append(facts, f)
	}
	return facts, rows.Err()
}
