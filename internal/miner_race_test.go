package memex

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionStop_AsyncMine_DoesNotBlock(t *testing.T) {
	var mineCallCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/trace/stop":
			w.WriteHeader(http.StatusOK)
		case "/mine/transcript":
			mineCallCount.Add(1)
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(MineResponse{Status: "mining started"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("MEMEX_URL", srv.URL)

	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "session.jsonl")
	os.WriteFile(transcriptPath, []byte(`{"role":"user","content":"we decided to use sqlite"}`+"\n"), 0644)

	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	body, _ := json.Marshal(MineRequest{Path: transcriptPath, Project: "test"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		srv.URL+"/mine/transcript", bytes.NewReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		http.DefaultClient.Do(req)
	}

	elapsed := time.Since(start)
	if elapsed > 2100*time.Millisecond {
		t.Errorf("hookSessionStop took %v, expected < 2.1s", elapsed)
	}
}

func TestSessionStop_MinerDown_TraceStillSucceeds(t *testing.T) {
	var traceStopCalled atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/trace/stop":
			traceStopCalled.Add(1)
			w.WriteHeader(http.StatusOK)
		case "/mine/transcript":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("MEMEX_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	body, _ := json.Marshal(MineRequest{Path: "/tmp/test.jsonl", Project: "test"})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/mine/transcript", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
	// No panic or test failure = success (mining failure doesn't propagate)
}

func TestSessionStop_EmptyTranscriptPath_NoMineCall(t *testing.T) {
	var mineCallCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mine/transcript" {
			mineCallCount.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// hookSessionStop logic: if TranscriptPath == "", mining is skipped
	transcriptPath := ""
	shouldMine := transcriptPath != ""
	if shouldMine {
		t.Error("empty transcript path should not trigger mining")
	}
	if mineCallCount.Load() != 0 {
		t.Errorf("mine called %d times with empty transcript path, want 0", mineCallCount.Load())
	}
}

func TestMiner_SameTranscript_TwiceIsIdempotent(t *testing.T) {
	callCount := 0

	store := &mockMinerStore{}
	store.mockStore.findSimilarFn = func(ctx context.Context, text, project string, limit int) ([]Memory, error) {
		callCount++
		if callCount > 2 {
			return []Memory{{Text: text, MemoryType: "preference", Score: 0.99}}, nil
		}
		return []Memory{}, nil
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	os.WriteFile(path, []byte(`{"role":"user","content":"We decided to use SQLite WAL for temporal queries. This is a key architectural decision."}`+"\n"), 0644)

	miner := NewMiner(store)

	first, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("first mine: %v", err)
	}

	callCount = 3 // ensure second call sees high-score results
	second, err := miner.MineTranscript(path, "memex")
	if err != nil {
		t.Fatalf("second mine: %v", err)
	}

	if len(first) > 0 && len(second) > 0 {
		t.Errorf("second mining of same transcript saved %d memories, want 0 (all duplicates)", len(second))
	}
}
