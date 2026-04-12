package memex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// transcriptMessage is one line of a Claude Code session JSONL file.
type transcriptMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// contentBlock is one element of an assistant's content array.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
}

// ParseTranscript reads a Claude Code session JSONL file and returns a slice
// of reasoning strings indexed by turn order (0-based). Each entry is the
// text block immediately preceding a tool_use block in an assistant message.
// Returns an empty slice (not an error) if no tool calls are found.
func ParseTranscript(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var reasoning []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg transcriptMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Role != "assistant" {
			continue
		}

		var blocks []contentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}

		var lastText string
		for _, block := range blocks {
			switch block.Type {
			case "text":
				lastText = block.Text
			case "tool_use":
				reasoning = append(reasoning, lastText)
				lastText = ""
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript: %w", err)
	}
	return reasoning, nil
}

// ParseConversation reads a Claude Code session JSONL and returns all user and
// assistant turns as ConversationTurn values. Tool-result turns are skipped.
// Assistant content arrays are joined into a single text string.
func ParseConversation(path string) ([]ConversationTurn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var turns []ConversationTurn
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg transcriptMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		switch msg.Role {
		case "user":
			var text string
			if err := json.Unmarshal(msg.Content, &text); err == nil {
				turns = append(turns, ConversationTurn{Role: "user", Text: text})
			} else {
				var blocks []contentBlock
				if err := json.Unmarshal(msg.Content, &blocks); err == nil {
					var sb strings.Builder
					for _, b := range blocks {
						if b.Type == "text" {
							sb.WriteString(b.Text)
						}
					}
					if s := sb.String(); s != "" {
						turns = append(turns, ConversationTurn{Role: "user", Text: s})
					}
				}
			}

		case "assistant":
			var blocks []contentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err != nil {
				continue
			}
			var sb strings.Builder
			for _, b := range blocks {
				if b.Type == "text" {
					sb.WriteString(b.Text)
				}
			}
			if s := sb.String(); s != "" {
				turns = append(turns, ConversationTurn{Role: "assistant", Text: s})
			}

		// "tool" role is skipped
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript: %w", err)
	}
	return turns, nil
}
