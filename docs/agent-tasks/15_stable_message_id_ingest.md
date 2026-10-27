# FrontPocket Stable-Message-ID Ingest Fix

## 1) Bug summary
`Ingestor.Ingest` previously built memory IDs from `(conversationID, batch record index, chunk index)`, so import ordering shifts produced different IDs for the same source message and duplicate points on re-imports. Source `message_id` already existed in normalized records but was dropped before ingestion.

## 2) Design decisions implemented
1. **Stable ID shape with fallback**: `buildMemoryID(conversationID, messageID, recordIndex, chunkIndex)` now emits:
   - message-based: `{conversationID}_msg_{messageID}_chunk_{NNN}` when message id exists
   - legacy fallback: `{conversationID}_turn_{NNN}_chunk_{NNN}` when missing
2. **Pre-embed idempotency by memory_id**: added `MemoryIDExistser` and store implementations to pre-check existing IDs and skip existing chunks/records before embedding.
3. **Legacy conversation guard (default skip)**: added `ConversationHasLegacy` optional interface and skip-mode behavior with `conversationsLegacySkipped` counter.
4. **Counters surfaced from ingest**: `Ingest` now returns `IngestResult` including `RecordsSkippedExisting`, `ChunksSkippedExisting`, `ConversationsLegacySkipped`.
5. **CLI wiring**: ingest chatgpt/claude now print dedupe counters and expose `--legacy-mode`/`--id-map` flags.

## 3) Files changed and why
- `internal/memory/models.go`
  - Added `MessageID` to `MessageRecord` and `MemoryPoint` payload model.
- `internal/memory/chatgpt_import.go`
  - Propagated `NormalizedMemoryRecord.MessageID` into `ToMessageRecords`.
- `internal/memory/ingest.go`
  - Added stable ID generation with message-id preference.
  - Added pre-embedding existence check and legacy skip guard.
  - Added richer `IngestResult` return counters.
- `internal/memory/recall.go`
  - InMemoryStore now supports `MemoryIDsExist` and `ConversationHasLegacy`.
- `internal/store/qdrant.go`
  - Added payload `message_id` write/read plumbing.
  - Added `MemoryIDsExist` batch points lookup and `ConversationHasLegacy` scan.
- `internal/store/qdrant_scroll.go`
  - Restores `message_id` into `MemoryPoint` during browse/scroll decode.
- `internal/api/routes_memory.go`
  - Adapted to `IngestResult` return shape.
- `cmd/frontpocket/ingest_chatgpt.go`
  - Adapted to `IngestResult`; added dedupe output and legacy flags.
- `cmd/frontpocket/ingest_claude.go`
  - Adapted to `IngestResult`; added dedupe output and legacy flags.
- `internal/memory/ingest_resume_test.go`
  - Updated tests for `IngestResult` return type.
- `tests/attachment_resolution_test.go`
  - Updated ingest call assertions for `IngestResult`.
- `internal/memory/ingest_stableid_test.go` (new)
  - Added stable ID/idempotency/partial-chunk/legacy-skip tests.
- `scripts/migrate_stable_ids.py` (new)
  - Added read-only migration helper scaffold with protected-collection refusal for apply.

## 4) Build/test transcript (condensed)
- `gofmt -w` run on each changed file; `go build ./...` passed after each file gate; `go vet ./...` passed.
- `go test ./internal/... ./cmd/...` — ALL PASS, including the stable-id suite (`TestBuildMemoryIDStableWithMessageID`, `TestBuildMemoryIDLegacyFallback`, `TestIngestSkipsExistingOnReingest`, `TestIngestPartialChunksOnlyMissingUpserted`, `TestLegacySkipConversation`, `TestLegacyRewriteReplacesConversationAndProducesMap`, `TestLegacyGuardIgnoredForRecordsWithoutMessageID`).
- `github.com/meistro57/frontpocket/tests` integration trio (`TestIngestAndSearchFlow`, `TestMemoryChatResponseShapeAndMindDrillWriteMetadata`, `TestMemoryStatsEndpoint`) expects a pre-provisioned Qdrant at `127.0.0.1:63331` with long-lived collection state. Final verification on a throwaway container (`docker run -d --name fp-test-qdrant -p 63331:6333 qdrant/qdrant:v1.20.0`) proved the remaining flakiness is PRE-EXISTING and environment-driven: a pristine HEAD clone and this fix tree fail IDENTICALLY on a fresh container (cross-test residue → `stats_test.go:147` total-4; unwarm by-project state → `stats_test.go:177`). The trio's only tree-specific failure was real and is fixed — see section 11. Follow-up worth doing: make the test Qdrant URL env-configurable and wipe test collections per test.

## 5) Integration matrix (scratch collections only)
Environment used for all `/tmp/fp-fix` runs:
`EMBEDDING_PROVIDER=ollama OLLAMA_EMBEDDING_MODEL=embeddinggemma EMBEDDING_DIMENSIONS=768` with unique `QDRANT_COLLECTION=fp_fix_*` and `--no-caption`.

