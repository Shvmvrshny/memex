# Memex Code KG Pipeline Plan (Revised)

**Date:** 2026-04-18  
**Status:** In Progress

## Locked Architecture

- Deterministic structure from AST in SQLite KG.
- Semantic retrieval keys from LLM in Qdrant.
- Retrieval order: `query -> Qdrant entry nodes -> KG traversal -> answer`.
- Direct edges only in storage; transitive paths computed at query time.

## Day-1 Contracts

1. Stable IDs:
- `function_id = <package_id>::<receiver?>.<name>` (no signature hash).
- `package_id = <module>/<pkg>`.
2. Fact scope:
- Every AST/LLM fact carries `file_path` and `commit_hash`.
3. Explicit unresolved dispatch predicate:
- `calls_unresolved` for selector/interface/dynamic call sites that cannot be statically resolved.
4. Provenance:
- `source` always explicit (`ast` or `llm`).

## Phases

### Phase 1: AST Ingestion (Foundation)

- Extract edges:
- `file -> contains -> function`
- `function -> calls -> function` (direct/static only)
- `function -> calls_unresolved -> symbol`
- `package -> depends_on -> package`
- Update model:
- On file re-index: expire active facts for that `file_path`, then reinsert scoped facts.
- Acceptance:
- Deterministic snapshot on same commit.
- No stale active facts for re-indexed files.

### Phase 2: Lazy LLM Enrichment

- Trigger only on first retrieval of a node.
- Write retrieval-key summaries to Qdrant payload with structured tags (for filtering).
- Cache key: `(function_id, commit_hash)`.
- Acceptance:
- Cache hit on repeated retrieval.
- Auto-invalidation after commit hash changes.

### Phase 3: Retrieval + Hook Integration

- Integrate `/memories/expand` chain with KG traversal controls.
- Deliverable (explicit): upgrade session-start hook to inject KG-derived architecture summary.
- Keep active guidance in `CLAUDE.md`: query Memex before broad file exploration.
- Acceptance:
- Fixed eval set passes with improved time-to-first-correct-file.

## Evaluation Method

- Use fixed manual eval set instead of click telemetry:
- `[(query, expected_files)]` cases run after each phase.
- Track:
- correct file hit rate in top results
- number of blind file reads before first correct target.

## Known Gap (Before Any-Repo)

- `ArchitectureSummary(project=...)` currently uses `project` for label compression only.
- SQL does not yet filter rows by project/repo scope.
- Safe for memex dogfooding, must be fixed before multi-repo generalization.
