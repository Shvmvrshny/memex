# memex Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local AI memory system — a Go HTTP service + Qdrant in Docker, exposed to Claude Code via a plugin with session hooks and MCP tools.

**Architecture:** A single Go binary with three subcommands: `serve` (HTTP API, runs in Docker), `mcp` (stdio MCP server, runs on host), and `hook` (session hook handler). The HTTP service stores and retrieves memories in Qdrant using full-text payload search. The plugin injects relevant memories at session start via hooks and exposes three MCP tools for Claude to use during sessions.

**Tech Stack:** Go 1.22, Qdrant v1.13 (Docker), `github.com/google/uuid` v1.6.0, `github.com/mark3labs/mcp-go` v0.20.0

---

## File Structure

```
memex/
├── main.go              # entry point, subcommand routing (serve/mcp/hook)
├── config.go            # config from env vars
├── models.go            # Memory, SaveMemoryRequest, SearchResponse types
├── store.go             # Store interface
├── qdrant.go            # Qdrant REST client (Init, Save, Search, List, Health)
├── handlers.go          # HTTP handlers (Health, SaveMemory, SearchMemories, Summarize)
├── mcp.go               # MCP stdio server + 3 tool handlers
├── hook.go              # session-start / session-stop hook logic
├── handlers_test.go     # handler unit tests (mock store)
├── qdrant_test.go       # Qdrant store unit tests
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── plugin/
    ├── plugin.json
    ├── hooks/
    │   ├── hooks.json
    │   ├── session-start    # calls: memex hook session-start
    │   └── session-stop     # calls: memex hook session-stop
    └── skills/
        └── memex/
            └── SKILL.md
```

---

## Task 1: Project Setup

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialise the Go module**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
go mod init github.com/shivamvarshney/memex
```

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/google/uuid@v1.6.0
go get github.com/mark3labs/mcp-go@v0.20.0
```

- [ ] **Step 3: Verify go.mod looks like this**

```
module github.com/shivamvarshney/memex

go 1.22

require (
    github.com/google/uuid v1.6.0
    github.com/mark3labs/mcp-go v0.20.0
)
```

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialise Go module with dependencies"
```

---

## Task 2: Models

**Files:**
- Create: `models.go`

- [ ] **Step 1: Write the failing test**

Create `models_test.go`:

```go
package main

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
    if len(got.Tags) != len(m.Tags) {
        t.Errorf("got %d tags, want %d", len(got.Tags), len(m.Tags))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./... -run TestMemoryJSONRoundtrip -v
```
Expected: FAIL — `Memory` undefined

- [ ] **Step 3: Write models.go**

```go
package main

import "time"

type Memory struct {
    ID           string    `json:"id"`
    Text         string    `json:"text"`
    Project      string    `json:"project"`
    Source       string    `json:"source"`
    Timestamp    time.Time `json:"timestamp"`
    Importance   float32   `json:"importance"`
    Tags         []string  `json:"tags"`
    LastAccessed time.Time `json:"last_accessed"`
}

type SaveMemoryRequest struct {
    Text       string   `json:"text"`
    Project    string   `json:"project"`
    Source     string   `json:"source"`
    Importance float32  `json:"importance"`
    Tags       []string `json:"tags"`
}

type SearchResponse struct {
    Memories []Memory `json:"memories"`
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./... -run TestMemoryJSONRoundtrip -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add models.go models_test.go
git commit -m "feat: add Memory, SaveMemoryRequest, SearchResponse types"
```

---

## Task 3: Config + Store Interface

**Files:**
- Create: `config.go`
- Create: `store.go`

- [ ] **Step 1: Write config.go**

```go
package main

import "os"

const defaultMemexURL = "http://localhost:8765"

type Config struct {
    Port      string
    QdrantURL string
}

func LoadConfig() Config {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8765"
    }
    qdrantURL := os.Getenv("QDRANT_URL")
    if qdrantURL == "" {
        qdrantURL = "http://localhost:6333"
    }
    return Config{Port: port, QdrantURL: qdrantURL}
}
```

- [ ] **Step 2: Write store.go**

```go
package main

import "context"

type Store interface {
    Init(ctx context.Context) error
    SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error)
    SearchMemories(ctx context.Context, query, project string, limit int) ([]Memory, error)
    ListMemories(ctx context.Context, project string) ([]Memory, error)
    Health(ctx context.Context) error
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add config.go store.go
git commit -m "feat: add Config and Store interface"
```

---

## Task 4: Qdrant Store — Init and Health

**Files:**
- Create: `qdrant.go`
- Create: `qdrant_test.go`

- [ ] **Step 1: Write the failing test**

Create `qdrant_test.go`:

```go
package main

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestQdrantHealth_OK(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/healthz" {
            w.WriteHeader(http.StatusOK)
        }
    }))
    defer srv.Close()

    store := NewQdrantStore(srv.URL)
    if err := store.Health(context.Background()); err != nil {
        t.Fatalf("expected healthy, got: %v", err)
    }
}

