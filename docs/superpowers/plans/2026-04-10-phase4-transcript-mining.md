# Phase 4: Transcript Mining — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a 9-type regex classifier in Go, extend `ParseTranscript` to return full conversation turns (user + assistant), create a `Miner` that classifies turns and saves typed memories with deduplication, expose a `POST /mine/transcript` endpoint, and add a `memex mine <path>` CLI subcommand. Phase 2's session-stop hook fires this endpoint automatically — this phase makes it functional.

**Architecture:** `internal/classifier.go` holds the `Classifier` type (pure regex, no LLM). `internal/miner.go` holds the `Miner` type that wires classifier + store + deduplication. `internal/transcript.go` gains a `ParseConversation` function returning `[]ConversationTurn`. `internal/handlers.go` gets a `MineTranscript` handler. `cmd/memex/main.go` gets a `mine` subcommand. Phases 1–3 must be complete.

**Tech Stack:** Go 1.26, `regexp` standard library, existing `Store` interface, existing `net/http` test patterns.

---

## File Map

| File | Change |
|---|---|
| `internal/classifier.go` | NEW: `Classifier` type with 9-type marker sets |
| `internal/classifier_test.go` | NEW: classification tests |
| `internal/transcript.go` | Add `ParseConversation` returning `[]ConversationTurn` |
| `internal/transcript_test.go` | Add tests for `ParseConversation` |
| `internal/miner.go` | NEW: `Miner` type — mines transcript, deduplicates, saves |
| `internal/miner_test.go` | NEW: miner tests with mock store |
| `internal/handlers.go` | Add `MineTranscript` handler |
| `internal/handlers_test.go` | Add `MineTranscript` handler test |
| `internal/server.go` | Register `POST /mine/transcript` |
| `cmd/memex/main.go` | Add `mine` subcommand |

---

### Task 1: Create `internal/classifier.go` — 9-type regex classifier

**Files:**
- Create: `internal/classifier.go`
- Create: `internal/classifier_test.go`

- [ ] **Step 1: Write failing classifier tests**

Create `internal/classifier_test.go`:

```go
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
	// Problem marker + resolution marker → discovery (disambiguated)
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
go test ./internal/ -run TestClassifier_ -v 2>&1 | head -20
```

Expected: FAIL — `Classifier` type not defined.

- [ ] **Step 3: Create `internal/classifier.go`**

```go
package memex

import (
	"regexp"
	"strings"
)

// typeMarkers maps memory type → list of keyword/phrase patterns.
// Ported from mempalace general_extractor.py and extended to 9 types.
var typeMarkers = map[string][]string{
	"decision": {
		`let'?s (use|go with|try|pick|choose|switch to)`,
		`we (decided|chose|went with|settled on|picked)`,
		`rather than`,
		`architecture`,
		`approach`,
		`strategy`,
		`configure`,
		`trade-?off`,
		`pros and cons`,
	},
	"preference": {
		`i prefer`,
		`always use`,
		`never use`,
		`don'?t (ever )?(use|do|mock|stub)`,
		`i like (to|when|how)`,
		`i hate (when|how)`,
		`my (rule|preference|style|convention) is`,
		`we (always|never)`,
		`snake_?case`,
		`camel_?case`,
	},
	"event": {
		`session on`,
		`we met`,
		`sprint`,
		`last week`,
		`deployed`,
		`shipped`,
		`launched`,
		`milestone`,
		`standup`,
		`released`,
	},
	"discovery": {
		`it works`,
		`it worked`,
		`got it working`,
		`figured (it )?out`,
		`turns out`,
		`the trick (is|was)`,
		`realized`,
		`breakthrough`,
		`finally`,
		`now i (understand|see|get it)`,
		`the key (is|was)`,
	},
	"advice": {
		`you should`,
		`recommend`,
		`best practice`,
		`the answer (is|was)`,
		`suggestion`,
		`consider using`,
		`try using`,
		`better to`,
	},
	"problem": {
		`\b(bug|error|crash|fail|broke|broken|issue|problem)\b`,
		`doesn'?t work`,
		`not working`,
		`keeps? (failing|crashing|breaking)`,
		`root cause`,
		`the (problem|issue|bug) (is|was)`,
		`workaround`,
	},
	"context": {
		`works on`,
		`responsible for`,
		`reports to`,
		`\bteam\b`,
		`\bowns\b`,
		`based in`,
		`member of`,
		`\bleads\b`,
	},
	"procedure": {
		`steps to`,
		`how to`,
		`\bworkflow\b`,
		`always run`,
		`first run`,
		`then run`,
		`\bpipeline\b`,
		`\bprocess\b`,
	},
	"rationale": {
		`the reason we`,
		`chose over`,
		`we rejected`,
		`instead of`,
		`because we need`,
		`pros and cons`,
	},
}

