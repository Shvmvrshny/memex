package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type QdrantStore struct {
	baseURL   string
	ollamaURL string
	client    *http.Client
}

func NewQdrantStore(baseURL, ollamaURL string) *QdrantStore {
	return &QdrantStore{
		baseURL:   baseURL,
		ollamaURL: ollamaURL,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (q *QdrantStore) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("health request: %w", err)
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qdrant unhealthy: status %d", resp.StatusCode)
	}
	return nil
}

// Init creates the memories collection and all payload indexes.
// Safe to call multiple times — existing collection and indexes are preserved.
func (q *QdrantStore) Init(ctx context.Context) error {
	return q.createCollection(ctx)
}

func (q *QdrantStore) createCollection(ctx context.Context) error {
	if err := q.put(ctx, "/collections/memories", map[string]interface{}{
		"vectors": map[string]interface{}{"size": 768, "distance": "Cosine"},
	}); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create collection: %w", err)
		}
	}

	indexes := []struct{ field, schema string }{
		{"text", "text"},
		{"project", "keyword"},
		{"memory_type", "keyword"},
		{"topic", "keyword"},
	}
	for _, idx := range indexes {
		if err := q.put(ctx, "/collections/memories/index", map[string]interface{}{
			"field_name":   idx.field,
			"field_schema": idx.schema,
		}); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("create %s index: %w", idx.field, err)
			}
		}
	}
	log.Printf("memex: memories collection ready")
	return nil
}

// buildFilter constructs a Qdrant filter from optional project/memoryType/topic values.
// Returns nil when all are empty (no filter).
func buildFilter(project, memoryType, topic string) map[string]any {
	var conditions []map[string]any
	if project != "" {
		conditions = append(conditions, map[string]any{
			"key": "project", "match": map[string]any{"value": project},
		})
	}
	if memoryType != "" {
		conditions = append(conditions, map[string]any{
			"key": "memory_type", "match": map[string]any{"value": memoryType},
		})
	}
	if topic != "" {
		conditions = append(conditions, map[string]any{
			"key": "topic", "match": map[string]any{"value": topic},
		})
	}
	if len(conditions) == 0 {
		return nil
	}
	return map[string]any{"must": conditions}
}

func (q *QdrantStore) SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if req.Source == "" {
		req.Source = "claude-code"
	}
	if req.Importance == 0 {
		req.Importance = 0.5
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	if req.Topic == "" {
		req.Topic = req.Project
	}

	vector, err := q.embed(ctx, req.Text)
	if err != nil {
		return Memory{}, fmt.Errorf("embed memory: %w", err)
	}

	body := map[string]any{
		"points": []map[string]any{{
			"id":     id,
			"vector": vector,
			"payload": map[string]any{
				"text":          req.Text,
				"project":       req.Project,
				"topic":         req.Topic,
				"memory_type":   req.MemoryType,
				"source":        req.Source,
				"timestamp":     now.Format(time.RFC3339),
				"importance":    req.Importance,
				"tags":          req.Tags,
				"last_accessed": now.Format(time.RFC3339),
			},
		}},
	}

	if err := q.put(ctx, "/collections/memories/points", body); err != nil {
		return Memory{}, fmt.Errorf("upsert point: %w", err)
	}

	return Memory{
		ID:           id,
		Text:         req.Text,
		Project:      req.Project,
		Topic:        req.Topic,
		MemoryType:   req.MemoryType,
		Source:       req.Source,
		Timestamp:    now,
		Importance:   req.Importance,
		Tags:         req.Tags,
		LastAccessed: now,
	}, nil
}

