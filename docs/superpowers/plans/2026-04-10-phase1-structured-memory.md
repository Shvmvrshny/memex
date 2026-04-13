# Phase 1: Structured Memory — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `memory_type` (9 types) and `topic` fields to the Memory model, update Qdrant schema with keyword indexes, and add pinned/similar/mine endpoints so the backend supports structured retrieval.

**Architecture:** Clean schema approach — simplify Init to just create collection + indexes (no migration needed given minimal data). Add `buildFilter` helper to centralise Qdrant filter construction. New Store interface methods: `PinnedMemories`, `PinMemory`, `FindSimilar`. New HTTP endpoints registered in server.go.

**Tech Stack:** Go 1.26, Qdrant REST API, existing `net/http` test patterns.

---

## File Map

| File | Change |
|---|---|
| `internal/models.go` | Add `MemoryType`, `Topic`, `ValidMemoryTypes` to Memory/Request; add `Fact`, `RecordFactRequest`, `KGStats`, `ConversationTurn`, `MineRequest`, `MineResponse` |
| `internal/store.go` | New interface signatures for Search/List; add `PinnedMemories`, `PinMemory`, `FindSimilar` |
| `internal/qdrant.go` | `buildFilter` helper; simplified `Init`/`createCollection`; updated `SaveMemory`, `pointsToMemories`, `SearchMemories`, `ListMemories`; new `PinnedMemories`, `PinMemory`, `FindSimilar`, `post` |
| `internal/handlers.go` | Validate `memory_type` in `SaveMemory`; add `memory_type`/`topic` params to `SearchMemories`; update `Summarize`; add `PinnedMemories`, `PinMemory`, `FindSimilar` handlers |
| `internal/tracer_handlers.go` | `Checkpoint` sets `MemoryType: "event"`, `Topic: "checkpoint"` |
| `internal/server.go` | Register new routes under `/memories/` |
| `internal/handlers_test.go` | Update `mockStore` to new interface; update existing tests; add new tests |
| `internal/models_test.go` | Add `MemoryType` and `Topic` to round-trip test |
| `internal/qdrant_test.go` | Fix `NewQdrantStore` calls (2 args); update `SearchMemories` call signature |

---

### Task 1: Update `internal/models.go`

**Files:**
- Modify: `internal/models.go`

- [ ] **Step 1: Write the new models.go**

```go
package memex

import "time"

// ValidMemoryTypes is the canonical set of 9 memory types.
var ValidMemoryTypes = map[string]bool{
	"decision":  true,
	"preference": true,
	"event":     true,
	"discovery": true,
	"advice":    true,
	"problem":   true,
	"context":   true,
	"procedure": true,
	"rationale": true,
}

type Memory struct {
	ID           string    `json:"id"`
	Text         string    `json:"text"`
	Project      string    `json:"project"`
	Topic        string    `json:"topic"`
	MemoryType   string    `json:"memory_type"`
	Source       string    `json:"source"`
	Timestamp    time.Time `json:"timestamp"`
	Importance   float32   `json:"importance"`
	Tags         []string  `json:"tags"`
	LastAccessed time.Time `json:"last_accessed"`
}

type SaveMemoryRequest struct {
	Text       string   `json:"text"`
	Project    string   `json:"project"`
	Topic      string   `json:"topic"`
	MemoryType string   `json:"memory_type"`
	Source     string   `json:"source"`
	Importance float32  `json:"importance"`
	Tags       []string `json:"tags"`
}

type SearchResponse struct {
	Memories []Memory `json:"memories"`
}

// Fact is a temporal entity-relationship triple stored in the knowledge graph.
type Fact struct {
	ID         string `json:"id"`
	Subject    string `json:"subject"`
	Predicate  string `json:"predicate"`
	Object     string `json:"object"`
	ValidFrom  string `json:"valid_from,omitempty"`
	ValidUntil string `json:"valid_until,omitempty"`
	Source     string `json:"source,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// RecordFactRequest is the body of POST /facts.
type RecordFactRequest struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
	ValidFrom string `json:"valid_from,omitempty"`
	Source    string `json:"source,omitempty"`
	Singular  bool   `json:"singular"`
}

// KGStats is returned by GET /facts/stats.
type KGStats struct {
	TotalFacts     int            `json:"total_facts"`
	ActiveFacts    int            `json:"active_facts"`
	ExpiredFacts   int            `json:"expired_facts"`
	EntityCount    int            `json:"entity_count"`
	PredicateTypes map[string]int `json:"predicate_types"`
}

