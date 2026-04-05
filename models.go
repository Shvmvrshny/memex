package main

import "time"

type Memory struct {
	ID           string    `json:"id"`
	Text         string    `json:"text"`
	Project      string    `json:"project"`
	Source       string    `json:"source"`
	Timestamp    time.Time `json:"timestamp"`
	Importance   float32   `json:"importance"`
	Tags         []string  `json:"tags"`
	LastAccessed time.Time `json:"last_accessed"`
}

type SaveMemoryRequest struct {
	Text       string   `json:"text"`
	Project    string   `json:"project"`
	Source     string   `json:"source"`
	Importance float32  `json:"importance"`
	Tags       []string `json:"tags"`
}

type SearchResponse struct {
	Memories []Memory `json:"memories"`
}
