package memex

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestKGHandlers(t *testing.T) *KGHandlers {
	t.Helper()
	kg, err := NewKnowledgeGraph(":memory:")
	if err != nil {
		t.Fatalf("NewKnowledgeGraph: %v", err)
	}
	if err := kg.Init(); err != nil {
		t.Fatalf("kg.Init: %v", err)
	}
	t.Cleanup(func() { kg.db.Close() })
	return NewKGHandlers(kg)
}

func TestKGHandlers_RecordFact(t *testing.T) {
	h := newTestKGHandlers(t)

	body := `{"subject":"alice","predicate":"works_on","object":"memex","source":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/facts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.RecordFact(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["id"] == "" {
		t.Error("response missing id")
	}
}

func TestKGHandlers_RecordFact_MissingFields(t *testing.T) {
	h := newTestKGHandlers(t)

	body := `{"subject":"alice"}` // missing predicate and object
	req := httptest.NewRequest(http.MethodPost, "/facts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.RecordFact(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestKGHandlers_QueryEntity(t *testing.T) {
	h := newTestKGHandlers(t)

	// Seed a fact
	h.kg.RecordFact("alice", "works_on", "memex", "", "test", false)

	req := httptest.NewRequest(http.MethodGet, "/facts?subject=alice", nil)
	w := httptest.NewRecorder()

	h.QueryEntity(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Facts []Fact `json:"facts"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Facts) != 1 {
		t.Errorf("facts count = %d, want 1", len(resp.Facts))
	}
}

func TestKGHandlers_ExpireFact(t *testing.T) {
	h := newTestKGHandlers(t)

	id, _ := h.kg.RecordFact("bob", "role", "engineer", "", "test", false)

	req := httptest.NewRequest(http.MethodDelete, "/facts/"+id, nil)
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()

	h.ExpireFact(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestKGHandlers_History(t *testing.T) {
	h := newTestKGHandlers(t)

	h.kg.RecordFact("carol", "project", "alpha", "", "test", false)
	h.kg.RecordFact("carol", "project", "beta", "", "test", true)

	req := httptest.NewRequest(http.MethodGet, "/facts/timeline?entity=carol", nil)
	w := httptest.NewRecorder()

	h.History(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Facts []Fact `json:"facts"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Facts) < 2 {
		t.Errorf("history count = %d, want >= 2", len(resp.Facts))
	}
}

func TestKGHandlers_Stats(t *testing.T) {
	h := newTestKGHandlers(t)

	h.kg.RecordFact("x", "rel", "y", "", "test", false)

	req := httptest.NewRequest(http.MethodGet, "/facts/stats", nil)
	w := httptest.NewRecorder()

	h.Stats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var stats KGStats
	json.NewDecoder(w.Body).Decode(&stats)
	if stats.TotalFacts != 1 {
		t.Errorf("TotalFacts = %d, want 1", stats.TotalFacts)
	}
}

func TestKGHandlers_Architecture(t *testing.T) {
	h := newTestKGHandlers(t)
	_, _ = h.kg.RecordFactScoped(Fact{
		Subject:   "github.com/shivamvarshney/memex/internal",
		Predicate: PredicateDependsOn,
		Object:    "net/http",
		Source:    "ast",
	}, false)

	req := httptest.NewRequest(http.MethodGet, "/facts/architecture?project=memex&limit=5", nil)
	w := httptest.NewRecorder()
	h.Architecture(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Packages []PackageDependency `json:"packages"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Packages) == 0 {
		t.Fatalf("expected at least one package in architecture response")
	}
}
