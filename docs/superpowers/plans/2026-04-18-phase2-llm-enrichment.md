# Phase 2: Lazy LLM Enrichment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Populate Qdrant with retrieval-oriented summaries of AST-indexed code nodes so that natural language queries can find the right entry nodes before KG traversal.

**Architecture:** An `Enricher` service fires asynchronously when `ExpandSearch` surfaces function nodes — it extracts source from the file, calls Ollama to generate a retrieval-key summary, and saves it to Qdrant as a `code_node` memory scoped to `(function_id, commit_hash)`. On subsequent calls the cache is checked and the LLM is skipped. Qdrant then becomes the semantic doorway into the KG: queries find code nodes by meaning, KG expansion finds their structural context.

**Tech Stack:** Go stdlib, go/ast, go/token, go/parser, Ollama `/api/generate`, existing `Store` / `KnowledgeGraph` interfaces.

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/enricher.go` | Create | Enricher struct, EnrichAsync, cache check, source extraction, LLM call, Qdrant write |
| `internal/enricher_test.go` | Create | Unit tests with mock HTTP servers for Ollama and a fake Store |
| `internal/models.go` | Modify | Add `code_node` to ValidMemoryTypes |
| `internal/config.go` | Modify | Add `OllamaModel` + `RepoRoot` config fields |
| `internal/handlers.go` | Modify | Add `enricher *Enricher` to Handlers, trigger from ExpandSearch |
| `internal/server.go` | Modify | Instantiate Enricher and pass to NewHandlers |

---

### Task 1: Add `code_node` memory type and extend Config

**Files:**
- Modify: `internal/models.go:6-16`
- Modify: `internal/config.go:17-54`

- [ ] **Step 1: Add `code_node` to ValidMemoryTypes in models.go**

```go
var ValidMemoryTypes = map[string]bool{
	"decision":   true,
	"preference": true,
	"event":      true,
	"discovery":  true,
	"advice":     true,
	"problem":    true,
	"context":    true,
	"procedure":  true,
	"rationale":  true,
	"code_node":  true, // AST-derived retrieval node
}
```

- [ ] **Step 2: Add `OllamaModel` and `RepoRoot` to Config struct and LoadConfig**

```go
type Config struct {
	Port         string
	QdrantURL    string
	OllamaURL    string
	OllamaModel  string // LLM model for enrichment (OLLAMA_MODEL env var)
	RepoRoot     string // absolute path to repo being indexed (REPO_ROOT env var)
	IdentityPath string
	KGPath       string
}

func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8765"
	}
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6333"
	}
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "llama3.2"
	}
	repoRoot := os.Getenv("REPO_ROOT")
	if repoRoot == "" {
		repoRoot = "."
	}
	home, _ := os.UserHomeDir()
	identityPath := os.Getenv("IDENTITY_PATH")
	if identityPath == "" {
		identityPath = filepath.Join(home, ".memex", "identity.md")
	}
	kgPath := os.Getenv("KG_PATH")
	if kgPath == "" {
		kgPath = filepath.Join(home, ".memex", "knowledge_graph.db")
	}
	return Config{
		Port:         port,
		QdrantURL:    qdrantURL,
		OllamaURL:    ollamaURL,
		OllamaModel:  ollamaModel,
		RepoRoot:     repoRoot,
		IdentityPath: identityPath,
		KGPath:       kgPath,
	}
}
```

- [ ] **Step 3: Verify it compiles**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
go build ./...
```

Expected: no output (clean build).

- [ ] **Step 4: Commit**

```bash
git add internal/models.go internal/config.go
git commit -m "feat(enricher): add code_node memory type and OllamaModel/RepoRoot config"
```

---

### Task 2: Function source extractor

**Files:**
- Create: `internal/enricher.go` (initial slice — source extraction only)
- Create: `internal/enricher_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/enricher_test.go`:

```go
package memex

import (
	"testing"
)

func TestExtractFunctionSource_PlainFunction(t *testing.T) {
	// Uses a real file in the repo so no fixture needed.
	src, err := extractFunctionSource(
		".",
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
		".",
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
		".",
		"internal/config.go",
		"github.com/shivamvarshney/memex/internal::DoesNotExist",
	)
	if err == nil {
		t.Fatal("expected error for missing function")
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
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal -run 'TestExtractFunctionSource' -v
```

Expected: `FAIL — undefined: extractFunctionSource`

- [ ] **Step 3: Implement `extractFunctionSource` in enricher.go**

Create `internal/enricher.go`:

```go
package memex

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// extractFunctionSource reads the named function/method from filePath (relative to repoRoot)
// and returns its source as a string.
//
// functionID format: "<package>::<FuncName>" or "<package>::<ReceiverType>.<MethodName>"
func extractFunctionSource(repoRoot, filePath, functionID string) (string, error) {
	parts := strings.SplitN(functionID, "::", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid function_id %q: missing :: separator", functionID)
	}
	qname := parts[1] // e.g. "LoadConfig" or "KnowledgeGraph.QueryEntity"

	var receiverType, fnName string
	if dot := strings.LastIndex(qname, "."); dot >= 0 {
		receiverType = qname[:dot] // "KnowledgeGraph"
		fnName = qname[dot+1:]    // "QueryEntity"
	} else {
		fnName = qname
	}

	absPath := filepath.Join(repoRoot, filePath)
	src, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", absPath, err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, src, 0)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", absPath, err)
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != fnName {
			continue
		}
		if receiverType != "" {
			if fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			recvName := receiverTypeName(fn.Recv.List[0].Type)
			if recvName != receiverType {
				continue
			}
		} else if fn.Recv != nil {
			continue // skip methods when seeking a plain function
		}
		start := fset.Position(fn.Pos()).Offset
		end := fset.Position(fn.End()).Offset
		return string(src[start:end]), nil
	}
	return "", fmt.Errorf("function %q not found in %s", qname, filePath)
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// Enricher generates and caches LLM retrieval summaries for AST code nodes.
// It is safe for concurrent use. All enrichment runs asynchronously.
type Enricher struct {
	store     Store
	kg        *KnowledgeGraph
	ollamaURL string
	model     string
	repoRoot  string
	inflight  sync.Map // function_id → struct{}{}
}

func NewEnricher(store Store, kg *KnowledgeGraph, ollamaURL, model, repoRoot string) *Enricher {
	return &Enricher{
		store:     store,
		kg:        kg,
		ollamaURL: ollamaURL,
		model:     model,
		repoRoot:  repoRoot,
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal -run 'TestExtractFunctionSource' -v
```

Expected:
```
--- PASS: TestExtractFunctionSource_PlainFunction
--- PASS: TestExtractFunctionSource_Method
--- PASS: TestExtractFunctionSource_NotFound
PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/enricher.go internal/enricher_test.go
git commit -m "feat(enricher): add function source extractor with AST parser"
```

---

### Task 3: LLM enrichment call

**Files:**
- Modify: `internal/enricher.go` (add `callOllamaGenerate`)
- Modify: `internal/enricher_test.go` (add LLM call tests)

- [ ] **Step 1: Write the failing test**

Add to `internal/enricher_test.go`:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

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
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal -run 'TestCallOllamaGenerate' -v
```

Expected: `FAIL — undefined: callOllamaGenerate`

- [ ] **Step 3: Implement `callOllamaGenerate` in enricher.go**

Add to `internal/enricher.go` (after the `NewEnricher` func, before the closing of the file):

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const enrichmentPromptTemplate = `Analyze this Go function.

Your goal is NOT to document it.
Your goal is to make it discoverable via natural language queries.

Write a description that answers:
- What does this function do?
- When is it used?
- What problems does it solve?
- What patterns are present (fallback, retry, caching, error handling, ranking, filtering, etc.)?

