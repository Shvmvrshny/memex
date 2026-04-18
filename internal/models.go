package memex

import "time"

// ValidMemoryTypes is the canonical set of 10 memory types.
var ValidMemoryTypes = map[string]bool{
	"decision":   true,
	"preference": true,
	"event":      true,
	"discovery":  true,
	"advice":     true,
	"problem":    true,
	"context":    true,
	"procedure":  true,
	"rationale":  true,
	"code_node":  true, // AST-derived retrieval node
}

type Memory struct {
	ID           string    `json:"id"`
	Text         string    `json:"text"`
	Project      string    `json:"project"`
	Topic        string    `json:"topic"`
	MemoryType   string    `json:"memory_type"`
	Source       string    `json:"source"`
	Timestamp    time.Time `json:"timestamp"`
	Importance   float32   `json:"importance"`
	Tags         []string  `json:"tags"`
	LastAccessed time.Time `json:"last_accessed"`
	Score        float32   `json:"score,omitempty"` // similarity score, not stored in Qdrant
}

type SaveMemoryRequest struct {
	Text       string   `json:"text"`
	Project    string   `json:"project"`
	Topic      string   `json:"topic"`
	MemoryType string   `json:"memory_type"`
	Source     string   `json:"source"`
	Importance float32  `json:"importance"`
	Tags       []string `json:"tags"`
}

type SearchResponse struct {
	Memories []Memory `json:"memories"`
}

// Fact is a temporal entity-relationship triple stored in the knowledge graph.
type Fact struct {
	ID         string  `json:"id"`
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Object     string  `json:"object"`
	ValidFrom  string  `json:"valid_from,omitempty"`
	ValidUntil string  `json:"valid_until,omitempty"`
	Source     string  `json:"source,omitempty"`
	FilePath   string  `json:"file_path,omitempty"`
	CommitHash string  `json:"commit_hash,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	MetaJSON   string  `json:"meta_json,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

// RecordFactRequest is the body of POST /facts.
type RecordFactRequest struct {
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Object     string  `json:"object"`
	ValidFrom  string  `json:"valid_from,omitempty"`
	Source     string  `json:"source,omitempty"`
	FilePath   string  `json:"file_path,omitempty"`
	CommitHash string  `json:"commit_hash,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	MetaJSON   string  `json:"meta_json,omitempty"`
	Singular   bool    `json:"singular"`
}

// KGStats is returned by GET /facts/stats.
type KGStats struct {
	TotalFacts     int            `json:"total_facts"`
	ActiveFacts    int            `json:"active_facts"`
	ExpiredFacts   int            `json:"expired_facts"`
	EntityCount    int            `json:"entity_count"`
	PredicateTypes map[string]int `json:"predicate_types"`
}

type PackageDependency struct {
	Package   string   `json:"package"`
	DependsOn []string `json:"depends_on"`
}

// ConversationTurn is one full turn from a Claude Code JSONL transcript.
type ConversationTurn struct {
	Role string // "user" or "assistant"
	Text string
}

// MineRequest is the body of POST /mine/transcript.
type MineRequest struct {
	Path    string `json:"path"`
	Project string `json:"project"`
}

// MineResponse is returned by POST /mine/transcript.
type MineResponse struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}
