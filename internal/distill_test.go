package memex

import (
	"strings"
	"testing"
	"time"
)

func TestDistill_ContainsRequiredFields(t *testing.T) {
	events := []TraceEvent{
		{Tool: "Read", Input: "internal/qdrant.go", DurationMs: 12, Timestamp: time.Now()},
		{Tool: "Grep", Input: "SaveMemory", DurationMs: 8, Timestamp: time.Now()},
		{Tool: "Edit", Input: "internal/qdrant.go", DurationMs: 24, Timestamp: time.Now()},
		{Tool: "Read", Input: "internal/models.go", DurationMs: 9, Timestamp: time.Now()},
		{Tool: "Bash", Input: "go test ./...", DurationMs: 340, Timestamp: time.Now()},
	}
	summary := Distill("memex", events)

	checks := []string{"project:", "tools:", "Read x2", "Grep x1", "Edit x1", "Bash x1"}
	for _, want := range checks {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q\ngot: %s", want, summary)
		}
	}
}

func TestDistill_EmptyEvents(t *testing.T) {
	summary := Distill("memex", nil)
	if !strings.Contains(summary, "project: memex") {
		t.Errorf("expected project in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "tools: 0") {
		t.Errorf("expected 0 tools in summary, got: %s", summary)
	}
}
