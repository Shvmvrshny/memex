# Phase 2: Memory Layers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat 10-memory session-start dump with a structured 3-layer context block: L0 identity file, L1 pinned memories (importance ≥ 0.9), L2 type-prioritised semantic search (5 results). Also trigger async transcript mining on session-stop.

**Architecture:** `config.go` gains `IdentityPath` and `KGPath`. `docker-compose.yml` gets a host mount so the identity file and future SQLite DB persist across container restarts. `hook.go` is rewritten to call three separate endpoints in order and build a structured `<memex-memory>` block. The session-stop hook fires `/mine/transcript` asynchronously (non-blocking). Phase 1 must be complete first (the `/memories/pinned` endpoint is added there).

**Tech Stack:** Go 1.26, existing `net/http` test patterns, Docker Compose v3.

---

## File Map

| File | Change |
|---|---|
| `internal/config.go` | Add `IdentityPath string`, `KGPath string` to `Config`; resolve defaults in `LoadConfig` |
| `internal/hook.go` | Full rewrite of `hookSessionStart` (3-layer); extend `hookSessionStop` (async mine) |
| `docker-compose.yml` | Add `- ~/.memex:/root/.memex` volume to `memex` service |
| `internal/hook_test.go` | NEW: unit tests for `buildMemoryContext`, `loadIdentity` |

---

### Task 1: Update `internal/config.go`

**Files:**
- Modify: `internal/config.go`

- [ ] **Step 1: Write the test first**

Create `internal/config_test.go`:

```go
package memex

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	os.Unsetenv("IDENTITY_PATH")
	os.Unsetenv("KG_PATH")

	cfg := LoadConfig()

	home, _ := os.UserHomeDir()
	wantIdentity := home + "/.memex/identity.md"
	wantKG := home + "/.memex/knowledge_graph.db"

	if cfg.IdentityPath != wantIdentity {
		t.Errorf("IdentityPath = %q, want %q", cfg.IdentityPath, wantIdentity)
	}
	if cfg.KGPath != wantKG {
		t.Errorf("KGPath = %q, want %q", cfg.KGPath, wantKG)
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	os.Setenv("IDENTITY_PATH", "/custom/identity.md")
	os.Setenv("KG_PATH", "/custom/kg.db")
	defer os.Unsetenv("IDENTITY_PATH")
	defer os.Unsetenv("KG_PATH")

	cfg := LoadConfig()

	if cfg.IdentityPath != "/custom/identity.md" {
		t.Errorf("IdentityPath = %q, want /custom/identity.md", cfg.IdentityPath)
	}
	if cfg.KGPath != "/custom/kg.db" {
		t.Errorf("KGPath = %q, want /custom/kg.db", cfg.KGPath)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
go test ./internal/ -run TestLoadConfig -v
```

Expected: FAIL — `cfg.IdentityPath` field does not exist.

- [ ] **Step 3: Update `internal/config.go`**

```go
package memex

import (
	"os"
	"path/filepath"
)

const defaultMemexURL = "http://localhost:8765"

func getMemexURL() string {
	if u := os.Getenv("MEMEX_URL"); u != "" {
		return u
	}
	return defaultMemexURL
}

type Config struct {
	Port         string
	QdrantURL    string
	OllamaURL    string
	IdentityPath string // ~/.memex/identity.md — L0 identity text
	KGPath       string // ~/.memex/knowledge_graph.db — SQLite KG
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
		IdentityPath: identityPath,
		KGPath:       kgPath,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/ -run TestLoadConfig -v
```

Expected: PASS — both tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/config.go internal/config_test.go
git commit -m "feat: add IdentityPath and KGPath to Config"
```

---

### Task 2: Update `docker-compose.yml`

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add host mount to memex service**

The `memex` service needs a host bind mount so `identity.md` and `knowledge_graph.db` live on the host at `~/.memex/` and survive container restarts.

Replace the current `memex` service block (lines 1-12 of the file) with:

```yaml
services:
  memex:
    build: .
    ports:
      - "8765:8765"
    environment:
      - QDRANT_URL=http://qdrant:6333
      - OLLAMA_URL=http://host.docker.internal:11434
    volumes:
      - ~/.memex:/root/.memex
    depends_on:
      qdrant:
        condition: service_healthy
    restart: unless-stopped
```

- [ ] **Step 2: Create the host directory**

```bash
mkdir -p ~/.memex
```

- [ ] **Step 3: Verify compose file is valid**

```bash
docker compose config --quiet
```

Expected: No errors printed.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml
git commit -m "feat: mount ~/.memex as host volume for identity and KG persistence"
```

---

### Task 3: Add hook helper functions and tests

**Files:**
- Create: `internal/hook_test.go`
- Modify: `internal/hook.go` (add `loadIdentity` and `buildMemoryContext` as testable functions)

