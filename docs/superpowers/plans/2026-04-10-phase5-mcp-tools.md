# Phase 5: MCP Tools — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 4 existing MCP tools with 13 purpose-built tools. The new tools are self-describing (Claude learns the 9-type taxonomy on first call via `memory_overview`), type+topic-filtered (better retrieval), include duplicate detection (`find_similar`), expose the full Knowledge Graph (`fact_*`), and add `pin_memory` and `digest_session`. All tools call the local HTTP server — no direct store access.

**Architecture:** `internal/mcp.go` is fully rewritten. All handler functions call `http://localhost:8765/...` (or `MEMEX_URL`). No new files. Phases 1–4 must be complete (all 13 endpoints must exist).

**Tech Stack:** Go 1.26, `github.com/mark3labs/mcp-go`, existing `net/http`.

---

## File Map

| File | Change |
|---|---|
| `internal/mcp.go` | Full rewrite: 4 tools → 13 tools |
| `internal/mcp_test.go` | NEW: integration tests that spin up a test HTTP server |

---

### Task 1: Write integration tests for the new MCP tool handlers

**Files:**
- Create: `internal/mcp_test.go`

The tests run a fake HTTP server that records which endpoints were called, then invoke each MCP handler function directly.

- [ ] **Step 1: Create `internal/mcp_test.go`**

