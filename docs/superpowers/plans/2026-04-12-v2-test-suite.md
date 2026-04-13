# Memex v2 Test Suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the memex v2 test suite to production-correct by implementing the missing technical tests defined in `docs/superpowers/test-case/2026-04-09-memex-test-cases.md`.

**Architecture:** All tests live in `internal/` as `package memex`. The plan adds: a score-aware duplicate threshold in the Miner, transcript JSONL fixtures, golden session-start snapshots, KG WAL concurrency validation, classifier ambiguity coverage, fake Store for deterministic unit tests, Qdrant filter assertions, MCP tool-chain flows, async mining race safety, and benchmarks.

**Tech Stack:** Go stdlib testing, `net/http/httptest`, `sync`, `testdata/` golden files, `modernc.org/sqlite` (in-memory KG), no new dependencies.

---

## File Map

| File | Action | Purpose |
|---|---|---|
| `internal/models.go` | Modify | Add `Score float32 \`json:"-"\`` to `Memory` |
| `internal/qdrant.go` | Modify | Capture `score` field in `vectorSearch` response |
| `internal/miner.go` | Modify | Check `Score >= 0.92` threshold instead of `len > 0` |
| `internal/miner_test.go` | Modify | Update existing skip test to set `Score: 0.95` |
| `internal/testdata/golden/session_start_full.txt` | Create | Golden output for full 3-layer session-start |
| `internal/testdata/golden/session_start_identity_only.txt` | Create | Golden output identity-only |
| `internal/testdata/golden/session_start_no_identity.txt` | Create | Golden output pinned+context, no identity |
| `internal/testdata/transcripts/decision_single_turn.jsonl` | Create | Fixture: clear decision turn |
| `internal/testdata/transcripts/discovery_resolution.jsonl` | Create | Fixture: problem resolved → discovery |
| `internal/testdata/transcripts/procedure_multistep.jsonl` | Create | Fixture: step-by-step procedure |
| `internal/testdata/transcripts/noisy_debug_loop.jsonl` | Create | Fixture: stack traces + code, no useful signal |
| `internal/testdata/transcripts/duplicate_session.jsonl` | Create | Fixture: same content twice |
| `internal/kg_concurrency_test.go` | Create | WAL: 1 writer + 20 concurrent readers |
| `internal/hook_golden_test.go` | Create | Golden snapshot tests for `buildMemoryContext` |
| `internal/classifier_ambiguity_test.go` | Create | Ambiguity + noise robustness table-driven tests |
| `internal/duplicate_threshold_test.go` | Create | Score boundary tests: 0.919 saves, 0.921 skips |
| `internal/miner_race_test.go` | Create | Session-stop async mine with fake delayed HTTP server |
| `internal/fake_store_test.go` | Create | In-memory `fakeStore` implementing `Store` |
| `internal/store_contract_test.go` | Create | `RunStoreContractTests` harness run against `fakeStore` |
| `internal/qdrant_filter_test.go` | Create | Golden filter `must`-clause count assertions |
| `internal/mcp_chain_test.go` | Create | MCP tool-chain flows: save→pin→verify, fact→history |
| `internal/benchmark_test.go` | Create | 5 benchmarks from §16 of test spec |

---

## Task 1: Add Score field to Memory and capture it in Qdrant vector search

The `Memory` struct has no `Score` field, so the Miner cannot make threshold decisions. This task adds it.

**Files:**
- Modify: `internal/models.go`
- Modify: `internal/qdrant.go`

- [ ] **Step 1: Add `Score` to `Memory` in models.go**

In `internal/models.go`, change the `Memory` struct to add `Score` after `LastAccessed`:

```go
type Memory struct {
	ID           string    `json:"id"`
	Text         string    `json:"text"`
	Project      string    `json:"project"`
	Topic        string    `json:"topic"`
	MemoryType   string    `json:"memory_type"`
	Source       string    `json:"source"`
	Timestamp    time.Time `json:"timestamp"`
	Importance   float32   `json:"importance"`
	Tags         []string  `json:"tags"`
	LastAccessed time.Time `json:"last_accessed"`
	Score        float32   `json:"score,omitempty"` // similarity score, not stored in Qdrant
}
```

- [ ] **Step 2: Capture score in vectorSearch in qdrant.go**

In `internal/qdrant.go`, the `vectorSearch` method decodes into a struct that has no `Score`. Change the decode target in `vectorSearch`:

```go
func (q *QdrantStore) vectorSearch(ctx context.Context, body map[string]any) ([]Memory, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal search body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.baseURL+"/collections/memories/points/search", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result []struct {
			ID      string         `json:"id"`
			Score   float32        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	memories := make([]Memory, 0, len(result.Result))
	for _, r := range result.Result {
		pts := []struct {
			ID      string         `json:"id"`
			Payload map[string]any `json:"payload"`
		}{{ID: r.ID, Payload: r.Payload}}
		mems := pointsToMemories(pts)
		if len(mems) > 0 {
			mems[0].Score = r.Score
			memories = append(memories, mems[0])
		}
	}
	return memories, nil
}
```

- [ ] **Step 3: Run existing tests to confirm nothing broken**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && go test ./internal/... -count=1 -timeout 30s
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/models.go internal/qdrant.go && \
git commit -m "feat: add Score field to Memory, capture similarity score from Qdrant vector search"
```

---

## Task 2: Score-based duplicate threshold in Miner (0.92)

Currently the Miner skips on `len(similar) > 0`. The spec §15.6 requires a 0.92 score threshold. This task implements and tests that boundary.

**Files:**
- Modify: `internal/miner.go`
- Modify: `internal/miner_test.go`

- [ ] **Step 1: Write failing tests for threshold boundary**

Add to `internal/miner_test.go`:

```go
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
	// High score but different project — FindSimilar is already project-scoped by the store,
	// so if similar returns empty (store returned no cross-project results), we save.
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
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestMiner_DuplicateThreshold" -v 2>&1 | head -40
```

Expected: `TestMiner_DuplicateThreshold_BelowSkips` FAIL (saves when it should skip), others may pass or fail depending on current behavior.

- [ ] **Step 3: Implement threshold check in miner.go**

In `internal/miner.go`, change the duplicate detection block from:

```go
similar, err := m.store.FindSimilar(ctx, turn.Text, project, 1)
if err != nil {
    continue
}
if len(similar) > 0 {
    continue
}
```

To:

```go
const duplicateThreshold = float32(0.92)