func TestQdrantHealth_Down(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusServiceUnavailable)
    }))
    defer srv.Close()

    store := NewQdrantStore(srv.URL)
    if err := store.Health(context.Background()); err == nil {
        t.Fatal("expected error when qdrant is down")
    }
}

func TestQdrantInit(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    }))
    defer srv.Close()

    store := NewQdrantStore(srv.URL)
    if err := store.Init(context.Background()); err != nil {
        t.Fatalf("Init failed: %v", err)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run "TestQdrant" -v
```
Expected: FAIL — `NewQdrantStore` undefined

- [ ] **Step 3: Write qdrant.go (Init and Health)**

```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"

    "github.com/google/uuid"
)

type QdrantStore struct {
    baseURL string
    client  *http.Client
}

func NewQdrantStore(baseURL string) *QdrantStore {
    return &QdrantStore{
        baseURL: baseURL,
        client:  &http.Client{Timeout: 10 * time.Second},
    }
}

func (q *QdrantStore) Health(ctx context.Context) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/healthz", nil)
    if err != nil {
        return err
    }
    resp, err := q.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("qdrant unhealthy: status %d", resp.StatusCode)
    }
    return nil
}

func (q *QdrantStore) Init(ctx context.Context) error {
    // Create collection (ignore "already exists" error)
    body := map[string]any{
        "vectors": map[string]any{
            "size":     1,
            "distance": "Dot",
        },
    }
    if err := q.put(ctx, "/collections/memories", body); err != nil {
        if !strings.Contains(err.Error(), "already exists") {
            return fmt.Errorf("create collection: %w", err)
        }
    }

    // Create full-text index on "text" payload field
    index := map[string]any{
        "field_name":   "text",
        "field_schema": "text",
    }
    if err := q.put(ctx, "/collections/memories/payload/index", index); err != nil {
        if !strings.Contains(err.Error(), "already exists") {
            return fmt.Errorf("create text index: %w", err)
        }
    }
    return nil
}

func (q *QdrantStore) put(ctx context.Context, path string, body any) error {
    data, err := json.Marshal(body)
    if err != nil {
        return err
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPut, q.baseURL+path, bytes.NewReader(data))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := q.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
    }
    return nil
}

// Placeholder methods to satisfy interface — implemented in later tasks
func (q *QdrantStore) SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error) {
    return Memory{}, fmt.Errorf("not implemented")
}

func (q *QdrantStore) SearchMemories(ctx context.Context, query, project string, limit int) ([]Memory, error) {
    return nil, fmt.Errorf("not implemented")
}

func (q *QdrantStore) ListMemories(ctx context.Context, project string) ([]Memory, error) {
    return nil, fmt.Errorf("not implemented")
}