```go
package memex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// fakeMCPServer records calls to memex HTTP endpoints during MCP handler tests.
type fakeMCPServer struct {
	mux     *http.ServeMux
	server  *httptest.Server
	called  map[string]int // endpoint → call count
	lastReq map[string][]byte
}

func newFakeMCPServer(t *testing.T) *fakeMCPServer {
	t.Helper()
	f := &fakeMCPServer{
		called:  make(map[string]int),
		lastReq: make(map[string][]byte),
	}
	f.mux = http.NewServeMux()

	// /memories POST — save
	f.mux.HandleFunc("/memories", func(w http.ResponseWriter, r *http.Request) {
		f.called[r.Method+" /memories"]++
		switch r.Method {
		case http.MethodPost:
			var req SaveMemoryRequest
			json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Memory{ID: "test-id", Text: req.Text, MemoryType: req.MemoryType})
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(SearchResponse{Memories: []Memory{
				{ID: "m1", Text: "prefer table-driven tests", MemoryType: "preference"},
			}})
		}
	})

	// /memories/pinned — list pinned
	f.mux.HandleFunc("/memories/pinned", func(w http.ResponseWriter, r *http.Request) {
		f.called["GET /memories/pinned"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{Memories: []Memory{
			{ID: "p1", Text: "critical preference", MemoryType: "preference", Importance: 1.0},
		}})
	})

	// /memories/similar — find similar
	f.mux.HandleFunc("/memories/similar", func(w http.ResponseWriter, r *http.Request) {
		f.called["POST /memories/similar"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{Memories: []Memory{}})
	})

	// /memories/ — delete + pin
	f.mux.HandleFunc("/memories/", func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " /memories/"
		f.called[key]++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// /facts POST — record fact
	f.mux.HandleFunc("/facts", func(w http.ResponseWriter, r *http.Request) {
		f.called[r.Method+" /facts"]++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "fact-id"})
	})

	// /facts/stats — stats
	f.mux.HandleFunc("/facts/stats", func(w http.ResponseWriter, r *http.Request) {
		f.called["GET /facts/stats"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(KGStats{TotalFacts: 5, ActiveFacts: 3, ExpiredFacts: 2})
	})

	// /facts/timeline — history
	f.mux.HandleFunc("/facts/timeline", func(w http.ResponseWriter, r *http.Request) {
		f.called["GET /facts/timeline"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"facts": []Fact{}})
	})

	// /facts/ — expire + query
	f.mux.HandleFunc("/facts/", func(w http.ResponseWriter, r *http.Request) {
		f.called[r.Method+" /facts/"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"facts": []Fact{}})
	})

	// /mine/transcript
	f.mux.HandleFunc("/mine/transcript", func(w http.ResponseWriter, r *http.Request) {
		f.called["POST /mine/transcript"]++
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(MineResponse{Status: "mining started"})
	})

	f.server = httptest.NewServer(f.mux)
	t.Cleanup(f.server.Close)
	return f
}

func callTool(fn func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error),
	args map[string]any) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return fn(context.Background(), req)
}

func TestMCP_SaveMemory(t *testing.T) {
	f := newFakeMCPServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	result, err := callTool(handleSaveMemory, map[string]any{
		"text":        "prefer table-driven tests in Go",
		"project":     "memex",
		"memory_type": "preference",
		"importance":  0.8,
	})
	if err != nil {
		t.Fatalf("handleSaveMemory: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
	if f.called["POST /memories"] != 1 {
		t.Errorf("POST /memories called %d times, want 1", f.called["POST /memories"])
	}
}

func TestMCP_SearchMemory(t *testing.T) {
	f := newFakeMCPServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	result, err := callTool(handleSearchMemory, map[string]any{
		"context": "testing preferences",
	})
	if err != nil {
		t.Fatalf("handleSearchMemory: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
}

func TestMCP_FindSimilar(t *testing.T) {
	f := newFakeMCPServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	result, err := callTool(handleFindSimilar, map[string]any{
		"text": "prefer table-driven tests",
	})
	if err != nil {
		t.Fatalf("handleFindSimilar: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
	if f.called["POST /memories/similar"] != 1 {
		t.Errorf("POST /memories/similar called %d times, want 1", f.called["POST /memories/similar"])
	}
}

func TestMCP_MemoryOverview(t *testing.T) {
	f := newFakeMCPServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	result, err := callTool(handleMemoryOverview, map[string]any{})
	if err != nil {
		t.Fatalf("handleMemoryOverview: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
	// Must contain the protocol text
	content := fmt.Sprintf("%v", result.Content)
	if len(content) < 50 {
		t.Errorf("memory_overview response too short (%d chars), expected protocol text", len(content))
	}
}

func TestMCP_PinMemory(t *testing.T) {
	f := newFakeMCPServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	result, err := callTool(handlePinMemory, map[string]any{
		"id": "mem-123",
	})
	if err != nil {
		t.Fatalf("handlePinMemory: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
}

func TestMCP_FactRecord(t *testing.T) {
	f := newFakeMCPServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	result, err := callTool(handleFactRecord, map[string]any{
		"subject":   "alice",
		"predicate": "works_on",
		"object":    "memex",
	})
	if err != nil {
		t.Fatalf("handleFactRecord: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
	if f.called["POST /facts"] != 1 {
		t.Errorf("POST /facts called %d times, want 1", f.called["POST /facts"])
	}
}

func TestMCP_FactStats(t *testing.T) {
	f := newFakeMCPServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	result, err := callTool(handleFactStats, map[string]any{})
	if err != nil {
		t.Fatalf("handleFactStats: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
	if f.called["GET /facts/stats"] != 1 {
		t.Errorf("GET /facts/stats called %d times, want 1", f.called["GET /facts/stats"])
	}
}

func TestMCP_DigestSession(t *testing.T) {
	f := newFakeMCPServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	result, err := callTool(handleDigestSession, map[string]any{
		"path": "/tmp/session.jsonl",
	})
	if err != nil {
		t.Fatalf("handleDigestSession: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
	if f.called["POST /mine/transcript"] != 1 {
		t.Errorf("POST /mine/transcript called %d times, want 1", f.called["POST /mine/transcript"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
go test ./internal/ -run TestMCP_ -v 2>&1 | head -30
```

Expected: FAIL — `handleFindSimilar`, `handleMemoryOverview`, `handlePinMemory`, `handleFactRecord`, `handleFactStats`, `handleDigestSession` not defined.

---

### Task 2: Rewrite `internal/mcp.go` with all 13 tools

**Files:**
- Modify: `internal/mcp.go`

- [ ] **Step 1: Replace `internal/mcp.go` entirely**

