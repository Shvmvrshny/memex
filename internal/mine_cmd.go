package memex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// RunMine is the CLI handler for `memex mine <path>`.
// It POSTs to the running memex server's /mine/transcript endpoint.
func RunMine(path string) {
	if path == "" {
		fmt.Fprintln(os.Stderr, "Usage: memex mine <transcript-path>")
		os.Exit(1)
	}

	project := getProjectName()
	body, _ := json.Marshal(MineRequest{Path: path, Project: project})

	resp, err := http.Post(getMemexURL()+"/mine/transcript", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "mine: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result MineResponse
	json.NewDecoder(resp.Body).Decode(&result)
	fmt.Printf("memex mine: %s (path: %s)\n", result.Status, result.Path)
}
