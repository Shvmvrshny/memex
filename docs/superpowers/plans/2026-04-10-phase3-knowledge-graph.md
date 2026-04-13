# Phase 3: Knowledge Graph — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a temporal entity-relationship fact store backed by SQLite with WAL mode. Facts are subject→predicate→object triples with `valid_from`/`valid_until` validity windows. Closing a fact (invalidation) preserves history. A `singular` flag auto-closes the previous active fact for the same subject+predicate before inserting a new one. Expose 5 HTTP endpoints for UI access and wire the KG into the server.

**Architecture:** New file `internal/kg.go` holds the `KnowledgeGraph` type and all SQL. New file `internal/kg_handlers.go` holds the HTTP handlers. `internal/server.go` initialises `KnowledgeGraph` alongside `QdrantStore` and registers `/facts/*` routes. `go.mod` adds `modernc.org/sqlite` (pure Go, no CGO). Phase 2 must be complete (the `~/.memex` host mount exists).

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (pure Go SQLite driver), `database/sql`, `github.com/google/uuid`.

---

## File Map

| File | Change |
|---|---|
| `go.mod` / `go.sum` | Add `modernc.org/sqlite` |
| `internal/kg.go` | NEW: `KnowledgeGraph` type, schema, all SQL methods |
| `internal/kg_test.go` | NEW: unit tests for all KG methods |
| `internal/kg_handlers.go` | NEW: HTTP handlers for `/facts/*` routes |
| `internal/kg_handlers_test.go` | NEW: handler tests using in-memory SQLite |
| `internal/server.go` | Init KG on startup, register `/facts/*` routes |

---

### Task 1: Add `modernc.org/sqlite` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
go get modernc.org/sqlite
```

- [ ] **Step 2: Verify it was added**

```bash
grep "modernc.org/sqlite" go.mod
```

Expected: Line like `modernc.org/sqlite v1.x.x`

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add modernc.org/sqlite (pure Go SQLite driver)"
```

---

### Task 2: Create `internal/kg.go` — KnowledgeGraph type

**Files:**
- Create: `internal/kg.go`

- [ ] **Step 1: Write the failing test first**

Create `internal/kg_test.go`:

