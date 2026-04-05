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