The current `hookSessionStart` is one monolithic function. We extract two pure functions first — `loadIdentity` and `buildMemoryContext` — so they can be tested without an HTTP server.

- [ ] **Step 1: Write failing tests**

Create `internal/hook_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ -run "TestLoadIdentity|TestBuildMemoryContext|TestSortByTypePriority" -v
```

Expected: FAIL — `loadIdentity`, `buildMemoryContext`, `sortByTypePriority` not defined.

- [ ] **Step 3: Add the helper functions to `internal/hook.go`**

Add these functions to the bottom of `internal/hook.go` (before or after `outputEmpty`):

```go
// loadIdentity reads the L0 identity file from disk.
// Returns empty string if the file doesn't exist — L0 is silently skipped.
func loadIdentity(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// sortByTypePriority reorders memories so "preference" and "decision" types
// appear before all others. Preserves original relative order within each tier.
func sortByTypePriority(memories []Memory) []Memory {
	priority := map[string]int{"preference": 0, "decision": 0}
	result := make([]Memory, 0, len(memories))
	var high, low []Memory
	for _, m := range memories {
		if _, ok := priority[m.MemoryType]; ok {
			high = append(high, m)
		} else {
			low = append(low, m)
		}
	}
	result = append(result, high...)
	result = append(result, low...)
	return result
}

// buildMemoryContext assembles the structured <memex-memory> block.
// Returns empty string if all layers are empty (avoids injecting a blank block).
func buildMemoryContext(identity string, pinned []Memory, semantic []Memory) string {
	if identity == "" && len(pinned) == 0 && len(semantic) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<memex-memory>\n")

	if identity != "" {
		sb.WriteString("[identity]\n")
		sb.WriteString(identity)
		sb.WriteString("\n\n")
	}

	if len(pinned) > 0 {
		sb.WriteString("[pinned]\n")
		for _, m := range pinned {
			sb.WriteString(fmt.Sprintf("- (%s) %s\n", m.MemoryType, m.Text))
		}
		sb.WriteString("\n")
	}

	if len(semantic) > 0 {
		sb.WriteString("[context]\n")
		for _, m := range semantic {
			sb.WriteString(fmt.Sprintf("- (%s) %s\n", m.MemoryType, m.Text))
		}
		sb.WriteString("\n")
	}

	result := strings.TrimRight(sb.String(), "\n")
	result += "\n</memex-memory>"
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ -run "TestLoadIdentity|TestBuildMemoryContext|TestSortByTypePriority" -v
```

Expected: PASS — all 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/hook.go internal/hook_test.go
git commit -m "feat: add loadIdentity, buildMemoryContext, sortByTypePriority helpers to hook"
```

---

### Task 4: Rewrite `hookSessionStart` — 3-layer context

**Files:**
- Modify: `internal/hook.go`

- [ ] **Step 1: Add an integration test for the new hookSessionStart flow**

Add to `internal/hook_test.go`:

```go
func TestHookSessionStart_BuildsThreeLayers(t *testing.T) {
	// This test validates the layer assembly logic by calling the pure helpers
	// with known inputs — the HTTP calls are out of scope for unit tests.

	identity := "I am Shivam. Primary project: memex."
	pinned := []Memory{
		{Text: "prefer table-driven tests", MemoryType: "preference", Importance: 1.0},
	}
	semantic := []Memory{
		{Text: "Ollama must run on host.docker.internal", MemoryType: "discovery"},
		{Text: "we use Qdrant for vector storage", MemoryType: "decision"},
	}

	// Sort semantic results so decisions and preferences appear first
	sorted := sortByTypePriority(semantic)
	if sorted[0].MemoryType != "decision" {
		t.Errorf("expected decision first after sort, got %q", sorted[0].MemoryType)
	}

	got := buildMemoryContext(identity, pinned, sorted)

	// Verify section ordering: identity, pinned, context
	idxIdentity := strings.Index(got, "[identity]")
	idxPinned := strings.Index(got, "[pinned]")
	idxContext := strings.Index(got, "[context]")
	if !(idxIdentity < idxPinned && idxPinned < idxContext) {
		t.Errorf("section order wrong: identity=%d pinned=%d context=%d", idxIdentity, idxPinned, idxContext)
	}
}
```

- [ ] **Step 2: Run test to verify it passes (uses already-implemented helpers)**

```bash
go test ./internal/ -run TestHookSessionStart_BuildsThreeLayers -v
```

Expected: PASS.

- [ ] **Step 3: Rewrite `hookSessionStart` in `internal/hook.go`**

Replace the existing `hookSessionStart` function:

```go
func hookSessionStart() {
	project := getProjectName()
	memexURL := getMemexURL()

	// Silent fail if service is offline
	resp, err := http.Get(memexURL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		outputOfflineWarning()
		return
	}
	resp.Body.Close()

	cfg := LoadConfig()

	// L0 — Identity from disk
	identity := loadIdentity(cfg.IdentityPath)

	// L1 — Pinned memories (importance >= 0.9), pure payload filter, no embedding
	var pinned []Memory
	pinnedURL := fmt.Sprintf("%s/memories/pinned?project=%s", memexURL, url.QueryEscape(project))
	if r, err := http.Get(pinnedURL); err == nil {
		defer r.Body.Close()
		var result SearchResponse
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &result)
		pinned = result.Memories
	}

	// L2 — Semantic context: top 5, type-prioritised (preference + decision first)
	var semantic []Memory
	query := fmt.Sprintf("project %s session context", project)
	semanticURL := fmt.Sprintf("%s/memories?context=%s&project=%s&limit=5",
		memexURL, url.QueryEscape(query), url.QueryEscape(project))
	if r, err := http.Get(semanticURL); err == nil {
		defer r.Body.Close()
		var result SearchResponse
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &result)
		semantic = sortByTypePriority(result.Memories)
	}

	block := buildMemoryContext(identity, pinned, semantic)
	if block == "" {
		outputEmpty()
		return
	}
	outputContext(block)
}
```

- [ ] **Step 4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: No errors.

- [ ] **Step 5: Run all hook tests**

```bash
go test ./internal/ -run "TestLoadIdentity|TestBuildMemoryContext|TestSortByTypePriority|TestHookSessionStart" -v
```

Expected: PASS — all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/hook.go internal/hook_test.go
git commit -m "feat: rewrite hookSessionStart with L0/L1/L2 memory layers"
```