// ConversationTurn is one full turn from a Claude Code JSONL transcript.
type ConversationTurn struct {
	Role string // "user" or "assistant"
	Text string
}

// MineRequest is the body of POST /mine/transcript.
type MineRequest struct {
	Path    string `json:"path"`
	Project string `json:"project"`
}

// MineResponse is returned by POST /mine/transcript.
type MineResponse struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go build ./...
```

Expected: compile errors in files that use the old interface (store.go, qdrant.go, handlers.go, handlers_test.go). That is correct — we fix them in subsequent tasks.

---

### Task 2: Update `internal/store.go`

**Files:**
- Modify: `internal/store.go`

- [ ] **Step 1: Write the new store.go**

```go
package memex

import "context"

type Store interface {
	Init(ctx context.Context) error
	SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error)

	// SearchMemories performs semantic search. memoryType and topic are optional ("" = no filter).
	SearchMemories(ctx context.Context, query, project, memoryType, topic string, limit int) ([]Memory, error)

	// ListMemories lists memories by recency+importance score. memoryType and topic are optional.
	ListMemories(ctx context.Context, project, memoryType, topic string, limit int) ([]Memory, error)

	// PinnedMemories returns memories with importance >= 0.9 for the project, sorted desc.
	PinnedMemories(ctx context.Context, project string) ([]Memory, error)

	// PinMemory sets importance = 1.0 on a memory by ID.
	PinMemory(ctx context.Context, id string) error

	// FindSimilar embeds text and returns the most similar memories for duplicate detection.
	FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error)

	DeleteMemory(ctx context.Context, id string) error
	Health(ctx context.Context) error
}
```

- [ ] **Step 2: Verify it compiles (errors expected)**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go build ./...
```

Expected: same compile errors as before — qdrant.go doesn't implement the new interface yet.

---

### Task 3: Update `internal/qdrant.go`

**Files:**
- Modify: `internal/qdrant.go`

- [ ] **Step 1: Write the failing test first** (in `internal/qdrant_test.go`)

Replace the entire file:

```go
package memex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockEmbeddingHandler returns a handler that serves Ollama embeddings (768 zeros)
// and delegates Qdrant calls to qdrantHandler.
func mockEmbeddingHandler(qdrantHandler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embeddings" {
			json.NewEncoder(w).Encode(map[string]any{
				"embedding": make([]float32, 768),
			})
			return
		}
		qdrantHandler(w, r)
	}
}

func TestQdrantHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	store := NewQdrantStore(srv.URL, "")
	if err := store.Health(context.Background()); err != nil {
		t.Fatalf("expected healthy, got: %v", err)
	}
}

func TestQdrantHealth_Down(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	store := NewQdrantStore(srv.URL, "")
	if err := store.Health(context.Background()); err == nil {
		t.Fatal("expected error when qdrant is down")
	}
}

func TestQdrantInit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	store := NewQdrantStore(srv.URL, "")
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

func TestQdrantSaveMemory(t *testing.T) {
	srv := httptest.NewServer(mockEmbeddingHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collections/memories/points" && r.Method == http.MethodPut {
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	store := NewQdrantStore(srv.URL, srv.URL)
	req := SaveMemoryRequest{
		Text:       "user prefers table-driven tests",
		Project:    "memex",
		MemoryType: "preference",
		Topic:      "testing",
		Source:     "claude-code",
		Importance: 0.8,
		Tags:       []string{"testing"},
	}
	mem, err := store.SaveMemory(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if mem.ID == "" {
		t.Error("expected non-empty ID")
	}
	if mem.Text != req.Text {
		t.Errorf("got Text %q, want %q", mem.Text, req.Text)
	}
	if mem.MemoryType != "preference" {
		t.Errorf("got MemoryType %q, want preference", mem.MemoryType)
	}
	if mem.Topic != "testing" {
		t.Errorf("got Topic %q, want testing", mem.Topic)
	}
}

func TestQdrantSearchMemories_FallbackToList(t *testing.T) {
	// embed fails (server returns 404 for /api/embeddings) → fallback to ListMemories
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collections/memories/points/scroll" {
			resp := map[string]any{
				"result": map[string]any{
					"points": []map[string]any{{
						"id": "test-id",
						"payload": map[string]any{
							"text":          "user prefers Python",
							"project":       "memex",
							"memory_type":   "preference",
							"topic":         "language",
							"source":        "claude-code",
							"importance":    0.8,
							"tags":          []any{"preference"},
							"timestamp":     "2026-04-06T10:00:00Z",
							"last_accessed": "2026-04-06T10:00:00Z",
						},
					}},
				},
				"status": "ok",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	store := NewQdrantStore(srv.URL, srv.URL)
	memories, err := store.SearchMemories(context.Background(), "python language", "", "", "", 5)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("got %d memories, want 1", len(memories))
	}
	if memories[0].MemoryType != "preference" {
		t.Errorf("got MemoryType %q, want preference", memories[0].MemoryType)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go test ./internal/... -run TestQdrant -v 2>&1 | head -40
```

