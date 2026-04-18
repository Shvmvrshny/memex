package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockStore struct {
	memories         []Memory
	err              error
	findSimilarFn    func(ctx context.Context, text, project string, limit int) ([]Memory, error)
	searchMemoriesFn func(ctx context.Context, query, project, memoryType, topic string, tags []string, limit int) ([]Memory, error)
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

func (m *mockStore) SearchMemories(ctx context.Context, query, project, memoryType, topic string, tags []string, limit int) ([]Memory, error) {
	if m.searchMemoriesFn != nil {
		return m.searchMemoriesFn(ctx, query, project, memoryType, topic, tags, limit)
	}
	return m.memories, m.err
}

func (m *mockStore) ListMemories(ctx context.Context, project, memoryType, topic string, tags []string, limit int) ([]Memory, error) {
	return m.memories, m.err
}

func (m *mockStore) PinnedMemories(ctx context.Context, project string) ([]Memory, error) {
	return m.memories, m.err
}

func (m *mockStore) PinMemory(ctx context.Context, id string) error { return m.err }

func (m *mockStore) FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error) {
	if m.findSimilarFn != nil {
		return m.findSimilarFn(ctx, text, project, limit)
	}
	return m.memories, m.err
}

func (m *mockStore) DeleteMemory(ctx context.Context, id string) error { return m.err }

// ── Health ──────────────────────────────────────────────────────────────────

