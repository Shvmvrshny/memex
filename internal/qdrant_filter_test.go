package memex

import (
	"encoding/json"
	"testing"
)

func mustClauseCount(filter map[string]any) int {
	if filter == nil {
		return 0
	}
	must, ok := filter["must"].([]map[string]any)
	if !ok {
		return 0
	}
	return len(must)
}

func TestBuildFilter_ProjectOnly(t *testing.T) {
	f := buildFilter("memex", "", "", nil)
	if f == nil {
		t.Fatal("expected non-nil filter for project only")
	}
	if n := mustClauseCount(f); n != 1 {
		t.Errorf("project-only filter: want 1 must clause, got %d", n)
	}
	must := f["must"].([]map[string]any)
	if must[0]["key"] != "project" {
		t.Errorf("clause key = %q, want project", must[0]["key"])
	}
}

func TestBuildFilter_ProjectAndType(t *testing.T) {
	f := buildFilter("memex", "decision", "", nil)
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if n := mustClauseCount(f); n != 2 {
		t.Errorf("project+type filter: want 2 must clauses, got %d", n)
	}
}

func TestBuildFilter_ProjectAndTopic(t *testing.T) {
	f := buildFilter("memex", "", "testing", nil)
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if n := mustClauseCount(f); n != 2 {
		t.Errorf("project+topic filter: want 2 must clauses, got %d", n)
	}
}

func TestBuildFilter_AllThree(t *testing.T) {
	f := buildFilter("memex", "preference", "testing", nil)
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if n := mustClauseCount(f); n != 3 {
		t.Errorf("project+type+topic filter: want 3 must clauses, got %d", n)
	}
}

func TestBuildFilter_Empty_ReturnsNil(t *testing.T) {
	f := buildFilter("", "", "", nil)
	if f != nil {
		t.Errorf("all-empty filter should return nil, got %+v", f)
	}
}

func TestBuildFilter_TypeOnly(t *testing.T) {
	f := buildFilter("", "decision", "", nil)
	if f == nil {
		t.Fatal("expected non-nil filter for type only")
	}
	if n := mustClauseCount(f); n != 1 {
		t.Errorf("type-only filter: want 1 must clause, got %d", n)
	}
}

func TestBuildFilter_SerializesCorrectly(t *testing.T) {
	f := buildFilter("memex", "decision", "kg", nil)
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("json.Marshal filter: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal filter: %v", err)
	}

	must, ok := decoded["must"].([]any)
	if !ok {
		t.Fatalf("decoded filter missing 'must' array")
	}
	if len(must) != 3 {
		t.Errorf("decoded must clauses: want 3, got %d", len(must))
	}

	for i, clause := range must {
		m, ok := clause.(map[string]any)
		if !ok {
			t.Fatalf("clause %d is not a map", i)
		}
		if _, ok := m["key"]; !ok {
			t.Errorf("clause %d missing 'key'", i)
		}
		if _, ok := m["match"]; !ok {
			t.Errorf("clause %d missing 'match'", i)
		}
	}
}