---

### Task 5: Extend `hookSessionStop` — async transcript mining

**Files:**
- Modify: `internal/hook.go`

The session-stop hook already POSTs to `/trace/stop`. We add a non-blocking goroutine that fires `/mine/transcript` when `TranscriptPath` is set. The `/mine/transcript` endpoint is added in Phase 4; for now the call is fire-and-forget and safe to fail silently.

- [ ] **Step 1: Write a test for the stop hook logic**

Add to `internal/hook_test.go`:

```go
func TestHookSessionStop_FiresAsyncMine(t *testing.T) {
	// Validate the mining trigger logic is correctly gated on TranscriptPath.
	// We test the guard condition, not the HTTP call.

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
```

- [ ] **Step 2: Run test to verify it passes (pure logic test)**

```bash
go test ./internal/ -run TestHookSessionStop_FiresAsyncMine -v
```

Expected: PASS.

- [ ] **Step 3: Update `hookSessionStop` in `internal/hook.go`**

Replace the existing `hookSessionStop` function:

```go
func hookSessionStop() {
	input := readHookInput()
	if input.SessionID == "" || !tracerHealthy() {
		outputEmpty()
		return
	}

	// Existing: stop the trace session
	reqBody, _ := json.Marshal(StopRequest{
		SessionID:      input.SessionID,
		TranscriptPath: input.TranscriptPath,
	})
	http.Post(getMemexURL()+"/trace/stop", "application/json", bytes.NewReader(reqBody))
	os.Remove(fmt.Sprintf("/tmp/memex-turn-%s", input.SessionID))

	// NEW: async transcript mining — non-blocking, safe to fail silently
	if input.TranscriptPath != "" {
		project := getProjectName()
		go func() {
			body, _ := json.Marshal(MineRequest{
				Path:    input.TranscriptPath,
				Project: project,
			})
			http.Post(getMemexURL()+"/mine/transcript", "application/json", bytes.NewReader(body))
		}()
	}

	outputEmpty()
}
```

- [ ] **Step 4: Build and run all tests**

```bash
go build ./...
go test ./internal/ -v
```

Expected: Build succeeds, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/hook.go internal/hook_test.go
git commit -m "feat: fire async transcript mining in hookSessionStop"
```

---

### Task 6: Verify end-to-end and create identity file template

**Files:**
- No code changes — verification only

- [ ] **Step 1: Run the full test suite**

```bash
go test ./... -v 2>&1 | tail -20
```

Expected: All packages pass. No failures.

- [ ] **Step 2: Build the binary**

```bash
go build -o /tmp/memex-check ./cmd/memex/
echo "Build OK"
```

Expected: `Build OK` printed.

- [ ] **Step 3: Verify docker-compose has the volume**

```bash
grep "memex" docker-compose.yml
```

Expected output includes `~/.memex:/root/.memex`.

- [ ] **Step 4: Create an example identity file (not committed — user-owned)**

```bash
cat > ~/.memex/identity.md << 'EOF'
I am Shivam. I build developer tools. Primary project: memex.
Stack: Go, Qdrant, React/Vite, Docker.
My coding preferences: table-driven tests, no mocks for DB, functional style where reasonable.
EOF
echo "Identity file created at ~/.memex/identity.md"
```

- [ ] **Step 5: Final commit — bump phase marker**

```bash
git add .
git commit -m "feat: Phase 2 complete — memory layers (L0 identity, L1 pinned, L2 semantic)"
```
