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
