package memex

import (
	"bytes"
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

	reqBody, _ := json.Marshal(StopRequest{
		SessionID:      input.SessionID,
		TranscriptPath: input.TranscriptPath,
	})
	http.Post(getMemexURL()+"/trace/stop", "application/json", bytes.NewReader(reqBody))
	os.Remove(fmt.Sprintf("/tmp/memex-turn-%s", input.SessionID))
	outputEmpty()
}

func hookSessionStart() {
	project := getProjectName()
	context := project

	// Silent fail if service is offline
	resp, err := http.Get(getMemexURL() + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		outputOfflineWarning()
		return
	}
	resp.Body.Close()

	// Fetch relevant memories
	apiURL := fmt.Sprintf("%s/memories?context=%s&limit=5",
		getMemexURL(), url.QueryEscape(context))
	resp2, err := http.Get(apiURL)
	if err != nil {
		outputEmpty()
		return
	}
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)
	var result SearchResponse
	if err := json.Unmarshal(body, &result); err != nil || len(result.Memories) == 0 {
		outputEmpty()
		return
	}

	var sb strings.Builder
	sb.WriteString("<memex-memory>\n")
	for _, m := range result.Memories {
		sb.WriteString(fmt.Sprintf("- %s\n", m.Text))
	}
	sb.WriteString("</memex-memory>")

	outputContext(sb.String())
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