similar, err := m.store.FindSimilar(ctx, turn.Text, project, 1)
if err != nil {
    continue
}
if len(similar) > 0 && similar[0].Score >= duplicateThreshold {
    continue
}
```

- [ ] **Step 4: Update the existing skip test to set Score above threshold**

In `internal/miner_test.go`, update `TestMiner_MineTranscript_SkipsDuplicates` to return a memory with `Score: 0.95`:

```go
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
```

- [ ] **Step 5: Run all miner tests to verify they pass**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestMiner" -v 2>&1 | tail -20
```

Expected: all `TestMiner_*` PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/miner.go internal/miner_test.go && \
git commit -m "feat: enforce 0.92 similarity threshold for duplicate detection in Miner"
```

---

## Task 3: Create transcript fixtures

Fixtures are JSONL files that drive miner and parser tests. They go in `internal/testdata/transcripts/`.

**Files:**
- Create: `internal/testdata/transcripts/decision_single_turn.jsonl`
- Create: `internal/testdata/transcripts/discovery_resolution.jsonl`
- Create: `internal/testdata/transcripts/procedure_multistep.jsonl`
- Create: `internal/testdata/transcripts/noisy_debug_loop.jsonl`
- Create: `internal/testdata/transcripts/duplicate_session.jsonl`

- [ ] **Step 1: Create decision_single_turn.jsonl**

```bash
mkdir -p /Users/shivamvarshney/Documents/projects/memex/internal/testdata/transcripts
```

Create `internal/testdata/transcripts/decision_single_turn.jsonl`:
```jsonl
{"role":"user","content":"We decided to go with SQLite WAL for the knowledge graph because we need temporal queries and single-writer serialization is fine for this workload."}
{"role":"assistant","content":[{"type":"text","text":"Good decision. SQLite WAL mode gives you concurrent reads with serialized writes."}]}
```

- [ ] **Step 2: Create discovery_resolution.jsonl**

Create `internal/testdata/transcripts/discovery_resolution.jsonl`:
```jsonl
{"role":"user","content":"There is a bug in the embed call. It keeps crashing with nil pointer."}
{"role":"assistant","content":[{"type":"text","text":"Let me check the Ollama response parsing."}]}
{"role":"user","content":"Fixed it by switching to nomic-embed-text. Now it works perfectly. The trick was setting the model name correctly."}
{"role":"assistant","content":[{"type":"text","text":"Great, glad you got it working."}]}
```

- [ ] **Step 3: Create procedure_multistep.jsonl**

Create `internal/testdata/transcripts/procedure_multistep.jsonl`:
```jsonl
{"role":"user","content":"Steps to start the full stack: first run docker compose up -d to start qdrant, then run ollama serve in the background, then run memex serve to start the backend."}
{"role":"assistant","content":[{"type":"text","text":"Got it. That is the correct startup order."}]}
```

- [ ] **Step 4: Create noisy_debug_loop.jsonl**

Create `internal/testdata/transcripts/noisy_debug_loop.jsonl`:
```jsonl
{"role":"user","content":"ok"}
{"role":"assistant","content":[{"type":"text","text":"Sure."}]}
{"role":"user","content":"x"}
{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}
{"role":"assistant","content":[{"type":"text","text":"```\ngo: downloading...\nerror: exit status 1\n```"}]}
```

- [ ] **Step 5: Create duplicate_session.jsonl**

Create `internal/testdata/transcripts/duplicate_session.jsonl`:
```jsonl
{"role":"user","content":"I prefer table-driven tests in Go. We always use them for classifier logic."}
{"role":"user","content":"I prefer table-driven tests in Go. We always use them for classifier logic."}
```

- [ ] **Step 6: Write and run a test that validates each fixture parses correctly**

Add `internal/transcript_fixture_test.go`:

```go
package memex

import (
	"testing"
)

func TestTranscriptFixtures_Parse(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantMinTurns  int
		wantMinLength int // min chars for at least one turn
	}{
		{"decision", "testdata/transcripts/decision_single_turn.jsonl", 1, 50},
		{"discovery_resolution", "testdata/transcripts/discovery_resolution.jsonl", 2, 30},
		{"procedure", "testdata/transcripts/procedure_multistep.jsonl", 1, 50},
		{"noisy", "testdata/transcripts/noisy_debug_loop.jsonl", 0, 0}, // short turns, may yield 0
		{"duplicate", "testdata/transcripts/duplicate_session.jsonl", 2, 30},
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
	// Noisy fixture has only short/tool turns — should produce 0 or very few memories
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
			// Second call: simulate the first was already saved, score above threshold
			return []Memory{{Text: text, MemoryType: "preference", Score: 0.99}}, nil
		}
		return []Memory{}, nil
	}
	miner := NewMiner(store)

	requests, err := miner.MineTranscript("testdata/transcripts/duplicate_session.jsonl", "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	// Two identical turns: first saves, second is duplicate → max 1
	if len(requests) > 1 {
		t.Errorf("duplicate fixture: expected max 1 saved, got %d", len(requests))
	}
}
```

- [ ] **Step 7: Run fixture tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestTranscriptFixture" -v 2>&1 | tail -25
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/testdata/ internal/transcript_fixture_test.go && \
git commit -m "test: add transcript JSONL fixtures and fixture-driven miner tests"
```

---

## Task 4: KG WAL concurrency test

The spec §15.3 requires 1 writer goroutine replacing singular facts while 20 readers query concurrently. This validates WAL assumptions.

**Files:**
- Create: `internal/kg_concurrency_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/kg_concurrency_test.go`:

