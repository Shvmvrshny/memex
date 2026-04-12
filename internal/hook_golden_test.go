package memex

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update-golden", false, "overwrite golden files with current output")

func readGolden(t *testing.T, name string) string {
	t.Helper()
	path := "testdata/golden/" + name
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file %q missing — run with -update-golden to create it: %v", path, err)
	}
	return string(data)
}

func writeGolden(t *testing.T, name, content string) {
	t.Helper()
	os.MkdirAll("testdata/golden", 0755)
	if err := os.WriteFile("testdata/golden/"+name, []byte(content), 0644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
}

func TestBuildMemoryContext_Golden_Full(t *testing.T) {
	identity := "I am Shivam. I build developer tools."
	pinned := []Memory{
		{Text: "prefer table-driven tests", MemoryType: "preference"},
		{Text: "use Qdrant for storage", MemoryType: "decision"},
	}
	semantic := []Memory{
		{Text: "Ollama must run on host.docker.internal", MemoryType: "discovery"},
	}

	got := buildMemoryContext(identity, pinned, semantic)

	if *updateGolden {
		writeGolden(t, "session_start_full.txt", got)
		t.Log("golden updated")
		return
	}

	want := strings.TrimRight(readGolden(t, "session_start_full.txt"), "\n")
	got = strings.TrimRight(got, "\n")
	if got != want {
		t.Errorf("session_start_full golden mismatch:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestBuildMemoryContext_Golden_IdentityOnly(t *testing.T) {
	got := buildMemoryContext("I am Shivam. I build developer tools.", nil, nil)

	if *updateGolden {
		writeGolden(t, "session_start_identity_only.txt", got)
		t.Log("golden updated")
		return
	}

	want := strings.TrimRight(readGolden(t, "session_start_identity_only.txt"), "\n")
	got = strings.TrimRight(got, "\n")
	if got != want {
		t.Errorf("session_start_identity_only golden mismatch:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestBuildMemoryContext_Golden_NoIdentity(t *testing.T) {
	pinned := []Memory{{Text: "critical preference", MemoryType: "preference"}}
	semantic := []Memory{{Text: "use Qdrant for storage", MemoryType: "decision"}}

	got := buildMemoryContext("", pinned, semantic)

	if *updateGolden {
		writeGolden(t, "session_start_no_identity.txt", got)
		t.Log("golden updated")
		return
	}

	want := strings.TrimRight(readGolden(t, "session_start_no_identity.txt"), "\n")
	got = strings.TrimRight(got, "\n")
	if got != want {
		t.Errorf("session_start_no_identity golden mismatch:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestBuildMemoryContext_SectionOrder_IdentityBeforePinnedBeforeContext(t *testing.T) {
	identity := "I am Shivam."
	pinned := []Memory{{Text: "critical pref", MemoryType: "preference"}}
	semantic := []Memory{{Text: "discovered fact", MemoryType: "discovery"}}

	got := buildMemoryContext(identity, pinned, semantic)

	idxIdentity := strings.Index(got, "[identity]")
	idxPinned := strings.Index(got, "[pinned]")
	idxContext := strings.Index(got, "[context]")

	if !(idxIdentity < idxPinned && idxPinned < idxContext) {
		t.Errorf("wrong section order: [identity]=%d [pinned]=%d [context]=%d\noutput:\n%s",
			idxIdentity, idxPinned, idxContext, got)
	}
}

func TestBuildMemoryContext_TokenBudget(t *testing.T) {
	pinned := make([]Memory, 10)
	for i := range pinned {
		pinned[i] = Memory{Text: "some pinned preference fact that is moderately long for testing budget purposes", MemoryType: "preference"}
	}
	semantic := make([]Memory, 5)
	for i := range semantic {
		semantic[i] = Memory{Text: "some semantic context memory that is also moderately long", MemoryType: "decision"}
	}

	got := buildMemoryContext("I am Shivam.", pinned, semantic)

	const maxChars = 8000
	if len(got) > maxChars {
		t.Errorf("session-start context is %d chars, exceeds budget of %d", len(got), maxChars)
	}
}