IMPORTANT:
- Include terms users would search for (e.g. "fallback", "ranking", "error handling")
- Prefer natural language over code terminology
- Include cause-effect phrasing ("if X fails, it does Y")
- 1-3 sentences max, dense with meaning

Function:
%s

Return ONLY valid JSON, no other text:
{"summary": "...", "patterns": ["pattern1", "pattern2"]}`

// callOllamaGenerate sends a generation request to Ollama and parses the summary + patterns.
// If the LLM response is not valid JSON, the raw text is used as the summary with no patterns.
func callOllamaGenerate(ctx context.Context, ollamaURL, model, prompt string) (summary string, patterns []string, err error) {
	body, _ := json.Marshal(map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, fmt.Errorf("decode ollama response: %w", err)
	}

	var parsed struct {
		Summary  string   `json:"summary"`
		Patterns []string `json:"patterns"`
	}
	raw := strings.TrimSpace(result.Response)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || parsed.Summary == "" {
		// Fallback: use raw text as summary, no structured patterns.
		return raw, nil, nil
	}
	return parsed.Summary, parsed.Patterns, nil
}
```

> **Note on imports:** The full import block at the top of `enricher.go` must include all packages used across the entire file. After adding `callOllamaGenerate`, update the import block to include `"bytes"`, `"context"`, `"net/http"`, `"time"` alongside the existing ones.

- [ ] **Step 4: Run tests**

```bash
go test ./internal -run 'TestCallOllamaGenerate' -v
```

Expected: all three tests PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./...
```

Expected: `ok github.com/shivamvarshney/memex/internal`

- [ ] **Step 6: Commit**

```bash
git add internal/enricher.go internal/enricher_test.go
git commit -m "feat(enricher): add Ollama LLM call with JSON pattern extraction"
```

---

### Task 4: Cache check and Qdrant write

**Files:**
- Modify: `internal/enricher.go` (add `isCached`, `resolveFilePath`, `enrich`)
- Modify: `internal/enricher_test.go` (add cache and write tests)

- [ ] **Step 1: Write failing tests**

Add to `internal/enricher_test.go`:

```go
func TestIsCached_Miss(t *testing.T) {
	e := &Enricher{store: &fakeStore{}}
	if e.isCached(context.Background(), "memex", "pkg::Foo", "abc123") {
		t.Fatal("expected cache miss on empty store")
	}
}

func TestIsCached_Hit(t *testing.T) {
	fs := &fakeStore{}
	fs.SaveMemory(context.Background(), SaveMemoryRequest{
		Text:       "summary",
		Project:    "memex",
		Topic:      "pkg::Foo",
		MemoryType: "code_node",
		Source:     "ast",
		Tags:       []string{"commit:abc123"},
	})
	e := &Enricher{store: fs}
	if !e.isCached(context.Background(), "memex", "pkg::Foo", "abc123") {
		t.Fatal("expected cache hit")
	}
}

