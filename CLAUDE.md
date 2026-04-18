# Memex Retrieval Workflow

Before broad file exploration, query Memex first.

1. Use `GET /memories/expand?entity=<symbol-or-package>&project=<project>` to retrieve KG-anchored context.
2. Start with files/functions surfaced by expansion results.
3. Read additional files only when expansion context is insufficient.

Notes:
- Expansion defaults to deterministic predicates (`contains`, `calls`, `depends_on`, `test_of`).
- `calls_unresolved` is excluded by default to reduce retrieval noise.
