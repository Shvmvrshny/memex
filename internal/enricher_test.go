package memex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExtractFunctionSource_PlainFunction(t *testing.T) {
	// Uses a real file in the repo so no fixture needed.
	src, err := extractFunctionSource(
		"..",
		"internal/config.go",
		"github.com/shivamvarshney/memex/internal::LoadConfig",
	)
	if err != nil {
		t.Fatalf("extractFunctionSource: %v", err)
	}
	if src == "" {
		t.Fatal("expected non-empty source")
	}
	if !contains(src, "LoadConfig") {
		t.Errorf("source does not contain function name: %s", src)
	}
}

func TestExtractFunctionSource_Method(t *testing.T) {
	src, err := extractFunctionSource(
		"..",
		"internal/kg.go",
		"github.com/shivamvarshney/memex/internal::KnowledgeGraph.QueryEntity",
	)
	if err != nil {
		t.Fatalf("extractFunctionSource: %v", err)
	}
	if src == "" {
		t.Fatal("expected non-empty source")
	}
	if !contains(src, "QueryEntity") {
		t.Errorf("source does not contain method name: %s", src)
	}
}

func TestExtractFunctionSource_NotFound(t *testing.T) {
	_, err := extractFunctionSource(
		"..",
		"internal/config.go",
		"github.com/shivamvarshney/memex/internal::DoesNotExist",
	)
	if err == nil {
		t.Fatal("expected error for missing function")
	}
}

func TestCallOllamaGenerate_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]any{
			"response": `{"summary": "handles session startup by loading memory context", "patterns": ["fallback", "caching"]}`,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	summary, patterns, err := callOllamaGenerate(context.Background(), srv.URL, "llama3.2", "test prompt")
	if err != nil {
		t.Fatalf("callOllamaGenerate: %v", err)
	}
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if len(patterns) == 0 {
		t.Fatal("expected at least one pattern")
	}
}

func TestCallOllamaGenerate_MalformedJSON_FallsBackToRawText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": "plain text summary with no JSON",
		})
	}))
	defer srv.Close()

	summary, patterns, err := callOllamaGenerate(context.Background(), srv.URL, "llama3.2", "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary == "" {
		t.Fatal("expected fallback summary from raw text")
	}
	if len(patterns) != 0 {
		t.Errorf("expected no patterns on fallback, got %v", patterns)
	}
}

func TestCallOllamaGenerate_Unavailable(t *testing.T) {
	_, _, err := callOllamaGenerate(context.Background(), "http://127.0.0.1:1", "llama3.2", "prompt")
	if err == nil {
		t.Fatal("expected error when ollama unavailable")
	}
}

func TestIsCached_Miss(t *testing.T) {
	e := &Enricher{store: newFakeStore()}
	if e.isCached(context.Background(), "memex", "pkg::Foo", "abc123") {
		t.Fatal("expected cache miss on empty store")
	}
}

func TestIsCached_Hit(t *testing.T) {
	fs := newFakeStore()
	_, _ = fs.SaveMemory(context.Background(), SaveMemoryRequest{
		Text:       "summary",
		Project:    "memex",
		Topic:      "pkg::Foo",
		MemoryType: "code_node",
		Source:     "llm-enrichment",
		Tags:       []string{"commit:abc123"},
	})
	e := &Enricher{store: fs}
	if !e.isCached(context.Background(), "memex", "pkg::Foo", "abc123") {
		t.Fatal("expected cache hit")
	}
}

func TestIsCached_WrongCommit(t *testing.T) {
	fs := newFakeStore()
	_, _ = fs.SaveMemory(context.Background(), SaveMemoryRequest{
		Text:       "summary",
		Project:    "memex",
		Topic:      "pkg::Foo",
		MemoryType: "code_node",
		Source:     "llm-enrichment",
		Tags:       []string{"commit:old123"},
	})
	e := &Enricher{store: fs}
	if e.isCached(context.Background(), "memex", "pkg::Foo", "newcommit") {
		t.Fatal("expected cache miss on commit mismatch")
	}
}