```go
package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const memexProtocol = `memex Memory Protocol:
1. ON SESSION START: memory context is pre-loaded. No need to call memory_overview immediately.
2. BEFORE SAVING: call find_similar first. If any results are returned, skip or update instead.
3. CHOOSING memory_type:
   - decision:   architecture choices, locked-in resolutions
   - preference: coding style, tool choices, habits (use pin_memory for critical ones)
   - event:      deployments, milestones, sessions
   - discovery:  breakthroughs, "it works" moments, key insights
   - advice:     recommendations, best practices, solutions
   - problem:    bugs, errors, root causes + fixes
   - context:    team members, org structure, project relationships
   - procedure:  workflows, build steps, repeatable processes
   - rationale:  WHY a decision was made, trade-offs considered
4. USE KG (fact_*) FOR: named entities, relationships, facts that change over time
   USE Qdrant (save_memory) FOR: unstructured knowledge, explanations, context blobs
5. PINNING: call pin_memory on any memory that must survive every session-start
6. WHEN FACTS CHANGE: call fact_expire on the old fact, fact_record for the new one`

// RunMCP starts the memex MCP server on stdio with 13 tools.
func RunMCP() {
	s := server.NewMCPServer("memex", "2.0.0",
		server.WithToolCapabilities(true),
	)

	// ── Memory tools (6) ──────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("save_memory",
			mcp.WithDescription("Save a typed memory to long-term storage. Call find_similar first to check for duplicates. Requires memory_type — one of: decision, preference, event, discovery, advice, problem, context, procedure, rationale."),
			mcp.WithString("text", mcp.Required(), mcp.Description("The memory text, written as a clear statement")),
			mcp.WithString("memory_type", mcp.Required(), mcp.Description("One of: decision, preference, event, discovery, advice, problem, context, procedure, rationale")),
			mcp.WithString("project", mcp.Description("Project name (optional)")),
			mcp.WithString("topic", mcp.Description("Topic slug e.g. 'auth-migration', 'ci-pipeline' (optional)")),
			mcp.WithNumber("importance", mcp.Description("0.0-1.0, default 0.5. Use 0.9+ for critical preferences/decisions.")),
		),
		handleSaveMemory,
	)

	s.AddTool(
		mcp.NewTool("search_memory",
			mcp.WithDescription("Semantic search over stored memories. Accepts optional memory_type, topic, and project filters for higher precision."),
			mcp.WithString("context", mcp.Required(), mcp.Description("What you want to recall")),
			mcp.WithString("project", mcp.Description("Filter by project (optional)")),
			mcp.WithString("memory_type", mcp.Description("Filter by type: decision|preference|event|discovery|advice|problem|context|procedure|rationale (optional)")),
			mcp.WithString("topic", mcp.Description("Filter by topic slug (optional)")),
		),
		handleSearchMemory,
	)

	s.AddTool(
		mcp.NewTool("list_memories",
			mcp.WithDescription("List stored memories, optionally filtered by project, memory_type, and topic."),
			mcp.WithString("project", mcp.Description("Filter by project (optional)")),
			mcp.WithString("memory_type", mcp.Description("Filter by type (optional)")),
			mcp.WithString("topic", mcp.Description("Filter by topic slug (optional)")),
		),
		handleListMemories,
	)

	s.AddTool(
		mcp.NewTool("delete_memory",
			mcp.WithDescription("Delete a memory by ID. Search for the old memory first, then delete it before saving the updated version."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Memory ID to delete")),
		),
		handleDeleteMemory,
	)

	s.AddTool(
		mcp.NewTool("find_similar",
			mcp.WithDescription("Embed candidate text and return the most similar existing memories with similarity scores. Call this before save_memory to detect duplicates. If results are returned, skip saving or update the existing memory instead."),
			mcp.WithString("text", mcp.Required(), mcp.Description("Candidate text to check for similarity")),
			mcp.WithString("project", mcp.Description("Scope to project (optional)")),
		),
		handleFindSimilar,
	)

	s.AddTool(
		mcp.NewTool("memory_overview",
			mcp.WithDescription("Returns the memex protocol, memory taxonomy, total counts, and KG stats. Call this when you need orientation — it teaches you when to save, which type to use, and when to use KG vs Qdrant."),
			mcp.WithString("project", mcp.Description("Show breakdown for this project (optional)")),
		),
		handleMemoryOverview,
	)

	// ── Knowledge Graph tools (5) ─────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("fact_record",
			mcp.WithDescription("Record a subject→predicate→object fact triple. If singular=true, closes any existing active fact for the same subject+predicate before inserting (use for facts that replace previous values, e.g. 'alice works_on X')."),
			mcp.WithString("subject", mcp.Required(), mcp.Description("The entity this fact is about")),
			mcp.WithString("predicate", mcp.Required(), mcp.Description("The relationship or property, snake_case e.g. 'works_on', 'reports_to', 'uses_tool'")),
			mcp.WithString("object", mcp.Required(), mcp.Description("The value or target entity")),
			mcp.WithString("valid_from", mcp.Description("ISO8601 timestamp when this became true (default: now)")),
			mcp.WithString("source", mcp.Description("Where this fact came from e.g. 'user-stated', 'inferred'")),
			mcp.WithBoolean("singular", mcp.Description("If true, auto-expires the previous active fact for same subject+predicate (default: false)")),
		),
		handleFactRecord,
	)

	s.AddTool(
		mcp.NewTool("fact_query",
			mcp.WithDescription("Return current facts about an entity (subject or object). Pass as_of to query what was true at a point in time."),
			mcp.WithString("entity", mcp.Required(), mcp.Description("Entity name to look up")),
			mcp.WithString("as_of", mcp.Description("ISO8601 timestamp for point-in-time query (default: now)")),
		),
		handleFactQuery,
	)

	s.AddTool(
		mcp.NewTool("fact_expire",
			mcp.WithDescription("Close a fact's validity window. The fact is preserved for history — it is never deleted. Use when a fact is no longer true before recording the replacement."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Fact ID to expire")),
		),
		handleFactExpire,
	)

	s.AddTool(
		mcp.NewTool("fact_history",
			mcp.WithDescription("Return the full chronological history of all facts (active and expired) for an entity."),
			mcp.WithString("entity", mcp.Required(), mcp.Description("Entity name to look up")),
		),
		handleFactHistory,
	)

	s.AddTool(
		mcp.NewTool("fact_stats",
			mcp.WithDescription("Return KG overview: total facts, active/expired counts, entity count, and relationship types."),
		),
		handleFactStats,
	)

	// ── Pinned tool (1) ───────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("pin_memory",
			mcp.WithDescription("Promote a memory to L1 (importance = 1.0) so it is always loaded on every session-start. Use this for critical preferences or decisions that must never be forgotten."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Memory ID to pin")),
		),
		handlePinMemory,
	)

	// ── Mining tool (1) ───────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("digest_session",
			mcp.WithDescription("Trigger transcript mining for a past session. Runs asynchronously — returns immediately. Use this to extract typed memories from a JSONL transcript you want indexed."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Path to the Claude Code JSONL transcript file")),
			mcp.WithString("project", mcp.Description("Project to associate mined memories with (optional)")),
		),
		handleDigestSession,
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}

// ─── Memory Handlers ─────────────────────────────────────────────────────────

func handleSaveMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, _ := req.Params.Arguments["text"].(string)
	memoryType, _ := req.Params.Arguments["memory_type"].(string)
	project, _ := req.Params.Arguments["project"].(string)
	topic, _ := req.Params.Arguments["topic"].(string)
	importance, _ := req.Params.Arguments["importance"].(float64)
	if importance == 0 {
		importance = 0.5
	}

	body := SaveMemoryRequest{
		Text:       text,
		MemoryType: memoryType,
		Project:    project,
		Topic:      topic,
		Source:     "claude-code",
		Importance: float32(importance),
	}
	data, _ := json.Marshal(body)

	resp, err := http.Post(getMemexURL()+"/memories", "application/json", bytes.NewReader(data))
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable — is Docker running?"), nil
	}
	defer resp.Body.Close()

	var mem Memory
	json.NewDecoder(resp.Body).Decode(&mem)
	return mcp.NewToolResultText(fmt.Sprintf("memory saved (id: %s, type: %s)", mem.ID, mem.MemoryType)), nil
}

func handleSearchMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, _ := req.Params.Arguments["context"].(string)
	project, _ := req.Params.Arguments["project"].(string)
	memoryType, _ := req.Params.Arguments["memory_type"].(string)
	topic, _ := req.Params.Arguments["topic"].(string)

	apiURL := fmt.Sprintf("%s/memories?context=%s&project=%s&memory_type=%s&topic=%s&limit=5",
		getMemexURL(),
		url.QueryEscape(query),
		url.QueryEscape(project),
		url.QueryEscape(memoryType),
		url.QueryEscape(topic),
	)
	resp, err := http.Get(apiURL)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()

	var result SearchResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Memories) == 0 {
		return mcp.NewToolResultText("no memories found"), nil
	}
	data, _ := json.MarshalIndent(result.Memories, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func handleListMemories(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, _ := req.Params.Arguments["project"].(string)
	memoryType, _ := req.Params.Arguments["memory_type"].(string)
	topic, _ := req.Params.Arguments["topic"].(string)

	apiURL := fmt.Sprintf("%s/memories?project=%s&memory_type=%s&topic=%s&limit=100",
		getMemexURL(),
		url.QueryEscape(project),
		url.QueryEscape(memoryType),
		url.QueryEscape(topic),
	)
	resp, err := http.Get(apiURL)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()

	var result SearchResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Memories) == 0 {
		return mcp.NewToolResultText("no memories stored yet"), nil
	}
	data, _ := json.MarshalIndent(result.Memories, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func handleDeleteMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, _ := req.Params.Arguments["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/memories/%s", getMemexURL(), url.PathEscape(id)), nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request"), nil
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return mcp.NewToolResultError(fmt.Sprintf("delete failed: status %d", resp.StatusCode)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("memory deleted (id: %s)", id)), nil
}

func handleFindSimilar(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, _ := req.Params.Arguments["text"].(string)
	project, _ := req.Params.Arguments["project"].(string)

	body := map[string]any{"text": text, "project": project, "limit": 5}
	data, _ := json.Marshal(body)

	resp, err := http.Post(getMemexURL()+"/memories/similar", "application/json", bytes.NewReader(data))
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()

	var result SearchResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Memories) == 0 {
		return mcp.NewToolResultText("no similar memories found — safe to save"), nil
	}
	data, _ = json.MarshalIndent(result.Memories, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("similar memories found (skip or update instead of saving):\n%s", string(data))), nil
}

func handleMemoryOverview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, _ := req.Params.Arguments["project"].(string)

	// Fetch memory list for counts
	apiURL := fmt.Sprintf("%s/memories?project=%s&limit=1000", getMemexURL(), url.QueryEscape(project))
	resp, err := http.Get(apiURL)
	if err != nil {
		// Return just the protocol if server is unavailable
		return mcp.NewToolResultText(memexProtocol), nil
	}
	defer resp.Body.Close()

	var result SearchResponse
	json.NewDecoder(resp.Body).Decode(&result)

	// Count by type
	typeCounts := make(map[string]int)
	for _, m := range result.Memories {
		typeCounts[m.MemoryType]++
	}

	// Fetch KG stats
	kgResp, err := http.Get(getMemexURL() + "/facts/stats")
	var kgStats KGStats
	if err == nil {
		defer kgResp.Body.Close()
		json.NewDecoder(kgResp.Body).Decode(&kgStats)
	}

	typeCountsJSON, _ := json.MarshalIndent(typeCounts, "", "  ")
	kgJSON, _ := json.MarshalIndent(kgStats, "", "  ")

	output := fmt.Sprintf("%s\n\n--- Memory Counts (project: %q) ---\n%s\n\n--- Knowledge Graph ---\n%s",
		memexProtocol,
		project,
		string(typeCountsJSON),
		string(kgJSON),
	)
	return mcp.NewToolResultText(output), nil
}

// ─── Knowledge Graph Handlers ─────────────────────────────────────────────────

func handleFactRecord(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	subject, _ := req.Params.Arguments["subject"].(string)
	predicate, _ := req.Params.Arguments["predicate"].(string)
	object, _ := req.Params.Arguments["object"].(string)
	validFrom, _ := req.Params.Arguments["valid_from"].(string)
	source, _ := req.Params.Arguments["source"].(string)
	singular, _ := req.Params.Arguments["singular"].(bool)

	body := RecordFactRequest{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		ValidFrom: validFrom,
		Source:    source,
		Singular:  singular,
	}
	data, _ := json.Marshal(body)

	resp, err := http.Post(getMemexURL()+"/facts", "application/json", bytes.NewReader(data))
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	return mcp.NewToolResultText(fmt.Sprintf("fact recorded (id: %s)", result["id"])), nil
}

func handleFactQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entity, _ := req.Params.Arguments["entity"].(string)
	asOf, _ := req.Params.Arguments["as_of"].(string)

	apiURL := fmt.Sprintf("%s/facts?subject=%s&as_of=%s",
		getMemexURL(), url.QueryEscape(entity), url.QueryEscape(asOf))
	resp, err := http.Get(apiURL)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func handleFactExpire(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, _ := req.Params.Arguments["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/facts/%s", getMemexURL(), url.PathEscape(id)), nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request"), nil
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()
	return mcp.NewToolResultText(fmt.Sprintf("fact expired (id: %s)", id)), nil
}

func handleFactHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entity, _ := req.Params.Arguments["entity"].(string)

	apiURL := fmt.Sprintf("%s/facts/timeline?entity=%s", getMemexURL(), url.QueryEscape(entity))
	resp, err := http.Get(apiURL)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func handleFactStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := http.Get(getMemexURL() + "/facts/stats")
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()

	var stats KGStats
	json.NewDecoder(resp.Body).Decode(&stats)
	data, _ := json.MarshalIndent(stats, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// ─── Pinned Handler ───────────────────────────────────────────────────────────

func handlePinMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, _ := req.Params.Arguments["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/memories/%s/pin", getMemexURL(), url.PathEscape(id)), nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request"), nil
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return mcp.NewToolResultError(fmt.Sprintf("pin failed: status %d", resp.StatusCode)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("memory pinned (id: %s) — will be loaded on every session-start", id)), nil
}

// ─── Mining Handler ───────────────────────────────────────────────────────────

func handleDigestSession(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := req.Params.Arguments["path"].(string)
	project, _ := req.Params.Arguments["project"].(string)

	body := MineRequest{Path: path, Project: project}
	data, _ := json.Marshal(body)

	resp, err := http.Post(getMemexURL()+"/mine/transcript", "application/json", bytes.NewReader(data))
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable"), nil
	}
	defer resp.Body.Close()

	var result MineResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return mcp.NewToolResultText(fmt.Sprintf("digest session: %s (path: %s)", result.Status, path)), nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
go test ./internal/ -run TestMCP_ -v
```

