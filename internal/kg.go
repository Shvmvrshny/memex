package memex

import (
	"database/sql"
	"fmt"
	"path"
	"sort"
	"strings"
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
			file_path   TEXT,
			commit_hash TEXT,
			confidence  REAL,
			meta_json   TEXT,
			created_at  TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_facts_subject          ON facts(subject);
		CREATE INDEX IF NOT EXISTS idx_facts_object           ON facts(object);
		CREATE INDEX IF NOT EXISTS idx_facts_subject_pred     ON facts(subject, predicate);
		CREATE INDEX IF NOT EXISTS idx_facts_valid_until      ON facts(valid_until);
	`)
	if err != nil {
		return err
	}
	// Backward-compatible migrations for DBs created before scoped fact columns existed.
	if err := kg.ensureFactColumn("file_path", "TEXT"); err != nil {
		return err
	}
	if err := kg.ensureFactColumn("commit_hash", "TEXT"); err != nil {
		return err
	}
	if err := kg.ensureFactColumn("confidence", "REAL"); err != nil {
		return err
	}
	if err := kg.ensureFactColumn("meta_json", "TEXT"); err != nil {
		return err
	}
	if _, err := kg.db.Exec(`CREATE INDEX IF NOT EXISTS idx_facts_file_commit ON facts(file_path, commit_hash)`); err != nil {
		return fmt.Errorf("create idx_facts_file_commit: %w", err)
	}
	return nil
}

// RecordFact inserts a new triple.
//   - validFrom: ISO8601 timestamp. Empty string means "now".
//   - singular: if true, closes any existing active fact for (subject, predicate) first.
//
// Idempotent: if an identical active triple exists (same subject+predicate+object, no valid_until),
// the existing ID is returned without insertion.
func (kg *KnowledgeGraph) RecordFact(subject, predicate, object, validFrom, source string, singular bool) (string, error) {
	return kg.RecordFactScoped(Fact{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		ValidFrom: validFrom,
		Source:    source,
	}, singular)
}

// RecordFactScoped inserts a fact with optional file/commit scoping and metadata.
func (kg *KnowledgeGraph) RecordFactScoped(fact Fact, singular bool) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if fact.ValidFrom == "" {
		fact.ValidFrom = now
	}

	// Idempotency check
	var existingID string
	err := kg.db.QueryRow(
		`SELECT id FROM facts
		 WHERE subject=? AND predicate=? AND object=? AND valid_until IS NULL
		   AND IFNULL(file_path, '') = IFNULL(?, '')
		   AND IFNULL(commit_hash, '') = IFNULL(?, '')`,
		fact.Subject, fact.Predicate, fact.Object, nullableString(fact.FilePath), nullableString(fact.CommitHash),
	).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}

	// Singular: close the current active fact for this (subject, predicate)
	if singular {
		query := `UPDATE facts SET valid_until=? WHERE subject=? AND predicate=? AND valid_until IS NULL`
		args := []any{now, fact.Subject, fact.Predicate}
		if fact.FilePath != "" || fact.CommitHash != "" {
			query += ` AND IFNULL(file_path, '') = IFNULL(?, '') AND IFNULL(commit_hash, '') = IFNULL(?, '')`
			args = append(args, nullableString(fact.FilePath), nullableString(fact.CommitHash))
		}
		_, err = kg.db.Exec(query, args...)
		if err != nil {
			return "", fmt.Errorf("expire old singular fact: %w", err)
		}
	}

	id := uuid.New().String()
	_, err = kg.db.Exec(
		`INSERT INTO facts (id, subject, predicate, object, valid_from, valid_until, source, file_path, commit_hash, confidence, meta_json, created_at)
		 VALUES (?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?)`,
		id,
		fact.Subject,
		fact.Predicate,
		fact.Object,
		fact.ValidFrom,
		nullableString(fact.Source),
		nullableString(fact.FilePath),
		nullableString(fact.CommitHash),
		nullableFloat64(fact.Confidence),
		nullableString(fact.MetaJSON),
		now,
	)
	if err != nil {
		return "", fmt.Errorf("insert fact: %w", err)
	}
	return id, nil
}

// ExpireFactsByScope closes currently active facts for an exact file path and commit hash.
func (kg *KnowledgeGraph) ExpireFactsByScope(filePath, commitHash string) (int64, error) {
	if strings.TrimSpace(filePath) == "" {
		return 0, fmt.Errorf("file_path is required")
	}
	if strings.TrimSpace(commitHash) == "" {
		return 0, fmt.Errorf("commit_hash is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := kg.db.Exec(
		`UPDATE facts
		 SET valid_until=?
		 WHERE valid_until IS NULL
		   AND file_path=?
		   AND commit_hash=?`,
		now, filePath, commitHash,
	)
	if err != nil {
		return 0, fmt.Errorf("expire facts by scope: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return rows, nil
}

// ExpireActiveFactsByFile closes currently active facts for a file path.
func (kg *KnowledgeGraph) ExpireActiveFactsByFile(filePath string) (int64, error) {
	if strings.TrimSpace(filePath) == "" {
		return 0, fmt.Errorf("file_path is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := kg.db.Exec(
		`UPDATE facts
		 SET valid_until=?
		 WHERE valid_until IS NULL
		   AND file_path=?`,
		now, filePath,
	)
	if err != nil {
		return 0, fmt.Errorf("expire facts by file: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return rows, nil
}

// ExpireActiveFactsByPrefix closes active facts whose file_path starts with prefix.
func (kg *KnowledgeGraph) ExpireActiveFactsByPrefix(prefix string) (int64, error) {
	if strings.TrimSpace(prefix) == "" {
		return 0, fmt.Errorf("prefix is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := kg.db.Exec(
		`UPDATE facts
		 SET valid_until=?
		 WHERE valid_until IS NULL
		   AND file_path LIKE ?`,
		now, prefix+"%",
	)
	if err != nil {
		return 0, fmt.Errorf("expire facts by prefix: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return rows, nil
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
			`SELECT id, subject, predicate, object, valid_from, valid_until, source, file_path, commit_hash, confidence, meta_json, created_at
			 FROM facts
			 WHERE (subject=? OR object=?) AND valid_until IS NULL
			 ORDER BY created_at DESC`,
			entity, entity,
		)
	} else {
		rows, err = kg.db.Query(
			`SELECT id, subject, predicate, object, valid_from, valid_until, source, file_path, commit_hash, confidence, meta_json, created_at
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
		`SELECT id, subject, predicate, object, valid_from, valid_until, source, file_path, commit_hash, confidence, meta_json, created_at
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

// LatestCommitHash returns the commit_hash most recently inserted into active facts.
// Returns "working-tree" if no commit hash is available.
func (kg *KnowledgeGraph) LatestCommitHash() string {
	row := kg.db.QueryRow(
		`SELECT commit_hash
		 FROM facts
		 WHERE commit_hash IS NOT NULL AND commit_hash != '' AND valid_until IS NULL
		 ORDER BY created_at DESC
		 LIMIT 1`,
	)
	var h string
	if err := row.Scan(&h); err != nil || strings.TrimSpace(h) == "" {
		return "working-tree"
	}
	return h
}

// ArchitectureSummary returns top-level local packages and their depends_on edges.
func (kg *KnowledgeGraph) ArchitectureSummary(project string, maxPackages, maxDeps int) ([]PackageDependency, error) {
	if maxPackages <= 0 {
		maxPackages = 6
	}
	if maxDeps <= 0 {
		maxDeps = 8
	}
	rows, err := kg.db.Query(
		`SELECT subject, object
		 FROM facts
		 WHERE predicate=? AND valid_until IS NULL
		 ORDER BY subject ASC, object ASC`,
		PredicateDependsOn,
	)
	if err != nil {
		return nil, fmt.Errorf("architecture query: %w", err)
	}
	defer rows.Close()

	edges := map[string]map[string]struct{}{}
	for rows.Next() {
		var subject, object string
		if err := rows.Scan(&subject, &object); err != nil {
			return nil, fmt.Errorf("architecture scan: %w", err)
		}
		top := topLevelFromPackageID(subject, project)
		if top == "" {
			continue
		}
		if _, ok := edges[top]; !ok {
			edges[top] = map[string]struct{}{}
		}
		dep := compactDependencyLabel(object, project)
		if dep == "" || dep == top {
			continue
		}
		edges[top][dep] = struct{}{}
	}

	type scoreRow struct {
		pkg  string
		deps []string
	}
	var scored []scoreRow
	for pkg, depSet := range edges {
		deps := make([]string, 0, len(depSet))
		for d := range depSet {
			deps = append(deps, d)
		}
		sort.Strings(deps)
		if len(deps) > maxDeps {
			deps = deps[:maxDeps]
		}
		scored = append(scored, scoreRow{pkg: pkg, deps: deps})
	}
	sort.Slice(scored, func(i, j int) bool {
		if len(scored[i].deps) != len(scored[j].deps) {
			return len(scored[i].deps) > len(scored[j].deps)
		}
		return scored[i].pkg < scored[j].pkg
	})
	if len(scored) > maxPackages {
		scored = scored[:maxPackages]
	}

	out := make([]PackageDependency, 0, len(scored))
	for _, row := range scored {
		out = append(out, PackageDependency{
			Package:   row.pkg,
			DependsOn: row.deps,
		})
	}
	return out, nil
}

func scanFacts(rows *sql.Rows) ([]Fact, error) {
	var facts []Fact
	for rows.Next() {
		var f Fact
		var validUntil sql.NullString
		var validFrom sql.NullString
		var source sql.NullString
		var filePath sql.NullString
		var commitHash sql.NullString
		var confidence sql.NullFloat64
		var metaJSON sql.NullString
		if err := rows.Scan(
			&f.ID, &f.Subject, &f.Predicate, &f.Object,
			&validFrom, &validUntil, &source, &filePath, &commitHash, &confidence, &metaJSON, &f.CreatedAt,
		); err != nil {
			return nil, err
		}
		f.ValidFrom = validFrom.String
		f.ValidUntil = validUntil.String
		f.Source = source.String
		f.FilePath = filePath.String
		f.CommitHash = commitHash.String
		if confidence.Valid {
			f.Confidence = confidence.Float64
		}
		f.MetaJSON = metaJSON.String
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

func (kg *KnowledgeGraph) ensureFactColumn(name, sqlType string) error {
	rows, err := kg.db.Query(`PRAGMA table_info(facts)`)
	if err != nil {
		return fmt.Errorf("inspect facts schema: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var colName string
		var colType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan table_info: %w", err)
		}
		if colName == name {
			return nil
		}
	}
	if _, err := kg.db.Exec(fmt.Sprintf(`ALTER TABLE facts ADD COLUMN %s %s`, name, sqlType)); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}
	return nil
}

func nullableString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func nullableFloat64(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

func topLevelFromPackageID(pkgID, project string) string {
	pkgID = strings.TrimSpace(pkgID)
	if pkgID == "" {
		return ""
	}
	project = strings.TrimSpace(project)
	parts := strings.Split(pkgID, "/")
	if project != "" {
		for i := range parts {
			if parts[i] != project {
				continue
			}
			if i+1 >= len(parts) {
				return project
			}
			return parts[i+1]
		}
	}
	base := path.Base(pkgID)
	if base == "." || base == "/" {
		return ""
	}
	return base
}

func compactDependencyLabel(dep, project string) string {
	dep = strings.TrimSpace(dep)
	if dep == "" {
		return ""
	}
	top := topLevelFromPackageID(dep, project)
	if top != "" {
		return top
	}
	return dep
}