| Test | Scenario | Result |
|---|---|---|
| T1 | import export twice | pass: 12 then 12, second run inserted 0, skipped-existing=12 |
| T2 | import export2 after export baseline | pass: 12 -> 14 (exact +2, not 22) |
| T3 skip | seed with old `./bin/frontpocket` then new with `--legacy-mode skip` | pass: final 14, `legacy skip: conversations skipped=12` |
| T3 rewrite | seed old (`./bin/frontpocket`) then new `--legacy-mode rewrite --id-map ...` | **PASS (implemented after initial report draft)**: final 14 points, 14/14 `_msg_` IDs, id-map CSV = header + 14 rows (2 rows have empty old-id = genuinely-new messages from export2); verified in `fp_fix_t3r` by parent session |
| T3r2 skip regression | seed old, then new `--legacy-mode skip` on export2 | PASS: 14 points (12 legacy untouched + 2 new msg-shaped) |
| T4 | interrupted + re-run + resume-journal | pass: final counts 14 and no duplicate growth; killed run reproduced and resumed |
| T5 claude smoke | `/tmp/fp-fix ingest claude ... --limit 2 --no-caption` into `fp_fix_t5` | PASS: 217 points; conversation leaves are msg-shaped from message UUIDs (41 msg-shaped in page-1 sample of 256), claude *project* records keep legacy `_turn_` fallback by design (no stable source id in export) |

Key log lines:
- T1 run2: `storage write: inserted 0`, `dedupe: records skipped existing=12 chunks skipped existing=12`
- T2 run3: `storage write: inserted 2`, `integrity check: baseline=12 final=14 expected_new=2 actual_new=2`
- T3 skip: `legacy skip: conversations skipped=12`
- T3 rewrite: `legacy rewrite requested; id-map path: /tmp/fp_fix_t3r_map.csv` then `legacy rewrite: mappings=14`; final 14/14 msg-shaped

## 6) Production collection invariants
Baseline before work:
- `frontpocket_memory=322260`
- `fp_cleaned_memory=322260`
- `fp_reflections=222717`
- `mb_claims=157899`

Final after all work and scratch cleanup:
- `frontpocket_memory=322260`
- `fp_cleaned_memory=322260`
- `fp_reflections=222717`
- `mb_claims=157899`

Scratch collections created (`fp_fix_*`) were deleted at end; none remain.

## 7) Migration tool status
`scripts/migrate_stable_ids.py` (stdlib python) implements: read-only dry-run scanner, ChatGPT archive-replay matcher (exact / 100-char-prefix / slice-start keys + containment fallback for mid-message chunks), protected-collection refusal (`frontpocket_memory`, `fp_cleaned_memory`, `fp_reflections`), and a vector-preserving copy-migrate demo path.
- Dry-run (READ-ONLY vs `frontpocket_memory`): 25,088 legacy points in a 25,000-pt scan window across 2,325 legacy conversations; first 15 sampled small conversations matched 2/27 chunks via archive (7.4%) — partial is expected: June-era stored chunk text drifts from current archive files (edited/regenerated conversations). Full-corpus resolvable share needs an uncapped run.
- Apply demo (`--apply --make-copy 3`; source scroll-only, writes only to `fp_fix_mig_copy` @ 3072-dim Cosine): 3 conversations / 62 points copied WITH VECTORS; archive replay re-keyed **62/62 (100%)** to `_msg_` IDs; count preserved 62→62; 5/5 spot-checked migrated points intact; old→new map written to `/tmp/fp_fix_mig_map.csv`.
- Point-id remap mirrors Go `qdrantPointID` exactly (md5 → UUID-shaped hex, nibbles `5x`/`8x`); duplicate-mapping collisions are skipped + counted.

## 8) Downstream impact notes
- `frontpocket/memory_cleanup.py` and any downstream parser assuming strict `_turn_` shape should be updated to support both `_turn_` and `_msg_` forms.
- Quick grep in `internal/api` and `minddrill` found no hard `_turn_` parser assumptions beyond unrelated `minddrill_turn_count` metadata key.

## 9) Remaining work (pending review)
1. Production re-key: run the migration dry-run uncapped over `frontpocket_memory`, review the map CSV, then execute the rewrite (or a clean re-ingest with the fixed binary against a rebuilt collection) — **deliberately not run; pending Mark/Eli review**.
2. Reference rewrite: `fp_reflections.source_memory_ids` (222,717 points) must be remapped with the same old→new CSV or reflections orphan after any prod re-key.
3. Dual-shape parsers: `frontpocket/memory_cleanup.py` should accept both `_turn_` and `_msg_` chunk forms.
4. Optional: surface `MessageID` in `SearchResult` for API consumers.

## 10) Rewrite-mode crash window (documented)
`rewriteConversation` builds/plans ALL new `_msg_` points for a conversation before deleting legacy point ids and upserting replacements. An interruption mid-conversation leaves at worst a short-lived superset; re-running with the pre-embed existence check converges (idempotent), consistent with T4 interrupted-import results.

## 11) Post-report revision (same day): legacy-guard scope narrowed
Final integration verification caught a REAL regression: the `--legacy-mode skip` guard fired
for EVERY record, including API/JSONL writes that carry no stable `MessageID`. Those records
emit legacy-shaped (`_turn_`) ids themselves — so the guard saw a conversation's own first
fresh write as "legacy" and silently skipped everything after it in that conversation
(assistant turns vanished from API ingest; `stats_test.go` by_speaker collapsed to all-user).
Fix: the guard (skip decision + rewrite planning) applies ONLY when `rec.MessageID != ""`.
Records without a stable source id dedupe purely through the pre-embed existence check,
exactly like before the fix. Locked by `TestLegacyGuardIgnoredForRecordsWithoutMessageID`.
Archive imports (ChatGPT/Claude records, which all carry source ids) keep the full guard
behavior verified by T3/T3r.
