# memex

A local AI memory system for Claude Code and Cursor. Remembers your preferences, decisions, and context across sessions using a Go HTTP service and Qdrant vector database running in Docker. All data stays on your machine — nothing leaves.

## How it works

- At session start, relevant memories are automatically injected into Claude's context
- During a session, Claude can save new memories via MCP tools
- Every tool call is automatically traced — captured with input, output, duration, and reasoning
- All data stays local — nothing leaves your machine
- Any AI tool that supports MCP or HTTP can connect to the same memex instance and share memory

## Why memex

- **Cross-session memory** — preferences set today are available tomorrow
- **Cross-platform memory** — Claude Code, Cursor, and local LLMs (e.g. Ollama) can all read and write to the same memory store. Save a preference in one tool, use it in another
- **Accurate memory** — updating a preference deletes the old one first, so you never have conflicting memories
- **Session tracer** — replay any past session: see every tool call, what it did, how long it took, and Claude's reasoning behind it
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

**4. (Optional) Add hooks**

Hooks enable automatic memory injection at session start and full session tracing.

**Claude Code** — add to `~/.claude/settings.json`:
```json
{
  "hooks": {
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "memex hook session-start" }] }
    ],
    "SessionEnd": [
      { "hooks": [{ "type": "command", "command": "memex hook session-stop", "async": true }] }
    ],
    "PreToolUse": [
      { "hooks": [{ "type": "command", "command": "memex hook pre-tool-use", "async": true }] }
    ],
    "PostToolUse": [
      { "hooks": [{ "type": "command", "command": "memex hook post-tool-use", "async": true }] }
    ]
  }
}
```

- `SessionStart` — injects relevant memories into context at the start of each session
- `PreToolUse` / `PostToolUse` — traces every tool call with input, output, and duration
- `SessionEnd` — finalises the session trace and backfills Claude's reasoning from the transcript

**Cursor** — the hook is included in the `.cursor-plugin/` directory and runs automatically when the plugin is installed.

**5. (Optional) Open the tracer UI**

After enabling the hooks and running a session, open `http://localhost:8765/ui/` to browse past sessions by project and replay every tool call in order.

## MCP Tools

| Tool | Description |
|------|-------------|
| `save_memory` | Save a preference, decision, or piece of context |
| `search_memory` | Search memories by keyword |
| `list_memories` | List all stored memories |
| `delete_memory` | Delete a memory by ID (use before saving an updated preference) |

## API

The HTTP service runs on `http://localhost:8765`.

**Memory**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/memories` | Save a memory |
| `GET` | `/memories?context=<query>` | Search memories |
| `DELETE` | `/memories/:id` | Delete a memory by ID |
| `POST` | `/summarize` | Save a session summary |

**Tracer**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/trace/event` | Record a tool call event |
| `POST` | `/trace/stop` | Finalise a session (backfills reasoning from transcript) |
| `GET` | `/trace/projects` | List all traced projects |
| `GET` | `/trace/sessions?project=<name>` | List sessions for a project |
| `GET` | `/trace/session/:id` | Get all events for a session |
| `POST` | `/checkpoint` | Save a session summary as a high-importance memory |

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
│   ├── qdrant.go         # Qdrant REST client (memory collection)
│   ├── handlers.go       # HTTP handlers (memory)
│   ├── tracer_models.go  # TraceEvent, Session, TraceEventRequest
│   ├── tracer.go         # TraceStore — Qdrant ops for traces collection
│   ├── tracer_handlers.go# HTTP handlers (tracer)
│   ├── transcript.go     # Claude transcript parser (reasoning backfill)
│   ├── distill.go        # session summary distillation
│   ├── server.go         # HTTP server setup
│   ├── hook.go           # hook logic (session-start/stop, pre/post-tool-use)
│   └── mcp.go            # MCP stdio server
├── ui/                   # React tracer UI (Vite)
├── plugin/               # Claude Code plugin manifest and skills
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