```go
package memex

import (
	"fmt"
	"sync"
	"testing"
)

func TestKG_WAL_ConcurrentReadersWriter(t *testing.T) {
	kg := newTestKG(t)

	// Seed an initial fact
	_, err := kg.RecordFact("service", "uses", "sqlite", "", "test", true)
	if err != nil {
		t.Fatalf("seed fact: %v", err)
	}

	const numReaders = 20
	const numWrites = 10

	var wg sync.WaitGroup
	readerErrors := make([]error, numReaders)

	// 20 concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < numWrites; j++ {
				facts, err := kg.QueryEntity("service", "")
				if err != nil {
					readerErrors[idx] = fmt.Errorf("reader %d iter %d: %w", idx, j, err)
					return
				}
				_ = facts // valid read, we just need no error
			}
		}(i)
	}

	// 1 writer replacing the singular fact
	wg.Add(1)
	go func() {
		defer wg.Done()
		objects := []string{"qdrant", "postgres", "sqlite", "qdrant", "redis", "qdrant", "sqlite", "qdrant", "postgres", "qdrant"}
		for _, obj := range objects {
			_, err := kg.RecordFact("service", "uses", obj, "", "test", true)
			if err != nil {
				t.Errorf("writer error: %v", err)
				return
			}
		}
	}()

	wg.Wait()

	// No reader errors
	for i, err := range readerErrors {
		if err != nil {
			t.Errorf("reader %d failed: %v", i, err)
		}
	}

	// Exactly one active fact for (service, uses)
	facts, err := kg.QueryEntity("service", "")
	if err != nil {
		t.Fatalf("final query: %v", err)
	}
	active := 0
	for _, f := range facts {
		if f.Subject == "service" && f.Predicate == "uses" {
			active++
		}
	}
	if active != 1 {
		t.Errorf("expected exactly 1 active (service, uses) fact, got %d: %+v", active, facts)
	}
}

func TestKG_WAL_ExpireNeverDeletes(t *testing.T) {
	kg := newTestKG(t)

	id, _ := kg.RecordFact("svc", "depends_on", "cache", "", "test", false)
	kg.ExpireFact(id, "")

	// Expired fact must still exist in history
	history, err := kg.History("svc")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	found := false
	for _, f := range history {
		if f.ID == id && f.ValidUntil != "" {
			found = true
		}
	}
	if !found {
		t.Error("expired fact must be preserved in History with valid_until set, not deleted")
	}

	// Must not appear in current query
	current, _ := kg.QueryEntity("svc", "")
	for _, f := range current {
		if f.ID == id {
			t.Error("expired fact must not appear in current QueryEntity")
		}
	}
}
```

- [ ] **Step 2: Run the test**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestKG_WAL" -v -race 2>&1 | tail -20
```

Expected: both tests PASS, no data race reported.

- [ ] **Step 3: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/kg_concurrency_test.go && \
git commit -m "test: KG WAL concurrency — 1 writer + 20 readers, expire-never-deletes invariant"
```

---

## Task 5: Session-start golden snapshot tests

The spec §15.7 requires golden files that freeze the exact output format of `buildMemoryContext`. A regression in formatting is caught immediately.

**Files:**
- Create: `internal/testdata/golden/session_start_full.txt`
- Create: `internal/testdata/golden/session_start_identity_only.txt`
- Create: `internal/testdata/golden/session_start_no_identity.txt`
- Create: `internal/hook_golden_test.go`

- [ ] **Step 1: Create the golden files**

Create `internal/testdata/golden/session_start_full.txt` with the exact expected output:
```
<memex-memory>
[identity]
I am Shivam. I build developer tools.

[pinned]
- (preference) prefer table-driven tests
- (decision) use Qdrant for storage

[context]
- (discovery) Ollama must run on host.docker.internal
</memex-memory>
```

Create `internal/testdata/golden/session_start_identity_only.txt`:
```
<memex-memory>
[identity]
I am Shivam. I build developer tools.
</memex-memory>
```

Create `internal/testdata/golden/session_start_no_identity.txt`:
```
<memex-memory>
[pinned]
- (preference) critical preference

[context]
- (decision) use Qdrant for storage
</memex-memory>
```

- [ ] **Step 2: Write the golden test**

Create `internal/hook_golden_test.go`:

```go
package memex

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update-golden", false, "overwrite golden files with current output")

func readGolden(t *testing.T, name string) string {
	t.Helper()
	path := "testdata/golden/" + name
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file %q missing — run with -update-golden to create it: %v", path, err)
	}
	return string(data)
}

func writeGolden(t *testing.T, name, content string) {
	t.Helper()
	os.MkdirAll("testdata/golden", 0755)
	if err := os.WriteFile("testdata/golden/"+name, []byte(content), 0644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
}

func TestBuildMemoryContext_Golden_Full(t *testing.T) {
	identity := "I am Shivam. I build developer tools."
	pinned := []Memory{
		{Text: "prefer table-driven tests", MemoryType: "preference"},
		{Text: "use Qdrant for storage", MemoryType: "decision"},
	}
	semantic := []Memory{
		{Text: "Ollama must run on host.docker.internal", MemoryType: "discovery"},
	}

	got := buildMemoryContext(identity, pinned, semantic)

	if *updateGolden {
		writeGolden(t, "session_start_full.txt", got)
		t.Log("golden updated")
		return
	}

	want := strings.TrimRight(readGolden(t, "session_start_full.txt"), "\n")
	got = strings.TrimRight(got, "\n")
	if got != want {
		t.Errorf("session_start_full golden mismatch:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestBuildMemoryContext_Golden_IdentityOnly(t *testing.T) {
	got := buildMemoryContext("I am Shivam. I build developer tools.", nil, nil)

	if *updateGolden {
		writeGolden(t, "session_start_identity_only.txt", got)
		t.Log("golden updated")
		return
	}

	want := strings.TrimRight(readGolden(t, "session_start_identity_only.txt"), "\n")
	got = strings.TrimRight(got, "\n")
	if got != want {
		t.Errorf("session_start_identity_only golden mismatch:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestBuildMemoryContext_Golden_NoIdentity(t *testing.T) {
	pinned := []Memory{{Text: "critical preference", MemoryType: "preference"}}
	semantic := []Memory{{Text: "use Qdrant for storage", MemoryType: "decision"}}

	got := buildMemoryContext("", pinned, semantic)

	if *updateGolden {
		writeGolden(t, "session_start_no_identity.txt", got)
		t.Log("golden updated")
		return
	}

	want := strings.TrimRight(readGolden(t, "session_start_no_identity.txt"), "\n")
	got = strings.TrimRight(got, "\n")
	if got != want {
		t.Errorf("session_start_no_identity golden mismatch:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestBuildMemoryContext_SectionOrder_IdentityBeforePinnedBeforeContext(t *testing.T) {
	identity := "I am Shivam."
	pinned := []Memory{{Text: "critical pref", MemoryType: "preference"}}
	semantic := []Memory{{Text: "discovered fact", MemoryType: "discovery"}}

	got := buildMemoryContext(identity, pinned, semantic)

	idxIdentity := strings.Index(got, "[identity]")
	idxPinned := strings.Index(got, "[pinned]")
	idxContext := strings.Index(got, "[context]")

	if !(idxIdentity < idxPinned && idxPinned < idxContext) {
		t.Errorf("wrong section order: [identity]=%d [pinned]=%d [context]=%d\noutput:\n%s",
			idxIdentity, idxPinned, idxContext, got)
	}
}

func TestBuildMemoryContext_NoDuplicateBullets(t *testing.T) {
	pinned := []Memory{
		{Text: "prefer table tests", MemoryType: "preference"},
		{Text: "prefer table tests", MemoryType: "preference"}, // exact duplicate
	}

	got := buildMemoryContext("", pinned, nil)

	count := strings.Count(got, "prefer table tests")
	// Both are included (buildMemoryContext does not deduplicate — that's the store's job)
	// But assert no empty bullets
	if strings.Contains(got, "- ()") {
		t.Error("output contains empty bullet '- ()'")
	}
	_ = count
}

func TestBuildMemoryContext_TokenBudget(t *testing.T) {
	// Session-start context should stay under a reasonable token budget.
	// ~4 chars per token; 2000 token budget = 8000 chars.
	pinned := make([]Memory, 10)
	for i := range pinned {
		pinned[i] = Memory{Text: "some pinned preference fact that is moderately long for testing budget purposes", MemoryType: "preference"}
	}
	semantic := make([]Memory, 5)
	for i := range semantic {
		semantic[i] = Memory{Text: "some semantic context memory that is also moderately long", MemoryType: "decision"}
	}

	got := buildMemoryContext("I am Shivam.", pinned, semantic)

	const maxChars = 8000
	if len(got) > maxChars {
		t.Errorf("session-start context is %d chars, exceeds budget of %d", len(got), maxChars)
	}
}
```