func (q *QdrantStore) vectorSearch(ctx context.Context, body map[string]any) ([]Memory, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal search body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.baseURL+"/collections/memories/points/search", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result []struct {
			ID      string         `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	points := make([]struct {
		ID      string         `json:"id"`
		Payload map[string]any `json:"payload"`
	}, len(result.Result))
	for i, r := range result.Result {
		points[i].ID = r.ID
		points[i].Payload = r.Payload
	}
	return pointsToMemories(points), nil
}

func (q *QdrantStore) SearchMemories(ctx context.Context, query, project, memoryType, topic string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}
	vector, err := q.embed(ctx, query)
	if err != nil {
		log.Printf("memex: embed fallback in SearchMemories: %v", err)
		return q.ListMemories(ctx, project, memoryType, topic, limit)
	}
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if f := buildFilter(project, memoryType, topic); f != nil {
		body["filter"] = f
	}
	return q.vectorSearch(ctx, body)
}

func (q *QdrantStore) ListMemories(ctx context.Context, project, memoryType, topic string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := limit * 10
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	body := map[string]any{
		"limit":        fetchLimit,
		"with_payload": true,
		"with_vector":  false,
	}
	if f := buildFilter(project, memoryType, topic); f != nil {
		body["filter"] = f
	}

	memories, err := q.scroll(ctx, body)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	score := func(m Memory) float64 {
		days := now.Sub(m.Timestamp).Hours() / 24
		recency := 1.0 / (1.0 + days/30.0)
		return 0.6*float64(m.Importance) + 0.4*recency
	}
	sort.Slice(memories, func(i, j int) bool {
		return score(memories[i]) > score(memories[j])
	})
	if len(memories) > limit {
		memories = memories[:limit]
	}
	return memories, nil
}

func (q *QdrantStore) PinnedMemories(ctx context.Context, project string) ([]Memory, error) {
	body := map[string]any{
		"limit":        10,
		"with_payload": true,
		"with_vector":  false,
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": "project", "match": map[string]any{"value": project}},
				{"key": "importance", "range": map[string]any{"gte": 0.9}},
			},
		},
	}
	memories, err := q.scroll(ctx, body)
	if err != nil {
		return nil, err
	}
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Importance > memories[j].Importance
	})
	return memories, nil
}

func (q *QdrantStore) PinMemory(ctx context.Context, id string) error {
	body := map[string]any{
		"payload": map[string]any{"importance": float64(1.0)},
		"points":  []string{id},
	}
	return q.post(ctx, "/collections/memories/points/payload", body)
}

func (q *QdrantStore) FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}
	vector, err := q.embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("embed for similarity: %w", err)
	}
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if project != "" {
		body["filter"] = map[string]any{
			"must": []map[string]any{
				{"key": "project", "match": map[string]any{"value": project}},
			},
		}
	}
	return q.vectorSearch(ctx, body)
}

func (q *QdrantStore) DeleteMemory(ctx context.Context, id string) error {
	body := map[string]any{"points": []string{id}}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.baseURL+"/collections/memories/points/delete", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (q *QdrantStore) embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]string{
		"model":  "nomic-embed-text",
		"prompt": text,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.ollamaURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding from ollama")
	}
	return result.Embedding, nil
}

type qdrantScrollResponse struct {
	Result struct {
		Points []struct {
			ID      string         `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"points"`
	} `json:"result"`
	Status string `json:"status"`
}

func (q *QdrantStore) scroll(ctx context.Context, body map[string]any) ([]Memory, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.baseURL+"/collections/memories/points/scroll", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result qdrantScrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode scroll response: %w", err)
	}
	return pointsToMemories(result.Result.Points), nil
}

func pointsToMemories(points []struct {
	ID      string         `json:"id"`
	Payload map[string]any `json:"payload"`
}) []Memory {
	memories := make([]Memory, 0, len(points))
	for _, p := range points {
		m := Memory{ID: p.ID}
		if v, ok := p.Payload["text"].(string); ok {
			m.Text = v
		}
		if v, ok := p.Payload["project"].(string); ok {
			m.Project = v
		}
		if v, ok := p.Payload["topic"].(string); ok {
			m.Topic = v
		}
		if v, ok := p.Payload["memory_type"].(string); ok {
			m.MemoryType = v
		}
		if v, ok := p.Payload["source"].(string); ok {
			m.Source = v
		}
		if v, ok := p.Payload["importance"].(float64); ok {
			m.Importance = float32(v)
		}
		if v, ok := p.Payload["timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				m.Timestamp = t
			} else {
				log.Printf("memex: parse timestamp %q: %v", v, err)
			}
		}
		if v, ok := p.Payload["last_accessed"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				m.LastAccessed = t
			} else {
				log.Printf("memex: parse last_accessed %q: %v", v, err)
			}
		}
		if v, ok := p.Payload["tags"].([]any); ok {
			for _, t := range v {
				if s, ok := t.(string); ok {
					m.Tags = append(m.Tags, s)
				}
			}
		}
		memories = append(memories, m)
	}
	return memories
}

func (q *QdrantStore) put(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, q.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (q *QdrantStore) post(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
