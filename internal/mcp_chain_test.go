package memex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chainServer tracks call order and simulates save→pin→verify and fact→history flows.
type chainServer struct {
	server   *httptest.Server
	calls    []string
	pinned   map[string]bool
	memories map[string]Memory
}

func newChainServer(t *testing.T) *chainServer {
	t.Helper()
	f := &chainServer{
		pinned:   make(map[string]bool),
		memories: make(map[string]Memory),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/memories/similar", func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, "find_similar")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{Memories: []Memory{}})
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
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(SearchResponse{Memories: nil})
		}
	})

	mux.HandleFunc("/facts/timeline", func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, "fact_history")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"facts": []Fact{
			{ID: "fact-chain-1", Subject: "memex", Predicate: "uses", Object: "qdrant"},
		}})
	})

	mux.HandleFunc("/facts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f.calls = append(f.calls, "fact_record")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": "fact-chain-1"})
		} else if r.Method == http.MethodGet {
			f.calls = append(f.calls, "fact_query")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"facts": []Fact{}})
		}
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func safeGet(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "<missing>"
}

func TestMCPChain_FindSimilar_Save_Pin(t *testing.T) {
	f := newChainServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	// Step 1: find_similar
	similar, err := callTool(handleFindSimilar, map[string]any{
		"text":    "always use table-driven tests in Go",
		"project": "memex",
	})
	if err != nil || similar.IsError {
		t.Fatalf("find_similar: err=%v isError=%v", err, similar.IsError)
	}

	// Step 2: save_memory
	saved, err := callTool(handleSaveMemory, map[string]any{
		"text":        "always use table-driven tests in Go",
		"project":     "memex",
		"memory_type": "preference",
		"importance":  0.95,
	})
	if err != nil || saved.IsError {
		t.Fatalf("save_memory: err=%v isError=%v", err, saved.IsError)
	}
	if !strings.Contains(fmt.Sprintf("%v", saved.Content), "chain-id-1") {
		t.Errorf("save_memory response missing expected id, got: %v", saved.Content)
	}

	// Step 3: pin_memory
	pinned, err := callTool(handlePinMemory, map[string]any{"id": "chain-id-1"})
	if err != nil || pinned.IsError {
		t.Fatalf("pin_memory: err=%v isError=%v", err, pinned.IsError)
	}

	// Verify call order
	want := []string{"find_similar", "save_memory", "pin_memory"}
	for i, call := range want {
		if i >= len(f.calls) || f.calls[i] != call {
			t.Errorf("call[%d] = %q, want %q (full sequence: %v)", i, safeGet(f.calls, i), call, f.calls)
		}
	}
}

func TestMCPChain_FactQuery_Record_History(t *testing.T) {
	f := newChainServer(t)
	t.Setenv("MEMEX_URL", f.server.URL)

	// Step 1: fact_query
	_, err := callTool(handleFactQuery, map[string]any{"entity": "memex"})
	if err != nil {
		t.Fatalf("fact_query: %v", err)
	}

	// Step 2: fact_record singular
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
	history, err := callTool(handleFactHistory, map[string]any{"entity": "memex"})
	if err != nil || history.IsError {
		t.Fatalf("fact_history: err=%v isError=%v", err, history.IsError)
	}

	// Verify call order
	want := []string{"fact_query", "fact_record", "fact_history"}
	for i, call := range want {
		if i >= len(f.calls) || f.calls[i] != call {
			t.Errorf("call[%d] = %q, want %q (full: %v)", i, safeGet(f.calls, i), call, f.calls)
		}
	}
}
