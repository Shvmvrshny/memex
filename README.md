# memex

A local AI memory system for Claude Code and Cursor. Remembers your preferences, decisions, and context across sessions using a Go HTTP service and Qdrant vector database running in Docker. All data stays on your machine — nothing leaves.

## How it works

- At session start, relevant memories are automatically injected into Claude's context
- During a session, Claude can save new memories via MCP tools
- All data stays local — nothing leaves your machine
- Any AI tool that supports MCP or HTTP can connect to the same memex instance and share memory

## Why memex

- **Cross-session memory** — preferences set today are available tomorrow
- **Cross-platform memory** — Claude Code, Cursor, and local LLMs (e.g. Ollama) can all read and write to the same memory store. Save a preference in one tool, use it in another
- **Accurate memory** — updating a preference deletes the old one first, so you never have conflicting memories
- **Private** — runs entirely on your machine, no cloud involved

## Requirements

- [Docker](https://www.docker.com/) (with Docker Compose)
- [Go 1.22+](https://go.dev/)
- [Claude Code](https://claude.ai/code) or [Cursor](https://www.cursor.com/)

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

**3. Register the MCP server**

**Claude Code:**
```bash
claude mcp add --scope user memex ~/go/bin/memex mcp
```

**Cursor:** Go to `Cursor Settings → MCP` and add:
```json
{
  "mcpServers": {
    "memex": {
      "command": "/Users/your-username/go/bin/memex",
      "args": ["mcp"]
    }
  }
}
```

Restart your editor. The `save_memory`, `search_memory`, `list_memories`, and `delete_memory` tools will be available in every session.

**4. (Optional) Add the session-start hook**

Automatically injects relevant memories at the start of each session.

**Claude Code** — add to `~/.claude/settings.json`:
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

**Cursor** — the hook is included in the `.cursor-plugin/` directory and runs automatically when the plugin is installed.

## Tracer Setup

Capture Claude Code tool calls and reasoning automatically. After starting memex, add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [{
      "hooks": [{
        "type": "command",
        "command": "~/.local/bin/memex-tracer-hook"
      }]
    }],
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "curl -s -X POST http://localhost:8765/trace/stop -H 'Content-Type: application/json' -d @-"
      }]
    }]
  }
}
```

Install the hook script:
```bash
cp plugin/tracer-hook.sh ~/.local/bin/memex-tracer-hook
chmod +x ~/.local/bin/memex-tracer-hook
```

Open the trace viewer at http://localhost:8765/ui/ after starting memex.

> **Note:** The hook script reads JSON from stdin. Verify the exact field names Claude Code sends by adding a test hook (`"command": "cat >> /tmp/hook-test.json"`) and inspecting `/tmp/hook-test.json` after running a tool call. Update `plugin/tracer-hook.sh` to map fields as needed.

## MCP Tools

| Tool | Description |
|------|-------------|
| `save_memory` | Save a preference, decision, or piece of context |
| `search_memory` | Search memories by keyword |
| `list_memories` | List all stored memories |
| `delete_memory` | Delete a memory by ID (use before saving an updated preference) |

## API

The HTTP service runs on `http://localhost:8765`.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/memories` | Save a memory |
| `GET` | `/memories?context=<query>` | Search memories |
| `DELETE` | `/memories/:id` | Delete a memory by ID |
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