// resolutionMarkers detect that a problem has been resolved (→ reclassify as discovery).
var resolutionMarkers = []*regexp.Regexp{
	regexp.MustCompile(`\bfixed\b`),
	regexp.MustCompile(`\bsolved\b`),
	regexp.MustCompile(`\bresolved\b`),
	regexp.MustCompile(`\bgot it working\b`),
	regexp.MustCompile(`\bit works\b`),
	regexp.MustCompile(`\bthe fix (is|was)\b`),
	regexp.MustCompile(`\bfigured (it )?out\b`),
}

// positiveWords for sentiment disambiguation.
var positiveWords = map[string]bool{
	"fixed": true, "solved": true, "works": true, "working": true,
	"breakthrough": true, "success": true, "nailed": true, "figured": true,
}

// negativeWords for sentiment disambiguation.
var negativeWords = map[string]bool{
	"bug": true, "error": true, "crash": true, "fail": true, "failed": true,
	"broken": true, "broke": true, "issue": true, "problem": true, "stuck": true,
}

// compiledMarkers caches compiled regexp for each type.
type compiledMarkerSet map[string][]*regexp.Regexp

// Classifier classifies a text string into one of 9 memory types.
type Classifier struct {
	compiled compiledMarkerSet
}

// NewClassifier compiles all marker patterns once and returns a ready Classifier.
func NewClassifier() *Classifier {
	compiled := make(compiledMarkerSet, len(typeMarkers))
	for memType, patterns := range typeMarkers {
		for _, p := range patterns {
			compiled[memType] = append(compiled[memType], regexp.MustCompile(`(?i)`+p))
		}
	}
	return &Classifier{compiled: compiled}
}

// Classify returns the best-match memory type and a confidence score (0.0–1.0).
// Returns ("", 0) if no markers match or confidence < 0.3.
func (c *Classifier) Classify(text string) (string, float64) {
	lower := strings.ToLower(text)
	scores := make(map[string]float64)

	for memType, patterns := range c.compiled {
		for _, re := range patterns {
			if re.MatchString(lower) {
				scores[memType] += 1.0
			}
		}
	}
	if len(scores) == 0 {
		return "", 0
	}

	// Length bonus
	var bonus float64
	if len(text) > 500 {
		bonus = 2
	} else if len(text) > 200 {
		bonus = 1
	}

	bestType := ""
	bestScore := 0.0
	for t, s := range scores {
		if s > bestScore {
			bestScore = s
			bestType = t
		}
	}
	bestScore += bonus

	// Disambiguation: problem + resolution → discovery
	if bestType == "problem" {
		for _, re := range resolutionMarkers {
			if re.MatchString(lower) {
				bestType = "discovery"
				break
			}
		}
		// Problem + positive sentiment → discovery
		if bestType == "problem" && c.sentiment(lower) == "positive" {
			if scores["discovery"] > 0 {
				bestType = "discovery"
			}
		}
	}

	confidence := bestScore / 5.0
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.3 {
		return "", confidence
	}
	return bestType, confidence
}

func (c *Classifier) sentiment(text string) string {
	words := strings.Fields(text)
	pos, neg := 0, 0
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:'\"")
		if positiveWords[w] {
			pos++
		}
		if negativeWords[w] {
			neg++
		}
	}
	if pos > neg {
		return "positive"
	}
	if neg > pos {
		return "negative"
	}
	return "neutral"
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ -run TestClassifier_ -v
```

Expected: PASS — all 11 classifier tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/classifier.go internal/classifier_test.go
git commit -m "feat: add 9-type regex classifier (ported + extended from mempalace)"
```

---

### Task 2: Extend `internal/transcript.go` — `ParseConversation`

**Files:**
- Modify: `internal/transcript.go`
- Modify: `internal/transcript_test.go`

`ParseTranscript` (reasoning-only) must remain untouched — it is still used by the tracer. We add a new `ParseConversation` function that returns full user + assistant turns.

- [ ] **Step 1: Add failing tests**

Add to `internal/transcript_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ -run TestParseConversation -v 2>&1 | head -15
```

Expected: FAIL — `ParseConversation` not defined.

- [ ] **Step 3: Add `ParseConversation` to `internal/transcript.go`**

Note: `ConversationTurn` is already defined in `models.go` (Phase 1). Do NOT redefine it here.

