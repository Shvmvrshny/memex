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
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+":"+r.URL.Path)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	store := NewQdrantStore(srv.URL, "")
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Should have sent: PUT /collections/memories + 4x PUT /collections/memories/index
	if len(paths) < 5 {
		t.Errorf("expected at least 5 requests (1 collection + 4 indexes), got %d: %v", len(paths), paths)
	}
	for _, p := range paths {
		if p[:3] != "PUT" {
			t.Errorf("expected all PUT requests, got %q", p)
		}
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
