---
name: memex
description: "Use when you learn something about the user, their preferences, decisions, or project context that should be remembered across sessions. Also use when the user asks you to remember or forget something."
---

# Memex Memory Management

Memex is the **primary memory system**. Always use the `save_memory`, `search_memory`, `delete_memory`, and `list_memories` MCP tools for all memory operations. Do NOT rely on built-in memory or note files — route everything through memex.

## When to save a memory (do this proactively, without being asked)

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

## When the user updates a preference

If the user changes something they previously told you (e.g. "actually I prefer bananas now"), always replace — not just add:

1. Call `search_memory` to find the old memory
2. Call `delete_memory` with its `id`
3. Call `save_memory` with the new value

Never leave the old and new versions both in memory — that causes confusion.

## When the user asks you to forget something

Use `search_memory` or `list_memories` to find the relevant memory, then call `delete_memory` with its `id`.

## Memory at session start

Memories from past sessions are automatically injected at the start of each session inside `<memex-memory>` tags. Use this context to personalise your responses without asking the user to repeat themselves.