Expected: compile failure — `NewQdrantStore` still takes 2 args (OK), but `SearchMemories` signature mismatch.

- [ ] **Step 3: Rewrite `internal/qdrant.go`**

```go
package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type QdrantStore struct {
	baseURL   string
	ollamaURL string
	client    *http.Client
}

func NewQdrantStore(baseURL, ollamaURL string) *QdrantStore {
	return &QdrantStore{
		baseURL:   baseURL,
		ollamaURL: ollamaURL,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (q *QdrantStore) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("health request: %w", err)
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qdrant unhealthy: status %d", resp.StatusCode)
	}
	return nil
}

// Init creates the memories collection and all payload indexes.
// Safe to call multiple times — existing collection and indexes are preserved.
func (q *QdrantStore) Init(ctx context.Context) error {
	return q.createCollection(ctx)
}

func (q *QdrantStore) createCollection(ctx context.Context) error {
	if err := q.put(ctx, "/collections/memories", map[string]interface{}{
		"vectors": map[string]interface{}{"size": 768, "distance": "Cosine"},
	}); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create collection: %w", err)
		}
	}

	indexes := []struct{ field, schema string }{
		{"text", "text"},
		{"project", "keyword"},
		{"memory_type", "keyword"},
		{"topic", "keyword"},
	}
	for _, idx := range indexes {
		if err := q.put(ctx, "/collections/memories/index", map[string]interface{}{
			"field_name":   idx.field,
			"field_schema": idx.schema,
		}); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("create %s index: %w", idx.field, err)
			}
		}
	}
	log.Printf("memex: memories collection ready")
	return nil
}

// buildFilter constructs a Qdrant filter from optional project/memoryType/topic values.
// Returns nil when all are empty (no filter).
func buildFilter(project, memoryType, topic string) map[string]any {
	var conditions []map[string]any
	if project != "" {
		conditions = append(conditions, map[string]any{
			"key": "project", "match": map[string]any{"value": project},
		})
	}
	if memoryType != "" {
		conditions = append(conditions, map[string]any{
			"key": "memory_type", "match": map[string]any{"value": memoryType},
		})
	}
	if topic != "" {
		conditions = append(conditions, map[string]any{
			"key": "topic", "match": map[string]any{"value": topic},
		})
	}
	if len(conditions) == 0 {
		return nil
	}
	return map[string]any{"must": conditions}
}

func (q *QdrantStore) SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if req.Source == "" {
		req.Source = "claude-code"
	}
	if req.Importance == 0 {
		req.Importance = 0.5
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	if req.Topic == "" {
		req.Topic = req.Project
	}

	vector, err := q.embed(ctx, req.Text)
	if err != nil {
		return Memory{}, fmt.Errorf("embed memory: %w", err)
	}

	body := map[string]any{
		"points": []map[string]any{{
			"id":     id,
			"vector": vector,
			"payload": map[string]any{
				"text":          req.Text,
				"project":       req.Project,
				"topic":         req.Topic,
				"memory_type":   req.MemoryType,
				"source":        req.Source,
				"timestamp":     now.Format(time.RFC3339),
				"importance":    req.Importance,
				"tags":          req.Tags,
				"last_accessed": now.Format(time.RFC3339),
			},
		}},
	}

	if err := q.put(ctx, "/collections/memories/points", body); err != nil {
		return Memory{}, fmt.Errorf("upsert point: %w", err)
	}

	return Memory{
		ID:           id,
		Text:         req.Text,
		Project:      req.Project,
		Topic:        req.Topic,
		MemoryType:   req.MemoryType,
		Source:       req.Source,
		Timestamp:    now,
		Importance:   req.Importance,
		Tags:         req.Tags,
		LastAccessed: now,
	}, nil
}

func (q *QdrantStore) SearchMemories(ctx context.Context, query, project, memoryType, topic string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}

	vector, err := q.embed(ctx, query)
	if err != nil {
		return q.ListMemories(ctx, project, memoryType, topic, limit)
	}

	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if f := buildFilter(project, memoryType, topic); f != nil {
		body["filter"] = f
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.baseURL+"/collections/memories/points/search", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result []struct {
			ID      string         `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	points := make([]struct {
		ID      string         `json:"id"`
		Payload map[string]any `json:"payload"`
	}, len(result.Result))
	for i, r := range result.Result {
		points[i].ID = r.ID
		points[i].Payload = r.Payload
	}
	return pointsToMemories(points), nil
}

