package memex

import (
	"os"
	"path/filepath"
	"testing"
)

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
