package memex

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Distill produces a caveman-format checkpoint summary from a list of trace events.
// Format keeps token count low: no articles, no filler words.
func Distill(project string, events []TraceEvent) string {
	toolCounts := map[string]int{}
	for _, e := range events {
		if e.Tool != "" {
			toolCounts[e.Tool]++
		}
	}

	toolNames := make([]string, 0, len(toolCounts))
	for name := range toolCounts {
		toolNames = append(toolNames, name)
	}
	sort.Strings(toolNames)

	toolParts := make([]string, 0, len(toolNames))
	for _, name := range toolNames {
		toolParts = append(toolParts, fmt.Sprintf("%s x%d", name, toolCounts[name]))
	}

	sessionDate := time.Now().UTC().Format("2006-01-02 15:04")
	if len(events) > 0 {
		sessionDate = events[0].Timestamp.UTC().Format("2006-01-02 15:04")
	}

	lines := []string{
		fmt.Sprintf("project: %s. session: %s.", project, sessionDate),
		"done: [fill in what was accomplished].",
		"decided: [fill in key decisions].",
		"next: [fill in what remains].",
		fmt.Sprintf("tools: %d calls. %s.", len(events), strings.Join(toolParts, ", ")),
	}
	return strings.Join(lines, "\n")
}