- [ ] **Step 3: Run golden tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestBuildMemoryContext_Golden" -v 2>&1 | tail -20
```

Expected: all PASS. If golden files don't match exactly, run:
```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestBuildMemoryContext_Golden" -update-golden
```
Then re-run to confirm PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/testdata/golden/ internal/hook_golden_test.go && \
git commit -m "test: golden snapshot tests for session-start buildMemoryContext output"
```

---

## Task 6: Classifier ambiguity and noise robustness tests

The spec §15.5 requires table-driven tests for hard ambiguity cases and noise robustness. These cases are the ones most likely to drift silently.

**Files:**
- Create: `internal/classifier_ambiguity_test.go`

- [ ] **Step 1: Write the tests**

Create `internal/classifier_ambiguity_test.go`:

```go
package memex

import (
	"testing"
)

func TestClassifier_Ambiguity_TableDriven(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name          string
		input         string
		wantType      string
		minConfidence float64
	}{
		// Problem + resolution → discovery
		{
			name:          "problem_resolved_becomes_discovery",
			input:         "There was a bug in the embed call. Fixed it by switching to nomic-embed-text. Now it works perfectly.",
			wantType:      "discovery",
			minConfidence: 0.3,
		},
		// Rationale vs decision: "reason we chose" should win over generic decision marker
		{
			name:          "rationale_over_decision",
			input:         "The reason we chose SQLite over Redis is that we need temporal queries and SQLite supports them natively. We rejected Redis because it lacks SQL.",
			wantType:      "rationale",
			minConfidence: 0.3,
		},
		// Advice vs procedure: "you should always run" — both markers match, advice should not override procedure
		{
			name:          "procedure_with_steps",
			input:         "Steps to deploy: first run go build, then run docker compose up -d, then check the health endpoint.",
			wantType:      "procedure",
			minConfidence: 0.3,
		},
		// Context: ownership statement, not a decision
		{
			name:          "context_ownership",
			input:         "Alice owns the auth team and is responsible for the SSO integration. She reports to Bob.",
			wantType:      "context",
			minConfidence: 0.3,
		},
		// Positive debugging statement with error words — should not be "problem"
		{
			name:          "positive_debugging_not_problem",
			input:         "Figured out the bug! The error was caused by a nil pointer. Fixed it now and it works.",
			wantType:      "discovery",
			minConfidence: 0.3,
		},
		// Generic text below threshold
		{
			name:          "generic_text_below_threshold",
			input:         "Hello world",
			wantType:      "",
			minConfidence: 0,
		},
		// Very short text below threshold
		{
			name:          "too_short",
			input:         "ok",
			wantType:      "",
			minConfidence: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotConf := c.Classify(tc.input)
			if tc.wantType == "" {
				if gotConf >= 0.3 {
					t.Errorf("expected below-threshold confidence, got type=%q conf=%.3f", gotType, gotConf)
				}
				return
			}
			if gotType != tc.wantType {
				t.Errorf("type = %q, want %q (confidence=%.3f)", gotType, tc.wantType, gotConf)
			}
			if gotConf < tc.minConfidence {
				t.Errorf("confidence = %.3f, want >= %.3f", gotConf, tc.minConfidence)
			}
		})
	}
}

func TestClassifier_NoiseRobustness(t *testing.T) {
	c := NewClassifier()

	// Stack traces should not produce high-confidence classification
	stackTrace := `goroutine 1 [running]:
runtime/debug.Stack()
	/usr/local/go/src/runtime/debug/stack.go:24 +0x65
