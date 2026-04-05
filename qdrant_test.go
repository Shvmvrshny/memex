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
