package memex

import "context"

type Store interface {
	Init(ctx context.Context) error
	SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error)

	// SearchMemories performs semantic search. memoryType, topic, and tags are optional filters.
	// Tags apply a soft scoring boost in ListMemories fallback; in vector search they filter hard.
	SearchMemories(ctx context.Context, query, project, memoryType, topic string, tags []string, limit int) ([]Memory, error)

	// ListMemories lists memories by recency+importance score. memoryType, topic, and tags are optional.
	// Tags apply a +0.15 soft boost to the ranking score (no hard filter).
	ListMemories(ctx context.Context, project, memoryType, topic string, tags []string, limit int) ([]Memory, error)

	// PinnedMemories returns memories with importance >= 0.9 for the project, sorted desc.
	PinnedMemories(ctx context.Context, project string) ([]Memory, error)

	// PinMemory sets importance = 1.0 on a memory by ID.
	PinMemory(ctx context.Context, id string) error

	// FindSimilar embeds text and returns the most similar memories for duplicate detection.
	FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error)

	DeleteMemory(ctx context.Context, id string) error
	Health(ctx context.Context) error
}
