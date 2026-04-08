package memex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTraceStoreInit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	ts := NewTraceStore(srv.URL)
	if err := ts.Init(context.Background()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

func TestTraceStoreSaveEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collections/traces/points" && r.Method == http.MethodPut {
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ts := NewTraceStore(srv.URL)
	req := TraceEventRequest{
		SessionID:  "sess-1",
		Project:    "memex",
		TurnIndex:  0,
		Tool:       "Read",
		Input:      "internal/qdrant.go",
		Output:     "package memex",
		DurationMs: 12,
		Timestamp:  time.Now().Format(time.RFC3339),
	}
	event, err := ts.SaveEvent(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	if event.ID == "" {
		t.Error("expected non-empty ID")
	}
	if event.Tool != "Read" {
		t.Errorf("got Tool %q, want Read", event.Tool)
	}
}

func TestTraceStoreUpsertReasoning(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collections/traces/points" && r.Method == http.MethodPut {
			called = true
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ts := NewTraceStore(srv.URL)
	err := ts.UpsertReasoning(context.Background(), "event-id", "sess-1", "I need to check the file first")
	if err != nil {
		t.Fatalf("UpsertReasoning: %v", err)
	}
	if !called {
		t.Error("expected PUT /collections/traces/points to be called")
	}
}

func TestTraceStoreListSessions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collections/traces/points/scroll" {
			resp := map[string]any{
				"result": map[string]any{
					"points": []map[string]any{{
						"id": "evt-1",
						"payload": map[string]any{
							"session_id":  "sess-1",
							"project":     "memex",
							"turn_index":  float64(0),
							"tool":        "Read",
							"input":       "internal/qdrant.go",
							"output":      "package memex",
							"reasoning":   "",
							"duration_ms": float64(12),
							"timestamp":   "2026-04-07T10:42:03Z",
							"skill":       "",
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

	ts := NewTraceStore(srv.URL)
	sessions, err := ts.ListSessions(context.Background(), "memex")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if sessions[0].SessionID != "sess-1" {
		t.Errorf("got SessionID %q, want sess-1", sessions[0].SessionID)
	}
}

func TestTraceStoreGetSessionEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collections/traces/points/scroll" {
			resp := map[string]any{
				"result": map[string]any{
					"points": []map[string]any{{
						"id": "evt-1",
						"payload": map[string]any{
							"session_id":  "sess-1",
							"project":     "memex",
							"turn_index":  float64(0),
							"tool":        "Grep",
							"input":       "SaveMemory",
							"output":      "internal/qdrant.go:94",
							"reasoning":   "checking where SaveMemory is defined",
							"duration_ms": float64(8),
							"timestamp":   "2026-04-07T10:42:09Z",
							"skill":       "",
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

	ts := NewTraceStore(srv.URL)
	events, err := ts.GetSessionEvents(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("GetSessionEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Tool != "Grep" {
		t.Errorf("got Tool %q, want Grep", events[0].Tool)
	}
}

func TestTraceStoreListProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collections/traces/points/scroll" {
			resp := map[string]any{
				"result": map[string]any{
					"points": []map[string]any{
						{"id": "1", "payload": map[string]any{"project": "memex", "session_id": "s1", "turn_index": float64(0), "tool": "Read", "input": "", "output": "", "reasoning": "", "duration_ms": float64(0), "timestamp": "2026-04-07T10:00:00Z", "skill": ""}},
						{"id": "2", "payload": map[string]any{"project": "gstack", "session_id": "s2", "turn_index": float64(0), "tool": "Bash", "input": "", "output": "", "reasoning": "", "duration_ms": float64(0), "timestamp": "2026-04-07T11:00:00Z", "skill": ""}},
						{"id": "3", "payload": map[string]any{"project": "memex", "session_id": "s1", "turn_index": float64(1), "tool": "Edit", "input": "", "output": "", "reasoning": "", "duration_ms": float64(0), "timestamp": "2026-04-07T10:01:00Z", "skill": ""}},
					},
				},
				"status": "ok",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ts := NewTraceStore(srv.URL)
	projects, err := ts.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2: %v", len(projects), projects)
	}
}