Expected: PASS — all 8 MCP tests pass.

- [ ] **Step 3: Build to verify no compile errors**

```bash
go build ./...
```

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp.go internal/mcp_test.go
git commit -m "feat: rewrite MCP server — 4 tools → 13 tools (memory, KG, pin, digest)"
```

---

### Task 3: Add `fmt` import to mcp_test.go if needed

The test file uses `fmt.Sprintf`. Verify imports are correct.

- [ ] **Step 1: Check and fix mcp_test.go imports**

```bash
go vet ./internal/
```

Expected: No vet errors. If any import is missing, `go vet` will tell you — add the missing import and re-run.

- [ ] **Step 2: Run all tests one final time**

```bash
go test ./... -count=1 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: All packages show `ok`.

- [ ] **Step 3: Commit**

```bash
git add internal/mcp_test.go
git commit -m "fix: ensure mcp_test.go imports are complete"
```

---

### Task 4: Final verification — all 5 phases

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -v 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL|ok)"
```

Expected: All tests pass. Zero failures.

- [ ] **Step 2: Build final binary**

```bash
go build -o /tmp/memex-v2 ./cmd/memex/
echo "Phase 5 build OK"
```

- [ ] **Step 3: Verify tool count in MCP server**

```bash
grep "s.AddTool" internal/mcp.go | wc -l
```

Expected: `13`

- [ ] **Step 4: Final commit**

```bash
git commit --allow-empty -m "feat: Phase 5 complete — 13 MCP tools, memex v2 backend done"
```

---

### Execution Handoff

All 5 phases are now planned. The implementation order is:

```
Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5
```

Each phase ends with its own commit. Do not push to GitHub until all 5 are complete and the user has reviewed and approved.
