package memex

import (
	"context"
	"regexp"
	"strings"
)

// Miner reads a transcript file, classifies each conversation turn into typed
// memories, deduplicates against existing memories, and saves new ones.
type Miner struct {
	store      Store
	classifier *Classifier
}

// NewMiner creates a Miner backed by the given store.
func NewMiner(store Store) *Miner {
	return &Miner{
		store:      store,
		classifier: NewClassifier(),
	}
}

// MineTranscript parses the JSONL transcript at path, classifies each turn,
// deduplicates against existing memories for project, and saves typed memories.
// Returns the SaveMemoryRequests that were actually saved (not duplicates).
func (m *Miner) MineTranscript(path, project string) ([]SaveMemoryRequest, error) {
	turns, err := ParseConversation(path)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	var saved []SaveMemoryRequest

	for _, turn := range turns {
		if len(strings.TrimSpace(turn.Text)) < 20 {
			continue
		}

		memType, confidence := m.classifier.Classify(turn.Text)
		if confidence < 0.3 || memType == "" {
			continue
		}

		// Duplicate detection: skip if FindSimilar returns any results
		similar, err := m.store.FindSimilar(ctx, turn.Text, project, 1)
		if err == nil && len(similar) > 0 {
			continue
		}

		topic := m.inferTopic(turn.Text)
		req := SaveMemoryRequest{
			Text:       turn.Text,
			Project:    project,
			Topic:      topic,
			MemoryType: memType,
			Source:     "transcript-mine",
			Importance: float32(confidence),
		}

		if _, err := m.store.SaveMemory(ctx, req); err == nil {
			saved = append(saved, req)
		}
	}
	return saved, nil
}

// hyphenWordRe matches two or more hyphenated lowercase words (e.g. "auth-migration").
var hyphenWordRe = regexp.MustCompile(`\b[a-z]+-[a-z]+(?:-[a-z]+)*\b`)

// inferTopic extracts a topic slug from text.
// Prefers hyphenated compound words (e.g. "auth-migration", "ci-pipeline").
// Falls back to "general" if nothing useful is found.
func (m *Miner) inferTopic(text string) string {
	lower := strings.ToLower(text)
	matches := hyphenWordRe.FindAllString(lower, -1)
	if len(matches) > 0 {
		return matches[0]
	}
	return "general"
}
