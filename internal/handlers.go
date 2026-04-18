package memex

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type Handlers struct {
	store    Store
	kg       *KnowledgeGraph
	enricher *Enricher
}

func NewHandlers(store Store, kg *KnowledgeGraph, enricher *Enricher) *Handlers {
	return &Handlers{store: store, kg: kg, enricher: enricher}
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
	depth := clampInt(parseIntOrDefault(r.URL.Query().Get("depth"), 1), 1, 3)
	fanout := clampInt(parseIntOrDefault(r.URL.Query().Get("fanout"), 10), 1, 50)
	maxNeighbors := clampInt(parseIntOrDefault(r.URL.Query().Get("max_neighbors"), 100), 1, 500)
	predicateAllowlist := parsePredicateAllowlist(r.URL.Query().Get("predicates"))

	neighbors := []string{}
	if h.kg != nil {
		neighbors = h.expandNeighbors(entity, depth, fanout, maxNeighbors, predicateAllowlist)
	}
	// Trigger lazy LLM enrichment for function nodes surfaced by KG traversal.
	if h.enricher != nil && h.kg != nil {
		commitHash := h.kg.LatestCommitHash()
		for _, n := range neighbors {
			if strings.Contains(n, "::") { // function_id, not a file path
				h.enricher.EnrichAsync(r.Context(), n, project, commitHash)
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
		"depth":          depth,
		"fanout":         fanout,
		"max_neighbors":  maxNeighbors,
		"predicates":     mapKeysSorted(predicateAllowlist),
		"memories":       merged,
	})
}

func (h *Handlers) expandNeighbors(entity string, maxDepth, fanout, maxNeighbors int, allowlist map[string]bool) []string {
	type candidate struct {
		next     string
		priority int
	}
	type node struct {
		name  string
		depth int
	}
	queue := []node{{name: entity, depth: 0}}
	visited := map[string]bool{entity: true}
	neighborsSeen := map[string]bool{}
	var neighbors []string

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= maxDepth {
			continue
		}

		facts, err := h.kg.QueryEntity(cur.name, "")
		if err != nil {
			continue
		}
		candidatesByNext := map[string]candidate{}
		functionFileCache := map[string][]string{}
		addCandidate := func(next string, priority int) {
			if strings.TrimSpace(next) == "" || neighborsSeen[next] {
				return
			}
			if existing, ok := candidatesByNext[next]; ok {
				if priority < existing.priority {
					candidatesByNext[next] = candidate{next: next, priority: priority}
				}
				return
			}
			candidatesByNext[next] = candidate{next: next, priority: priority}
		}
		functionFiles := func(functionID string) []string {
			if cached, ok := functionFileCache[functionID]; ok {
				return cached
			}
			var out []string
			ffacts, err := h.kg.QueryEntity(functionID, "")
			if err == nil {
				for _, ff := range ffacts {
					if ff.Predicate == PredicateContainsFunction && ff.Object == functionID {
						out = append(out, ff.Subject)
					}
				}
			}
			functionFileCache[functionID] = out
			return out
		}
		addedThisNode := 0
		for _, f := range facts {
			next := ""
			priority := 99
			reverseCall := false
			switch {
			case f.Object == cur.name && f.Predicate == PredicateContainsFunction && allowlist[PredicateContainsFunction]:
				// Reverse traversal on contains lets function/package entities map back to file anchors.
				next = f.Subject
				priority = 0
			case f.Object == cur.name && f.Predicate == PredicateTestOf && allowlist[PredicateTestOf]:
				// Reverse traversal on test_of lets source files map to their test files.
				next = f.Subject
				priority = 0
			case f.Object == cur.name && f.Predicate == PredicateCalls && allowlist[PredicateCalls]:
				// Reverse traversal on calls lets retrieval surface callers, not only callees.
				next = f.Subject
				priority = 1
				reverseCall = true
			case f.Subject == cur.name && allowlist[f.Predicate]:
				next = f.Object
				priority = 2
			default:
				continue
			}
			addCandidate(next, priority)
			if reverseCall && strings.Contains(next, "::") {
				for _, file := range functionFiles(next) {
					addCandidate(file, priority)
				}
			}
		}

		candidates := make([]candidate, 0, len(candidatesByNext))
		for _, c := range candidatesByNext {
			candidates = append(candidates, c)
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].priority != candidates[j].priority {
				return candidates[i].priority < candidates[j].priority
			}
			return candidates[i].next < candidates[j].next
		})

		for _, c := range candidates {
			neighborsSeen[c.next] = true
			neighbors = append(neighbors, c.next)
			if len(neighbors) >= maxNeighbors {
				return neighbors
			}
			addedThisNode++
			if !visited[c.next] {
				visited[c.next] = true
				queue = append(queue, node{name: c.next, depth: cur.depth + 1})
			}
			if addedThisNode >= fanout {
				break
			}
		}
	}
	return neighbors
}

func parsePredicateAllowlist(raw string) map[string]bool {
	// Default retrieval expansion avoids unresolved edges to reduce noise.
	allow := map[string]bool{
		PredicateContainsFunction: true,
		PredicateCalls:            true,
		PredicateDependsOn:        true,
		PredicateTestOf:           true,
	}
	if strings.TrimSpace(raw) == "" {
		return allow
	}
	custom := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		custom[p] = true
	}
	if len(custom) == 0 {
		return allow
	}
	return custom
}

func parseIntOrDefault(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func mapKeysSorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
