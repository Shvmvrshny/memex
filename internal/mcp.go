package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func RunMCP() {
	s := server.NewMCPServer("memex", "1.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(
		mcp.NewTool("save_memory",
			mcp.WithDescription("Save something important to long-term memory. Use this when the user states a preference, makes a decision, or shares context that should persist across sessions."),
			mcp.WithString("text", mcp.Required(), mcp.Description("The memory to save, written as a clear statement e.g. 'user prefers table-driven tests in Go'")),
			mcp.WithString("project", mcp.Description("Project name to associate this memory with (optional)")),
			mcp.WithNumber("importance", mcp.Description("Importance score 0.0-1.0, default 0.5. Use 0.9+ for critical preferences.")),
		),
		handleSaveMemory,
	)

	s.AddTool(
		mcp.NewTool("search_memory",
			mcp.WithDescription("Search long-term memory for relevant context about the user or project."),
			mcp.WithString("context", mcp.Required(), mcp.Description("What you want to remember — e.g. 'user language preferences'")),
			mcp.WithString("project", mcp.Description("Filter by project name (optional)")),
		),
		handleSearchMemory,
	)

	s.AddTool(
		mcp.NewTool("list_memories",
			mcp.WithDescription("List all stored memories, optionally filtered by project."),
			mcp.WithString("project", mcp.Description("Filter by project name (optional)")),
		),
		handleListMemories,
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}

func handleSaveMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, _ := req.Params.Arguments["text"].(string)
	project, _ := req.Params.Arguments["project"].(string)
	importance, _ := req.Params.Arguments["importance"].(float64)
	if importance == 0 {
		importance = 0.5
	}

	body := SaveMemoryRequest{
		Text:       text,
		Project:    project,
		Source:     "claude-code",
		Importance: float32(importance),
	}
	data, _ := json.Marshal(body)

	resp, err := http.Post(defaultMemexURL+"/memories", "application/json", bytes.NewReader(data))
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable — is Docker running?"), nil
	}
	defer resp.Body.Close()

	var mem Memory
	json.NewDecoder(resp.Body).Decode(&mem)
	return mcp.NewToolResultText(fmt.Sprintf("memory saved (id: %s)", mem.ID)), nil
}

func handleSearchMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, _ := req.Params.Arguments["context"].(string)
	project, _ := req.Params.Arguments["project"].(string)

	apiURL := fmt.Sprintf("%s/memories?context=%s&project=%s&limit=5",
		defaultMemexURL, url.QueryEscape(query), url.QueryEscape(project))
	resp, err := http.Get(apiURL)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable — is Docker running?"), nil
	}
	defer resp.Body.Close()

	var result SearchResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Memories) == 0 {
		return mcp.NewToolResultText("no memories found"), nil
	}

	data, _ := json.MarshalIndent(result.Memories, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func handleListMemories(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, _ := req.Params.Arguments["project"].(string)

	apiURL := fmt.Sprintf("%s/memories?project=%s&limit=100",
		defaultMemexURL, url.QueryEscape(project))
	resp, err := http.Get(apiURL)
	if err != nil {
		return mcp.NewToolResultError("memex service unavailable — is Docker running?"), nil
	}
	defer resp.Body.Close()

	var result SearchResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Memories) == 0 {
		return mcp.NewToolResultText("no memories stored yet"), nil
	}

	data, _ := json.MarshalIndent(result.Memories, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