func (q *QdrantStore) ListMemories(ctx context.Context, project, memoryType, topic string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := limit * 10
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	body := map[string]any{
		"limit":        fetchLimit,
		"with_payload": true,
		"with_vector":  false,
	}
	if f := buildFilter(project, memoryType, topic); f != nil {
		body["filter"] = f
	}

	memories, err := q.scroll(ctx, body)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	score := func(m Memory) float64 {
		days := now.Sub(m.Timestamp).Hours() / 24
		recency := 1.0 / (1.0 + days/30.0)
		return 0.6*float64(m.Importance) + 0.4*recency
	}
	sort.Slice(memories, func(i, j int) bool {
		return score(memories[i]) > score(memories[j])
	})
	if len(memories) > limit {
		memories = memories[:limit]
	}
	return memories, nil
}

func (q *QdrantStore) PinnedMemories(ctx context.Context, project string) ([]Memory, error) {
	body := map[string]any{
		"limit":        10,
		"with_payload": true,
		"with_vector":  false,
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": "project", "match": map[string]any{"value": project}},
				{"key": "importance", "range": map[string]any{"gte": 0.9}},
			},
		},
	}
	memories, err := q.scroll(ctx, body)
	if err != nil {
		return nil, err
	}
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Importance > memories[j].Importance
	})
	return memories, nil
}

func (q *QdrantStore) PinMemory(ctx context.Context, id string) error {
	body := map[string]any{
		"payload": map[string]any{"importance": float64(1.0)},
		"points":  []string{id},
	}
	return q.post(ctx, "/collections/memories/points/payload", body)
}

