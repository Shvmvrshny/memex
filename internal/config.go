package memex

import (
	"os"
	"path/filepath"
)

const defaultMemexURL = "http://localhost:8765"

func getMemexURL() string {
	if u := os.Getenv("MEMEX_URL"); u != "" {
		return u
	}
	return defaultMemexURL
}

type Config struct {
	Port         string
	QdrantURL    string
	OllamaURL    string
	OllamaModel  string // LLM model for enrichment (OLLAMA_MODEL env var)
	RepoRoot     string // absolute path to repo being indexed (REPO_ROOT env var)
	IdentityPath string // ~/.memex/identity.md — L0 identity text
	KGPath       string // ~/.memex/knowledge_graph.db — SQLite KG
}

func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8765"
	}
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6333"
	}
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "llama3.2"
	}
	repoRoot := os.Getenv("REPO_ROOT")
	if repoRoot == "" {
		repoRoot = "."
	}
	home, _ := os.UserHomeDir()
	identityPath := os.Getenv("IDENTITY_PATH")
	if identityPath == "" {
		identityPath = filepath.Join(home, ".memex", "identity.md")
	}
	kgPath := os.Getenv("KG_PATH")
	if kgPath == "" {
		kgPath = filepath.Join(home, ".memex", "knowledge_graph.db")
	}
	return Config{
		Port:         port,
		QdrantURL:    qdrantURL,
		OllamaURL:    ollamaURL,
		OllamaModel:  ollamaModel,
		RepoRoot:     repoRoot,
		IdentityPath: identityPath,
		KGPath:       kgPath,
	}
}
