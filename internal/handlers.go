package memex

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type Handlers struct {
	store Store
	kg    *KnowledgeGraph
}

func NewHandlers(store Store, kg *KnowledgeGraph) *Handlers {
	return &Handlers{store: store, kg: kg}
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
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
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		writeJSONError(w, "text is required", http.StatusBadRequest)
		return
	}
	if !ValidMemoryTypes[req.MemoryType] {
		writeJSONError(w, "memory_type is required and must be one of: decision, preference, event, discovery, advice, problem, context, procedure, rationale", http.StatusBadRequest)
		return
	}

	memory, err := h.store.SaveMemory(r.Context(), req)
	if err != nil {
		writeJSONError(w, "failed to save memory", http.StatusInternalServerError)
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
	var tags []string
	if t := r.URL.Query().Get("tag"); t != "" {
		tags = strings.Split(t, ",")
	}

	var (
		memories []Memory
		err      error
	)
	if query == "" {
		memories, err = h.store.ListMemories(r.Context(), project, memoryType, topic, tags, limit)
	} else {
		memories, err = h.store.SearchMemories(r.Context(), query, project, memoryType, topic, tags, limit)
	}
	if err != nil {
		writeJSONError(w, "search failed", http.StatusInternalServerError)
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
		writeJSONError(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteMemory(r.Context(), id); err != nil {
		writeJSONError(w, "failed to delete memory", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Summarize(w http.ResponseWriter, r *http.Request) {
	var req SaveMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		writeJSONError(w, "text is required", http.StatusBadRequest)
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
		writeJSONError(w, "failed to save summary", http.StatusInternalServerError)
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
		writeJSONError(w, "project is required", http.StatusBadRequest)
		return
	}
	memories, err := h.store.PinnedMemories(r.Context(), project)
	if err != nil {
		writeJSONError(w, "failed to fetch pinned memories", http.StatusInternalServerError)
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
		writeJSONError(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := h.store.PinMemory(r.Context(), id); err != nil {
		writeJSONError(w, "failed to pin memory", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MineTranscript handles POST /mine/transcript
// Body: {"path": "...", "project": "..."}
// Starts transcript mining asynchronously, returns 202 immediately.
func (h *Handlers) MineTranscript(w http.ResponseWriter, r *http.Request) {
	var req MineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		writeJSONError(w, "path is required", http.StatusBadRequest)
		return
	}
	project := req.Project
	if project == "" {
		project = "default"
	}

	miner := NewMiner(h.store)
	go func() {
		miner.MineTranscript(req.Path, project)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(MineResponse{Status: "mining started", Path: req.Path})
}

// ExpandSearch is the entry-point retrieval endpoint.
// GET /memories/expand?entity=X&project=Y&limit=N
//
// It walks the KG outward from the named entity, builds an expanded query
// from the entity name and all its direct neighbor objects, then runs a
// semantic search so results are anchored to the graph structure rather
// than raw cosine distance from the raw query string.
func (h *Handlers) ExpandSearch(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("entity")
	if entity == "" {
		writeJSONError(w, "entity is required", http.StatusBadRequest)
		return
	}
	project := r.URL.Query().Get("project")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	// Walk one hop outward from entity in the KG.
	var neighbors []string
	if h.kg != nil {
		facts, err := h.kg.QueryEntity(entity, "")
		if err == nil {
			for _, f := range facts {
				if f.Subject == entity {
					neighbors = append(neighbors, f.Object)
				}
			}
		}
	}

	// Build expanded query: entity name + all neighbor names.
	parts := append([]string{entity}, neighbors...)
	expandedQuery := strings.Join(parts, " ")

	// L1: pinned structural anchors for the project (importance >= 0.9).
	// These are guaranteed entry points regardless of cosine distance.
	pinned, _ := h.store.PinnedMemories(r.Context(), project)

	// L2: semantic search using the KG-expanded query.
	semantic, err := h.store.SearchMemories(r.Context(), expandedQuery, project, "", "", nil, limit)
	if err != nil {
		writeJSONError(w, "expand search failed", http.StatusInternalServerError)
		return
	}

	// Merge L1 + L2, deduplicate by ID, sort by importance desc.
	seen := make(map[string]bool)
	var merged []Memory
	for _, m := range pinned {
		if !seen[m.ID] {
			seen[m.ID] = true
			merged = append(merged, m)
		}
	}
	for _, m := range semantic {
		if !seen[m.ID] {
			seen[m.ID] = true
			merged = append(merged, m)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Importance > merged[j].Importance
	})
	if len(merged) > limit {
		merged = merged[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"entity":         entity,
		"neighbors":      neighbors,
		"expanded_query": expandedQuery,
		"memories":       merged,
	})
}

// FindSimilar returns the most similar memories to the given text.
// GET /memories/similar?text=X&project=Y&limit=5
func (h *Handlers) FindSimilar(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		writeJSONError(w, "text is required", http.StatusBadRequest)
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
		writeJSONError(w, "similarity search failed", http.StatusInternalServerError)
		return
	}
	if memories == nil {
		memories = []Memory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Memories: memories})
}
