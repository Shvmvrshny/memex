package memex

import (
	"context"
	"log"
	"net/http"
	"strings"
)

func RunServe() {
	cfg := LoadConfig()
	store := NewQdrantStore(cfg.QdrantURL, cfg.OllamaURL)
	traceStore := NewTraceStore(cfg.QdrantURL)

	kg, err := NewKnowledgeGraph(cfg.KGPath)
	if err != nil {
		log.Fatalf("init knowledge graph: %v", err)
	}

	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		log.Fatalf("init memory store: %v", err)
	}
	if err := traceStore.Init(ctx); err != nil {
		log.Fatalf("init trace store: %v", err)
	}
	if err := kg.Init(); err != nil {
		log.Fatalf("init knowledge graph schema: %v", err)
	}

	h := NewHandlers(store)
	th := NewTraceHandlers(store, traceStore)
	kgh := NewKGHandlers(kg)

	mux := http.NewServeMux()

	// Memory routes
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/memories/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/memories/")
		switch {
		case path == "pinned" && r.Method == http.MethodGet:
			h.PinnedMemories(w, r)
		case path == "similar" && r.Method == http.MethodGet:
			h.FindSimilar(w, r)
		case strings.HasSuffix(path, "/pin") && r.Method == http.MethodPatch:
			h.PinMemory(w, r)
		case r.Method == http.MethodDelete:
			h.DeleteMemory(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/memories", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.SearchMemories(w, r)
		case http.MethodPost:
			h.SaveMemory(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/summarize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.Summarize(w, r)
	})

	// Knowledge Graph routes
	mux.HandleFunc("/facts/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		kgh.Stats(w, r)
	})
	mux.HandleFunc("/facts/timeline", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		kgh.History(w, r)
	})
	mux.HandleFunc("/facts/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			kgh.ExpireFact(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/facts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			kgh.RecordFact(w, r)
		case http.MethodGet:
			kgh.QueryEntity(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Trace routes
	mux.HandleFunc("/trace/event", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.TraceEvent(w, r)
	})
	mux.HandleFunc("/trace/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.TraceStop(w, r)
	})
	mux.HandleFunc("/trace/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.ListSessions(w, r)
	})
	mux.HandleFunc("/trace/session/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.GetSession(w, r)
	})
	mux.HandleFunc("/trace/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.ListProjects(w, r)
	})
	mux.HandleFunc("/checkpoint", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		th.Checkpoint(w, r)
	})

	// Serve UI static files
	mux.HandleFunc("/ui/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/ui")
		if path == "" || path == "/" {
			path = "/index.html"
		}
		http.ServeFile(w, r, "ui/dist"+path)
	})

	addr := ":" + cfg.Port
	log.Printf("memex listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
