package memex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

func RunHook(event string) {
	switch event {
	case "session-start":
		hookSessionStart()
	case "session-stop":
		// v1: no-op — summarization is manual via MCP tool
		outputEmpty()
	default:
		fmt.Fprintf(os.Stderr, "unknown hook event: %s\n", event)
		os.Exit(1)
	}
}

func hookSessionStart() {
	project := getProjectName()
	context := project

	// Silent fail if service is offline
	resp, err := http.Get(defaultMemexURL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		outputOfflineWarning()
		return
	}
	resp.Body.Close()

	// Fetch relevant memories
	apiURL := fmt.Sprintf("%s/memories?context=%s&limit=5",
		defaultMemexURL, url.QueryEscape(context))
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

func outputContext(additionalContext string) {
	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "SessionStart",
			"additionalContext": additionalContext,
		},
	}
	json.NewEncoder(os.Stdout).Encode(output)
}

func outputOfflineWarning() {
	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "SessionStart",
			"additionalContext": "<memex> memory service offline — starting without memory context",
		},
	}
	json.NewEncoder(os.Stdout).Encode(output)
}

func outputEmpty() {
	os.Stdout.Write([]byte("{}\n"))
}
