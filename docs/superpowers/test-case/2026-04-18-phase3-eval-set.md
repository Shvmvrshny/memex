# Phase 3 Fixed Eval Set (Memex Repo)

Use these queries after each retrieval change and verify expected files appear in top results.

| Query | Expected files |
|---|---|
| `how does session-start memory injection work` | `internal/hook.go`, `internal/hook_test.go` |
| `where is KG fact storage implemented` | `internal/kg.go`, `internal/kg_handlers.go` |
| `how does expanded retrieval work` | `internal/handlers.go` |
| `where are MCP fact tools wired` | `internal/mcp.go` |
| `how are code facts indexed` | `internal/code_indexer.go`, `internal/index_cmd.go` |
