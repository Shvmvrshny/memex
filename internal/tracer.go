package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type TraceStore struct {
	baseURL string
	client  *http.Client
}

func NewTraceStore(baseURL string) *TraceStore {
	return &TraceStore{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TraceStore) Init(ctx context.Context) error {
	collectionBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     1,
			"distance": "Dot",
		},
	}
	if err := t.put(ctx, "/collections/traces", collectionBody); err != nil {
		if !isAlreadyExists(err) {
			return fmt.Errorf("create traces collection: %w", err)
		}
	}

	indexBody := map[string]interface{}{
		"field_name":   "project",
		"field_schema": "keyword",
	}
	if err := t.put(ctx, "/collections/traces/index", indexBody); err != nil {
		if !isAlreadyExists(err) {
			return fmt.Errorf("create project index: %w", err)
		}
	}
	return nil
}

func (t *TraceStore) SaveEvent(ctx context.Context, req TraceEventRequest) (TraceEvent, error) {
	id := uuid.New().String()
	ts, err := time.Parse(time.RFC3339, req.Timestamp)
	if err != nil {
		ts = time.Now().UTC()
	}

	body := map[string]any{
		"points": []map[string]any{{
			"id":     id,
			"vector": []float32{0.0},
			"payload": map[string]any{
				"session_id":  req.SessionID,
				"project":     req.Project,
				"turn_index":  req.TurnIndex,
				"tool":        req.Tool,
				"input":       req.Input,
				"output":      req.Output,
				"reasoning":   "",
				"duration_ms": req.DurationMs,
				"timestamp":   ts.Format(time.RFC3339),
				"skill":       req.Skill,
			},
		}},
	}
	if err := t.put(ctx, "/collections/traces/points", body); err != nil {
		return TraceEvent{}, fmt.Errorf("save trace event: %w", err)
	}
	return TraceEvent{
		ID:         id,
		SessionID:  req.SessionID,
		Project:    req.Project,
		TurnIndex:  req.TurnIndex,
		Tool:       req.Tool,
		Input:      req.Input,
		Output:     req.Output,
		DurationMs: req.DurationMs,
		Timestamp:  ts,
		Skill:      req.Skill,
	}, nil
}

// UpsertReasoning updates an existing trace event point with reasoning text.
func (t *TraceStore) UpsertReasoning(ctx context.Context, eventID, sessionID, reasoning string) error {
	body := map[string]any{
		"points": []map[string]any{{
			"id":     eventID,
			"vector": []float32{0.0},
			"payload": map[string]any{
				"session_id": sessionID,
				"reasoning":  reasoning,
			},
		}},
	}
	return t.put(ctx, "/collections/traces/points", body)
}

func (t *TraceStore) ListSessions(ctx context.Context, project string) ([]Session, error) {
	body := map[string]any{
		"limit":        1000,
		"with_payload": true,
		"with_vector":  false,
	}
	if project != "" {
		body["filter"] = map[string]any{
			"must": []map[string]any{{
				"key":   "project",
				"match": map[string]any{"value": project},
			}},
		}
	}
	events, err := t.scroll(ctx, body)
	if err != nil {
		return nil, err
	}
	return eventsToSessions(events), nil
}

func (t *TraceStore) GetSessionEvents(ctx context.Context, sessionID string) ([]TraceEvent, error) {
	body := map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{{
				"key":   "session_id",
				"match": map[string]any{"value": sessionID},
			}},
		},
		"limit":        1000,
		"with_payload": true,
		"with_vector":  false,
	}
	return t.scroll(ctx, body)
}

func (t *TraceStore) ListProjects(ctx context.Context) ([]string, error) {
	body := map[string]any{
		"limit":        10000,
		"with_payload": true,
		"with_vector":  false,
	}
	events, err := t.scroll(ctx, body)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var projects []string
	for _, e := range events {
		if e.Project != "" && !seen[e.Project] {
			seen[e.Project] = true
			projects = append(projects, e.Project)
		}
	}
	return projects, nil
}

// eventsToSessions groups events by session_id and derives session metadata.
func eventsToSessions(events []TraceEvent) []Session {
	type sessionAcc struct {
		session Session
		count   int
	}
	acc := map[string]*sessionAcc{}
	for _, e := range events {
		if _, ok := acc[e.SessionID]; !ok {
			acc[e.SessionID] = &sessionAcc{
				session: Session{
					SessionID: e.SessionID,
					Project:   e.Project,
					StartTime: e.Timestamp,
					Skill:     e.Skill,
				},
			}
		}
		a := acc[e.SessionID]
		a.count++
		if e.Timestamp.Before(a.session.StartTime) {
			a.session.StartTime = e.Timestamp
		}
	}
	sessions := make([]Session, 0, len(acc))
	for _, a := range acc {
		a.session.ToolCount = a.count
		sessions = append(sessions, a.session)
	}
	return sessions
}

type traceScrollResponse struct {
	Result struct {
		Points []struct {
			ID      string         `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"points"`
	} `json:"result"`
}

func (t *TraceStore) scroll(ctx context.Context, body map[string]any) ([]TraceEvent, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		t.baseURL+"/collections/traces/points/scroll", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result traceScrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode scroll response: %w", err)
	}
	events := make([]TraceEvent, 0, len(result.Result.Points))
	for _, p := range result.Result.Points {
		events = append(events, pointToTraceEvent(p.ID, p.Payload))
	}
	return events, nil
}

func pointToTraceEvent(id string, payload map[string]any) TraceEvent {
	e := TraceEvent{ID: id}
	if v, ok := payload["session_id"].(string); ok { e.SessionID = v }
	if v, ok := payload["project"].(string); ok { e.Project = v }
	if v, ok := payload["tool"].(string); ok { e.Tool = v }
	if v, ok := payload["input"].(string); ok { e.Input = v }
	if v, ok := payload["output"].(string); ok { e.Output = v }
	if v, ok := payload["reasoning"].(string); ok { e.Reasoning = v }
	if v, ok := payload["skill"].(string); ok { e.Skill = v }
	if v, ok := payload["turn_index"].(float64); ok { e.TurnIndex = int(v) }
	if v, ok := payload["duration_ms"].(float64); ok { e.DurationMs = int64(v) }
	if v, ok := payload["timestamp"].(string); ok {
		e.Timestamp, _ = time.Parse(time.RFC3339, v)
	}
	return e
}

func (t *TraceStore) put(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, t.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
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

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return bytes.Contains([]byte(err.Error()), []byte("already exists"))
}