func TestResolveFilePath_Found(t *testing.T) {
	kg, err := NewKnowledgeGraph(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := kg.Init(); err != nil {
		t.Fatal(err)
	}
	_, err = kg.RecordFactScoped(Fact{
		Subject:    "internal/hook.go",
		Predicate:  PredicateContainsFunction,
		Object:     "github.com/shivamvarshney/memex/internal::hookSessionStart",
		FilePath:   "internal/hook.go",
		CommitHash: "abc123",
		Source:     "ast",
		Confidence: 1,
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	e := &Enricher{kg: kg}
	fp, err := e.resolveFilePath("github.com/shivamvarshney/memex/internal::hookSessionStart")
	if err != nil {
		t.Fatalf("resolveFilePath: %v", err)
	}
	if fp != "internal/hook.go" {
		t.Errorf("expected internal/hook.go, got %s", fp)
	}
}

func TestResolveFilePath_NotFound(t *testing.T) {
	kg, err := NewKnowledgeGraph(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := kg.Init(); err != nil {
		t.Fatal(err)
	}
	e := &Enricher{kg: kg}
	_, err = e.resolveFilePath("pkg::NoSuchFunc")
	if err == nil {
		t.Fatal("expected error for unknown function")
	}
}

func TestEnrich_StoresCodeNode(t *testing.T) {
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": `{"summary":"starts a session by loading pinned and semantic memories","patterns":["caching","fallback"]}`,
		})
	}))
	defer ollamaSrv.Close()

	kg, _ := NewKnowledgeGraph(":memory:")
	_ = kg.Init()
	_, _ = kg.RecordFactScoped(Fact{
		Subject:    "internal/hook.go",
		Predicate:  PredicateContainsFunction,
		Object:     "github.com/shivamvarshney/memex/internal::hookSessionStart",
		FilePath:   "internal/hook.go",
		CommitHash: "abc123",
		Source:     "ast",
		Confidence: 1,
	}, false)

	fs := newFakeStore()
	e := NewEnricher(fs, kg, ollamaSrv.URL, "llama3.2", "..")

	if err := e.enrich(context.Background(), "github.com/shivamvarshney/memex/internal::hookSessionStart", "memex", "abc123"); err != nil {
		t.Fatalf("enrich: %v", err)
	}

	results, _ := fs.ListMemories(context.Background(), "memex", "code_node", "github.com/shivamvarshney/memex/internal::hookSessionStart", nil, 10)
	if len(results) == 0 {
		t.Fatal("expected code_node to be saved in store")
	}
	if results[0].MemoryType != "code_node" {
		t.Errorf("wrong memory_type: %s", results[0].MemoryType)
	}

	foundCommitTag := false
	for _, tag := range results[0].Tags {
		if tag == "commit:abc123" {
			foundCommitTag = true
		}
	}
	if !foundCommitTag {
		t.Errorf("commit tag missing from %v", results[0].Tags)
	}
}

func TestEnrichAsync_DeduplicatesInflight(t *testing.T) {
	calls := 0
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": `{"summary":"test","patterns":[]}`,
		})
	}))
	defer ollamaSrv.Close()

	kg, _ := NewKnowledgeGraph(":memory:")
	_ = kg.Init()
	_, _ = kg.RecordFactScoped(Fact{
		Subject:    "internal/hook.go",
		Predicate:  PredicateContainsFunction,
		Object:     "pkg::Fn",
		FilePath:   "internal/hook.go",
		CommitHash: "abc",
		Source:     "ast",
		Confidence: 1,
	}, false)

	fs := newFakeStore()
	e := NewEnricher(fs, kg, ollamaSrv.URL, "llama3.2", ".")

	e.inflight.Store("pkg::Fn|memex|abc", struct{}{})
	e.EnrichAsync(context.Background(), "pkg::Fn", "memex", "abc")
	time.Sleep(50 * time.Millisecond)

	if calls != 0 {
		t.Errorf("expected 0 LLM calls when inflight, got %d", calls)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