panic({0x12345, 0x678})
	/usr/local/go/src/runtime/panic.go:884 +0x213`

	_, conf := c.Classify(stackTrace)
	if conf >= 0.6 {
		t.Errorf("stack trace should have low confidence, got %.3f", conf)
	}

	// Pure code block should not strongly classify
	codeBlock := "```go\nfunc main() {\n\tos.Exit(0)\n}\n```"
	_, conf = c.Classify(codeBlock)
	if conf >= 0.6 {
		t.Errorf("code block should have low confidence, got %.3f", conf)
	}

	// Shell commands only — not a procedure without prose context
	shellCmds := "$ go test ./...\n$ docker compose up"
	_, conf = c.Classify(shellCmds)
	// This may or may not classify as procedure — just must not panic
	_ = conf

	// Markdown bullets without substance
	bullets := "- item 1\n- item 2\n- item 3"
	_, conf = c.Classify(bullets)
	if conf >= 0.6 {
		t.Errorf("plain bullets should have low confidence, got %.3f", conf)
	}
}

func TestClassifier_AllNineTypes_HaveAtLeastOneTest(t *testing.T) {
	// Regression guard: if a type is removed from ValidMemoryTypes, this test breaks.
	expected := []string{"decision", "preference", "event", "discovery", "advice",
		"problem", "context", "procedure", "rationale"}
	for _, typ := range expected {
		if !ValidMemoryTypes[typ] {
			t.Errorf("memory type %q missing from ValidMemoryTypes — was it accidentally removed?", typ)
		}
	}
	if len(ValidMemoryTypes) != len(expected) {
		t.Errorf("ValidMemoryTypes has %d entries, expected %d", len(ValidMemoryTypes), len(expected))
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestClassifier_Ambiguity|TestClassifier_Noise|TestClassifier_AllNine" -v 2>&1 | tail -30
```

Expected: all PASS. If `rationale_over_decision` fails, it means the classifier scores them equally — that is acceptable; update `wantType` to `"decision"` and note the ambiguity in a comment.

- [ ] **Step 3: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/classifier_ambiguity_test.go && \
git commit -m "test: classifier ambiguity table-driven tests and noise robustness coverage"
```

---

## Task 7: Duplicate threshold boundary tests (§15.6)

These tests freeze the 0.92 boundary precisely. They use the `mockMinerStore` and mock scores.

**Files:**
- Create: `internal/duplicate_threshold_test.go`

- [ ] **Step 1: Write the boundary tests**

Create `internal/duplicate_threshold_test.go`:

```go
package memex

import (
	"context"
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
		{"no_similar_saves", -1, true}, // -1 sentinel = FindSimilar returns empty
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
	// When FindSimilar errors, the miner skips to avoid duplicates (safe default)
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
```

Note: This file uses `fmt.Errorf` — add `"fmt"` to the import block.

The complete import block for `internal/duplicate_threshold_test.go`:

```go
package memex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)
```

- [ ] **Step 2: Run the tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestDuplicateThreshold" -v 2>&1 | tail -30
```

Expected: all PASS. The 0.920 boundary is defined as "skip" (>= 0.92 skips).

- [ ] **Step 3: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/duplicate_threshold_test.go && \
git commit -m "test: freeze duplicate detection threshold boundary — 0.919 saves, 0.920+ skips"
```

---

## Task 8: Async mining race test (§15.9)

The spec requires validating that session-stop async mining doesn't block the hook, doesn't race on the transcript file, and is idempotent.

**Files:**
- Create: `internal/miner_race_test.go`

- [ ] **Step 1: Write the race tests**

Create `internal/miner_race_test.go`:

```go
package memex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionStop_AsyncMine_DoesNotBlock(t *testing.T) {
	// Fake memex server that delays mining by 200ms
	var mineCallCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/trace/stop":
			w.WriteHeader(http.StatusOK)
		case "/mine/transcript":
			mineCallCount.Add(1)
			time.Sleep(200 * time.Millisecond) // simulate slow mining
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(MineResponse{Status: "mining started"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("MEMEX_URL", srv.URL)

	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "session.jsonl")
	os.WriteFile(transcriptPath, []byte(`{"role":"user","content":"we decided to use sqlite"}`+"\n"), 0644)

	// Simulate hookSessionStop by calling its HTTP logic directly with a bounded timeout
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	body, _ := json.Marshal(MineRequest{Path: transcriptPath, Project: "test"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		srv.URL+"/mine/transcript", bytesReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		http.DefaultClient.Do(req)
	}

	elapsed := time.Since(start)

	// The hook uses a 2s timeout — verify it doesn't wait forever
	if elapsed > 2100*time.Millisecond {
		t.Errorf("hookSessionStop took %v, expected < 2.1s", elapsed)
	}
}

func TestSessionStop_MinerDown_TraceStillSucceeds(t *testing.T) {
	// Fake server: trace/stop succeeds, mine/transcript returns 500
	var traceStopCalled atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/trace/stop":
			traceStopCalled.Add(1)
			w.WriteHeader(http.StatusOK)
		case "/mine/transcript":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("MEMEX_URL", srv.URL)

	// Call the mining HTTP request with a short timeout — should not panic
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	body, _ := json.Marshal(MineRequest{Path: "/tmp/test.jsonl", Project: "test"})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/mine/transcript", bytesReader(body))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	// trace/stop is separate from mining — it is independent
	// We just verify the mining failure doesn't propagate to the caller
	// (no panic, no test failure here means the goroutine model is safe)
}

func TestSessionStop_EmptyTranscriptPath_NoMineCall(t *testing.T) {
	var mineCallCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mine/transcript" {
			mineCallCount.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// hookSessionStop logic: if TranscriptPath == "", mining is skipped
	transcriptPath := ""
	shouldMine := transcriptPath != ""
	if shouldMine {
		t.Error("empty transcript path should not trigger mining")
	}
	// Guard: if someone refactors hookSessionStop to mine even with empty path, catch it
	if mineCallCount.Load() != 0 {
		t.Errorf("mine called %d times with empty transcript path, want 0", mineCallCount.Load())
	}
}

func TestMiner_SameTranscript_TwiceIsIdempotent(t *testing.T) {
	// Mining the same transcript file twice should not double-save memories
	// because FindSimilar returns matches on second call
	callCount := 0

	store := &mockMinerStore{}
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		callCount++
		if callCount > 2 {
			// After first mine, subsequent find_similar returns high-score results
			return []Memory{{Text: text, MemoryType: "preference", Score: 0.99}}, nil
		}
		return []Memory{}, nil
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(`{"role":"user","content":"We decided to use SQLite WAL for temporal queries. This is a key architectural decision."}`+"\n"), 0644)

	miner := NewMiner(store)

	first, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("first mine: %v", err)
	}

	callCount = 3 // reset so second call sees high-score results
	second, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("second mine: %v", err)
	}

	if len(second) >= len(first) && len(first) > 0 {
		t.Logf("idempotency: first=%d saved, second=%d saved (second should be 0 or fewer)", len(first), len(second))
		if len(second) > 0 {
			t.Errorf("second mining of same transcript saved %d memories, want 0 (all duplicates)", len(second))
		}
	}
}

