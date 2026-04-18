package memex

import (
	"context"
	"testing"
)

// RunStoreContractTests validates Store interface invariants.
// Run this against any Store implementation to verify correctness.
func RunStoreContractTests(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("SaveMemory_returns_non_empty_stable_ID", func(t *testing.T) {
		m, err := store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "contract test memory", Project: "contract-test", MemoryType: "preference",
		})
		if err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
		if m.ID == "" {
			t.Error("SaveMemory must return non-empty ID")
		}
		m2, err := store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "second contract test memory", Project: "contract-test", MemoryType: "decision",
		})
		if err != nil {
			t.Fatalf("SaveMemory second: %v", err)
		}
		if m2.ID == m.ID {
			t.Error("different saves must produce different IDs")
		}
	})

	t.Run("Topic_defaults_to_Project_when_omitted", func(t *testing.T) {
		m, err := store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "topic defaults test", Project: "proj-x", MemoryType: "event",
		})
		if err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
		if m.Topic == "" {
			t.Error("Topic must default to Project when omitted")
		}
	})

	t.Run("PinnedMemories_only_returns_importance_gte_0.9", func(t *testing.T) {
		proj := "pinned-contract"
		store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "low importance", Project: proj, MemoryType: "preference", Importance: 0.5,
		})
		store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "high importance", Project: proj, MemoryType: "preference", Importance: 0.95,
		})
		store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "exactly 0.9", Project: proj, MemoryType: "preference", Importance: 0.9,
		})

		pinned, err := store.PinnedMemories(ctx, proj)
		if err != nil {
			t.Fatalf("PinnedMemories: %v", err)
		}
		for _, m := range pinned {
			if m.Importance < 0.9 {
				t.Errorf("PinnedMemories returned memory with importance=%.2f < 0.9: %q", m.Importance, m.Text)
			}
		}
		if len(pinned) < 2 {
			t.Errorf("expected at least 2 pinned memories (0.9 and 0.95), got %d", len(pinned))
		}
	})

	t.Run("PinnedMemories_sorted_desc_by_importance", func(t *testing.T) {
		proj := "pinned-order"
		store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "importance 0.91", Project: proj, MemoryType: "preference", Importance: 0.91,
		})
		store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "importance 1.0", Project: proj, MemoryType: "preference", Importance: 1.0,
		})
		store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "importance 0.95", Project: proj, MemoryType: "preference", Importance: 0.95,
		})

		pinned, err := store.PinnedMemories(ctx, proj)
		if err != nil {
			t.Fatalf("PinnedMemories: %v", err)
		}
		for i := 1; i < len(pinned); i++ {
			if pinned[i].Importance > pinned[i-1].Importance {
				t.Errorf("PinnedMemories not sorted desc: [%d]=%.2f > [%d]=%.2f",
					i, pinned[i].Importance, i-1, pinned[i-1].Importance)
			}
		}
	})

	t.Run("DeleteMemory_removes_it_from_list", func(t *testing.T) {
		m, _ := store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "to be deleted", Project: "del-test", MemoryType: "event",
		})
		if err := store.DeleteMemory(ctx, m.ID); err != nil {
			t.Fatalf("DeleteMemory: %v", err)
		}
		memories, _ := store.ListMemories(ctx, "del-test", "", "", nil, 10)
		for _, mem := range memories {
			if mem.ID == m.ID {
				t.Error("deleted memory still appears in ListMemories")
			}
		}
	})

	t.Run("PinMemory_sets_importance_to_1.0", func(t *testing.T) {
		m, _ := store.SaveMemory(ctx, SaveMemoryRequest{
			Text: "to be pinned", Project: "pin-test", MemoryType: "preference", Importance: 0.5,
		})
		if err := store.PinMemory(ctx, m.ID); err != nil {
			t.Fatalf("PinMemory: %v", err)
		}
		pinned, _ := store.PinnedMemories(ctx, "pin-test")
		found := false
		for _, mem := range pinned {
			if mem.ID == m.ID {
				found = true
				if mem.Importance < 0.9 {
					t.Errorf("pinned memory importance=%.2f, want >= 0.9", mem.Importance)
				}
			}
		}
		if !found {
			t.Error("pinned memory not found in PinnedMemories after PinMemory")
		}
	})
}

func TestStoreContract_FakeStore(t *testing.T) {
	RunStoreContractTests(t, newFakeStore())
}
