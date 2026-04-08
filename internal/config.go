package memex

import "os"

const defaultMemexURL = "http://localhost:8765"

func getMemexURL() string {
	if u := os.Getenv("MEMEX_URL"); u != "" {
		return u
	}
	return defaultMemexURL
}

type Config struct {
	Port      string
	QdrantURL string
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
	return Config{Port: port, QdrantURL: qdrantURL}
}