// url and uuid imported for use in later tasks
var _ = url.QueryEscape
var _ = uuid.New
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestQdrant" -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add qdrant.go qdrant_test.go
git commit -m "feat: add QdrantStore with Init and Health"
```

---

## Task 5: Qdrant Store — SaveMemory

**Files:**
- Modify: `qdrant.go` (replace SaveMemory stub)
- Modify: `qdrant_test.go` (add test)

- [ ] **Step 1: Add the failing test to qdrant_test.go**

```go
func TestQdrantSaveMemory(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/collections/memories/points" && r.Method == http.MethodPut {
            json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
            return
        }
        w.WriteHeader(http.StatusNotFound)
    }))
    defer srv.Close()

    store := NewQdrantStore(srv.URL)
    req := SaveMemoryRequest{
        Text:       "user prefers table-driven tests",
        Project:    "memex",
        Source:     "claude-code",
        Importance: 0.8,
        Tags:       []string{"testing", "preference"},
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
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./... -run TestQdrantSaveMemory -v
```
Expected: FAIL — returns "not implemented"

- [ ] **Step 3: Replace SaveMemory stub in qdrant.go**

```go
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

    body := map[string]any{
        "points": []map[string]any{{
            "id":     id,
            "vector": []float32{0.0},
            "payload": map[string]any{
                "text":          req.Text,
                "project":       req.Project,
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
        Source:       req.Source,
        Timestamp:    now,
        Importance:   req.Importance,
        Tags:         req.Tags,
        LastAccessed: now,
    }, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./... -run TestQdrantSaveMemory -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add qdrant.go qdrant_test.go
git commit -m "feat: implement QdrantStore.SaveMemory"
```

---

## Task 6: Qdrant Store — SearchMemories and ListMemories

**Files:**
- Modify: `qdrant.go` (replace Search and List stubs)
- Modify: `qdrant_test.go` (add tests)

- [ ] **Step 1: Add the failing tests to qdrant_test.go**

```go
func TestQdrantSearchMemories(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/collections/memories/points/scroll" {
            resp := map[string]any{
                "result": map[string]any{
                    "points": []map[string]any{{
                        "id": "test-id",
                        "payload": map[string]any{
                            "text":          "user prefers Python",
                            "project":       "memex",
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

    store := NewQdrantStore(srv.URL)
    memories, err := store.SearchMemories(context.Background(), "python language", "", 5)
    if err != nil {
        t.Fatalf("SearchMemories: %v", err)
    }
    if len(memories) != 1 {
        t.Fatalf("got %d memories, want 1", len(memories))
    }
    if memories[0].Text != "user prefers Python" {
        t.Errorf("got Text %q, want 'user prefers Python'", memories[0].Text)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./... -run TestQdrantSearchMemories -v
```
Expected: FAIL — returns "not implemented"

- [ ] **Step 3: Add scroll helper and replace Search/List stubs in qdrant.go**

Add this helper struct after the `put` method:

```go
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
```

Replace the SearchMemories stub:

```go
func (q *QdrantStore) SearchMemories(ctx context.Context, query, project string, limit int) ([]Memory, error) {
    if limit <= 0 {
        limit = 5
    }

    mustClauses := []map[string]any{{
        "key":   "text",
        "match": map[string]any{"text": query},
    }}
    if project != "" {
        mustClauses = append(mustClauses, map[string]any{
            "key":   "project",
            "match": map[string]any{"value": project},
        })
    }

    return q.scroll(ctx, map[string]any{
        "filter":       map[string]any{"must": mustClauses},
        "limit":        limit,
        "with_payload": true,
        "with_vector":  false,
    })
}
```

Replace the ListMemories stub:

```go
func (q *QdrantStore) ListMemories(ctx context.Context, project string) ([]Memory, error) {
    body := map[string]any{
        "limit":        100,
        "with_payload": true,
        "with_vector":  false,
    }
    if project != "" {
        body["filter"] = map[string]any{
            "must": []map[string]any{{
                "key":   "project",
                "match": map[string]any{"value": project},
            }},
        }
    }
    return q.scroll(ctx, body)
}
```

Remove the unused placeholder lines at the bottom of qdrant.go:
```go
// DELETE these two lines:
var _ = url.QueryEscape
var _ = uuid.New
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestQdrant" -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add qdrant.go qdrant_test.go
git commit -m "feat: implement QdrantStore SearchMemories and ListMemories"
```

---

## Task 7: HTTP Handlers

**Files:**
- Create: `handlers.go`
- Create: `handlers_test.go`

- [ ] **Step 1: Write the failing tests**

Create `handlers_test.go`:

```go
package main

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

func (m *mockStore) Init(ctx context.Context) error  { return m.err }
func (m *mockStore) Health(ctx context.Context) error { return m.err }

func (m *mockStore) SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error) {
    if m.err != nil {
        return Memory{}, m.err
    }
    mem := Memory{
        ID:           "test-id",
        Text:         req.Text,
        Project:      req.Project,
        Source:       "claude-code",
        Timestamp:    time.Now(),
        Importance:   req.Importance,
        Tags:         req.Tags,
        LastAccessed: time.Now(),
    }
    m.memories = append(m.memories, mem)
    return mem, nil
}

func (m *mockStore) SearchMemories(ctx context.Context, query, project string, limit int) ([]Memory, error) {
    return m.memories, m.err
}

func (m *mockStore) ListMemories(ctx context.Context, project string) ([]Memory, error) {
    return m.memories, m.err
}

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

func TestSaveMemoryHandler(t *testing.T) {
    h := NewHandlers(&mockStore{})
    body, _ := json.Marshal(SaveMemoryRequest{
        Text:    "user prefers Go",
        Project: "memex",
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
}

func TestSaveMemoryHandler_MissingText(t *testing.T) {
    h := NewHandlers(&mockStore{})
    body, _ := json.Marshal(SaveMemoryRequest{Project: "memex"})
    r := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader(body))
    r.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    h.SaveMemory(w, r)
    if w.Code != http.StatusBadRequest {
        t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
    }
}

func TestSearchMemoriesHandler(t *testing.T) {
    store := &mockStore{memories: []Memory{{ID: "1", Text: "user prefers Python"}}}
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
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run "TestHealthHandler|TestSaveMemory|TestSearch|TestSummarize" -v
```
Expected: FAIL — `NewHandlers` undefined

- [ ] **Step 3: Write handlers.go**

```go
package main

import (
    "encoding/json"
    "net/http"
    "strconv"
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
    limit := 5
    if l := r.URL.Query().Get("limit"); l != "" {
        if n, err := strconv.Atoi(l); err == nil && n > 0 {
            limit = n
        }
    }

    memories, err := h.store.SearchMemories(r.Context(), query, project, limit)
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestHealthHandler|TestSaveMemory|TestSearch|TestSummarize" -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add handlers.go handlers_test.go
git commit -m "feat: add HTTP handlers for health, memories, and summarize"
```

---

## Task 8: HTTP Server + main.go

**Files:**
- Create: `main.go`

- [ ] **Step 1: Write main.go**

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, "Usage: memex <serve|mcp|hook <session-start|session-stop>>")
        os.Exit(1)
    }

    switch os.Args[1] {
    case "serve":
        runServe()
    case "mcp":
        runMCP()
    case "hook":
        if len(os.Args) < 3 {
            fmt.Fprintln(os.Stderr, "Usage: memex hook <session-start|session-stop>")
            os.Exit(1)
        }
        runHook(os.Args[2])
    default:
        fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
        os.Exit(1)
    }
}

func runServe() {
    cfg := LoadConfig()
    store := NewQdrantStore(cfg.QdrantURL)

    ctx := context.Background()
    if err := store.Init(ctx); err != nil {
        log.Fatalf("init store: %v", err)
    }

    h := NewHandlers(store)
    mux := http.NewServeMux()
    mux.HandleFunc("/health", h.Health)
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

    addr := ":" + cfg.Port
    log.Printf("memex listening on %s", addr)
    log.Fatal(http.ListenAndServe(addr, mux))
}
```

- [ ] **Step 2: Verify the project builds**

```bash
go build ./...
```
Expected: no errors (runMCP and runHook are not defined yet — add stubs)

Add stubs temporarily so it builds (these get replaced in later tasks):

Create `mcp.go` with just:
```go
package main

func runMCP() {}
```

Create `hook.go` with just:
```go
package main

func runHook(event string) {}
```

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add main.go mcp.go hook.go
git commit -m "feat: add HTTP server entry point and subcommand routing"
```

---

## Task 9: Hook Handler

**Files:**
- Modify: `hook.go` (replace stub)

- [ ] **Step 1: Replace hook.go**

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "os/exec"
    "strings"
)

func runHook(event string) {
    switch event {
    case "session-start":
        hookSessionStart()
    case "session-stop":
        // v1: no-op — summarization is manual via MCP tool
        outputEmpty()
    default:
        fmt.Fprintf(os.Stderr, "unknown hook event: %s\n", event)
        os.Exit(1)
    }
}

func hookSessionStart() {
    project := getProjectName()
    context := fmt.Sprintf("working on project: %s", project)

    // Silent fail if service is offline
    resp, err := http.Get(defaultMemexURL + "/health")
    if err != nil || resp.StatusCode != http.StatusOK {
        outputOfflineWarning()
        return
    }
    resp.Body.Close()

    // Fetch relevant memories
    apiURL := fmt.Sprintf("%s/memories?context=%s&limit=5",
        defaultMemexURL, url.QueryEscape(context))
    resp2, err := http.Get(apiURL)
    if err != nil {
        outputEmpty()
        return
    }
    defer resp2.Body.Close()

    body, _ := io.ReadAll(resp2.Body)
    var result SearchResponse
    if err := json.Unmarshal(body, &result); err != nil || len(result.Memories) == 0 {
        outputEmpty()
        return
    }

    var sb strings.Builder
    sb.WriteString("<memex-memory>\n")
    for _, m := range result.Memories {
        sb.WriteString(fmt.Sprintf("- %s\n", m.Text))
    }
    sb.WriteString("</memex-memory>")

    outputContext(sb.String())
}

func getProjectName() string {
    out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
    if err != nil {
        wd, _ := os.Getwd()
        parts := strings.Split(strings.TrimRight(wd, "/"), "/")
        return parts[len(parts)-1]
    }
    parts := strings.Split(strings.TrimSpace(string(out)), "/")
    return parts[len(parts)-1]
}

func outputContext(additionalContext string) {
    output := map[string]any{
        "hookSpecificOutput": map[string]any{
            "hookEventName":    "SessionStart",
            "additionalContext": additionalContext,
        },
    }
    json.NewEncoder(os.Stdout).Encode(output)
}

func outputOfflineWarning() {
    output := map[string]any{
        "hookSpecificOutput": map[string]any{
            "hookEventName":    "SessionStart",
            "additionalContext": "<memex> memory service offline — starting without memory context",
        },
    }
    json.NewEncoder(os.Stdout).Encode(output)
}

func outputEmpty() {
    os.Stdout.Write([]byte("{}\n"))
}
```

- [ ] **Step 2: Verify it builds**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add hook.go
git commit -m "feat: implement session-start hook with memory injection"
```

---

## Task 10: MCP Server

**Files:**
- Modify: `mcp.go` (replace stub)

- [ ] **Step 1: Replace mcp.go**

```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "os"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

func runMCP() {
    s := server.NewMCPServer("memex", "1.0.0",
        server.WithToolCapabilities(true),
    )

    s.AddTool(
        mcp.NewTool("save_memory",
            mcp.WithDescription("Save something important to long-term memory. Use this when the user states a preference, makes a decision, or shares context that should persist across sessions."),
            mcp.WithString("text", mcp.Required(), mcp.Description("The memory to save, written as a clear statement e.g. 'user prefers table-driven tests in Go'")),
            mcp.WithString("project", mcp.Description("Project name to associate this memory with (optional)")),
            mcp.WithNumber("importance", mcp.Description("Importance score 0.0-1.0, default 0.5. Use 0.9+ for critical preferences.")),
        ),
        handleSaveMemory,
    )

    s.AddTool(
        mcp.NewTool("search_memory",
            mcp.WithDescription("Search long-term memory for relevant context about the user or project."),
            mcp.WithString("context", mcp.Required(), mcp.Description("What you want to remember — e.g. 'user language preferences'")),
            mcp.WithString("project", mcp.Description("Filter by project name (optional)")),
        ),
        handleSearchMemory,
    )

    s.AddTool(
        mcp.NewTool("list_memories",
            mcp.WithDescription("List all stored memories, optionally filtered by project."),
            mcp.WithString("project", mcp.Description("Filter by project name (optional)")),
        ),
        handleListMemories,
    )

    if err := server.ServeStdio(s); err != nil {
        fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
        os.Exit(1)
    }
}

func handleSaveMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    text, _ := req.Params.Arguments["text"].(string)
    project, _ := req.Params.Arguments["project"].(string)
    importance, _ := req.Params.Arguments["importance"].(float64)
    if importance == 0 {
        importance = 0.5
    }

    body := SaveMemoryRequest{
        Text:       text,
        Project:    project,
        Source:     "claude-code",
        Importance: float32(importance),
    }
    data, _ := json.Marshal(body)

    resp, err := http.Post(defaultMemexURL+"/memories", "application/json", bytes.NewReader(data))
    if err != nil {
        return mcp.NewToolResultError("memex service unavailable — is Docker running?"), nil
    }
    defer resp.Body.Close()

    var mem Memory
    json.NewDecoder(resp.Body).Decode(&mem)
    return mcp.NewToolResultText(fmt.Sprintf("memory saved (id: %s)", mem.ID)), nil
}

func handleSearchMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    query, _ := req.Params.Arguments["context"].(string)
    project, _ := req.Params.Arguments["project"].(string)

    apiURL := fmt.Sprintf("%s/memories?context=%s&project=%s&limit=5",
        defaultMemexURL, url.QueryEscape(query), url.QueryEscape(project))
    resp, err := http.Get(apiURL)
    if err != nil {
        return mcp.NewToolResultError("memex service unavailable — is Docker running?"), nil
    }
    defer resp.Body.Close()

    var result SearchResponse
    json.NewDecoder(resp.Body).Decode(&result)
    if len(result.Memories) == 0 {
        return mcp.NewToolResultText("no memories found"), nil
    }

    data, _ := json.MarshalIndent(result.Memories, "", "  ")
    return mcp.NewToolResultText(string(data)), nil
}

func handleListMemories(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    project, _ := req.Params.Arguments["project"].(string)

    apiURL := fmt.Sprintf("%s/memories?project=%s&limit=100",
        defaultMemexURL, url.QueryEscape(project))
    resp, err := http.Get(apiURL)
    if err != nil {
        return mcp.NewToolResultError("memex service unavailable — is Docker running?"), nil
    }
    defer resp.Body.Close()

    var result SearchResponse
    json.NewDecoder(resp.Body).Decode(&result)
    if len(result.Memories) == 0 {
        return mcp.NewToolResultText("no memories stored yet"), nil
    }

    data, _ := json.MarshalIndent(result.Memories, "", "  ")
    return mcp.NewToolResultText(string(data)), nil
}
```

- [ ] **Step 2: Verify it builds**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add mcp.go
git commit -m "feat: implement MCP server with save_memory, search_memory, list_memories tools"
```

---

## Task 11: Docker Setup

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`

- [ ] **Step 1: Write Dockerfile**

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o memex .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/memex /usr/local/bin/memex
EXPOSE 8765
CMD ["memex", "serve"]
```

- [ ] **Step 2: Write docker-compose.yml**

```yaml
services:
  memex:
    build: .
    ports:
      - "8765:8765"
    environment:
      - QDRANT_URL=http://qdrant:6333
    depends_on:
      qdrant:
        condition: service_healthy
    restart: unless-stopped

  qdrant:
    image: qdrant/qdrant:v1.13.0
    ports:
      - "6333:6333"
    volumes:
      - qdrant_data:/qdrant/storage
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:6333/healthz || exit 1"]
      interval: 5s
      timeout: 3s
      retries: 10
    restart: unless-stopped

volumes:
  qdrant_data:
```

- [ ] **Step 3: Build the Docker image**

```bash
docker compose build
```
Expected: image builds successfully

- [ ] **Step 4: Start the stack**

```bash
docker compose up -d
```
Expected: both containers start

- [ ] **Step 5: Verify the service is healthy**

```bash
curl http://localhost:8765/health
```
Expected:
```json
{"status":"ok"}
```

- [ ] **Step 6: Stop the stack**

```bash
docker compose down
```

- [ ] **Step 7: Commit**

```bash
git add Dockerfile docker-compose.yml
git commit -m "feat: add Dockerfile and docker-compose for memex + Qdrant"
```

---

## Task 12: Plugin Hooks and Manifest

**Files:**
- Create: `plugin/plugin.json`
- Create: `plugin/hooks/hooks.json`
- Create: `plugin/hooks/session-start`
- Create: `plugin/hooks/session-stop`

- [ ] **Step 1: Write plugin/plugin.json**

```json
{
  "name": "memex",
  "displayName": "Memex — Local AI Memory",
  "description": "Local AI memory system. Remembers your preferences and context across sessions using Docker + Qdrant.",
  "version": "1.0.0",
  "author": {
    "name": "Shivam Varshney"
  },
  "repository": "https://github.com/shivamvarshney/memex",
  "license": "MIT",
  "keywords": ["memory", "context", "local", "qdrant"],
  "skills": "./skills/",
  "hooks": "./hooks/hooks.json"
}
```

- [ ] **Step 2: Write plugin/hooks/hooks.json**

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "memex hook session-start",
            "async": false
          }
        ]
      }
    ],
    "SessionStop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "memex hook session-stop",
            "async": true
          }
        ]
      }
    ]
  }
}
```

- [ ] **Step 3: Write plugin/hooks/session-start**

```bash
#!/usr/bin/env bash
set -euo pipefail
memex hook session-start
```

- [ ] **Step 4: Write plugin/hooks/session-stop**

```bash
#!/usr/bin/env bash
set -euo pipefail
memex hook session-stop
```

- [ ] **Step 5: Make hooks executable**

```bash
chmod +x plugin/hooks/session-start plugin/hooks/session-stop
```

- [ ] **Step 6: Commit**

```bash
git add plugin/
git commit -m "feat: add Claude Code plugin manifest and session hooks"
```

---

## Task 13: Plugin Skill

**Files:**
- Create: `plugin/skills/memex/SKILL.md`

- [ ] **Step 1: Write plugin/skills/memex/SKILL.md**

```markdown
---
name: memex
description: "Use when you learn something about the user, their preferences, decisions, or project context that should be remembered across sessions. Also use when the user asks you to remember or forget something."
---

# Memex Memory Management

You have access to a local memory system via three MCP tools: `save_memory`, `search_memory`, and `list_memories`.

## When to save a memory

Save a memory when:
- The user states a preference ("I prefer X over Y", "always use X", "never do Y")
- The user makes a significant decision ("we decided to use X for Y")
- The user shares important context about themselves or their project
- The user explicitly asks you to remember something

Do NOT save a memory for:
- Temporary task state or in-progress work
- Things already obvious from the code
- Every single message — only save things worth remembering next session

## How to save a memory

Write memories as clear, standalone statements that will make sense out of context:

Good: `"user prefers table-driven tests in Go"`
Bad: `"prefers that"`

Good: `"project uses SQLite for local storage, no external DB"`
Bad: `"uses sqlite"`

Use `importance: 0.9` for strong preferences or decisions. Use `importance: 0.5` (default) for general context.

## When the user asks you to forget something

Use `list_memories` to find the relevant memory, then inform the user that direct deletion is not yet supported in v1 — they can run `docker compose down -v && docker compose up -d` in the memex directory to reset all memories.

## Memory at session start

Memories from past sessions are automatically injected at the start of each session inside `<memex-memory>` tags. Use this context to personalise your responses without asking the user to repeat themselves.
```

- [ ] **Step 2: Run all tests one final time**

```bash
go test ./... -v
```
Expected: all PASS

- [ ] **Step 3: Commit**

```bash
git add plugin/skills/
git commit -m "feat: add memex skill with memory management instructions"
```

---

## Task 14: Install and Smoke Test

- [ ] **Step 1: Build the binary**

```bash
go build -o memex .
```

- [ ] **Step 2: Start the Docker stack**

```bash
docker compose up -d
```

- [ ] **Step 3: Test health endpoint**

```bash
curl http://localhost:8765/health
```
Expected: `{"status":"ok"}`

- [ ] **Step 4: Save a test memory**

```bash
curl -X POST http://localhost:8765/memories \
  -H "Content-Type: application/json" \
  -d '{"text":"user is building memex, a local AI memory system","project":"memex","importance":0.8}'
```
Expected: JSON response with an `id` field

- [ ] **Step 5: Search for it**

```bash
curl "http://localhost:8765/memories?context=memex+project&limit=5"
```
Expected: JSON with the memory you just saved in `memories` array

- [ ] **Step 6: Test the hook**

```bash
./memex hook session-start
```
Expected: JSON with `hookSpecificOutput.additionalContext` containing the saved memory wrapped in `<memex-memory>` tags

- [ ] **Step 7: Install the binary globally**

```bash
go install .
```

- [ ] **Step 8: Commit**

```bash
git add .
git commit -m "chore: final smoke test and binary install verified"
```

---

## MCP Configuration for Claude Code

After `go install`, add this to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "memex": {
      "command": "memex",
      "args": ["mcp"]
    }
  }
}
```

Restart Claude Code. The `save_memory`, `search_memory`, and `list_memories` tools will be available.