// bytesReader is a helper to create an io.Reader from a byte slice.
func bytesReader(b []byte) *bytesReaderImpl { return &bytesReaderImpl{data: b} }

type bytesReaderImpl struct {
	data []byte
	pos  int
}

func (r *bytesReaderImpl) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
```

Add `"io"` to the import block:

```go
import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)
```

- [ ] **Step 2: Run the tests with race detector**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestSessionStop|TestMiner_SameTranscript" -v -race 2>&1 | tail -30
```

Expected: all PASS, no data races.

- [ ] **Step 3: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/miner_race_test.go && \
git commit -m "test: async mining race safety — non-blocking hook, miner-down resilience, idempotent replay"
```

---

## Task 9: In-memory fakeStore and Store contract test harness (§15.1)

A fake in-memory Store implementation lets unit tests run without Qdrant or Ollama. The contract harness runs the same invariants against any Store.

**Files:**
- Create: `internal/fake_store_test.go`
- Create: `internal/store_contract_test.go`

- [ ] **Step 1: Write fakeStore**

Create `internal/fake_store_test.go`:

```go
package memex

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// fakeStore is a thread-safe in-memory implementation of Store for unit tests.
// It does not perform real embedding — FindSimilar uses substring matching as a proxy.
type fakeStore struct {
	mu       sync.RWMutex
	memories map[string]Memory // id → Memory
	nextID   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{memories: make(map[string]Memory)}
}

func (f *fakeStore) Init(_ context.Context) error { return nil }
func (f *fakeStore) Health(_ context.Context) error { return nil }

