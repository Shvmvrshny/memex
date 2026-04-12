package memex

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func BenchmarkClassifierLargeTranscript(b *testing.B) {
	c := NewClassifier()
	turns := make([]string, 1000)
	samples := []string{
		"We decided to use SQLite WAL for temporal queries because single-writer is fine.",
		"I prefer table-driven tests in Go. We always use them for classifier logic.",
		"There was a bug in the embed call. Fixed it by switching to nomic-embed-text.",
		"Steps to deploy: first run docker compose, then start Ollama, then run memex serve.",
		"Alice owns the auth team and is responsible for the SSO integration.",
	}
	for i := range turns {
		turns[i] = samples[i%len(samples)]
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, turn := range turns {
			c.Classify(turn)
		}
	}
}

func BenchmarkKGSingularReplacement(b *testing.B) {
	kg, err := NewKnowledgeGraph(":memory:")
	if err != nil {
		b.Fatalf("NewKnowledgeGraph: %v", err)
	}
	if err := kg.Init(); err != nil {
		b.Fatalf("kg.Init: %v", err)
	}
	defer kg.db.Close()

	kg.RecordFact("service", "uses", "sqlite", "", "bench", true)

	objects := []string{"qdrant", "postgres", "sqlite", "redis", "qdrant"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := objects[i%len(objects)]
		kg.RecordFact("service", "uses", obj, "", "bench", true)
	}
}

func BenchmarkPinnedMemories1000(b *testing.B) {
	store := newFakeStore()
	ctx := context.Background()

	for i := 0; i < 1000; i++ {
		imp := float32(0.5)
		if i%10 == 0 {
			imp = 0.95
		}
		store.SaveMemory(ctx, SaveMemoryRequest{
			Text:       fmt.Sprintf("memory %d with some content about the project architecture", i),
			Project:    "bench-project",
			MemoryType: "preference",
			Importance: imp,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.PinnedMemories(ctx, "bench-project")
	}
}

func BenchmarkSessionStartHook(b *testing.B) {
	identity := "I am Shivam. I build developer tools. Primary project is memex."
	pinned := make([]Memory, 10)
	for i := range pinned {
		pinned[i] = Memory{
			Text:       fmt.Sprintf("pinned preference %d about how we approach testing and architecture", i),
			MemoryType: "preference",
			Importance: 1.0,
		}
	}
	semantic := make([]Memory, 5)
	for i := range semantic {
		semantic[i] = Memory{
			Text:       fmt.Sprintf("semantic context memory %d about project structure and conventions", i),
			MemoryType: "decision",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildMemoryContext(identity, pinned, semantic)
	}
}

func BenchmarkMineDirectory(b *testing.B) {
	c := NewClassifier()
	sessions := make([][]string, 100)
	baseTurns := []string{
		"We decided to use Qdrant because it has better vector search performance than Redis.",
		"I prefer table-driven tests in Go. We always use them for classifier logic.",
		"Fixed the embed bug by switching to nomic-embed-text. Now it works perfectly.",
		"Steps to deploy: first run docker compose, then start Ollama, then run memex serve.",
		"There is a bug in the auth middleware. Error: nil pointer dereference keeps crashing.",
		"The reason we chose SQLite over Redis is that we need temporal queries.",
		"Alice owns the auth team and is responsible for the SSO integration.",
		"We shipped version 2.0 last week. The sprint ended and we delivered all milestones.",
		"You should always run go vet before committing. Best practice is structured logging.",
		"It works! Turns out the trick is setting journal_mode=WAL before any reads.",
	}
	for i := range sessions {
		sessions[i] = baseTurns
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, session := range sessions {
			for _, turn := range session {
				if len(strings.TrimSpace(turn)) >= 20 {
					c.Classify(turn)
				}
			}
		}
	}
}

func TestBenchmarkBudgets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark budget test in short mode")
	}

	t.Run("KG_singular_replace_under_5ms", func(t *testing.T) {
		kg, _ := NewKnowledgeGraph(":memory:")
		kg.Init()
		defer kg.db.Close()
		kg.RecordFact("s", "p", "old", "", "test", true)

		start := time.Now()
		for i := 0; i < 100; i++ {
			kg.RecordFact("s", "p", fmt.Sprintf("obj%d", i), "", "test", true)
		}
		avg := time.Since(start) / 100
		if avg > 5*time.Millisecond {
			t.Errorf("KG singular replace avg=%v, budget=5ms", avg)
		}
	})

	t.Run("session_start_context_build_under_1ms", func(t *testing.T) {
		pinned := make([]Memory, 10)
		for i := range pinned {
			pinned[i] = Memory{Text: "pinned memory", MemoryType: "preference"}
		}
		semantic := make([]Memory, 5)
		for i := range semantic {
			semantic[i] = Memory{Text: "context memory", MemoryType: "decision"}
		}

		start := time.Now()
		for i := 0; i < 1000; i++ {
			buildMemoryContext("identity text", pinned, semantic)
		}
		avg := time.Since(start) / 1000
		if avg > 1*time.Millisecond {
			t.Errorf("buildMemoryContext avg=%v, budget=1ms", avg)
		}
	})
}
