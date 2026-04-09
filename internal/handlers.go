package memex

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type Handlers struct {
	store Store
}

func NewHandlers(store Store) *Handlers {
	return &Handlers{store: store}
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Health(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "qdrant unavailable"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handlers) SaveMemory(w http.ResponseWriter, r *http.Request) {
	var req SaveMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}
	if !ValidMemoryTypes[req.MemoryType] {
		http.Error(w, `{"error":"memory_type is required and must be one of: decision, preference, event, discovery, advice, problem, context, procedure, rationale"}`, http.StatusBadRequest)
		return
	}

	memory, err := h.store.SaveMemory(r.Context(), req)
	if err != nil {
		http.Error(w, `{"error":"failed to save memory"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(memory)
}

func (h *Handlers) SearchMemories(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("context")
	project := r.URL.Query().Get("project")
	memoryType := r.URL.Query().Get("memory_type")
	topic := r.URL.Query().Get("topic")
	limit := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	var (
		memories []Memory
		err      error
	)
	if query == "" {
		memories, err = h.store.ListMemories(r.Context(), project, memoryType, topic, limit)
	} else {
		memories, err = h.store.SearchMemories(r.Context(), query, project, memoryType, topic, limit)
	}
	if err != nil {
		http.Error(w, `{"error":"search failed"}`, http.StatusInternalServerError)
		return
	}
	if memories == nil {
		memories = []Memory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Memories: memories})
}

func (h *Handlers) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/memories/")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteMemory(r.Context(), id); err != nil {
		http.Error(w, `{"error":"failed to delete memory"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Summarize(w http.ResponseWriter, r *http.Request) {
	var req SaveMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}

	req.Importance = 0.9
	req.MemoryType = "event"
	req.Topic = "session-summary"
	if req.Tags == nil {
		req.Tags = []string{"session-summary"}
	}

	memory, err := h.store.SaveMemory(r.Context(), req)
	if err != nil {
		http.Error(w, `{"error":"failed to save summary"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(memory)
}

// PinnedMemories returns memories with importance >= 0.9 for the project.
// GET /memories/pinned?project=X
func (h *Handlers) PinnedMemories(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		http.Error(w, `{"error":"project is required"}`, http.StatusBadRequest)
		return
	}
	memories, err := h.store.PinnedMemories(r.Context(), project)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch pinned memories"}`, http.StatusInternalServerError)
		return
	}
	if memories == nil {
		memories = []Memory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Memories: memories})
}

// PinMemory sets importance = 1.0 on a memory.
// PATCH /memories/:id/pin
func (h *Handlers) PinMemory(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/memories/")
	id := strings.TrimSuffix(path, "/pin")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.PinMemory(r.Context(), id); err != nil {
		http.Error(w, `{"error":"failed to pin memory"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// FindSimilar returns the most similar memories to the given text.
// GET /memories/similar?text=X&project=Y&limit=5
func (h *Handlers) FindSimilar(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}
	project := r.URL.Query().Get("project")
	limit := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	memories, err := h.store.FindSimilar(r.Context(), text, project, limit)
	if err != nil {
		http.Error(w, `{"error":"similarity search failed"}`, http.StatusInternalServerError)
		return
	}
	if memories == nil {
		memories = []Memory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Memories: memories})
}