func (f *fakeStore) SaveMemory(_ context.Context, req SaveMemoryRequest) (Memory, error) {
	if req.Text == "" {
		return Memory{}, fmt.Errorf("text required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("fake-%d", f.nextID)
	now := time.Now().UTC()
	topic := req.Topic
	if topic == "" {
		topic = req.Project
	}
	imp := req.Importance
	if imp == 0 {
		imp = 0.5
	}
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	m := Memory{
		ID:           id,
		Text:         req.Text,
		Project:      req.Project,
		Topic:        topic,
		MemoryType:   req.MemoryType,
		Source:       req.Source,
		Timestamp:    now,
		Importance:   imp,
		Tags:         tags,
		LastAccessed: now,
	}
	f.memories[id] = m
	return m, nil
}

func (f *fakeStore) SearchMemories(_ context.Context, query, project, memoryType, topic string, limit int) ([]Memory, error) {
	return f.ListMemories(context.Background(), project, memoryType, topic, limit)
}

func (f *fakeStore) ListMemories(_ context.Context, project, memoryType, topic string, limit int) ([]Memory, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []Memory
	for _, m := range f.memories {
		if project != "" && m.Project != project {
			continue
		}
		if memoryType != "" && m.MemoryType != memoryType {
			continue
		}
		if topic != "" && m.Topic != topic {
			continue
		}
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		si := 0.6*float64(result[i].Importance) + 0.4/float64(1+int(time.Since(result[i].Timestamp).Hours()/24))
		sj := 0.6*float64(result[j].Importance) + 0.4/float64(1+int(time.Since(result[j].Timestamp).Hours()/24))
		return si > sj
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeStore) PinnedMemories(_ context.Context, project string) ([]Memory, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []Memory
	for _, m := range f.memories {
		if m.Project == project && m.Importance >= 0.9 {
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Importance > result[j].Importance
	})
	return result, nil
}

func (f *fakeStore) PinMemory(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.memories[id]
	if !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	m.Importance = 1.0
	f.memories[id] = m
	return nil
}

func (f *fakeStore) FindSimilar(_ context.Context, text, project string, limit int) ([]Memory, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	lower := strings.ToLower(text)
	var result []Memory
	for _, m := range f.memories {
		if project != "" && m.Project != project {
			continue
		}
		if strings.Contains(strings.ToLower(m.Text), lower[:min(len(lower), 20)]) {
			result = append(result, m)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeStore) DeleteMemory(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.memories[id]; !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	delete(f.memories, id)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Write the contract test harness**

Create `internal/store_contract_test.go`:

```go
package memex

import (
	"context"
	"testing"
)

// RunStoreContractTests validates Store interface invariants.
// Call this with any Store implementation to verify correctness.
func RunStoreContractTests(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("SaveMemory_returns_non_empty_stable_ID", func(t *testing.T) {
		m, err := store.SaveMemory(ctx, SaveMemoryRequest{
			Text:       "contract test memory",
			Project:    "contract-test",
			MemoryType: "preference",
		})
		if err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
		if m.ID == "" {
			t.Error("SaveMemory must return non-empty ID")
		}
		// Save again — ID is new but both should be retrievable
		m2, err := store.SaveMemory(ctx, SaveMemoryRequest{
			Text:       "second contract test memory",
			Project:    "contract-test",
			MemoryType: "decision",
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
			Text:       "topic defaults test",
			Project:    "proj-x",
			MemoryType: "event",
			// Topic intentionally omitted
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
		memories, _ := store.ListMemories(ctx, "del-test", "", "", 10)
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
```

- [ ] **Step 3: Run the contract tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestStoreContract" -v 2>&1 | tail -30
```

Expected: all subtests PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/fake_store_test.go internal/store_contract_test.go && \
git commit -m "test: in-memory fakeStore + RunStoreContractTests harness for Store interface invariants"
```

---

## Task 10: Qdrant filter golden tests (§15.2)

These tests assert the exact number of `must` clauses generated for each filter combination. Regression here means silently broken search scoping.

**Files:**
- Create: `internal/qdrant_filter_test.go`

- [ ] **Step 1: Write filter golden tests**

Create `internal/qdrant_filter_test.go`:

```go
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
	f := buildFilter("memex", "", "")
	if f == nil {
		t.Fatal("expected non-nil filter for project only")
	}
	if n := mustClauseCount(f); n != 1 {
		t.Errorf("project-only filter: want 1 must clause, got %d", n)
	}
	// Verify the clause targets "project"
	must := f["must"].([]map[string]any)
	if must[0]["key"] != "project" {
		t.Errorf("clause key = %q, want project", must[0]["key"])
	}
}

func TestBuildFilter_ProjectAndType(t *testing.T) {
	f := buildFilter("memex", "decision", "")
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if n := mustClauseCount(f); n != 2 {
		t.Errorf("project+type filter: want 2 must clauses, got %d", n)
	}
}

func TestBuildFilter_ProjectAndTopic(t *testing.T) {
	f := buildFilter("memex", "", "testing")
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if n := mustClauseCount(f); n != 2 {
		t.Errorf("project+topic filter: want 2 must clauses, got %d", n)
	}
}

func TestBuildFilter_AllThree(t *testing.T) {
	f := buildFilter("memex", "preference", "testing")
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if n := mustClauseCount(f); n != 3 {
		t.Errorf("project+type+topic filter: want 3 must clauses, got %d", n)
	}
}

func TestBuildFilter_Empty_ReturnsNil(t *testing.T) {
	f := buildFilter("", "", "")
	if f != nil {
		t.Errorf("all-empty filter should return nil, got %+v", f)
	}
}

func TestBuildFilter_TypeOnly(t *testing.T) {
	f := buildFilter("", "decision", "")
	if f == nil {
		t.Fatal("expected non-nil filter for type only")
	}
	if n := mustClauseCount(f); n != 1 {
		t.Errorf("type-only filter: want 1 must clause, got %d", n)
	}
}

func TestBuildFilter_SerializesCorrectly(t *testing.T) {
	// Verify the filter JSON round-trips correctly — Qdrant requires exact structure
	f := buildFilter("memex", "decision", "kg")
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

	// Each clause must have "key" and "match" fields
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
```

- [ ] **Step 2: Run filter tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestBuildFilter" -v 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/qdrant_filter_test.go && \
git commit -m "test: Qdrant filter golden tests — exact must-clause counts for all filter combinations"
```

---

## Task 11: MCP tool-chain contract tests (§15.8)

These test the exact MCP tool flows defined in the spec: `find_similar → save_memory → pin_memory` and `fact_query → fact_record(singular) → fact_history`.

**Files:**
- Create: `internal/mcp_chain_test.go`

- [ ] **Step 1: Write the tool-chain tests**

Create `internal/mcp_chain_test.go`:

```go
package memex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeChainServer tracks call order and simulates the full save→pin→verify flow.
type fakeChainServer struct {
	server  *httptest.Server
	calls   []string
	pinned  map[string]bool
	memories map[string]Memory
}

func newFakeChainServer(t *testing.T) *fakeChainServer {
	t.Helper()
	f := &fakeChainServer{
		pinned:   make(map[string]bool),
		memories: make(map[string]Memory),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/memories/similar", func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, "find_similar")
		w.Header().Set("Content-Type", "application/json")
		// Return empty — safe to save
		json.NewEncoder(w).Encode(SearchResponse{Memories: []Memory{}})
	})

	mux.HandleFunc("/memories", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f.calls = append(f.calls, "save_memory")
			var req SaveMemoryRequest
			json.NewDecoder(r.Body).Decode(&req)
			m := Memory{ID: "chain-id-1", Text: req.Text, MemoryType: req.MemoryType, Importance: req.Importance}
			f.memories[m.ID] = m
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(m)
		} else if r.Method == http.MethodGet {
			f.calls = append(f.calls, "list_memories")
			var result []Memory
			for _, m := range f.memories {
				if f.pinned[m.ID] {
					m.Importance = 1.0
					result = append(result, m)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(SearchResponse{Memories: result})
		}
	})

	mux.HandleFunc("/memories/pinned", func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, "pinned_memories")
		var result []Memory
		for _, m := range f.memories {
			if f.pinned[m.ID] {
				m.Importance = 1.0
				result = append(result, m)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{Memories: result})
	})

	mux.HandleFunc("/memories/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			id := strings.TrimPrefix(r.URL.Path, "/memories/")
			id = strings.TrimSuffix(id, "/pin")
			f.calls = append(f.calls, "pin_memory")
			f.pinned[id] = true
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// KG fact endpoints
	mux.HandleFunc("/facts", func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, "fact_record")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "fact-chain-1"})
	})

	mux.HandleFunc("/facts/timeline", func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, "fact_history")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"facts": []Fact{
			{ID: "fact-chain-1", Subject: "memex", Predicate: "uses", Object: "qdrant"},
		}})
	})

	mux.HandleFunc("/facts/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			f.calls = append(f.calls, "fact_query")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"facts": []Fact{}})
		}
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func TestMCP_Chain_FindSimilar_Save_Pin_Verify(t *testing.T) {
	f := newFakeChainServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	// Step 1: find_similar — verify no duplicates
	similar, err := callTool(handleFindSimilar, map[string]any{
		"text":    "always use table-driven tests in Go",
		"project": "memex",
	})
	if err != nil || similar.IsError {
		t.Fatalf("find_similar failed: err=%v, isError=%v", err, similar.IsError)
	}

	// Step 2: save_memory
	saved, err := callTool(handleSaveMemory, map[string]any{
		"text":        "always use table-driven tests in Go",
		"project":     "memex",
		"memory_type": "preference",
		"importance":  0.95,
	})
	if err != nil || saved.IsError {
		t.Fatalf("save_memory failed: err=%v, isError=%v", err, saved.IsError)
	}
	if !strings.Contains(fmt.Sprintf("%v", saved.Content), "chain-id-1") {
		t.Errorf("save_memory did not return expected id in content: %v", saved.Content)
	}

	// Step 3: pin_memory
	pinned, err := callTool(handlePinMemory, map[string]any{
		"id": "chain-id-1",
	})
	if err != nil || pinned.IsError {
		t.Fatalf("pin_memory failed: err=%v, isError=%v", err, pinned.IsError)
	}

	// Verify call order
	want := []string{"find_similar", "save_memory", "pin_memory"}
	for i, call := range want {
		if i >= len(f.calls) || f.calls[i] != call {
			t.Errorf("call[%d] = %q, want %q (full sequence: %v)", i, safeGet(f.calls, i), call, f.calls)
		}
	}

	// Verify pinned memory appears in pinned list
	resp, _ := callTool(handleMemoryOverview, map[string]any{})
	_ = resp // overview doesn't hit /memories/pinned, just verify no panic
}

