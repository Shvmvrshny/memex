package memex

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
