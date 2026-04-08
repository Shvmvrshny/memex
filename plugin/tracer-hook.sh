#!/bin/bash
# Claude Code PostToolUse hook — pipes event JSON to memex tracer
# Install: add to ~/.claude/settings.json (see memex README)
read -r event
curl -s -X POST http://localhost:8765/trace/event \
  -H 'Content-Type: application/json' \
  -d "$event" > /dev/null 2>&1
exit 0
