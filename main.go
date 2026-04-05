package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: memex <serve|mcp|hook <session-start|session-stop>>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		runServe()
	case "mcp":
		runMCP()
	case "hook":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: memex hook <session-start|session-stop>")
			os.Exit(1)
		}
		runHook(os.Args[2])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runServe() {
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