```go
package memex

import (
	"testing"
	"time"
)

func newTestKG(t *testing.T) *KnowledgeGraph {
	t.Helper()
	kg, err := NewKnowledgeGraph(":memory:")
	if err != nil {
		t.Fatalf("NewKnowledgeGraph: %v", err)
	}
	if err := kg.Init(); err != nil {
		t.Fatalf("kg.Init: %v", err)
	}
	t.Cleanup(func() { kg.db.Close() })
	return kg
}

func TestKG_RecordFact_Basic(t *testing.T) {
	kg := newTestKG(t)

	id, err := kg.RecordFact("alice", "works_on", "memex", "", "test", false)
	if err != nil {
		t.Fatalf("RecordFact: %v", err)
	}
	if id == "" {
		t.Error("RecordFact returned empty id")
	}
}

func TestKG_RecordFact_Idempotent(t *testing.T) {
	kg := newTestKG(t)

	id1, _ := kg.RecordFact("alice", "works_on", "memex", "", "test", false)
	id2, _ := kg.RecordFact("alice", "works_on", "memex", "", "test", false)

	if id1 != id2 {
		t.Errorf("identical active fact should return same id: got %q and %q", id1, id2)
	}
}

func TestKG_RecordFact_Singular_ClosesOldFact(t *testing.T) {
	kg := newTestKG(t)

	id1, _ := kg.RecordFact("alice", "works_on", "memex", "", "test", false)
	_, _ = kg.RecordFact("alice", "works_on", "palace", "", "test", true)

	// Old fact should now be closed
	facts, err := kg.QueryEntity("alice", "")
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	for _, f := range facts {
		if f.ID == id1 {
			t.Error("old fact with id1 should have been expired and not appear in current query")
		}
	}
	// New fact should be the only active one
	if len(facts) != 1 || facts[0].Object != "palace" {
		t.Errorf("expected 1 active fact with object=palace, got %+v", facts)
	}
}

func TestKG_QueryEntity_AsOf(t *testing.T) {
	kg := newTestKG(t)

	past := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)

	// Fact valid from 2h ago, closed 1h ago
	id1, _ := kg.RecordFact("bob", "role", "engineer", past, "test", false)
	kg.ExpireFact(id1, recent)

	// New fact from 1h ago, still active
	_, _ = kg.RecordFact("bob", "role", "lead", recent, "test", false)

	// As of "now" should return only "lead"
	current, err := kg.QueryEntity("bob", "")
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	if len(current) != 1 || current[0].Object != "lead" {
		t.Errorf("current facts: want [{role lead}], got %+v", current)
	}

	// As of the past (before closing) should return "engineer"
	historical, err := kg.QueryEntity("bob", past)
	if err != nil {
		t.Fatalf("QueryEntity as_of: %v", err)
	}
	found := false
	for _, f := range historical {
		if f.Object == "engineer" {
			found = true
		}
	}
	if !found {
		t.Errorf("historical query should include engineer fact, got %+v", historical)
	}
}

func TestKG_ExpireFact(t *testing.T) {
	kg := newTestKG(t)

	id, _ := kg.RecordFact("carol", "owns", "auth-service", "", "test", false)
	now := time.Now().UTC().Format(time.RFC3339)
	err := kg.ExpireFact(id, now)
	if err != nil {
		t.Fatalf("ExpireFact: %v", err)
	}

	facts, _ := kg.QueryEntity("carol", "")
	for _, f := range facts {
		if f.ID == id {
			t.Error("expired fact should not appear in current QueryEntity")
		}
	}
}

func TestKG_History(t *testing.T) {
	kg := newTestKG(t)

	id1, _ := kg.RecordFact("dan", "project", "alpha", "", "test", false)
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	kg.ExpireFact(id1, past)
	kg.RecordFact("dan", "project", "beta", past, "test", false)

	history, err := kg.History("dan")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) < 2 {
		t.Errorf("expected at least 2 history entries for dan, got %d", len(history))
	}
}

func TestKG_Stats(t *testing.T) {
	kg := newTestKG(t)

	kg.RecordFact("e1", "rel", "e2", "", "test", false)
	id, _ := kg.RecordFact("e1", "rel2", "e3", "", "test", false)
	now := time.Now().UTC().Format(time.RFC3339)
	kg.ExpireFact(id, now)

	stats, err := kg.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalFacts != 2 {
		t.Errorf("TotalFacts = %d, want 2", stats.TotalFacts)
	}
	if stats.ActiveFacts != 1 {
		t.Errorf("ActiveFacts = %d, want 1", stats.ActiveFacts)
	}
	if stats.ExpiredFacts != 1 {
		t.Errorf("ExpiredFacts = %d, want 1", stats.ExpiredFacts)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ -run TestKG_ -v 2>&1 | head -30
```

Expected: FAIL — `KnowledgeGraph` type not defined.

- [ ] **Step 3: Create `internal/kg.go`**

```go
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
	row.Scan(&stats.TotalFacts)

	row = kg.db.QueryRow(`SELECT COUNT(*) FROM facts WHERE valid_until IS NULL`)
	row.Scan(&stats.ActiveFacts)

	stats.ExpiredFacts = stats.TotalFacts - stats.ActiveFacts

	row = kg.db.QueryRow(`SELECT COUNT(DISTINCT subject) FROM facts`)
	row.Scan(&stats.EntityCount)

	rows, err := kg.db.Query(`SELECT predicate, COUNT(*) FROM facts GROUP BY predicate ORDER BY predicate`)
	if err == nil {
		defer rows.Close()
		stats.PredicateTypes = make(map[string]int)
		for rows.Next() {
			var p string
			var count int
			rows.Scan(&p, &count)
			stats.PredicateTypes[p] = count
		}
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ -run TestKG_ -v
```

Expected: PASS — all 7 KG tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/kg.go internal/kg_test.go
git commit -m "feat: add KnowledgeGraph (SQLite WAL temporal fact store)"
```

---

### Task 3: Create `internal/kg_handlers.go` — HTTP handlers

**Files:**
- Create: `internal/kg_handlers.go`
- Create: `internal/kg_handlers_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `internal/kg_handlers_test.go`:

