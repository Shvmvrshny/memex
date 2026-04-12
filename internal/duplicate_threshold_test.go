package memex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// newOneTurnFixture writes a single-turn JSONL to a temp file and returns the path.
func newOneTurnFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	line := `{"role":"user","content":"` + content + `"}` + "\n"
	os.WriteFile(path, []byte(line), 0644)
	return path
}

func TestDuplicateThreshold_ExactBoundary(t *testing.T) {
	text := "I prefer table-driven tests in Go. We always use them."

	tests := []struct {
		name      string
		score     float32
		wantSaved bool
	}{
		{"score_0.919_saves", 0.919, true},
		{"score_0.920_skips", 0.920, false},
		{"score_0.921_skips", 0.921, false},
		{"score_0.950_skips", 0.950, false},
		{"score_0.000_saves", 0.000, true},
		{"no_similar_saves", -1, true}, // -1 = FindSimilar returns empty
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := newOneTurnFixture(t, text)

			store := &mockMinerStore{}
			store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
				if tc.score < 0 {
					return []Memory{}, nil
				}
				return []Memory{{Text: "similar text", MemoryType: "preference", Score: tc.score}}, nil
			}

			miner := NewMiner(store)
			requests, err := miner.MineTranscript(path, "memex")
			if err != nil {
				t.Fatalf("MineTranscript: %v", err)
			}

			saved := len(requests) > 0
			if saved != tc.wantSaved {
				t.Errorf("score=%.3f: saved=%v, want saved=%v", tc.score, saved, tc.wantSaved)
			}
		})
	}
}

func TestDuplicateThreshold_FindSimilarError_Skips(t *testing.T) {
	path := newOneTurnFixture(t, "I prefer table-driven tests in Go. We always use them.")

	store := &mockMinerStore{}
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		return nil, fmt.Errorf("embedding service unavailable")
	}

	miner := NewMiner(store)
	requests, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) != 0 {
		t.Errorf("FindSimilar error should cause skip (safe default), got %d saved", len(requests))
	}
}
