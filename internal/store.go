package memex

import "context"

type Store interface {
	Init(ctx context.Context) error
	SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error)
	SearchMemories(ctx context.Context, query, project string, limit int) ([]Memory, error)
	ListMemories(ctx context.Context, project string) ([]Memory, error)
	DeleteMemory(ctx context.Context, id string) error
	Health(ctx context.Context) error
}
