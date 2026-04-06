package memex

import (
	"context"
	"log"
	"net/http"
)

func RunServe() {
	cfg := LoadConfig()
	store := NewQdrantStore(cfg.QdrantURL)

	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		log.Fatalf("init store: %v", err)
	}

	h := NewHandlers(store)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Health)
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

	addr := ":" + cfg.Port
	log.Printf("memex listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
