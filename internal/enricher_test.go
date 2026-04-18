package memex

import (
	"testing"
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
