package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type hookInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	ToolName       string          `json:"tool_name"`
	ToolUseID      string          `json:"tool_use_id"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
}

func readHookInput() hookInput {
	var input hookInput
	data, _ := io.ReadAll(os.Stdin)
	json.Unmarshal(data, &input)
	return input
}

func tracerHealthy() bool {
	resp, err := http.Get(getMemexURL() + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	resp.Body.Close()
	return true
}

func RunHook(event string) {
	switch event {
	case "session-start":
		hookSessionStart()
	case "session-stop":
		hookSessionStop()
	case "pre-tool-use":
		hookPreToolUse()
	case "post-tool-use":
		hookPostToolUse()
	default:
		fmt.Fprintf(os.Stderr, "unknown hook event: %s\n", event)
		os.Exit(1)
	}
}

func hookPreToolUse() {
	input := readHookInput()
	if input.SessionID == "" || input.ToolUseID == "" {
		outputEmpty()
		return
	}
	startFile := fmt.Sprintf("/tmp/memex-start-%s-%s", input.SessionID, input.ToolUseID)
	nowMs := time.Now().UnixMilli()
	os.WriteFile(startFile, fmt.Appendf(nil, "%d", nowMs), 0600)
	outputEmpty()
}

func hookPostToolUse() {
	input := readHookInput()
	if input.SessionID == "" || input.ToolName == "" {
		outputEmpty()
		return
	}
	if !tracerHealthy() {
		outputEmpty()
		return
	}

	// Turn index — increment per tool call within the session
	counterFile := fmt.Sprintf("/tmp/memex-turn-%s", input.SessionID)
	turnIndex := 0
	if data, err := os.ReadFile(counterFile); err == nil {
		fmt.Sscanf(string(data), "%d", &turnIndex)
	}
	os.WriteFile(counterFile, fmt.Appendf(nil, "%d", turnIndex+1), 0600)

	// Duration — paired with pre-tool-use start time
	var durationMs int64
	if input.ToolUseID != "" {
		startFile := fmt.Sprintf("/tmp/memex-start-%s-%s", input.SessionID, input.ToolUseID)
		if data, err := os.ReadFile(startFile); err == nil {
			var startMs int64
			fmt.Sscanf(string(data), "%d", &startMs)
			durationMs = time.Now().UnixMilli() - startMs
			os.Remove(startFile)
		}
	}

	// Project from cwd
	project := filepath.Base(input.Cwd)

	toolInput := "{}"
	if len(input.ToolInput) > 0 {
		toolInput = string(input.ToolInput)
	}
	toolOutput := "{}"
	if len(input.ToolResponse) > 0 {
		toolOutput = string(input.ToolResponse)
	}

	reqBody, _ := json.Marshal(TraceEventRequest{
		SessionID:  input.SessionID,
		Project:    project,
		TurnIndex:  turnIndex,
		Tool:       input.ToolName,
		Input:      toolInput,
		Output:     toolOutput,
		DurationMs: durationMs,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	})
	http.Post(getMemexURL()+"/trace/event", "application/json", bytes.NewReader(reqBody))
	outputEmpty()
}

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

	// Transcript mining — bounded 2s timeout, silent on failure
	if input.TranscriptPath != "" {
		project := getProjectName()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		body, _ := json.Marshal(MineRequest{
			Path:    input.TranscriptPath,
			Project: project,
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			getMemexURL()+"/mine/transcript", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			http.DefaultClient.Do(req)
		}
	}

	outputEmpty()
}

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

func getProjectName() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		wd, _ := os.Getwd()
		parts := strings.Split(strings.TrimRight(wd, "/"), "/")
		return parts[len(parts)-1]
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "/")
	return parts[len(parts)-1]
}

func isCursor() bool {
	return os.Getenv("CURSOR_PLUGIN_ROOT") != ""
}

func outputContext(additionalContext string) {
	var output map[string]any
	if isCursor() {
		output = map[string]any{
			"additional_context": additionalContext,
		}
	} else {
		output = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "SessionStart",
				"additionalContext": additionalContext,
			},
		}
	}
	json.NewEncoder(os.Stdout).Encode(output)
}

func outputOfflineWarning() {
	outputContext("<memex> memory service offline — starting without memory context")
}

func outputEmpty() {
	os.Stdout.Write([]byte("{}\n"))
}

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
	result := make([]Memory, 0, len(memories))
	var high, low []Memory
	for _, m := range memories {
		if m.MemoryType == "preference" || m.MemoryType == "decision" {
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
