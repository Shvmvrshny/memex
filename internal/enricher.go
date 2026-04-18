package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
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
		fnName = qname[dot+1:]     // "QueryEntity"
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

// callOllamaGenerate sends a generation request to Ollama and parses the summary + patterns.
// If the LLM response is not valid JSON, the raw text is used as the summary with no patterns.
func callOllamaGenerate(ctx context.Context, ollamaURL, model, prompt string) (summary string, patterns []string, err error) {
	body, _ := json.Marshal(map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(ollamaURL, "/")+"/api/generate", bytes.NewReader(body))
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

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", nil, fmt.Errorf("ollama status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return "", nil, fmt.Errorf("decode ollama response: %w", err)
	}

	var parsed struct {
		Summary  string   `json:"summary"`
		Patterns []string `json:"patterns"`
	}
	raw := strings.TrimSpace(result.Response)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || strings.TrimSpace(parsed.Summary) == "" {
		// Fallback: use raw text as summary, no structured patterns.
		return raw, nil, nil
	}
	return strings.TrimSpace(parsed.Summary), parsed.Patterns, nil
}

// isCached returns true if a code_node memory exists for this function at the given commit.
func (e *Enricher) isCached(ctx context.Context, project, functionID, commitHash string) bool {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(functionID) == "" || strings.TrimSpace(commitHash) == "" {
		return false
	}
	results, err := e.store.ListMemories(ctx, project, "code_node", functionID, nil, 50)
	if err != nil || len(results) == 0 {
		return false
	}
	commitTag := "commit:" + commitHash
	for _, m := range results {
		for _, t := range m.Tags {
			if t == commitTag {
				return true
			}
		}
	}
	return false
}

// resolveFilePath finds the source file that contains this function_id via the KG.
func (e *Enricher) resolveFilePath(functionID string) (string, error) {
	if e.kg == nil {
		return "", fmt.Errorf("knowledge graph unavailable")
	}
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

// EnrichAsync fires a background goroutine to enrich the given function_id.
// Calls for the same key that are already in-flight are silently dropped.
func (e *Enricher) EnrichAsync(ctx context.Context, functionID, project, commitHash string) {
	key := functionID + "|" + project + "|" + commitHash
	if _, loaded := e.inflight.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	go func() {
		defer e.inflight.Delete(key)
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
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("llm returned empty summary for %s", functionID)
	}

	tags := make([]string, 0, len(patterns)+2)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		tags = append(tags, "pattern:"+p)
	}
	tags = append(tags, "role:retrieval", "commit:"+commitHash)

	_, err = e.store.SaveMemory(ctx, SaveMemoryRequest{
		Text:       summary,
		Project:    project,
		Topic:      functionID,
		MemoryType: "code_node",
		Source:     "llm-enrichment",
		Importance: 0.85,
		Tags:       tags,
	})
	return err
}