func TestHealthHandler_OK(t *testing.T) {
	h := NewHandlers(&mockStore{}, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHealthHandler_Down(t *testing.T) {
	h := NewHandlers(&mockStore{err: errors.New("qdrant down")}, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// ── SaveMemory ───────────────────────────────────────────────────────────────

func TestSaveMemoryHandler(t *testing.T) {
	h := NewHandlers(&mockStore{}, nil, nil)
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
	h := NewHandlers(&mockStore{}, nil, nil)
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
	h := NewHandlers(&mockStore{}, nil, nil)
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
	h := NewHandlers(&mockStore{}, nil, nil)
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
	h := NewHandlers(store, nil, nil)
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
	h := NewHandlers(store, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/memories?memory_type=decision&project=memex", nil)
	w := httptest.NewRecorder()
	h.SearchMemories(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

// ── Summarize ────────────────────────────────────────────────────────────────

func TestSummarizeHandler(t *testing.T) {
	h := NewHandlers(&mockStore{}, nil, nil)
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
	h := NewHandlers(store, nil, nil)
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
	h := NewHandlers(&mockStore{}, nil, nil)
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
	h := NewHandlers(store, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/memories/similar?text=python+preference", nil)
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestFindSimilarHandler_MissingText(t *testing.T) {
	h := NewHandlers(&mockStore{}, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/memories/similar", nil)
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlers_MineTranscript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(`{"role":"user","content":"I prefer table-driven tests."}`+"\n"), 0644)

	store := &mockStore{}
	h := NewHandlers(store, nil, nil)

	body := MineRequest{Path: path, Project: "memex"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mine/transcript", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.MineTranscript(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
	var resp MineResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Status != "mining started" {
		t.Errorf("status = %q, want 'mining started'", resp.Status)
	}
}

func TestExpandSearch_UsesKGTraversalWithDefaults(t *testing.T) {
	kg := newTestKG(t)
	// Root entity edges
	_, _ = kg.RecordFactScoped(Fact{Subject: "A", Predicate: PredicateContainsFunction, Object: "B", Source: "ast"}, false)
	_, _ = kg.RecordFactScoped(Fact{Subject: "A", Predicate: PredicateCalls, Object: "C", Source: "ast"}, false)
	_, _ = kg.RecordFactScoped(Fact{Subject: "A", Predicate: PredicateDependsOn, Object: "pkgX", Source: "ast"}, false)
	_, _ = kg.RecordFactScoped(Fact{Subject: "A", Predicate: PredicateCallsUnresolved, Object: "noise", Source: "ast"}, false)

	var capturedQuery string
	store := &mockStore{
		memories: []Memory{{ID: "m1", Text: "result", Importance: 0.5}},
		searchMemoriesFn: func(ctx context.Context, query, project, memoryType, topic string, tags []string, limit int) ([]Memory, error) {
			capturedQuery = query
			return []Memory{{ID: "s1", Text: "semantic", Importance: 0.4}}, nil
		},
	}
	h := NewHandlers(store, kg, nil)

	req := httptest.NewRequest(http.MethodGet, "/memories/expand?entity=A&project=memex&limit=5", nil)
	w := httptest.NewRecorder()
	h.ExpandSearch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(capturedQuery, "noise") {
		t.Fatalf("expanded query should not include unresolved edge target: %q", capturedQuery)
	}

	var resp struct {
		Neighbors  []string `json:"neighbors"`
		Depth      int      `json:"depth"`
		Fanout     int      `json:"fanout"`
		Predicates []string `json:"predicates"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Depth != 1 {
		t.Errorf("depth = %d, want 1", resp.Depth)
	}
	if resp.Fanout != 10 {
		t.Errorf("fanout = %d, want 10", resp.Fanout)
	}
	if containsString(resp.Neighbors, "noise") {
		t.Errorf("neighbors should exclude unresolved edges, got %+v", resp.Neighbors)
	}
}

func TestExpandSearch_DepthAndFanoutControls(t *testing.T) {
	kg := newTestKG(t)
	_, _ = kg.RecordFactScoped(Fact{Subject: "root", Predicate: PredicateCalls, Object: "n1", Source: "ast"}, false)
	_, _ = kg.RecordFactScoped(Fact{Subject: "root", Predicate: PredicateCalls, Object: "n2", Source: "ast"}, false)
	_, _ = kg.RecordFactScoped(Fact{Subject: "n1", Predicate: PredicateCalls, Object: "leaf", Source: "ast"}, false)

	store := &mockStore{
		searchMemoriesFn: func(ctx context.Context, query, project, memoryType, topic string, tags []string, limit int) ([]Memory, error) {
			return []Memory{{ID: "s1", Text: fmt.Sprintf("q:%s", query), Importance: 0.2}}, nil
		},
	}
	h := NewHandlers(store, kg, nil)

	req := httptest.NewRequest(http.MethodGet, "/memories/expand?entity=root&depth=2&fanout=1&predicates=calls", nil)
	w := httptest.NewRecorder()
	h.ExpandSearch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Neighbors []string `json:"neighbors"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Neighbors) > 2 {
		t.Errorf("neighbors should be capped by fanout traversal, got %d", len(resp.Neighbors))
	}
}

func TestExpandSearch_ReverseContainsReturnsFileAnchors(t *testing.T) {
	kg := newTestKG(t)
	_, _ = kg.RecordFactScoped(Fact{
		Subject:   "internal/hook.go",
		Predicate: PredicateContainsFunction,
		Object:    "github.com/shivamvarshney/memex/internal::hookSessionStart",
		Source:    "ast",
	}, false)

	h := NewHandlers(&mockStore{memories: []Memory{}}, kg, nil)
	req := httptest.NewRequest(http.MethodGet, "/memories/expand?entity=github.com/shivamvarshney/memex/internal::hookSessionStart&project=memex&limit=5", nil)
	w := httptest.NewRecorder()
	h.ExpandSearch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Neighbors []string `json:"neighbors"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !containsString(resp.Neighbors, "internal/hook.go") {
		t.Fatalf("expected reverse contains to include file anchor, got %+v", resp.Neighbors)
	}
}

func TestExpandSearch_ReverseCallsReturnsCallers(t *testing.T) {
	kg := newTestKG(t)
	_, _ = kg.RecordFactScoped(Fact{
		Subject:   "github.com/shivamvarshney/memex/internal::Caller",
		Predicate: PredicateCalls,
		Object:    "github.com/shivamvarshney/memex/internal::Callee",
		Source:    "ast",
	}, false)

	h := NewHandlers(&mockStore{memories: []Memory{}}, kg, nil)
	req := httptest.NewRequest(http.MethodGet, "/memories/expand?entity=github.com/shivamvarshney/memex/internal::Callee&project=memex&limit=5", nil)
	w := httptest.NewRecorder()
	h.ExpandSearch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Neighbors []string `json:"neighbors"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !containsString(resp.Neighbors, "github.com/shivamvarshney/memex/internal::Caller") {
		t.Fatalf("expected reverse calls to include caller, got %+v", resp.Neighbors)
	}
}

func TestExpandSearch_ReverseCallsAlsoProjectCallerFiles(t *testing.T) {
	kg := newTestKG(t)
	callerFn := "github.com/shivamvarshney/memex/internal::KGHandlers.RecordFact"
	calleeFn := "github.com/shivamvarshney/memex/internal::KnowledgeGraph.RecordFactScoped"
	_, _ = kg.RecordFactScoped(Fact{
		Subject:   "internal/kg_handlers.go",
		Predicate: PredicateContainsFunction,
		Object:    callerFn,
		Source:    "ast",
	}, false)
	_, _ = kg.RecordFactScoped(Fact{
		Subject:   callerFn,
		Predicate: PredicateCalls,
		Object:    calleeFn,
		Source:    "ast",
	}, false)

	h := NewHandlers(&mockStore{memories: []Memory{}}, kg, nil)
	req := httptest.NewRequest(http.MethodGet, "/memories/expand?entity="+calleeFn+"&project=memex&limit=5&depth=2", nil)
	w := httptest.NewRecorder()
	h.ExpandSearch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Neighbors []string `json:"neighbors"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !containsString(resp.Neighbors, "internal/kg_handlers.go") {
		t.Fatalf("expected caller file anchor from reverse calls, got %+v", resp.Neighbors)
	}
}

func TestExpandSearch_TestOfLinksBringInTestFiles(t *testing.T) {
	kg := newTestKG(t)
	fn := "github.com/shivamvarshney/memex/internal::hookSessionStart"
	_, _ = kg.RecordFactScoped(Fact{
		Subject:   "internal/hook.go",
		Predicate: PredicateContainsFunction,
		Object:    fn,
		Source:    "ast",
	}, false)
	_, _ = kg.RecordFactScoped(Fact{
		Subject:   "internal/hook_test.go",
		Predicate: PredicateTestOf,
		Object:    "internal/hook.go",
		Source:    "ast",
	}, false)

	h := NewHandlers(&mockStore{memories: []Memory{}}, kg, nil)
	req := httptest.NewRequest(http.MethodGet, "/memories/expand?entity="+fn+"&project=memex&limit=5&depth=2", nil)
	w := httptest.NewRecorder()
	h.ExpandSearch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Neighbors []string `json:"neighbors"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !containsString(resp.Neighbors, "internal/hook_test.go") {
		t.Fatalf("expected test_of traversal to include hook_test.go, got %+v", resp.Neighbors)
	}
}

func TestExpandSearch_PrioritizesFileAnchorsUnderFanout(t *testing.T) {
	kg := newTestKG(t)
	fn := "github.com/shivamvarshney/memex/internal::targetFn"
	_, _ = kg.RecordFactScoped(Fact{
		Subject:   fn,
		Predicate: PredicateCalls,
		Object:    "fmt::Sprintf",
		Source:    "ast",
	}, false)
	_, _ = kg.RecordFactScoped(Fact{
		Subject:   "internal/target.go",
		Predicate: PredicateContainsFunction,
		Object:    fn,
		Source:    "ast",
	}, false)

	h := NewHandlers(&mockStore{memories: []Memory{}}, kg, nil)
	req := httptest.NewRequest(http.MethodGet, "/memories/expand?entity="+fn+"&project=memex&limit=5&fanout=1&depth=1", nil)
	w := httptest.NewRecorder()
	h.ExpandSearch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Neighbors []string `json:"neighbors"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Neighbors) == 0 || resp.Neighbors[0] != "internal/target.go" {
		t.Fatalf("expected prioritized file anchor first under fanout=1, got %+v", resp.Neighbors)
	}
}

func containsString(items []string, target string) bool {
	for _, v := range items {
		if v == target {
			return true
		}
	}
	return false
}
