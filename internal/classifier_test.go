package memex

import (
	"testing"
)

func TestClassifier_Decision(t *testing.T) {
	c := NewClassifier()
	memType, confidence := c.Classify("We decided to go with Qdrant because it has better performance than Redis for vector search.")
	if memType != "decision" {
		t.Errorf("type = %q, want decision", memType)
	}
	if confidence < 0.3 {
		t.Errorf("confidence = %.2f, want >= 0.3", confidence)
	}
}

func TestClassifier_Preference(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("I prefer table-driven tests in Go. We always use snake_case for variable names.")
	if memType != "preference" {
		t.Errorf("type = %q, want preference", memType)
	}
}

func TestClassifier_Discovery(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("It works! Turns out the trick is setting journal_mode=WAL before any reads.")
	if memType != "discovery" {
		t.Errorf("type = %q, want discovery", memType)
	}
}

func TestClassifier_Problem(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("There's a bug in the auth middleware. The error is 'nil pointer dereference' and it keeps crashing.")
	if memType != "problem" {
		t.Errorf("type = %q, want problem", memType)
	}
}

func TestClassifier_Problem_WithResolution_BecomesDiscovery(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("There was a bug in the embed call. Fixed it by switching to nomic-embed-text. Now it works perfectly.")
	if memType != "discovery" {
		t.Errorf("type = %q, want discovery (resolved problem)", memType)
	}
}

func TestClassifier_Procedure(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("Steps to deploy: first run go build, then run docker compose up -d, then check the health endpoint.")
	if memType != "procedure" {
		t.Errorf("type = %q, want procedure", memType)
	}
}

func TestClassifier_Context(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("Alice works on the auth team and reports to Bob. She is responsible for the SSO integration.")
	if memType != "context" {
		t.Errorf("type = %q, want context", memType)
	}
}

func TestClassifier_Rationale(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("The reason we chose SQLite over Redis is that we need temporal queries and SQLite supports them natively. We rejected Redis because it lacks SQL.")
	if memType != "rationale" {
		t.Errorf("type = %q, want rationale", memType)
	}
}

func TestClassifier_Event(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("We deployed version 2.0 to production last week. The sprint ended and we shipped all milestones.")
	if memType != "event" {
		t.Errorf("type = %q, want event", memType)
	}
}

func TestClassifier_Advice(t *testing.T) {
	c := NewClassifier()
	memType, _ := c.Classify("You should always run go vet before committing. Best practice is to use structured logging.")
	if memType != "advice" {
		t.Errorf("type = %q, want advice", memType)
	}
}

func TestClassifier_BelowMinConfidence(t *testing.T) {
	c := NewClassifier()
	_, confidence := c.Classify("Hello world")
	if confidence >= 0.3 {
		t.Errorf("generic text confidence = %.2f, expected < 0.3", confidence)
	}
}
