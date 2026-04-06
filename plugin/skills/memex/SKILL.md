---
name: memex
description: "Use when you learn something about the user, their preferences, decisions, or project context that should be remembered across sessions. Also use when the user asks you to remember or forget something."
---

# Memex Memory Management

You have access to a local memory system via three MCP tools: `save_memory`, `search_memory`, and `list_memories`.

## When to save a memory

Save a memory when:
- The user states a preference ("I prefer X over Y", "always use X", "never do Y")
- The user makes a significant decision ("we decided to use X for Y")
- The user shares important context about themselves or their project
- The user explicitly asks you to remember something

Do NOT save a memory for:
- Temporary task state or in-progress work
- Things already obvious from the code
- Every single message — only save things worth remembering next session

## How to save a memory

Write memories as clear, standalone statements that will make sense out of context:

Good: `"user prefers table-driven tests in Go"`
Bad: `"prefers that"`

Good: `"project uses SQLite for local storage, no external DB"`
Bad: `"uses sqlite"`

Use `importance: 0.9` for strong preferences or decisions. Use `importance: 0.5` (default) for general context.

## When the user asks you to forget something

Use `list_memories` to find the relevant memory, then inform the user that direct deletion is not yet supported in v1 — they can run `docker compose down -v && docker compose up -d` in the memex directory to reset all memories.

## Memory at session start

Memories from past sessions are automatically injected at the start of each session inside `<memex-memory>` tags. Use this context to personalise your responses without asking the user to repeat themselves.
