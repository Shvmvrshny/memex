# memex

A local AI memory system for Claude Code. Remembers your preferences, decisions, and context across sessions using a Go HTTP service and Qdrant vector database running in Docker.

## How it works

- At session start, relevant memories are automatically injected into Claude's context
- During a session, Claude can save new memories via MCP tools
- All data stays local — nothing leaves your machine

## Requirements

- [Docker](https://www.docker.com/) (with Docker Compose)
- [Go 1.22+](https://go.dev/)
- [Claude Code](https://claude.ai/code)

## Installation

**1. Clone and build**

```bash
git clone https://github.com/Shvmvrshny/memex
cd memex
go install ./cmd/memex
```

**2. Start the Docker stack**

```bash
docker compose up -d
```

This starts two containers:
- `memex` — HTTP API on port `8765`
- `qdrant` — vector database on port `6333`

**3. Register the MCP server with Claude Code**

```bash
claude mcp add --scope user memex ~/go/bin/memex mcp
```

Restart Claude Code. The `save_memory`, `search_memory`, and `list_memories` tools will be available in every session.

**4. (Optional) Add the session-start hook**

To automatically inject memories at the start of each session, add this to your `~/.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "memex hook session-start"
          }
        ]
      }
    ]
  }
}
```

## MCP Tools

| Tool | Description |
|------|-------------|
| `save_memory` | Save a preference, decision, or piece of context |
| `search_memory` | Search memories by keyword |
| `list_memories` | List all stored memories |

## API

The HTTP service runs on `http://localhost:8765`.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/memories` | Save a memory |
| `GET` | `/memories?context=<query>` | Search memories |
| `POST` | `/summarize` | Save a session summary |

## Project structure

```
memex/
├── cmd/
│   └── memex/
│       └── main.go       # entry point
├── internal/
│   ├── config.go         # env config
│   ├── models.go         # Memory, SaveMemoryRequest, SearchResponse
│   ├── store.go          # Store interface
│   ├── qdrant.go         # Qdrant REST client
│   ├── handlers.go       # HTTP handlers
│   ├── server.go         # HTTP server setup
│   ├── hook.go           # session-start/stop hook logic
│   └── mcp.go            # MCP stdio server
├── plugin/               # Claude Code plugin manifest and skill
├── Dockerfile
└── docker-compose.yml
```

## Stopping

```bash
docker compose down
```

To reset all memories:

```bash
docker compose down -v && docker compose up -d
```