```go
package memex

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestKGHandlers(t *testing.T) *KGHandlers {
	t.Helper()
	kg, err := NewKnowledgeGraph(":memory:")
	if err != nil {
		t.Fatalf("NewKnowledgeGraph: %v", err)
	}
	if err := kg.Init(); err != nil {
		t.Fatalf("kg.Init: %v", err)
	}
	t.Cleanup(func() { kg.db.Close() })
	return NewKGHandlers(kg)
}

func TestKGHandlers_RecordFact(t *testing.T) {
	h := newTestKGHandlers(t)

	body := `{"subject":"alice","predicate":"works_on","object":"memex","source":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/facts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.RecordFact(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["id"] == "" {
		t.Error("response missing id")
	}
}

func TestKGHandlers_RecordFact_MissingFields(t *testing.T) {
	h := newTestKGHandlers(t)

	body := `{"subject":"alice"}` // missing predicate and object
	req := httptest.NewRequest(http.MethodPost, "/facts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.RecordFact(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestKGHandlers_QueryEntity(t *testing.T) {
	h := newTestKGHandlers(t)

	// Seed a fact
	h.kg.RecordFact("alice", "works_on", "memex", "", "test", false)

	req := httptest.NewRequest(http.MethodGet, "/facts?subject=alice", nil)
	w := httptest.NewRecorder()

	h.QueryEntity(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Facts []Fact `json:"facts"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Facts) != 1 {
		t.Errorf("facts count = %d, want 1", len(resp.Facts))
	}
}

func TestKGHandlers_ExpireFact(t *testing.T) {
	h := newTestKGHandlers(t)

	id, _ := h.kg.RecordFact("bob", "role", "engineer", "", "test", false)

	req := httptest.NewRequest(http.MethodDelete, "/facts/"+id, nil)
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()

	h.ExpireFact(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestKGHandlers_History(t *testing.T) {
	h := newTestKGHandlers(t)

	h.kg.RecordFact("carol", "project", "alpha", "", "test", false)
	h.kg.RecordFact("carol", "project", "beta", "", "test", true)

	req := httptest.NewRequest(http.MethodGet, "/facts/timeline?entity=carol", nil)
	w := httptest.NewRecorder()

	h.History(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Facts []Fact `json:"facts"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Facts) < 2 {
		t.Errorf("history count = %d, want >= 2", len(resp.Facts))
	}
}

func TestKGHandlers_Stats(t *testing.T) {
	h := newTestKGHandlers(t)

	h.kg.RecordFact("x", "rel", "y", "", "test", false)

	req := httptest.NewRequest(http.MethodGet, "/facts/stats", nil)
	w := httptest.NewRecorder()

	h.Stats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var stats KGStats
	json.NewDecoder(w.Body).Decode(&stats)
	if stats.TotalFacts != 1 {
		t.Errorf("TotalFacts = %d, want 1", stats.TotalFacts)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ -run TestKGHandlers_ -v 2>&1 | head -20
```

Expected: FAIL — `KGHandlers` type not defined.

- [ ] **Step 3: Create `internal/kg_handlers.go`**

```go
package memex

import (
	"encoding/json"
	"net/http"
	"strings"
)

// KGHandlers holds HTTP handlers for the /facts/* routes.
type KGHandlers struct {
	kg *KnowledgeGraph
}

func NewKGHandlers(kg *KnowledgeGraph) *KGHandlers {
	return &KGHandlers{kg: kg}
}

// RecordFact handles POST /facts
// Body: {"subject","predicate","object","valid_from","source","singular"}
func (h *KGHandlers) RecordFact(w http.ResponseWriter, r *http.Request) {
	var req RecordFactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Subject == "" || req.Predicate == "" || req.Object == "" {
		http.Error(w, "subject, predicate, and object are required", http.StatusBadRequest)
		return
	}
	id, err := h.kg.RecordFact(req.Subject, req.Predicate, req.Object, req.ValidFrom, req.Source, req.Singular)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// QueryEntity handles GET /facts?subject=X&as_of=Y
func (h *KGHandlers) QueryEntity(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("subject")
	if entity == "" {
		entity = r.URL.Query().Get("entity")
	}
	asOf := r.URL.Query().Get("as_of")

	facts, err := h.kg.QueryEntity(entity, asOf)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if facts == nil {
		facts = []Fact{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"facts": facts})
}

// ExpireFact handles DELETE /facts/:id
func (h *KGHandlers) ExpireFact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		// Fallback: parse from URL path for older Go versions
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/facts/"), "/")
		if len(parts) > 0 {
			id = parts[0]
		}
	}
	if id == "" {
		http.Error(w, "fact id required", http.StatusBadRequest)
		return
	}
	if err := h.kg.ExpireFact(id, ""); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "expired"})
}

// History handles GET /facts/timeline?entity=X
func (h *KGHandlers) History(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("entity")
	facts, err := h.kg.History(entity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if facts == nil {
		facts = []Fact{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"facts": facts})
}

// Stats handles GET /facts/stats
func (h *KGHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.kg.Stats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ -run TestKGHandlers_ -v
```

Expected: PASS — all 6 handler tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/kg_handlers.go internal/kg_handlers_test.go
git commit -m "feat: add KGHandlers HTTP handlers for /facts/* routes"
```

---

### Task 4: Wire KG into `internal/server.go`

**Files:**
- Modify: `internal/server.go`

The server needs to initialise `KnowledgeGraph` on startup and register the `/facts/*` routes.

- [ ] **Step 1: Update `internal/server.go`**

Replace the full `RunServe` function:

```go
func RunServe() {
	cfg := LoadConfig()
	store := NewQdrantStore(cfg.QdrantURL, cfg.OllamaURL)
	traceStore := NewTraceStore(cfg.QdrantURL)

	kg, err := NewKnowledgeGraph(cfg.KGPath)
	if err != nil {
		log.Fatalf("init knowledge graph: %v", err)
	}

	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		log.Fatalf("init memory store: %v", err)
	}
	if err := traceStore.Init(ctx); err != nil {
		log.Fatalf("init trace store: %v", err)
	}
	if err := kg.Init(); err != nil {
		log.Fatalf("init knowledge graph schema: %v", err)
	}

	h := NewHandlers(store)
	th := NewTraceHandlers(store, traceStore)
	kgh := NewKGHandlers(kg)

	mux := http.NewServeMux()

	// Memory routes
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/memories/pinned", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.PinnedMemories(w, r)
	})
	mux.HandleFunc("/memories/similar", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.FindSimilar(w, r)
	})
	mux.HandleFunc("/memories/", func(w http.ResponseWriter, r *http.Request) {
		// /memories/:id/pin
		if strings.HasSuffix(r.URL.Path, "/pin") && r.Method == http.MethodPost {
			h.PinMemory(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			h.DeleteMemory(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/memories", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.SearchMemories(w, r)
		case http.MethodPost:
			h.SaveMemory(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/summarize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.Summarize(w, r)
	})

	// Knowledge Graph routes
	mux.HandleFunc("/facts/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		kgh.Stats(w, r)
	})
	mux.HandleFunc("/facts/timeline", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		kgh.History(w, r)
	})
	mux.HandleFunc("/facts/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			kgh.ExpireFact(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/facts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			kgh.RecordFact(w, r)
		case http.MethodGet:
			kgh.QueryEntity(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Trace routes
	mux.HandleFunc("/trace/event", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.TraceEvent(w, r)
	})
	mux.HandleFunc("/trace/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.TraceStop(w, r)
	})
	mux.HandleFunc("/trace/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.ListSessions(w, r)
	})
	mux.HandleFunc("/trace/session/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.GetSession(w, r)
	})
	mux.HandleFunc("/trace/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.ListProjects(w, r)
	})
	mux.HandleFunc("/checkpoint", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.Checkpoint(w, r)
	})

	// Serve UI static files
	mux.HandleFunc("/ui/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/ui")
		if path == "" || path == "/" {
			path = "/index.html"
		}
		http.ServeFile(w, r, "ui/dist"+path)
	})

	addr := ":" + cfg.Port
	log.Printf("memex listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: No errors.

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -v 2>&1 | tail -30
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/server.go
git commit -m "feat: wire KnowledgeGraph into server, register /facts/* routes"
```

---

### Task 5: Final verification

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -count=1 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: All packages show `ok`.

- [ ] **Step 2: Build and check binary**

```bash
go build -o /tmp/memex-phase3 ./cmd/memex/
/tmp/memex-phase3 --help 2>&1 | head -5
```

Expected: Binary builds without error.

- [ ] **Step 3: Commit phase marker**

```bash
git commit --allow-empty -m "feat: Phase 3 complete — knowledge graph (SQLite WAL temporal facts)"
```
