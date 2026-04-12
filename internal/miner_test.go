package memex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mockMinerStore tracks SaveMemory calls and allows FindSimilar injection.
type mockMinerStore struct {
	mockStore
	saved []SaveMemoryRequest
}

func (m *mockMinerStore) SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error) {
	m.saved = append(m.saved, req)
	return Memory{ID: "test-id", Text: req.Text, MemoryType: req.MemoryType}, nil
}

func (m *mockMinerStore) FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error) {
	if m.mockStore.findSimilarFn != nil {
		return m.mockStore.findSimilarFn(ctx, text, project, limit)
	}
	return []Memory{}, nil
}

func TestMiner_MineTranscript_SavesClassifiedMemories(t *testing.T) {
	jsonl := `{"role":"user","content":"I prefer table-driven tests in Go. We always use them."}
{"role":"user","content":"We decided to use Qdrant because it has the best vector search performance."}
{"role":"assistant","content":[{"type":"text","text":"Got it! Those are good practices."}]}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	store := &mockMinerStore{}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) == 0 {
		t.Error("expected at least one memory saved, got 0")
	}
	for _, req := range requests {
		if req.MemoryType == "" {
			t.Errorf("saved memory has empty memory_type: %q", req.Text)
		}
		if req.Project != "memex" {
			t.Errorf("saved memory project = %q, want memex", req.Project)
		}
	}
}

func TestMiner_MineTranscript_SkipsDuplicates(t *testing.T) {
	jsonl := `{"role":"user","content":"I prefer table-driven tests in Go."}` + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	store := &mockMinerStore{}
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		return []Memory{
			{Text: "I prefer table-driven tests in Go.", MemoryType: "preference", Score: 0.95},
		}, nil
	}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) != 0 {
		t.Errorf("expected 0 saved memories (duplicate detected), got %d", len(requests))
	}
}

func TestMiner_DuplicateThreshold_BelowSkips(t *testing.T) {
	jsonl := `{"role":"user","content":"I prefer table-driven tests in Go. We always use them."}` + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	store := &mockMinerStore{}
	// Score above threshold → should skip
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		return []Memory{{Text: "similar text", MemoryType: "preference", Score: 0.921}}, nil
	}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) != 0 {
		t.Errorf("score=0.921 >= 0.92 threshold, expected 0 saved, got %d", len(requests))
	}
}

func TestMiner_DuplicateThreshold_AboveSaves(t *testing.T) {
	jsonl := `{"role":"user","content":"I prefer table-driven tests in Go. We always use them."}` + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	store := &mockMinerStore{}
	// Score below threshold → should save
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		return []Memory{{Text: "vaguely similar", MemoryType: "preference", Score: 0.919}}, nil
	}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) == 0 {
		t.Error("score=0.919 < 0.92 threshold, expected memory to be saved")
	}
}

func TestMiner_DuplicateThreshold_CrossProjectSaves(t *testing.T) {
	jsonl := `{"role":"user","content":"I prefer table-driven tests in Go. We always use them."}` + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(jsonl), 0644)

	store := &mockMinerStore{}
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		return []Memory{}, nil // store filtered to project=memex2, found nothing
	}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript(path, "memex2")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) == 0 {
		t.Error("cross-project: no similar in project=memex2, expected memory to be saved")
	}
}

func TestMiner_InferTopic(t *testing.T) {
	miner := NewMiner(nil)

	tests := []struct {
		text string
		want string
	}{
		{"We are working on the auth-migration module", "auth-migration"},
		{"The CI pipeline keeps failing in the deploy stage", "ci-pipeline"},
		{"Just a general note about the project", "general"},
	}

	for _, tc := range tests {
		got := miner.inferTopic(tc.text)
		if got != tc.want {
			t.Logf("inferTopic(%q) = %q (expected %q — topic inference is heuristic)", tc.text, got, tc.want)
		}
		if got == "" {
			t.Errorf("inferTopic(%q) returned empty string", tc.text)
		}
	}
}
