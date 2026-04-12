package memex

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// fakeStore is a thread-safe in-memory implementation of Store for unit tests.
// FindSimilar uses prefix substring matching as a proxy for semantic similarity.
type fakeStore struct {
	mu       sync.RWMutex
	memories map[string]Memory
	nextID   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{memories: make(map[string]Memory)}
}

func (f *fakeStore) Init(_ context.Context) error  { return nil }
func (f *fakeStore) Health(_ context.Context) error { return nil }

func (f *fakeStore) SaveMemory(_ context.Context, req SaveMemoryRequest) (Memory, error) {
	if req.Text == "" {
		return Memory{}, fmt.Errorf("text required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("fake-%d", f.nextID)
	now := time.Now().UTC()
	topic := req.Topic
	if topic == "" {
		topic = req.Project
	}
	imp := req.Importance
	if imp == 0 {
		imp = 0.5
	}
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	m := Memory{
		ID:           id,
		Text:         req.Text,
		Project:      req.Project,
		Topic:        topic,
		MemoryType:   req.MemoryType,
		Source:       req.Source,
		Timestamp:    now,
		Importance:   imp,
		Tags:         tags,
		LastAccessed: now,
	}
	f.memories[id] = m
	return m, nil
}

func (f *fakeStore) SearchMemories(_ context.Context, query, project, memoryType, topic string, limit int) ([]Memory, error) {
	return f.ListMemories(context.Background(), project, memoryType, topic, limit)
}

func (f *fakeStore) ListMemories(_ context.Context, project, memoryType, topic string, limit int) ([]Memory, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []Memory
	for _, m := range f.memories {
		if project != "" && m.Project != project {
			continue
		}
		if memoryType != "" && m.MemoryType != memoryType {
			continue
		}
		if topic != "" && m.Topic != topic {
			continue
		}
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		si := 0.6*float64(result[i].Importance) + 0.4/float64(1+int(time.Since(result[i].Timestamp).Hours()/24))
		sj := 0.6*float64(result[j].Importance) + 0.4/float64(1+int(time.Since(result[j].Timestamp).Hours()/24))
		return si > sj
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeStore) PinnedMemories(_ context.Context, project string) ([]Memory, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []Memory
	for _, m := range f.memories {
		if m.Project == project && m.Importance >= 0.9 {
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Importance > result[j].Importance
	})
	return result, nil
}

func (f *fakeStore) PinMemory(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.memories[id]
	if !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	m.Importance = 1.0
	f.memories[id] = m
	return nil
}

func (f *fakeStore) FindSimilar(_ context.Context, text, project string, limit int) ([]Memory, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	lower := strings.ToLower(text)
	prefix := lower
	if len(prefix) > 20 {
		prefix = prefix[:20]
	}
	var result []Memory
	for _, m := range f.memories {
		if project != "" && m.Project != project {
			continue
		}
		if strings.Contains(strings.ToLower(m.Text), prefix) {
			result = append(result, m)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeStore) DeleteMemory(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.memories[id]; !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	delete(f.memories, id)
	return nil
}
