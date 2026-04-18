package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockStore struct {
	memories      []Memory
	err           error
	findSimilarFn func(ctx context.Context, text, project string, limit int) ([]Memory, error)
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
	h := NewHandlers(&mockStore{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHealthHandler_Down(t *testing.T) {
	h := NewHandlers(&mockStore{err: errors.New("qdrant down")}, nil)
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// ── SaveMemory ───────────────────────────────────────────────────────────────

func TestSaveMemoryHandler(t *testing.T) {
	h := NewHandlers(&mockStore{}, nil)
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
	h := NewHandlers(&mockStore{}, nil)
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
	h := NewHandlers(&mockStore{}, nil)
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
	h := NewHandlers(&mockStore{}, nil)
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
	h := NewHandlers(store, nil)
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
	h := NewHandlers(store, nil)
	r := httptest.NewRequest(http.MethodGet, "/memories?memory_type=decision&project=memex", nil)
	w := httptest.NewRecorder()
	h.SearchMemories(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

// ── Summarize ────────────────────────────────────────────────────────────────

func TestSummarizeHandler(t *testing.T) {
	h := NewHandlers(&mockStore{}, nil)
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
	h := NewHandlers(store, nil)
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
	h := NewHandlers(&mockStore{}, nil)
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
	h := NewHandlers(store, nil)
	r := httptest.NewRequest(http.MethodGet, "/memories/similar?text=python+preference", nil)
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestFindSimilarHandler_MissingText(t *testing.T) {
	h := NewHandlers(&mockStore{}, nil)
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
	h := NewHandlers(store, nil)

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
