package memex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConversation_ReturnsBothRoles(t *testing.T) {
	jsonl := `{"role":"user","content":"I prefer table-driven tests in Go."}
{"role":"assistant","content":[{"type":"text","text":"Noted! Table-driven tests are a great Go pattern."}]}
{"role":"user","content":"We decided to use Qdrant because it has better perf."}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	turns, err := ParseConversation(path)
	if err != nil {
		t.Fatalf("ParseConversation: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turns count = %d, want 3", len(turns))
	}
	if turns[0].Role != "user" || turns[0].Text != "I prefer table-driven tests in Go." {
		t.Errorf("turn 0: got {%q, %q}", turns[0].Role, turns[0].Text)
	}
	if turns[1].Role != "assistant" {
		t.Errorf("turn 1 role = %q, want assistant", turns[1].Role)
	}
	if turns[2].Role != "user" {
		t.Errorf("turn 2 role = %q, want user", turns[2].Role)
	}
}

func TestParseConversation_SkipsToolRoles(t *testing.T) {
	jsonl := `{"role":"user","content":"run git status"}
{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}}]}
{"role":"tool","content":[{"type":"tool_result","tool_use_id":"t1","content":"nothing to commit"}]}
{"role":"user","content":"thanks"}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	turns, err := ParseConversation(path)
	if err != nil {
		t.Fatalf("ParseConversation: %v", err)
	}
	for _, turn := range turns {
		if turn.Role == "tool" {
			t.Error("tool role should be skipped")
		}
	}
}

func TestParseConversation_AssistantContentArray(t *testing.T) {
	jsonl := `{"role":"assistant","content":[{"type":"text","text":"Hello"},{"type":"text","text":" world"}]}` + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	turns, err := ParseConversation(path)
	if err != nil {
		t.Fatalf("ParseConversation: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns count = %d, want 1", len(turns))
	}
	if turns[0].Text != "Hello world" {
		t.Errorf("text = %q, want 'Hello world'", turns[0].Text)
	}
}

func TestParseTranscript_ExtractsReasoning(t *testing.T) {
	jsonl := `{"role":"user","content":"review this PR"}
{"role":"assistant","content":[{"type":"text","text":"I need to check the diff first."},{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"git diff"}}]}
{"role":"tool","content":[{"type":"tool_result","tool_use_id":"tu_1","content":"+ line added"}]}
{"role":"assistant","content":[{"type":"text","text":"Now I will read the file."},{"type":"tool_use","id":"tu_2","name":"Read","input":{"file_path":"main.go"}}]}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte(jsonl), 0644); err != nil {
		t.Fatal(err)
	}

	reasoning, err := ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	if got := reasoning[0]; got != "I need to check the diff first." {
		t.Errorf("turn 0: got %q, want 'I need to check the diff first.'", got)
	}
	if got := reasoning[1]; got != "Now I will read the file." {
		t.Errorf("turn 1: got %q, want 'Now I will read the file.'", got)
	}
}

func TestParseTranscript_MissingFile(t *testing.T) {
	_, err := ParseTranscript("/nonexistent/path/session.jsonl")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseTranscript_NoToolCalls(t *testing.T) {
	jsonl := `{"role":"user","content":"what is 2+2"}
{"role":"assistant","content":[{"type":"text","text":"4"}]}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	reasoning, err := ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	if len(reasoning) != 0 {
		t.Errorf("got %d entries, want 0", len(reasoning))
	}
}