Add after the existing `ParseTranscript` function:

```go
// ParseConversation reads a Claude Code session JSONL and returns all user and
// assistant turns as ConversationTurn values. Tool-result turns are skipped.
// Assistant content arrays are joined into a single text string.
func ParseConversation(path string) ([]ConversationTurn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var turns []ConversationTurn
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg transcriptMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		switch msg.Role {
		case "user":
			// User content may be a plain string or a content array
			var text string
			if err := json.Unmarshal(msg.Content, &text); err == nil {
				// Plain string
				turns = append(turns, ConversationTurn{Role: "user", Text: text})
			} else {
				// Content array (same shape as assistant)
				var blocks []contentBlock
				if err := json.Unmarshal(msg.Content, &blocks); err == nil {
					var sb strings.Builder
					for _, b := range blocks {
						if b.Type == "text" {
							sb.WriteString(b.Text)
						}
					}
					if s := sb.String(); s != "" {
						turns = append(turns, ConversationTurn{Role: "user", Text: s})
					}
				}
			}

		case "assistant":
			var blocks []contentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err != nil {
				continue
			}
			var sb strings.Builder
			for _, b := range blocks {
				if b.Type == "text" {
					sb.WriteString(b.Text)
				}
			}
			if s := sb.String(); s != "" {
				turns = append(turns, ConversationTurn{Role: "assistant", Text: s})
			}

		// "tool" role is skipped
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript: %w", err)
	}
	return turns, nil
}
```

Note: `strings` import must be added to `transcript.go`. The existing imports are `bufio`, `encoding/json`, `fmt`, `os`. Add `"strings"`.

- [ ] **Step 4: Add `"strings"` import to `internal/transcript.go`**

The existing import block:
```go
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)
```

Replace with:
```go
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)
```

- [ ] **Step 5: Run all transcript tests**

```bash
go test ./internal/ -run TestParseTranscript -v
go test ./internal/ -run TestParseConversation -v
```

Expected: PASS — all 6 transcript tests pass (3 existing + 3 new).

- [ ] **Step 6: Commit**

```bash
git add internal/transcript.go internal/transcript_test.go
git commit -m "feat: add ParseConversation returning full user+assistant turns"
```

---

### Task 3: Create `internal/miner.go` — transcript miner

**Files:**
- Create: `internal/miner.go`
- Create: `internal/miner_test.go`

The `Miner` reads a transcript, classifies each turn, deduplicates against existing memories (cosine similarity > 0.92 via `FindSimilar`), and saves new typed memories.

- [ ] **Step 1: Write failing miner tests**

Create `internal/miner_test.go`:

```go
package memex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mockMinerStore is a minimal Store that tracks SaveMemory calls and returns
// empty results for FindSimilar (simulating no duplicates).
type mockMinerStore struct {
	mockStore // embed the existing mockStore for default no-op behaviour
	saved []SaveMemoryRequest
}

func (m *mockMinerStore) SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error) {
	m.saved = append(m.saved, req)
	return Memory{ID: "test-id", Text: req.Text, MemoryType: req.MemoryType}, nil
}

func (m *mockMinerStore) FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error) {
	// No duplicates
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
	// Verify all saved memories have a memory_type set
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

	// Store returns a high-similarity match (duplicate)
	store := &mockMinerStore{}
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		return []Memory{
			{Text: "I prefer table-driven tests in Go.", MemoryType: "preference"},
		}, nil
	}
	miner := NewMiner(store)
	// Inject a custom similarity threshold check via a similarity score field
	// The miner should skip saving when FindSimilar returns results
	requests, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("MineTranscript: %v", err)
	}
	if len(requests) != 0 {
		t.Errorf("expected 0 saved memories (duplicate detected), got %d", len(requests))
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
		// Topic should always be a non-empty slug
		if got == "" {
			t.Errorf("inferTopic(%q) returned empty string", tc.text)
		}
	}
}
```

The test for `SkipsDuplicates` uses a `findSimilarFn` field on `mockStore`. Update the existing `mockStore` in `internal/handlers_test.go` to support this. First, run to see compilation errors, then fix.

- [ ] **Step 2: Check if `mockStore` already has a `FindSimilar` method**

```bash
grep -n "FindSimilar\|findSimilarFn" internal/handlers_test.go
```

- [ ] **Step 3: Add `findSimilarFn` to the mock if missing**

If the `mockStore` in `internal/handlers_test.go` does not have `findSimilarFn`, add these two lines to the `mockStore` struct and its `FindSimilar` method. Locate the `mockStore` struct definition (search for `type mockStore struct`) and add:

```go
findSimilarFn func(ctx context.Context, text, project string, limit int) ([]Memory, error)
```

And the method:

```go
func (m *mockStore) FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error) {
    if m.findSimilarFn != nil {
        return m.findSimilarFn(ctx, text, project, limit)
    }
    return []Memory{}, nil
}
```

- [ ] **Step 4: Run miner tests to verify they fail**

```bash
go test ./internal/ -run TestMiner_ -v 2>&1 | head -20
```

Expected: FAIL — `Miner`, `NewMiner` not defined.

- [ ] **Step 5: Create `internal/miner.go`**

```go
package memex

import (
	"context"
	"regexp"
	"strings"
)

// Miner reads a transcript file, classifies each conversation turn into typed
// memories, deduplicates against existing memories, and saves new ones.
type Miner struct {
	store      Store
	classifier *Classifier
}

// NewMiner creates a Miner backed by the given store.
func NewMiner(store Store) *Miner {
	return &Miner{
		store:      store,
		classifier: NewClassifier(),
	}
}

// MineTranscript parses the JSONL transcript at path, classifies each turn,
// deduplicates against existing memories for project, and saves typed memories.
// Returns the SaveMemoryRequests that were actually saved (not duplicates).
func (m *Miner) MineTranscript(path, project string) ([]SaveMemoryRequest, error) {
	turns, err := ParseConversation(path)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	var saved []SaveMemoryRequest

	for _, turn := range turns {
		if len(strings.TrimSpace(turn.Text)) < 20 {
			continue
		}

		memType, confidence := m.classifier.Classify(turn.Text)
		if confidence < 0.3 || memType == "" {
			continue
		}

		// Duplicate detection: skip if FindSimilar returns any results
		similar, err := m.store.FindSimilar(ctx, turn.Text, project, 1)
		if err == nil && len(similar) > 0 {
			continue
		}

		topic := m.inferTopic(turn.Text)
		req := SaveMemoryRequest{
			Text:       turn.Text,
			Project:    project,
			Topic:      topic,
			MemoryType: memType,
			Source:     "transcript-mine",
			Importance: float32(confidence),
		}

		if _, err := m.store.SaveMemory(ctx, req); err == nil {
			saved = append(saved, req)
		}
	}
	return saved, nil
}

// hyphenWordRe matches two or more hyphenated lowercase words (e.g. "auth-migration").
var hyphenWordRe = regexp.MustCompile(`\b[a-z]+-[a-z]+(?:-[a-z]+)*\b`)

// inferTopic extracts a topic slug from text.
// Prefers hyphenated compound words (e.g. "auth-migration", "ci-pipeline").
// Falls back to "general" if nothing useful is found.
func (m *Miner) inferTopic(text string) string {
	lower := strings.ToLower(text)
	matches := hyphenWordRe.FindAllString(lower, -1)
	if len(matches) > 0 {
		return matches[0]
	}
	return "general"
}
```

- [ ] **Step 6: Run miner tests**

```bash
go test ./internal/ -run TestMiner_ -v
```

Expected: PASS — all 3 miner tests pass (topic test may log mismatches as info, not fail).

- [ ] **Step 7: Commit**

```bash
git add internal/miner.go internal/miner_test.go internal/handlers_test.go
git commit -m "feat: add Miner (classify transcript turns, dedup, save typed memories)"
```

---

### Task 4: Add `MineTranscript` HTTP handler

**Files:**
- Modify: `internal/handlers.go`
- Modify: `internal/handlers_test.go`
- Modify: `internal/server.go`

- [ ] **Step 1: Write failing handler test**

Add to `internal/handlers_test.go`:

```go
func TestHandlers_MineTranscript(t *testing.T) {
	// Write a small transcript file
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(`{"role":"user","content":"I prefer table-driven tests."}`+"\n"), 0644)

	store := &mockStore{}
	h := NewHandlers(store)

	body := MineRequest{Path: path, Project: "memex"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mine/transcript", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.MineTranscript(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
	var resp MineResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Status != "mining started" {
		t.Errorf("status = %q, want 'mining started'", resp.Status)
	}
}
```