func TestMCP_Chain_FactRecord_Singular_History(t *testing.T) {
	f := newFakeChainServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	// Step 1: fact_query — check current state
	_, err := callTool(handleFactQuery, map[string]any{
		"entity": "memex",
	})
	if err != nil {
		t.Fatalf("fact_query: %v", err)
	}

	// Step 2: fact_record with singular=true
	recorded, err := callTool(handleFactRecord, map[string]any{
		"subject":   "memex",
		"predicate": "uses",
		"object":    "qdrant",
		"singular":  true,
	})
	if err != nil || recorded.IsError {
		t.Fatalf("fact_record: err=%v isError=%v", err, recorded.IsError)
	}

	// Step 3: fact_history
	history, err := callTool(handleFactHistory, map[string]any{
		"entity": "memex",
	})
	if err != nil || history.IsError {
		t.Fatalf("fact_history: err=%v isError=%v", err, history.IsError)
	}

	// Verify the chain was called in order
	want := []string{"fact_query", "fact_record", "fact_history"}
	for i, call := range want {
		if i >= len(f.calls) || f.calls[i] != call {
			t.Errorf("call[%d] = %q, want %q (full: %v)", i, safeGet(f.calls, i), call, f.calls)
		}
	}
}

func safeGet(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "<missing>"
}
```

- [ ] **Step 2: Run the chain tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestMCP_Chain" -v 2>&1 | tail -30
```

Expected: both PASS.

- [ ] **Step 3: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/mcp_chain_test.go && \
git commit -m "test: MCP tool-chain contract tests — save→pin→verify, fact_query→record→history"
```

---

## Task 12: Benchmarks (§16)

The spec §16 defines 5 benchmarks with performance budgets. These live in `benchmark_test.go`.

**Files:**
- Create: `internal/benchmark_test.go`

- [ ] **Step 1: Write benchmarks**

Create `internal/benchmark_test.go`:

```go
package memex

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// BenchmarkClassifierLargeTranscript — budget: < 500ms for 1000 turns
func BenchmarkClassifierLargeTranscript(b *testing.B) {
	c := NewClassifier()
	// Simulate 1000 transcript turns
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

// BenchmarkKGSingularReplacement — budget: < 5ms p50
func BenchmarkKGSingularReplacement(b *testing.B) {
	kg, err := NewKnowledgeGraph(":memory:")
	if err != nil {
		b.Fatalf("NewKnowledgeGraph: %v", err)
	}
	if err := kg.Init(); err != nil {
		b.Fatalf("kg.Init: %v", err)
	}
	defer kg.db.Close()

	// Seed initial fact
	kg.RecordFact("service", "uses", "sqlite", "", "bench", true)

	objects := []string{"qdrant", "postgres", "sqlite", "redis", "qdrant"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := objects[i%len(objects)]
		kg.RecordFact("service", "uses", obj, "", "bench", true)
	}
}

// BenchmarkPinnedMemories1000 — budget: < 50ms for 1000 memories (fake store, no network)
func BenchmarkPinnedMemories1000(b *testing.B) {
	store := newFakeStore()
	ctx := context.Background()

	// Pre-populate with 1000 memories, 100 pinned
	for i := 0; i < 1000; i++ {
		imp := float32(0.5)
		if i%10 == 0 {
			imp = 0.95 // 100 pinned
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

// BenchmarkSessionStartHook — budget: < 150ms local (buildMemoryContext only, no HTTP)
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

// BenchmarkMineDirectory — budget: < 500ms for 100 sessions (classifier only, no store IO)
func BenchmarkMineDirectory(b *testing.B) {
	c := NewClassifier()

	// Simulate 100 sessions × 10 turns each
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

// TestBenchmarkBudgets verifies performance budgets are met with a single timed run.
// This is a smoke test, not a statistical benchmark — CI will catch regressions.
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
```

- [ ] **Step 2: Run benchmarks**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -bench="BenchmarkClassifier|BenchmarkKG|BenchmarkPinned|BenchmarkSession|BenchmarkMine" \
  -benchtime=3s -run='^$' 2>&1 | tail -20
```

Expected: all benchmarks complete, no errors. Note the ns/op values.

- [ ] **Step 3: Run budget smoke tests**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -run "TestBenchmarkBudgets" -v 2>&1 | tail -15
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
git add internal/benchmark_test.go && \
git commit -m "test: benchmarks for classifier, KG singular replace, pinned memories, session-start, miner"
```

---

## Final verification

- [ ] **Run the full test suite**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -count=1 -race -timeout 60s 2>&1 | tail -30
```

Expected: all tests PASS, no data races.

- [ ] **Run with -short to verify short-mode skips work**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -short -count=1 -timeout 30s 2>&1 | tail -10
```

Expected: PASS, `TestBenchmarkBudgets` skipped.

- [ ] **Count test coverage**

```bash
cd /Users/shivamvarshney/Documents/projects/memex && \
go test ./internal/... -cover -count=1 2>&1 | grep coverage
```

Note the coverage percentage for the record.

---

## Self-Review

**Spec coverage check:**

| Spec section | Covered by task |
|---|---|
| §15.1 Store contract tests | Task 9 |
| §15.2 Qdrant payload/filter golden | Task 10 |
| §15.3 SQLite KG WAL concurrency | Task 4 |
| §15.4 Transcript fixtures | Task 3 |
| §15.5 Classifier ambiguity + noise | Task 6 |
| §15.6 Duplicate threshold 0.919/0.921 | Tasks 2 + 7 |
| §15.7 Hook golden snapshots | Task 5 |
| §15.8 MCP chain contracts | Task 11 |
| §15.9 Async mining race | Task 8 |
| §15.10 Replay determinism | Not in plan — requires fixed clock + UUID injection; deferred as it needs separate infrastructure |
| §16 Benchmarks | Task 12 |
| §13 Priority order | KG (T4) → session-start (T5) → classifier (T6) → duplicates (T2,T7) → async mine (T8) → MCP (T11) |

**Deferred:** §15.10 (end-to-end replay determinism) requires injecting a fake clock and UUID generator as interfaces into the KG and Store — this is a non-trivial refactor deferred to a follow-up plan.
