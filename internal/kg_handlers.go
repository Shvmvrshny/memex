package memex

import (
	"encoding/json"
	"net/http"
	"strings"
)

// KGHandlers holds HTTP handlers for the /facts/* routes.
type KGHandlers struct {
	kg *KnowledgeGraph
}

func NewKGHandlers(kg *KnowledgeGraph) *KGHandlers {
	return &KGHandlers{kg: kg}
}

// RecordFact handles POST /facts
// Body: {"subject","predicate","object","valid_from","source","singular"}
func (h *KGHandlers) RecordFact(w http.ResponseWriter, r *http.Request) {
	var req RecordFactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Subject == "" || req.Predicate == "" || req.Object == "" {
		http.Error(w, "subject, predicate, and object are required", http.StatusBadRequest)
		return
	}
	id, err := h.kg.RecordFact(req.Subject, req.Predicate, req.Object, req.ValidFrom, req.Source, req.Singular)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// QueryEntity handles GET /facts?subject=X&as_of=Y
func (h *KGHandlers) QueryEntity(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("subject")
	if entity == "" {
		entity = r.URL.Query().Get("entity")
	}
	asOf := r.URL.Query().Get("as_of")

	facts, err := h.kg.QueryEntity(entity, asOf)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if facts == nil {
		facts = []Fact{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"facts": facts})
}

// ExpireFact handles DELETE /facts/:id
func (h *KGHandlers) ExpireFact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		// Fallback: parse from URL path
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/facts/"), "/")
		if len(parts) > 0 {
			id = parts[0]
		}
	}
	if id == "" {
		http.Error(w, "fact id required", http.StatusBadRequest)
		return
	}
	if err := h.kg.ExpireFact(id, ""); err != nil {
		if strings.Contains(err.Error(), "not found or already expired") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "expired"})
}

// History handles GET /facts/timeline?entity=X
func (h *KGHandlers) History(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("entity")
	facts, err := h.kg.History(entity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if facts == nil {
		facts = []Fact{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"facts": facts})
}

// Stats handles GET /facts/stats
func (h *KGHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.kg.Stats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