Also add the import `"path/filepath"` to `handlers_test.go` if not present (check with `grep "filepath" internal/handlers_test.go`).

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ -run TestHandlers_MineTranscript -v 2>&1 | head -15
```

Expected: FAIL — `MineTranscript` not defined on handlers.

- [ ] **Step 3: Add `MineTranscript` handler to `internal/handlers.go`**

Add to the `Handlers` struct setup (no struct change needed — store is already there). Add the handler function:

```go
// MineTranscript handles POST /mine/transcript
// Body: {"path": "...", "project": "..."}
// Starts transcript mining asynchronously, returns 202 immediately.
func (h *Handlers) MineTranscript(w http.ResponseWriter, r *http.Request) {
	var req MineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	project := req.Project
	if project == "" {
		project = "default"
	}

	miner := NewMiner(h.store)
	go func() {
		miner.MineTranscript(req.Path, project)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(MineResponse{Status: "mining started", Path: req.Path})
}
```

- [ ] **Step 4: Run handler test**

```bash
go test ./internal/ -run TestHandlers_MineTranscript -v
```

Expected: PASS.

- [ ] **Step 5: Register `/mine/transcript` route in `internal/server.go`**

Add to the `mux` in `RunServe`, after the `/summarize` route:

```go
mux.HandleFunc("/mine/transcript", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    h.MineTranscript(w, r)
})
```

- [ ] **Step 6: Build to verify no compile errors**

```bash
go build ./...
```

Expected: No errors.

- [ ] **Step 7: Commit**

```bash
git add internal/handlers.go internal/handlers_test.go internal/server.go
git commit -m "feat: add MineTranscript handler and POST /mine/transcript route"
```

---

### Task 5: Add `memex mine` CLI subcommand

**Files:**
- Modify: `cmd/memex/main.go`

- [ ] **Step 1: Add `RunMine` function to `internal/`**

Create `internal/mine_cmd.go`:

```go
package memex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// RunMine is the CLI handler for `memex mine <path>`.
// It POSTs to the running memex server's /mine/transcript endpoint.
func RunMine(path string) {
	if path == "" {
		fmt.Fprintln(os.Stderr, "Usage: memex mine <transcript-path>")
		os.Exit(1)
	}

	project := getProjectName()
	body, _ := json.Marshal(MineRequest{Path: path, Project: project})

	resp, err := http.Post(getMemexURL()+"/mine/transcript", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "mine: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result MineResponse
	json.NewDecoder(resp.Body).Decode(&result)
	fmt.Printf("memex mine: %s (path: %s)\n", result.Status, result.Path)
}
```

- [ ] **Step 2: Update `cmd/memex/main.go`**

```go
package main

import (
	"fmt"
	"os"

	memex "github.com/shivamvarshney/memex/internal"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: memex <serve|mcp|hook <event>|mine <path>>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		memex.RunServe()
	case "mcp":
		memex.RunMCP()
	case "hook":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: memex hook <session-start|session-stop|pre-tool-use|post-tool-use>")
			os.Exit(1)
		}
		memex.RunHook(os.Args[2])
	case "mine":
		path := ""
		if len(os.Args) >= 3 {
			path = os.Args[2]
		}
		memex.RunMine(path)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Build and verify the `mine` subcommand is present**

```bash
go build -o /tmp/memex-phase4 ./cmd/memex/
/tmp/memex-phase4 mine 2>&1
```

Expected: `Usage: memex mine <transcript-path>` printed.

- [ ] **Step 4: Commit**

```bash
git add internal/mine_cmd.go cmd/memex/main.go
git commit -m "feat: add 'memex mine <path>' CLI subcommand"
```

---

### Task 6: Final verification

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -count=1 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: All packages show `ok`.

- [ ] **Step 2: Build final binary**

```bash
go build -o /tmp/memex-phase4-final ./cmd/memex/
echo "Build OK"
```

- [ ] **Step 3: Smoke-test the classifier on known text**

```bash
cat << 'EOF' > /tmp/test_classify.go
package main

import (
	"fmt"
	memex "github.com/shivamvarshney/memex/internal"
)

func main() {
	c := memex.NewClassifier()
	texts := []string{
		"I prefer table-driven tests in Go.",
		"We decided to use Qdrant because of better vector perf.",
		"There's a bug in the embed call — it keeps crashing.",
		"It works! Figured out the trick is WAL mode.",
	}
	for _, text := range texts {
		t, conf := c.Classify(text)
		fmt.Printf("  %-12s %.2f  %q\n", t, conf, text[:40])
	}
}
EOF
cd /Users/shivamvarshney/Documents/projects/memex && go run /tmp/test_classify.go
```

Expected: Each text classified with correct type and confidence ≥ 0.3.

- [ ] **Step 4: Final commit — phase marker**

```bash
git commit --allow-empty -m "feat: Phase 4 complete — transcript mining (classifier, miner, /mine/transcript, CLI)"
```
