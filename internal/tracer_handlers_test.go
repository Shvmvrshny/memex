package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockTraceStore implements the methods TraceHandlers needs.
type mockTraceStore struct {
	events   []TraceEvent
	sessions []Session
	projects []string
	err      error
}

func (m *mockTraceStore) SaveEvent(ctx context.Context, req TraceEventRequest) (TraceEvent, error) {
	if m.err != nil {
		return TraceEvent{}, m.err
	}
	e := TraceEvent{ID: "test-event-id", Tool: req.Tool, SessionID: req.SessionID}
	m.events = append(m.events, e)
	return e, nil
}

func (m *mockTraceStore) UpsertReasoning(ctx context.Context, eventID, sessionID, reasoning string) error {
	return m.err
}

func (m *mockTraceStore) ListSessions(ctx context.Context, project string) ([]Session, error) {
	return m.sessions, m.err
}

func (m *mockTraceStore) GetSessionEvents(ctx context.Context, sessionID string) ([]TraceEvent, error) {
	return m.events, m.err
}

func (m *mockTraceStore) ListProjects(ctx context.Context) ([]string, error) {
	return m.projects, m.err
}

func TestTraceEventHandler(t *testing.T) {
	ms := &mockStore{}
	mts := &mockTraceStore{}
	h := NewTraceHandlers(ms, mts)

	body, _ := json.Marshal(TraceEventRequest{
		SessionID: "sess-1", Project: "memex", Tool: "Read",
		Input: "internal/qdrant.go", DurationMs: 12,
		Timestamp: time.Now().Format(time.RFC3339),
	})
	r := httptest.NewRequest(http.MethodPost, "/trace/event", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.TraceEvent(w, r)

	if w.Code != http.StatusCreated {
		t.Errorf("got %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var event TraceEvent
	json.NewDecoder(w.Body).Decode(&event)
	if event.ID == "" {
		t.Error("expected non-empty ID in response")
	}
}

func TestTraceEventHandler_MissingFields(t *testing.T) {
	h := NewTraceHandlers(&mockStore{}, &mockTraceStore{})
	body, _ := json.Marshal(TraceEventRequest{SessionID: "sess-1"}) // missing Tool
	r := httptest.NewRequest(http.MethodPost, "/trace/event", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.TraceEvent(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestListSessionsHandler(t *testing.T) {
	mts := &mockTraceStore{sessions: []Session{{SessionID: "sess-1", Project: "memex", ToolCount: 5}}}
	h := NewTraceHandlers(&mockStore{}, mts)
	r := httptest.NewRequest(http.MethodGet, "/trace/sessions?project=memex", nil)
	w := httptest.NewRecorder()
	h.ListSessions(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
	var sessions []Session
	json.NewDecoder(w.Body).Decode(&sessions)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
}

func TestGetSessionHandler(t *testing.T) {
	mts := &mockTraceStore{events: []TraceEvent{{ID: "evt-1", Tool: "Read"}}}
	h := NewTraceHandlers(&mockStore{}, mts)
	r := httptest.NewRequest(http.MethodGet, "/trace/session/sess-1", nil)
	w := httptest.NewRecorder()
	h.GetSession(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
	var events []TraceEvent
	json.NewDecoder(w.Body).Decode(&events)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
}

func TestListProjectsHandler(t *testing.T) {
	mts := &mockTraceStore{projects: []string{"memex", "gstack"}}
	h := NewTraceHandlers(&mockStore{}, mts)
	r := httptest.NewRequest(http.MethodGet, "/projects", nil)
	w := httptest.NewRecorder()
	h.ListProjects(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
	var projects []string
	json.NewDecoder(w.Body).Decode(&projects)
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
}

func TestCheckpointHandler(t *testing.T) {
	ms := &mockStore{}
	h := NewTraceHandlers(ms, &mockTraceStore{})
	body, _ := json.Marshal(CheckpointRequest{
		Project: "memex",
		Summary: "done: tracer. decided: extend memex. next: UI.",
	})
	r := httptest.NewRequest(http.MethodPost, "/checkpoint", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Checkpoint(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	if len(ms.memories) != 1 {
		t.Fatalf("expected 1 memory saved, got %d", len(ms.memories))
	}
	if ms.memories[0].Importance != 0.9 {
		t.Errorf("expected importance 0.9, got %f", ms.memories[0].Importance)
	}
}