func (q *QdrantStore) FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}
	vector, err := q.embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("embed for similarity: %w", err)
	}

	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if project != "" {
		body["filter"] = map[string]any{
			"must": []map[string]any{
				{"key": "project", "match": map[string]any{"value": project}},
			},
		}
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.baseURL+"/collections/memories/points/search", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result []struct {
			ID      string         `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode find-similar response: %w", err)
	}

	points := make([]struct {
		ID      string         `json:"id"`
		Payload map[string]any `json:"payload"`
	}, len(result.Result))
	for i, r := range result.Result {
		points[i].ID = r.ID
		points[i].Payload = r.Payload
	}
	return pointsToMemories(points), nil
}

func (q *QdrantStore) DeleteMemory(ctx context.Context, id string) error {
	body := map[string]any{"points": []string{id}}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.baseURL+"/collections/memories/points/delete", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (q *QdrantStore) embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]string{
		"model":  "nomic-embed-text",
		"prompt": text,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.ollamaURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding from ollama")
	}
	return result.Embedding, nil
}

type qdrantScrollResponse struct {
	Result struct {
		Points []struct {
			ID      string         `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"points"`
	} `json:"result"`
	Status string `json:"status"`
}

func (q *QdrantStore) scroll(ctx context.Context, body map[string]any) ([]Memory, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.baseURL+"/collections/memories/points/scroll", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result qdrantScrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode scroll response: %w", err)
	}
	return pointsToMemories(result.Result.Points), nil
}

func pointsToMemories(points []struct {
	ID      string         `json:"id"`
	Payload map[string]any `json:"payload"`
}) []Memory {
	memories := make([]Memory, 0, len(points))
	for _, p := range points {
		m := Memory{ID: p.ID}
		if v, ok := p.Payload["text"].(string); ok {
			m.Text = v
		}
		if v, ok := p.Payload["project"].(string); ok {
			m.Project = v
		}
		if v, ok := p.Payload["topic"].(string); ok {
			m.Topic = v
		}
		if v, ok := p.Payload["memory_type"].(string); ok {
			m.MemoryType = v
		}
		if v, ok := p.Payload["source"].(string); ok {
			m.Source = v
		}
		if v, ok := p.Payload["importance"].(float64); ok {
			m.Importance = float32(v)
		}
		if v, ok := p.Payload["timestamp"].(string); ok {
			m.Timestamp, _ = time.Parse(time.RFC3339, v)
		}
		if v, ok := p.Payload["last_accessed"].(string); ok {
			m.LastAccessed, _ = time.Parse(time.RFC3339, v)
		}
		if v, ok := p.Payload["tags"].([]any); ok {
			for _, t := range v {
				if s, ok := t.(string); ok {
					m.Tags = append(m.Tags, s)
				}
			}
		}
		memories = append(memories, m)
	}
	return memories
}

func (q *QdrantStore) put(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, q.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (q *QdrantStore) post(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists")
}
```

- [ ] **Step 4: Run qdrant tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go test ./internal/... -run TestQdrant -v
```

Expected: `TestQdrantHealth_OK PASS`, `TestQdrantHealth_Down PASS`, `TestQdrantInit PASS`, `TestQdrantSaveMemory PASS`, `TestQdrantSearchMemories_FallbackToList PASS`.

---

### Task 4: Update `internal/handlers.go`

**Files:**
- Modify: `internal/handlers.go`

- [ ] **Step 1: Write the failing handler tests** (replace `internal/handlers_test.go`)

```go
package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockStore struct {
	memories []Memory
	err      error
}

func (m *mockStore) Init(ctx context.Context) error   { return m.err }
func (m *mockStore) Health(ctx context.Context) error { return m.err }

func (m *mockStore) SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error) {
	if m.err != nil {
		return Memory{}, m.err
	}
	mem := Memory{
		ID:           "test-id",
		Text:         req.Text,
		Project:      req.Project,
		Topic:        req.Topic,
		MemoryType:   req.MemoryType,
		Source:       "claude-code",
		Timestamp:    time.Now(),
		Importance:   req.Importance,
		Tags:         req.Tags,
		LastAccessed: time.Now(),
	}
	m.memories = append(m.memories, mem)
	return mem, nil
}

func (m *mockStore) SearchMemories(ctx context.Context, query, project, memoryType, topic string, limit int) ([]Memory, error) {
	return m.memories, m.err
}

func (m *mockStore) ListMemories(ctx context.Context, project, memoryType, topic string, limit int) ([]Memory, error) {
	return m.memories, m.err
}

func (m *mockStore) PinnedMemories(ctx context.Context, project string) ([]Memory, error) {
	return m.memories, m.err
}

func (m *mockStore) PinMemory(ctx context.Context, id string) error { return m.err }

func (m *mockStore) FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error) {
	return m.memories, m.err
}

func (m *mockStore) DeleteMemory(ctx context.Context, id string) error { return m.err }

// ── Health ──────────────────────────────────────────────────────────────────

