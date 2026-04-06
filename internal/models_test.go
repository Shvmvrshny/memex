package memex

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryJSONRoundtrip(t *testing.T) {
	m := Memory{
		ID:           "test-id",
		Text:         "user prefers Python",
		Project:      "memex",
		Source:       "claude-code",
		Timestamp:    time.Now().UTC().Truncate(time.Second),
		Importance:   0.8,
		Tags:         []string{"preference", "python"},
		LastAccessed: time.Now().UTC().Truncate(time.Second),
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Memory
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Text != m.Text {
		t.Errorf("got Text %q, want %q", got.Text, m.Text)
	}
	if len(got.Tags) != len(m.Tags) {
		t.Errorf("got %d tags, want %d", len(got.Tags), len(m.Tags))
	}
}
