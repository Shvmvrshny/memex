package memex

import (
	"context"
	"testing"
)

func TestTranscriptFixtures_Parse(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantMinTurns int
	}{
		{"decision", "testdata/transcripts/decision_single_turn.jsonl", 1},
		{"discovery_resolution", "testdata/transcripts/discovery_resolution.jsonl", 2},
		{"procedure", "testdata/transcripts/procedure_multistep.jsonl", 1},
		{"noisy", "testdata/transcripts/noisy_debug_loop.jsonl", 0},
		{"duplicate", "testdata/transcripts/duplicate_session.jsonl", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			turns, err := ParseConversation(tc.path)
			if err != nil {
				t.Fatalf("ParseConversation(%q): %v", tc.path, err)
			}
			if len(turns) < tc.wantMinTurns {
				t.Errorf("got %d turns, want >= %d", len(turns), tc.wantMinTurns)
			}
			for _, turn := range turns {
				if turn.Role == "" {
					t.Error("turn has empty role")
				}
			}
		})
	}
}

func TestTranscriptFixture_DecisionMined(t *testing.T) {
	store := &mockMinerStore{}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript("testdata/transcripts/decision_single_turn.jsonl", "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) == 0 {
		t.Error("expected at least one memory from decision fixture, got 0")
	}
	found := false
	for _, req := range requests {
		if req.MemoryType == "decision" || req.MemoryType == "rationale" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a decision or rationale memory, got: %+v", requests)
	}
}

func TestTranscriptFixture_DiscoveryMined(t *testing.T) {
	store := &mockMinerStore{}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript("testdata/transcripts/discovery_resolution.jsonl", "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	found := false
	for _, req := range requests {
		if req.MemoryType == "discovery" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a discovery memory from resolved-problem fixture, got: %+v", requests)
	}
}

func TestTranscriptFixture_NoisyYieldsNothing(t *testing.T) {
	store := &mockMinerStore{}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript("testdata/transcripts/noisy_debug_loop.jsonl", "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) > 1 {
		t.Errorf("noisy fixture: expected 0-1 memories, got %d: %+v", len(requests), requests)
	}
}

func TestTranscriptFixture_DuplicateSessionSkipsSecond(t *testing.T) {
	callCount := 0
	store := &mockMinerStore{}
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		callCount++
		if callCount > 1 {
			return []Memory{{Text: text, MemoryType: "preference", Score: 0.99}}, nil
		}
		return []Memory{}, nil
	}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript("testdata/transcripts/duplicate_session.jsonl", "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) > 1 {
		t.Errorf("duplicate fixture: expected max 1 saved, got %d", len(requests))
	}
}