func TestIsCached_WrongCommit(t *testing.T) {
	fs := &fakeStore{}
	fs.SaveMemory(context.Background(), SaveMemoryRequest{
		Text:       "summary",
		Project:    "memex",
		Topic:      "pkg::Foo",
		MemoryType: "code_node",
		Source:     "ast",
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
	_, err = kg.RecordFactScoped(RecordFactRequest{
		Subject:    "internal/hook.go",
		Predicate:  PredicateContainsFunction,
		Object:     "github.com/shivamvarshney/memex/internal::hookSessionStart",
		FilePath:   "internal/hook.go",
		CommitHash: "abc123",
		Source:     "ast",
		Confidence: 1,
	})
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
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal -run 'TestIsCached|TestResolveFilePath' -v
```

Expected: `FAIL — undefined: isCached, resolveFilePath`

- [ ] **Step 3: Implement `isCached` and `resolveFilePath` in enricher.go**

Add to `internal/enricher.go`:

```go
// isCached returns true if a code_node memory exists for this function at the given commit.
func (e *Enricher) isCached(ctx context.Context, project, functionID, commitHash string) bool {
	results, err := e.store.ListMemories(ctx, "", "code_node", functionID, nil, 1)
	if err != nil || len(results) == 0 {
		return false
	}
	commitTag := "commit:" + commitHash
	for _, t := range results[0].Tags {
		if t == commitTag {
			return true
		}
	}
	return false
}

// resolveFilePath finds the source file that contains this function_id via the KG.
func (e *Enricher) resolveFilePath(functionID string) (string, error) {
	facts, err := e.kg.QueryEntity(functionID, "")
	if err != nil {
		return "", fmt.Errorf("KG query for %s: %w", functionID, err)
	}
	for _, f := range facts {
		if f.Predicate == PredicateContainsFunction && f.Object == functionID {
			return f.Subject, nil // subject is the file path
		}
	}
	return "", fmt.Errorf("no contains fact found for function %s", functionID)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal -run 'TestIsCached|TestResolveFilePath' -v
```

Expected: all five tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/enricher.go internal/enricher_test.go
git commit -m "feat(enricher): add cache check and KG file-path resolver"
```

---

### Task 5: Enricher orchestrator — `enrich` and `EnrichAsync`

**Files:**
- Modify: `internal/enricher.go` (add `enrich`, `EnrichAsync`, `CurrentCommitHash`)
- Modify: `internal/enricher_test.go` (integration test with mock Ollama)

- [ ] **Step 1: Write failing test**

Add to `internal/enricher_test.go`:

```go
func TestEnrichAsync_StoresCodeNode(t *testing.T) {
	// Mock Ollama that returns a valid enrichment response.
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": `{"summary": "starts a session by loading pinned and semantic memories", "patterns": ["caching", "fallback"]}`,
		})
	}))
	defer ollamaSrv.Close()

	kg, _ := NewKnowledgeGraph(":memory:")
	kg.Init()
	// Seed a contains fact so resolveFilePath works.
	kg.RecordFactScoped(RecordFactRequest{
		Subject:    "internal/hook.go",
		Predicate:  PredicateContainsFunction,
		Object:     "github.com/shivamvarshney/memex/internal::hookSessionStart",
		FilePath:   "internal/hook.go",
		CommitHash: "abc123",
		Source:     "ast",
		Confidence: 1,
	})

	fs := &fakeStore{}
	e := NewEnricher(fs, kg, ollamaSrv.URL, "llama3.2", ".")

	done := make(chan struct{})
	// Patch: replace inflight signal with a direct call to enrich for test determinism.
	err := e.enrich(context.Background(),
		"github.com/shivamvarshney/memex/internal::hookSessionStart",
		"memex",
		"abc123",
	)
	close(done)

	if err != nil {
		t.Fatalf("enrich: %v", err)
	}

	results, _ := fs.ListMemories(context.Background(), "", "code_node",
		"github.com/shivamvarshney/memex/internal::hookSessionStart", nil, 1)
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
		json.NewEncoder(w).Encode(map[string]any{
			"response": `{"summary": "test", "patterns": []}`,
		})
	}))
	defer ollamaSrv.Close()

	kg, _ := NewKnowledgeGraph(":memory:")
	kg.Init()
	kg.RecordFactScoped(RecordFactRequest{
		Subject:    "internal/hook.go",
		Predicate:  PredicateContainsFunction,
		Object:     "pkg::Fn",
		FilePath:   "internal/hook.go",
		CommitHash: "abc",
		Source:     "ast",
		Confidence: 1,
	})

	fs := &fakeStore{}
	e := NewEnricher(fs, kg, ollamaSrv.URL, "llama3.2", ".")

	// Mark as in-flight manually — second call should be a no-op.
	e.inflight.Store("pkg::Fn", struct{}{})
	e.EnrichAsync(context.Background(), "pkg::Fn", "memex", "abc")

	// Give goroutine time to not-run.
	time.Sleep(50 * time.Millisecond)
	if calls != 0 {
		t.Errorf("expected 0 LLM calls when inflight, got %d", calls)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal -run 'TestEnrichAsync' -v
```

Expected: `FAIL — undefined: enrich, EnrichAsync`

- [ ] **Step 3: Implement `enrich`, `EnrichAsync`, and `CurrentCommitHash` in enricher.go**

Add to `internal/enricher.go`:

```go
// EnrichAsync fires a background goroutine to enrich the given function_id.
// Calls for the same function_id that are already in-flight are silently dropped.
func (e *Enricher) EnrichAsync(ctx context.Context, functionID, project, commitHash string) {
	if _, loaded := e.inflight.LoadOrStore(functionID, struct{}{}); loaded {
		return
	}
	go func() {
		defer e.inflight.Delete(functionID)
		if err := e.enrich(context.Background(), functionID, project, commitHash); err != nil {
			log.Printf("enricher: %s: %v", functionID, err)
		}
	}()
}

// enrich performs the full enrichment pipeline for a single function node.
func (e *Enricher) enrich(ctx context.Context, functionID, project, commitHash string) error {
	if e.isCached(ctx, project, functionID, commitHash) {
		return nil
	}

	filePath, err := e.resolveFilePath(functionID)
	if err != nil {
		return fmt.Errorf("resolve file: %w", err)
	}

	source, err := extractFunctionSource(e.repoRoot, filePath, functionID)
	if err != nil {
		return fmt.Errorf("extract source: %w", err)
	}

	prompt := fmt.Sprintf(enrichmentPromptTemplate, source)
	tctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	summary, patterns, err := callOllamaGenerate(tctx, e.ollamaURL, e.model, prompt)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	if summary == "" {
		return fmt.Errorf("llm returned empty summary for %s", functionID)
	}

	tags := make([]string, 0, len(patterns)+1)
	tags = append(tags, patterns...)
	tags = append(tags, "commit:"+commitHash)

	_, err = e.store.SaveMemory(ctx, SaveMemoryRequest{
		Text:       summary,
		Project:    project,
		Topic:      functionID,
		MemoryType: "code_node",
		Source:     "ast",
		Importance: 0.7,
		Tags:       tags,
	})
	return err
}

// CurrentCommitHash returns the most recent commit hash seen in active KG facts.
// Returns "working-tree" if no facts are indexed.
func (e *Enricher) CurrentCommitHash() string {
	facts, err := e.kg.QueryEntity("", "")
	// QueryEntity with empty entity returns nothing useful; use a direct query instead.
	_ = facts
	_ = err
	// Direct SQLite access not exposed — caller should pass commit hash from index context.
	// This is a stub; see Task 6 for how commitHash is resolved in the handler.
	return "working-tree"
}
```

> **Note:** `log.Printf` requires adding `"log"` to the import block.

- [ ] **Step 4: Run tests**

```bash
go test ./internal -run 'TestEnrichAsync' -v
```

Expected: both tests PASS.

- [ ] **Step 5: Full suite**

```bash
go test ./...
```

Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add internal/enricher.go internal/enricher_test.go
git commit -m "feat(enricher): add EnrichAsync orchestrator with inflight dedup"
```

---

### Task 6: Wire Enricher into ExpandSearch and server

**Files:**
- Modify: `internal/handlers.go:210-280` (add enricher field, trigger from ExpandSearch)
- Modify: `internal/server.go` (instantiate Enricher)
- Modify: `internal/handlers_test.go` (update NewHandlers calls)

The commit hash for enrichment is resolved at request time from the KG: query the most recently active fact for its `commit_hash`. Add a helper to `KnowledgeGraph`:

- [ ] **Step 1: Add `LatestCommitHash` to kg.go**

Add to `internal/kg.go`:

```go
// LatestCommitHash returns the commit_hash most recently inserted into the active facts.
// Returns "working-tree" if no AST facts are present.
func (kg *KnowledgeGraph) LatestCommitHash() string {
	row := kg.db.QueryRow(
		`SELECT commit_hash FROM facts
		 WHERE commit_hash IS NOT NULL AND commit_hash != '' AND valid_until IS NULL
		 ORDER BY created_at DESC LIMIT 1`,
	)
	var h string
	if err := row.Scan(&h); err != nil || h == "" {
		return "working-tree"
	}
	return h
}
```

Add test to `internal/kg_test.go`:

```go
func TestKG_LatestCommitHash_Empty(t *testing.T) {
	kg, _ := NewKnowledgeGraph(":memory:")
	kg.Init()
	if got := kg.LatestCommitHash(); got != "working-tree" {
		t.Errorf("expected working-tree, got %s", got)
	}
}

func TestKG_LatestCommitHash_WithFact(t *testing.T) {
	kg, _ := NewKnowledgeGraph(":memory:")
	kg.Init()
	kg.RecordFactScoped(RecordFactRequest{
		Subject: "a", Predicate: "p", Object: "b",
		CommitHash: "deadbeef", FilePath: "x.go", Source: "ast", Confidence: 1,
	})
	if got := kg.LatestCommitHash(); got != "deadbeef" {
		t.Errorf("expected deadbeef, got %s", got)
	}
}
```

- [ ] **Step 2: Run KG tests**

```bash
go test ./internal -run 'TestKG_LatestCommitHash' -v
```

Expected: both PASS.

- [ ] **Step 3: Add `enricher` to Handlers and update NewHandlers**

In `internal/handlers.go`, update the struct and constructor:

```go
type Handlers struct {
	store    Store
	kg       *KnowledgeGraph
	enricher *Enricher // nil = enrichment disabled
}

func NewHandlers(store Store, kg *KnowledgeGraph, enricher *Enricher) *Handlers {
	return &Handlers{store: store, kg: kg, enricher: enricher}
}
```

- [ ] **Step 4: Fix all NewHandlers call sites in handlers_test.go**

Every `NewHandlers(&mockStore{}, nil)` call becomes `NewHandlers(&mockStore{}, nil, nil)`.

Run to find them all:

```bash
grep -n "NewHandlers" internal/handlers_test.go
```

Then update each one. There are ~14 call sites — all follow the same pattern:

```go
// Before:
h := NewHandlers(&mockStore{}, nil)
// After:
h := NewHandlers(&mockStore{}, nil, nil)
```

- [ ] **Step 5: Trigger enrichment from ExpandSearch**

In `internal/handlers.go`, at the end of `ExpandSearch` — after `neighbors` is built and before the Qdrant call, add the async trigger:

```go
// Trigger lazy LLM enrichment for function nodes surfaced by KG traversal.
if h.enricher != nil {
    commitHash := h.kg.LatestCommitHash()
    for _, n := range neighbors {
        if strings.Contains(n, "::") { // function_id, not a file path
            h.enricher.EnrichAsync(r.Context(), n, project, commitHash)
        }
    }
}
```

Place this block immediately after the `neighbors := []string{}` / `expandNeighbors` block and before the pinned memories fetch.

- [ ] **Step 6: Instantiate Enricher in server.go**

In `internal/server.go`, update `RunServe`:

```go
func RunServe() {
	cfg := LoadConfig()
	store := NewQdrantStore(cfg.QdrantURL, cfg.OllamaURL)
	traceStore := NewTraceStore(cfg.QdrantURL)

	kg, err := NewKnowledgeGraph(cfg.KGPath)
	if err != nil {
		log.Fatalf("init knowledge graph: %v", err)
	}

	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		log.Fatalf("init memory store: %v", err)
	}
	if err := traceStore.Init(ctx); err != nil {
		log.Fatalf("init trace store: %v", err)
	}
	if err := kg.Init(); err != nil {
		log.Fatalf("init knowledge graph schema: %v", err)
	}

	enricher := NewEnricher(store, kg, cfg.OllamaURL, cfg.OllamaModel, cfg.RepoRoot)
	h := NewHandlers(store, kg, enricher)
	// ... rest unchanged
```

- [ ] **Step 7: Build and test**

```bash
go build ./...
go test ./...
```

Expected: clean build, all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/handlers.go internal/handlers_test.go internal/kg.go internal/kg_test.go internal/server.go
git commit -m "feat(enricher): wire Enricher into ExpandSearch and server; add LatestCommitHash"
```

---

### Task 7: Eval verification

**Goal:** Confirm that after calling ExpandSearch, Qdrant contains `code_node` entries for surfaced functions, and that a natural language query now finds them.

- [ ] **Step 1: Start services with REPO_ROOT and OLLAMA_MODEL set**

```bash
docker compose down && REPO_ROOT=/repo OLLAMA_MODEL=llama3.2 docker compose up -d
```

If running locally (not Docker):
```bash
REPO_ROOT=/Users/shivamvarshney/Documents/projects/memex OLLAMA_MODEL=llama3.2 go run ./cmd/memex serve
```

- [ ] **Step 2: Trigger enrichment via ExpandSearch**

```bash
curl -s "http://localhost:8765/memories/expand?entity=github.com/shivamvarshney/memex/internal::hookSessionStart&project=memex&depth=2" | jq '.neighbors | length'
```

Expected: >0 neighbors (enrichment fires asynchronously in background).

- [ ] **Step 3: Wait 10s for Ollama enrichment to complete, then check Qdrant**

```bash
sleep 10
curl -s "http://localhost:8765/memories?context=session+start+memory+injection&project=memex&memory_type=code_node&limit=5" | jq '[.[] | {text: .text, topic: .topic, tags: .tags}]'
```

Expected: at least one result with `memory_type: "code_node"` and tags including `"commit:<hash>"`.

- [ ] **Step 4: Run full eval set queries as natural language**

```bash
# Q1
curl -s "http://localhost:8765/memories?context=session+start+memory+injection&project=memex&limit=5" | jq '[.[].topic]'

# Q2
curl -s "http://localhost:8765/memories?context=KG+fact+storage&project=memex&limit=5" | jq '[.[].topic]'

# Q5
curl -s "http://localhost:8765/memories?context=code+facts+indexed&project=memex&limit=5" | jq '[.[].topic]'
```

Expected: topic fields contain function IDs from `hook.go`, `kg.go`, `code_indexer.go` files respectively. If missing, check `docker logs memex-memex-1` for enricher errors.

- [ ] **Step 5: Commit eval notes to plan doc**

Update `docs/superpowers/test-case/2026-04-18-phase3-eval-set.md` with Phase 2 results. No code change needed.

```bash
git add docs/superpowers/test-case/2026-04-18-phase3-eval-set.md
git commit -m "docs: record Phase 2 eval results"
```

---

## Known Limitations (for post-Phase 2 cleanup)

1. **`REPO_ROOT` in Docker** — the container needs the repo mounted or `REPO_ROOT` set. Add to `docker-compose.yml` once the path is stable.
2. **Commit hash from KG** — `LatestCommitHash()` queries the last-inserted fact. If multiple repos are indexed (future any-repo work), this will need to be project-scoped.
3. **Q2 `kg_handlers.go` miss** — still blocked on `calls_unresolved` gap. Fixed by `go/packages` upgrade before any-repo generalization.
4. **No delete path for stale code_nodes** — when a function is removed and facts are expired, the corresponding Qdrant `code_node` memory is not deleted. Acceptable for Phase 2; addressed with a periodic cleanup job later.
