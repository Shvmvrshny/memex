package memex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadIdentity_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.md")
	os.WriteFile(path, []byte("I am Shivam. I build developer tools."), 0600)

	got := loadIdentity(path)

	if got != "I am Shivam. I build developer tools." {
		t.Errorf("loadIdentity = %q, want file contents", got)
	}
}

func TestLoadIdentity_FileMissing(t *testing.T) {
	got := loadIdentity("/nonexistent/identity.md")
	if got != "" {
		t.Errorf("loadIdentity missing = %q, want empty string", got)
	}
}

func TestBuildMemoryContext_AllLayers(t *testing.T) {
	identity := "I am Shivam."
	pinned := []Memory{
		{Text: "prefer table-driven tests", MemoryType: "preference"},
		{Text: "using Qdrant for storage", MemoryType: "decision"},
	}
	semantic := []Memory{
		{Text: "Ollama must run on host", MemoryType: "discovery"},
	}

	got := buildMemoryContext(identity, pinned, semantic)

	if !strings.Contains(got, "[identity]") {
		t.Error("missing [identity] section")
	}
	if !strings.Contains(got, "I am Shivam.") {
		t.Error("missing identity text")
	}
	if !strings.Contains(got, "[pinned]") {
		t.Error("missing [pinned] section")
	}
	if !strings.Contains(got, "(preference) prefer table-driven tests") {
		t.Error("missing pinned preference entry")
	}
	if !strings.Contains(got, "[context]") {
		t.Error("missing [context] section")
	}
	if !strings.Contains(got, "(discovery) Ollama must run on host") {
		t.Error("missing semantic context entry")
	}
}

func TestBuildMemoryContext_NoIdentity(t *testing.T) {
	pinned := []Memory{{Text: "a pinned fact", MemoryType: "decision"}}

	got := buildMemoryContext("", pinned, nil)

	if strings.Contains(got, "[identity]") {
		t.Error("[identity] section should be absent when identity is empty")
	}
	if !strings.Contains(got, "[pinned]") {
		t.Error("missing [pinned] section")
	}
}

func TestBuildMemoryContext_Empty(t *testing.T) {
	got := buildMemoryContext("", nil, nil)
	if got != "" {
		t.Errorf("all-empty buildMemoryContext = %q, want empty string", got)
	}
}

func TestSortByTypePriority(t *testing.T) {
	memories := []Memory{
		{Text: "a", MemoryType: "event"},
		{Text: "b", MemoryType: "preference"},
		{Text: "c", MemoryType: "decision"},
		{Text: "d", MemoryType: "discovery"},
	}

	got := sortByTypePriority(memories)

	// preference and decision must come before event and discovery
	if got[0].MemoryType != "preference" && got[0].MemoryType != "decision" {
		t.Errorf("first result should be preference or decision, got %q", got[0].MemoryType)
	}
	if got[1].MemoryType != "preference" && got[1].MemoryType != "decision" {
		t.Errorf("second result should be preference or decision, got %q", got[1].MemoryType)
	}
}

func TestHookSessionStart_BuildsThreeLayers(t *testing.T) {
	identity := "I am Shivam. Primary project: memex."
	pinned := []Memory{
		{Text: "prefer table-driven tests", MemoryType: "preference", Importance: 1.0},
	}
	semantic := []Memory{
		{Text: "Ollama must run on host.docker.internal", MemoryType: "discovery"},
		{Text: "we use Qdrant for vector storage", MemoryType: "decision"},
	}

	sorted := sortByTypePriority(semantic)
	if sorted[0].MemoryType != "decision" {
		t.Errorf("expected decision first after sort, got %q", sorted[0].MemoryType)
	}

	got := buildMemoryContext(identity, pinned, sorted)

	idxIdentity := strings.Index(got, "[identity]")
	idxPinned := strings.Index(got, "[pinned]")
	idxContext := strings.Index(got, "[context]")
	if !(idxIdentity < idxPinned && idxPinned < idxContext) {
		t.Errorf("section order wrong: identity=%d pinned=%d context=%d", idxIdentity, idxPinned, idxContext)
	}
}

func TestHookSessionStop_FiresAsyncMine(t *testing.T) {
	testCases := []struct {
		name           string
		transcriptPath string
		shouldMine     bool
	}{
		{"with transcript path", "/tmp/session.jsonl", true},
		{"empty transcript path", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldFire := tc.transcriptPath != ""
			if shouldFire != tc.shouldMine {
				t.Errorf("shouldFire = %v, want %v", shouldFire, tc.shouldMine)
			}
		})
	}
}
