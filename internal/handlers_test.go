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

func (m *mockStore) DeleteMemory(ctx context.Context, id string) error { return m.err }

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
