package memex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// fakeMCPServer records calls to memex HTTP endpoints during MCP handler tests.
type fakeMCPServer struct {
	mux    *http.ServeMux
	server *httptest.Server
	called map[string]int
}

func newFakeMCPServer(t *testing.T) *fakeMCPServer {
	t.Helper()
	f := &fakeMCPServer{
		called: make(map[string]int),
	}
	f.mux = http.NewServeMux()

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

	f.mux.HandleFunc("/memories/pinned", func(w http.ResponseWriter, r *http.Request) {
		f.called["GET /memories/pinned"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{Memories: []Memory{
			{ID: "p1", Text: "critical preference", MemoryType: "preference", Importance: 1.0},
		}})
	})

	f.mux.HandleFunc("/memories/similar", func(w http.ResponseWriter, r *http.Request) {
		f.called["GET /memories/similar"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{Memories: []Memory{}})
	})

	f.mux.HandleFunc("/memories/", func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " /memories/"
		f.called[key]++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	})

	f.mux.HandleFunc("/facts", func(w http.ResponseWriter, r *http.Request) {
		f.called[r.Method+" /facts"]++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "fact-id"})
	})

	f.mux.HandleFunc("/facts/stats", func(w http.ResponseWriter, r *http.Request) {
		f.called["GET /facts/stats"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(KGStats{TotalFacts: 5, ActiveFacts: 3, ExpiredFacts: 2})
	})

	f.mux.HandleFunc("/facts/timeline", func(w http.ResponseWriter, r *http.Request) {
		f.called["GET /facts/timeline"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"facts": []Fact{}})
	})

	f.mux.HandleFunc("/facts/", func(w http.ResponseWriter, r *http.Request) {
		f.called[r.Method+" /facts/"]++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"facts": []Fact{}})
	})

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
	if f.called["GET /memories/similar"] != 1 {
		t.Errorf("GET /memories/similar called %d times, want 1", f.called["GET /memories/similar"])
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
	content := fmt.Sprintf("%v", result.Content)
	if !strings.Contains(content, "memex Memory Protocol") {
		t.Errorf("memory_overview response missing protocol header, got: %s", content)
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
	if f.called["PATCH /memories/"] != 1 {
		t.Errorf("PATCH /memories/ called %d times, want 1", f.called["PATCH /memories/"])
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
