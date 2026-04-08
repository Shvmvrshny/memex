package memex

import "time"

// TraceEvent is one tool call stored in the "traces" Qdrant collection.
type TraceEvent struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Project    string    `json:"project"`
	TurnIndex  int       `json:"turn_index"`
	Tool       string    `json:"tool"`
	Input      string    `json:"input"`
	Output     string    `json:"output"`
	Reasoning  string    `json:"reasoning"`
	DurationMs int64     `json:"duration_ms"`
	Timestamp  time.Time `json:"timestamp"`
	Skill      string    `json:"skill"`
}

// Session is derived at query time by aggregating TraceEvents sharing a SessionID.
type Session struct {
	SessionID string    `json:"session_id"`
	Project   string    `json:"project"`
	StartTime time.Time `json:"start_time"`
	ToolCount int       `json:"tool_count"`
	Skill     string    `json:"skill"`
}

// TraceEventRequest is the body of POST /trace/event.
type TraceEventRequest struct {
	SessionID  string `json:"session_id"`
	Project    string `json:"project"`
	TurnIndex  int    `json:"turn_index"`
	Tool       string `json:"tool"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	DurationMs int64  `json:"duration_ms"`
	Timestamp  string `json:"timestamp"`
	Skill      string `json:"skill"`
}

// StopRequest is the body of POST /trace/stop.
type StopRequest struct {
	SessionID      string `json:"session_id"`
	Project        string `json:"project"`
	TranscriptPath string `json:"transcript_path"`
}

// CheckpointRequest is the body of POST /checkpoint.
type CheckpointRequest struct {
	Project string `json:"project"`
	Summary string `json:"summary"`
}
