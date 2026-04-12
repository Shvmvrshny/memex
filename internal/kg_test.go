package memex

import (
	"testing"
	"time"
)

func newTestKG(t *testing.T) *KnowledgeGraph {
	t.Helper()
	kg, err := NewKnowledgeGraph(":memory:")
	if err != nil {
		t.Fatalf("NewKnowledgeGraph: %v", err)
	}
	if err := kg.Init(); err != nil {
		t.Fatalf("kg.Init: %v", err)
	}
	t.Cleanup(func() { kg.db.Close() })
	return kg
}

func TestKG_RecordFact_Basic(t *testing.T) {
	kg := newTestKG(t)

	id, err := kg.RecordFact("alice", "works_on", "memex", "", "test", false)
	if err != nil {
		t.Fatalf("RecordFact: %v", err)
	}
	if id == "" {
		t.Error("RecordFact returned empty id")
	}
}

func TestKG_RecordFact_Idempotent(t *testing.T) {
	kg := newTestKG(t)

	id1, _ := kg.RecordFact("alice", "works_on", "memex", "", "test", false)
	id2, _ := kg.RecordFact("alice", "works_on", "memex", "", "test", false)

	if id1 != id2 {
		t.Errorf("identical active fact should return same id: got %q and %q", id1, id2)
	}
}

func TestKG_RecordFact_Singular_ClosesOldFact(t *testing.T) {
	kg := newTestKG(t)

	id1, _ := kg.RecordFact("alice", "works_on", "memex", "", "test", false)
	_, _ = kg.RecordFact("alice", "works_on", "palace", "", "test", true)

	// Old fact should now be closed
	facts, err := kg.QueryEntity("alice", "")
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	for _, f := range facts {
		if f.ID == id1 {
			t.Error("old fact with id1 should have been expired and not appear in current query")
		}
	}
	// New fact should be the only active one
	if len(facts) != 1 || facts[0].Object != "palace" {
		t.Errorf("expected 1 active fact with object=palace, got %+v", facts)
	}
}

func TestKG_QueryEntity_AsOf(t *testing.T) {
	kg := newTestKG(t)

	past := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)

	// Fact valid from 2h ago, closed 1h ago
	id1, _ := kg.RecordFact("bob", "role", "engineer", past, "test", false)
	kg.ExpireFact(id1, recent)

	// New fact from 1h ago, still active
	_, _ = kg.RecordFact("bob", "role", "lead", recent, "test", false)

	// As of "now" should return only "lead"
	current, err := kg.QueryEntity("bob", "")
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	if len(current) != 1 || current[0].Object != "lead" {
		t.Errorf("current facts: want [{role lead}], got %+v", current)
	}

	// As of the past (before closing) should return "engineer"
	historical, err := kg.QueryEntity("bob", past)
	if err != nil {
		t.Fatalf("QueryEntity as_of: %v", err)
	}
	found := false
	for _, f := range historical {
		if f.Object == "engineer" {
			found = true
		}
	}
	if !found {
		t.Errorf("historical query should include engineer fact, got %+v", historical)
	}
}

func TestKG_ExpireFact(t *testing.T) {
	kg := newTestKG(t)

	id, _ := kg.RecordFact("carol", "owns", "auth-service", "", "test", false)
	now := time.Now().UTC().Format(time.RFC3339)
	err := kg.ExpireFact(id, now)
	if err != nil {
		t.Fatalf("ExpireFact: %v", err)
	}

	facts, _ := kg.QueryEntity("carol", "")
	for _, f := range facts {
		if f.ID == id {
			t.Error("expired fact should not appear in current QueryEntity")
		}
	}
}

func TestKG_History(t *testing.T) {
	kg := newTestKG(t)

	id1, _ := kg.RecordFact("dan", "project", "alpha", "", "test", false)
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	kg.ExpireFact(id1, past)
	kg.RecordFact("dan", "project", "beta", past, "test", false)

	history, err := kg.History("dan")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) < 2 {
		t.Errorf("expected at least 2 history entries for dan, got %d", len(history))
	}
}

func TestKG_Stats(t *testing.T) {
	kg := newTestKG(t)

	kg.RecordFact("e1", "rel", "e2", "", "test", false)
	id, _ := kg.RecordFact("e1", "rel2", "e3", "", "test", false)
	now := time.Now().UTC().Format(time.RFC3339)
	kg.ExpireFact(id, now)

	stats, err := kg.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalFacts != 2 {
		t.Errorf("TotalFacts = %d, want 2", stats.TotalFacts)
	}
	if stats.ActiveFacts != 1 {
		t.Errorf("ActiveFacts = %d, want 1", stats.ActiveFacts)
	}
	if stats.ExpiredFacts != 1 {
		t.Errorf("ExpiredFacts = %d, want 1", stats.ExpiredFacts)
	}
}