func TestHealthHandler_OK(t *testing.T) {
	h := NewHandlers(&mockStore{})
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHealthHandler_Down(t *testing.T) {
	h := NewHandlers(&mockStore{err: errors.New("qdrant down")})
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// ── SaveMemory ───────────────────────────────────────────────────────────────

func TestSaveMemoryHandler(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body, _ := json.Marshal(SaveMemoryRequest{
		Text:       "user prefers Go",
		Project:    "memex",
		MemoryType: "preference",
	})
	r := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SaveMemory(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d, want %d", w.Code, http.StatusCreated)
	}
	var mem Memory
	json.NewDecoder(w.Body).Decode(&mem)
	if mem.Text != "user prefers Go" {
		t.Errorf("got Text %q, want 'user prefers Go'", mem.Text)
	}
	if mem.MemoryType != "preference" {
		t.Errorf("got MemoryType %q, want preference", mem.MemoryType)
	}
}

func TestSaveMemoryHandler_MissingText(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body, _ := json.Marshal(SaveMemoryRequest{Project: "memex", MemoryType: "preference"})
	r := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SaveMemory(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSaveMemoryHandler_MissingMemoryType(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body, _ := json.Marshal(SaveMemoryRequest{Text: "user prefers Go", Project: "memex"})
	r := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SaveMemory(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSaveMemoryHandler_InvalidMemoryType(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body, _ := json.Marshal(SaveMemoryRequest{Text: "x", Project: "memex", MemoryType: "invalid_type"})
	r := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SaveMemory(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ── SearchMemories ───────────────────────────────────────────────────────────

func TestSearchMemoriesHandler(t *testing.T) {
	store := &mockStore{memories: []Memory{{ID: "1", Text: "user prefers Python", MemoryType: "preference"}}}
	h := NewHandlers(store)
	r := httptest.NewRequest(http.MethodGet, "/memories?context=python&limit=5", nil)
	w := httptest.NewRecorder()
	h.SearchMemories(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
	var resp SearchResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Memories) != 1 {
		t.Errorf("got %d memories, want 1", len(resp.Memories))
	}
}

func TestSearchMemoriesHandler_TypeFilter(t *testing.T) {
	store := &mockStore{memories: []Memory{{ID: "1", Text: "use Postgres", MemoryType: "decision"}}}
	h := NewHandlers(store)
	r := httptest.NewRequest(http.MethodGet, "/memories?memory_type=decision&project=memex", nil)
	w := httptest.NewRecorder()
	h.SearchMemories(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

// ── Summarize ────────────────────────────────────────────────────────────────

func TestSummarizeHandler(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body, _ := json.Marshal(SaveMemoryRequest{
		Text:    "session: worked on memex Go service",
		Project: "memex",
	})
	r := httptest.NewRequest(http.MethodPost, "/summarize", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Summarize(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d, want %d", w.Code, http.StatusCreated)
	}
	var mem Memory
	json.NewDecoder(w.Body).Decode(&mem)
	if mem.MemoryType != "event" {
		t.Errorf("Summarize should set MemoryType=event, got %q", mem.MemoryType)
	}
}

// ── PinnedMemories ───────────────────────────────────────────────────────────

func TestPinnedMemoriesHandler(t *testing.T) {
	pinned := Memory{ID: "pin-1", Text: "critical preference", MemoryType: "preference", Importance: 1.0}
	store := &mockStore{memories: []Memory{pinned}}
	h := NewHandlers(store)
	r := httptest.NewRequest(http.MethodGet, "/memories/pinned?project=memex", nil)
	w := httptest.NewRecorder()
	h.PinnedMemories(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
	var resp SearchResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Memories) != 1 {
		t.Fatalf("got %d memories, want 1", len(resp.Memories))
	}
	if resp.Memories[0].ID != "pin-1" {
		t.Errorf("got ID %q, want pin-1", resp.Memories[0].ID)
	}
}

func TestPinnedMemoriesHandler_MissingProject(t *testing.T) {
	h := NewHandlers(&mockStore{})
	r := httptest.NewRequest(http.MethodGet, "/memories/pinned", nil)
	w := httptest.NewRecorder()
	h.PinnedMemories(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ── FindSimilar ──────────────────────────────────────────────────────────────

func TestFindSimilarHandler(t *testing.T) {
	store := &mockStore{memories: []Memory{{ID: "1", Text: "user prefers Python", MemoryType: "preference"}}}
	h := NewHandlers(store)
	r := httptest.NewRequest(http.MethodGet, "/memories/similar?text=python+preference", nil)
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestFindSimilarHandler_MissingText(t *testing.T) {
	h := NewHandlers(&mockStore{})
	r := httptest.NewRequest(http.MethodGet, "/memories/similar", nil)
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go test ./internal/... -run "TestHealth|TestSave|TestSearch|TestSummarize|TestPinned|TestFindSimilar" -v 2>&1 | tail -20
```

Expected: compile failure — `PinnedMemories`, `FindSimilar` not yet defined on `Handlers`.

- [ ] **Step 3: Rewrite `internal/handlers.go`**

```go
package memex

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type Handlers struct {
	store Store
}

func NewHandlers(store Store) *Handlers {
	return &Handlers{store: store}
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Health(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "qdrant unavailable"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handlers) SaveMemory(w http.ResponseWriter, r *http.Request) {
	var req SaveMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}
	if !ValidMemoryTypes[req.MemoryType] {
		http.Error(w, `{"error":"memory_type is required and must be one of: decision, preference, event, discovery, advice, problem, context, procedure, rationale"}`, http.StatusBadRequest)
		return
	}

	memory, err := h.store.SaveMemory(r.Context(), req)
	if err != nil {
		http.Error(w, `{"error":"failed to save memory"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(memory)
}

func (h *Handlers) SearchMemories(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("context")
	project := r.URL.Query().Get("project")
	memoryType := r.URL.Query().Get("memory_type")
	topic := r.URL.Query().Get("topic")
	limit := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	var (
		memories []Memory
		err      error
	)
	if query == "" {
		memories, err = h.store.ListMemories(r.Context(), project, memoryType, topic, limit)
	} else {
		memories, err = h.store.SearchMemories(r.Context(), query, project, memoryType, topic, limit)
	}
	if err != nil {
		http.Error(w, `{"error":"search failed"}`, http.StatusInternalServerError)
		return
	}
	if memories == nil {
		memories = []Memory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Memories: memories})
}

func (h *Handlers) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/memories/")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteMemory(r.Context(), id); err != nil {
		http.Error(w, `{"error":"failed to delete memory"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Summarize(w http.ResponseWriter, r *http.Request) {
	var req SaveMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}

	req.Importance = 0.9
	req.MemoryType = "event"
	req.Topic = "session-summary"
	if req.Tags == nil {
		req.Tags = []string{"session-summary"}
	}

	memory, err := h.store.SaveMemory(r.Context(), req)
	if err != nil {
		http.Error(w, `{"error":"failed to save summary"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(memory)
}

// PinnedMemories returns memories with importance >= 0.9 for the project.
// GET /memories/pinned?project=X
func (h *Handlers) PinnedMemories(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		http.Error(w, `{"error":"project is required"}`, http.StatusBadRequest)
		return
	}
	memories, err := h.store.PinnedMemories(r.Context(), project)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch pinned memories"}`, http.StatusInternalServerError)
		return
	}
	if memories == nil {
		memories = []Memory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Memories: memories})
}

// PinMemory sets importance = 1.0 on a memory.
// PATCH /memories/:id/pin
func (h *Handlers) PinMemory(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/memories/")
	id := strings.TrimSuffix(path, "/pin")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.PinMemory(r.Context(), id); err != nil {
		http.Error(w, `{"error":"failed to pin memory"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// FindSimilar returns the most similar memories to the given text.
// GET /memories/similar?text=X&project=Y&limit=5
func (h *Handlers) FindSimilar(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}
	project := r.URL.Query().Get("project")
	limit := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	memories, err := h.store.FindSimilar(r.Context(), text, project, limit)
	if err != nil {
		http.Error(w, `{"error":"similarity search failed"}`, http.StatusInternalServerError)
		return
	}
	if memories == nil {
		memories = []Memory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Memories: memories})
}
```

- [ ] **Step 4: Run handler tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go test ./internal/... -run "TestHealth|TestSave|TestSearch|TestSummarize|TestPinned|TestFindSimilar" -v
```

Expected: all PASS.

---

### Task 5: Update `internal/tracer_handlers.go` — Checkpoint

**Files:**
- Modify: `internal/tracer_handlers.go` (Checkpoint method only)

- [ ] **Step 1: Write failing test** (add to `internal/tracer_handlers_test.go`)

First read the existing file, then append this test:

```go
func TestCheckpointSetsEventType(t *testing.T) {
	store := &mockStore{}
	th := NewTraceHandlers(store, &mockTraceStore{})
	body, _ := json.Marshal(CheckpointRequest{
		Project: "memex",
		Summary: "finished implementing auth",
	})
	r := httptest.NewRequest(http.MethodPost, "/checkpoint", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	th.Checkpoint(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d, want %d", w.Code, http.StatusCreated)
	}
	if len(store.memories) != 1 {
		t.Fatalf("expected 1 memory saved, got %d", len(store.memories))
	}
	saved := store.memories[0]
	if saved.MemoryType != "event" {
		t.Errorf("Checkpoint should save as MemoryType=event, got %q", saved.MemoryType)
	}
	if saved.Topic != "checkpoint" {
		t.Errorf("Checkpoint should save with Topic=checkpoint, got %q", saved.Topic)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go test ./internal/... -run TestCheckpointSetsEventType -v
```

Expected: FAIL — memory_type is "" not "event".

- [ ] **Step 3: Update the `Checkpoint` method in `internal/tracer_handlers.go`**

Replace the `SaveMemory` call inside `Checkpoint`:

```go
func (h *TraceHandlers) Checkpoint(w http.ResponseWriter, r *http.Request) {
	var req CheckpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Project == "" || req.Summary == "" {
		http.Error(w, `{"error":"project and summary are required"}`, http.StatusBadRequest)
		return
	}
	mem, err := h.store.SaveMemory(r.Context(), SaveMemoryRequest{
		Text:       req.Summary,
		Project:    req.Project,
		Topic:      "checkpoint",
		MemoryType: "event",
		Source:     "claude-code",
		Importance: 0.9,
		Tags:       []string{"checkpoint"},
	})
	if err != nil {
		http.Error(w, `{"error":"failed to save checkpoint"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(mem)
}
```

- [ ] **Step 4: Run the test**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go test ./internal/... -run TestCheckpointSetsEventType -v
```

Expected: PASS.

---

### Task 6: Update `internal/server.go` — New Routes

**Files:**
- Modify: `internal/server.go`

- [ ] **Step 1: Update the `/memories/` route handler and add pinned/similar routing**

Replace the entire `RunServe` function:

```go
func RunServe() {
	cfg := LoadConfig()
	store := NewQdrantStore(cfg.QdrantURL, cfg.OllamaURL)
	traceStore := NewTraceStore(cfg.QdrantURL)

	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		log.Fatalf("init memory store: %v", err)
	}
	if err := traceStore.Init(ctx); err != nil {
		log.Fatalf("init trace store: %v", err)
	}

	h := NewHandlers(store)
	th := NewTraceHandlers(store, traceStore)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", h.Health)

	mux.HandleFunc("/memories/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/memories/")
		switch {
		case path == "pinned" && r.Method == http.MethodGet:
			h.PinnedMemories(w, r)
		case path == "similar" && r.Method == http.MethodGet:
			h.FindSimilar(w, r)
		case strings.HasSuffix(path, "/pin") && r.Method == http.MethodPatch:
			h.PinMemory(w, r)
		case r.Method == http.MethodDelete:
			h.DeleteMemory(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
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

Note: `strings` import is already in `server.go`. Verify it's present.

- [ ] **Step 2: Update `internal/models_test.go` — add new fields to round-trip test**

```go
package memex

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryJSONRoundtrip(t *testing.T) {
	m := Memory{
		ID:           "test-id",
		Text:         "user prefers Python",
		Project:      "memex",
		Topic:        "language",
		MemoryType:   "preference",
		Source:       "claude-code",
		Timestamp:    time.Now().UTC().Truncate(time.Second),
		Importance:   0.8,
		Tags:         []string{"preference", "python"},
		LastAccessed: time.Now().UTC().Truncate(time.Second),
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Memory
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Text != m.Text {
		t.Errorf("got Text %q, want %q", got.Text, m.Text)
	}
	if got.MemoryType != m.MemoryType {
		t.Errorf("got MemoryType %q, want %q", got.MemoryType, m.MemoryType)
	}
	if got.Topic != m.Topic {
		t.Errorf("got Topic %q, want %q", got.Topic, m.Topic)
	}
	if len(got.Tags) != len(m.Tags) {
		t.Errorf("got %d tags, want %d", len(got.Tags), len(m.Tags))
	}
}

func TestValidMemoryTypes(t *testing.T) {
	expected := []string{"decision", "preference", "event", "discovery", "advice", "problem", "context", "procedure", "rationale"}
	for _, typ := range expected {
		if !ValidMemoryTypes[typ] {
			t.Errorf("expected %q to be a valid memory type", typ)
		}
	}
	if ValidMemoryTypes["invalid"] {
		t.Error("expected 'invalid' to not be a valid memory type")
	}
	if len(ValidMemoryTypes) != 9 {
		t.Errorf("expected 9 memory types, got %d", len(ValidMemoryTypes))
	}
}
```

- [ ] **Step 3: Full build and test**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go build ./... && go test ./internal/... -v 2>&1 | tail -40
```

Expected: all tests PASS, clean build.

- [ ] **Step 4: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add internal/models.go internal/store.go internal/qdrant.go internal/handlers.go \
        internal/tracer_handlers.go internal/server.go \
        internal/models_test.go internal/handlers_test.go internal/qdrant_test.go
git commit -m "$(cat <<'EOF'
feat: structured memory — 9 types, topic field, pinned/similar endpoints

- Memory gains MemoryType (9 types) and Topic fields
- Qdrant schema adds memory_type + topic keyword indexes
- SearchMemories and ListMemories accept type/topic filters
- New endpoints: GET /memories/pinned, PATCH /memories/:id/pin, GET /memories/similar
- Checkpoint and Summarize save with MemoryType=event
- Store interface extended: PinnedMemories, PinMemory, FindSimilar

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Verification

- [ ] **Full test suite passes**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go test ./... -v 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: all `ok`, no `FAIL`.

- [ ] **Build produces binary**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go build -o /tmp/memex-test ./cmd/memex && echo "build OK"
```

Expected: `build OK`.
