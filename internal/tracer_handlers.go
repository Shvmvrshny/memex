package memex

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// traceStoreInterface defines what TraceHandlers needs from TraceStore.
type traceStoreInterface interface {
	SaveEvent(ctx context.Context, req TraceEventRequest) (TraceEvent, error)
	UpsertReasoning(ctx context.Context, eventID, sessionID, reasoning string) error
	ListSessions(ctx context.Context, project string) ([]Session, error)
	GetSessionEvents(ctx context.Context, sessionID string) ([]TraceEvent, error)
	ListProjects(ctx context.Context) ([]string, error)
}

type TraceHandlers struct {
	store      Store
	traceStore traceStoreInterface
}

func NewTraceHandlers(store Store, traceStore traceStoreInterface) *TraceHandlers {
	return &TraceHandlers{store: store, traceStore: traceStore}
}

func (h *TraceHandlers) TraceEvent(w http.ResponseWriter, r *http.Request) {
	var req TraceEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.Tool == "" {
		http.Error(w, `{"error":"session_id and tool are required"}`, http.StatusBadRequest)
		return
	}
	event, err := h.traceStore.SaveEvent(r.Context(), req)
	if err != nil {
		http.Error(w, `{"error":"failed to save trace event"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(event)
}

func (h *TraceHandlers) TraceStop(w http.ResponseWriter, r *http.Request) {
	var req StopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, `{"error":"session_id is required"}`, http.StatusBadRequest)
		return
	}

	if req.TranscriptPath != "" {
		reasoning, err := ParseTranscript(req.TranscriptPath)
		if err == nil {
			events, _ := h.traceStore.GetSessionEvents(r.Context(), req.SessionID)
			for _, e := range events {
				if e.TurnIndex < len(reasoning) && reasoning[e.TurnIndex] != "" {
					h.traceStore.UpsertReasoning(r.Context(), e.ID, req.SessionID, reasoning[e.TurnIndex])
				}
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TraceHandlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	sessions, err := h.traceStore.ListSessions(r.Context(), project)
	if err != nil {
		http.Error(w, `{"error":"failed to list sessions"}`, http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []Session{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func (h *TraceHandlers) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/trace/session/")
	if sessionID == "" {
		http.Error(w, `{"error":"session_id is required"}`, http.StatusBadRequest)
		return
	}
	events, err := h.traceStore.GetSessionEvents(r.Context(), sessionID)
	if err != nil {
		http.Error(w, `{"error":"failed to get session"}`, http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []TraceEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (h *TraceHandlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.traceStore.ListProjects(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to list projects"}`, http.StatusInternalServerError)
		return
	}
	if projects == nil {
		projects = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (h *TraceHandlers) Checkpoint(w http.ResponseWriter, r *http.Request) {
	var req CheckpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Project == "" || req.Summary == "" {
		http.Error(w, `{"error":"project and summary are required"}`, http.StatusBadRequest)
		return
	}
	mem, err := h.store.SaveMemory(r.Context(), SaveMemoryRequest{
		Text:       req.Summary,
		Project:    req.Project,
		Topic:      "checkpoint",
		MemoryType: "event",
		Source:     "claude-code",
		Importance: 0.9,
		Tags:       []string{"checkpoint"},
	})
	if err != nil {
		http.Error(w, `{"error":"failed to save checkpoint"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(mem)
}
