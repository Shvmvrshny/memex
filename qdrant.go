package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type QdrantStore struct {
	baseURL string
	client  *http.Client
}

func NewQdrantStore(baseURL string) *QdrantStore {
	return &QdrantStore{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
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

func (q *QdrantStore) Init(ctx context.Context) error {
	collectionBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     1,
			"distance": "Dot",
		},
	}
	if err := q.put(ctx, "/collections/memories", collectionBody); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create collection: %w", err)
		}
	}

	indexBody := map[string]interface{}{
		"field_name":   "text",
		"field_schema": "text",
	}
	if err := q.put(ctx, "/collections/memories/payload/index", indexBody); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create payload index: %w", err)
		}
	}

	return nil
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

	body := map[string]any{
		"points": []map[string]any{{
			"id":     id,
			"vector": []float32{0.0},
			"payload": map[string]any{
				"text":          req.Text,
				"project":       req.Project,
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
		Source:       req.Source,
		Timestamp:    now,
		Importance:   req.Importance,
		Tags:         req.Tags,
		LastAccessed: now,
	}, nil
}

func (q *QdrantStore) SearchMemories(ctx context.Context, query, project string, limit int) ([]Memory, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *QdrantStore) ListMemories(ctx context.Context, project string) ([]Memory, error) {
	return nil, fmt.Errorf("not implemented")
}

var _ = url.QueryEscape
var _ = uuid.New
