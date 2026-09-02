# Getting Started

## 1) Prepare config

```bash
cp .env.example .env
```

To use Gemini embeddings through OpenRouter, set:

```env
EMBEDDING_PROVIDER=openrouter
OPENROUTER_EMBEDDING_MODEL=google/gemini-embedding-2-preview
```

If you plan to use the MindDrill UI or another browser app, list its origin so the API
returns CORS headers for it (the default already includes MindDrill's `:8089`):

```env
CORS_ALLOW_ORIGINS=http://localhost:3000,http://localhost:5173,http://localhost:8080,http://localhost:8089
```

MindDrill chat memory defaults:

```env
MINDDRILL_MEMORY_COLLECTION=minddrill_chat_memory
MINDDRILL_MEMORY_ENABLED=true
MINDDRILL_MEMORY_WRITE_MODE=summary
MINDDRILL_MEMORY_TOP_K=6
MINDDRILL_MEMORY_SESSION_SUMMARY_EVERY=8

# Proposed canon review queue file path (used by API + CLI memory loop)
FRONTPOCKET_PROPOSED_CANON_PATH=data/proposed_canon.json

# Optional local helper endpoints for MindDrill session reset/search UI actions:
# DEV_DEBUG_ENDPOINTS=true
```

To enable generated chat answers (instead of the retrieval-only fallback that just lists
memory hits as text), set a chat provider. For DeepSeek, use the official API
(https://api-docs.deepseek.com/) via the OpenAI-compatible provider settings:

```env
CHAT_PROVIDER=openai
OPENAI_BASE_URL=https://api.deepseek.com/v1
OPENAI_CHAT_MODEL=deepseek-chat
OPENAI_API_KEY=your-deepseek-api-key
```

With `CHAT_PROVIDER=none` (the default), `/memory/chat` still works but returns a templated
summary of retrieved memory rather than a generated answer — useful for testing retrieval
without burning API credits, not intended as the normal operating mode.

Search result volume also affects chat quality — the default `SEARCH_DEFAULT_LIMIT` controls
how many memory chunks feed each chat turn. A low limit (e.g. 5) can starve broad questions
of relevant context even when the corpus has plenty of matching material:

```env
SEARCH_DEFAULT_LIMIT=15
SEARCH_MAX_LIMIT=30
```

## 2) Build + ensure helper scripts are executable

```bash
./make_all.sh
```

## 3) Install missing local dependencies

```bash
./scripts/install_qdrant_redis.sh
```

## 4) Start FrontPocket stack

```bash
docker compose up --build
```

## 5) Verify health

```bash
curl http://localhost:8088/health
```

## 6) Check OpenAPI schema

```bash
curl http://localhost:8088/openapi.json
```

## 7) Check memory stats

```bash
curl 'http://localhost:8088/memory/stats?project=FrontPocket'
```

`/memory/stats` is optimized for large datasets, it uses Qdrant count/collection-info endpoints
plus cached distinct-field aggregation instead of request-time full scans.

## 8) Save session state

```bash
curl -X POST http://localhost:8088/memory/session \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"frontpocket-dev","project":"FrontPocket","active_summary":"Working on memory endpoints"}'
```

## 9) Delete session state (optional)

```bash
curl -X DELETE 'http://localhost:8088/memory/session?session_id=frontpocket-dev'
```

## 10) Chat with dual memory retrieval

```bash
curl -X POST http://localhost:8088/memory/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"minddrill-dev","message":"remember this: keep responses concise","system_prompt":"Use a concise, practical persona and call out uncertainty.","remember_this":true}'
```

The optional `system_prompt` field is for persona or tone guidance on the chat turn.
The response includes `answer`, `used_frontpocket_memories`, `used_minddrill_memories`,
`context_pack`, `model`, and `provider`.

## 11) Ingest mixed documents & media from a folder (DOCX, PDF slides, diagrams, audio)

FrontPocket can scan and ingest any folder containing mixed documents, presentation slides, diagrams, and audio files (e.g. `./incoming`):

```bash
# Preview what would be processed without storing
frontpocket ingest folder incoming --dry-run

# Ingest all documents and media with a project tag
frontpocket ingest folder incoming --project UTMGR

# Ingest documents, slides, and diagrams first (skip audio)
frontpocket ingest folder incoming --project UTMGR --no-audio

# Ingest without vision model calls (skip slide/diagram vision extraction)
frontpocket ingest folder incoming --project UTMGR --no-vision

# Write normalized records to a JSONL file
frontpocket ingest folder incoming --project UTMGR --out data/processed/incoming_records.jsonl

# Interruption-safe run using a persistent progress journal
frontpocket ingest folder incoming --project UTMGR --resume .frontpocket_cache/resume_journal.json
```

Extraction details:
- **Word Documents (`.docx`)**: Native Go OpenXML parser extracts document structure, section headings, and body paragraphs.
- **Presentations & Slides (`.pdf`)**: Extracts embedded text if present. For visual slides (such as NotebookLM slide presentations), slide pages are rendered at native resolution and transcribed/described using `VISION_MODEL`.
- **Diagrams & Images (`.png`, `.jpg`, `.jpeg`, `.webp`)**: Transcribes labels, formulas, node connections, flow, and concepts using `VISION_MODEL`.
- **Audio & Video (`.m4a`, `.mp3`, `.wav`, `.mp4`)**: GPU-accelerated speech-to-text using WhisperX/Whisper with time-coded segment chunks (`[MM:SS - MM:SS]`).
- **Persistent Caching**: Vision captions and audio transcripts are saved in `.frontpocket_cache/` so rerunning or interrupted imports never repeat expensive vision or audio processing.

## 12) Ingest a ChatGPT export (zip or folder)

```bash
frontpocket ingest chatgpt ./chatgpt-export.zip --dry-run
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket
frontpocket ingest chatgpt ./chatgpt-export.zip --out data/processed/chatgpt_normalized.jsonl
frontpocket ingest chatgpt ./unzipped-chatgpt-export/
```

Useful flags for targeted runs and debugging:

```bash
# cap how many conversations get processed (0 = no limit)
frontpocket ingest chatgpt ./export --limit 50

# scope to exactly one conversation by id (exact match, not substring)
frontpocket ingest chatgpt ./export --conversation-id 6a3a89c7-bf88-83ea-a918-334e6e1e8801

# substring match against conversation title or id
frontpocket ingest chatgpt ./export --conversation "planning"

# resolve attachment metadata (filenames/mime types) without spending on
# any vision captioning API calls
frontpocket ingest chatgpt ./export --no-caption

# resume an interrupted import from a progress journal
frontpocket ingest chatgpt ./export --resume data/progress.json
```

Attachments/assets are resolved against the export's `conversation_asset_file_names.json`
and `library_files.json` mapping files, then embedded as real content:

- Images get a real vision-model caption (see `VISION_MODEL` in `docs/providers.md`)
  as their stored/searchable text.
- Non-image attachments (PDF, audio, video, pasted text/markdown) get a
  metadata-only description with no vision API call.
- Unresolvable references fall back to a placeholder stub rather than
  blocking the import.
- `--dry-run` reports resolved/unresolved counts and how many attachments
  would trigger a real vision call — always check this before a full run,
  since captioning has real API cost at scale (a full ~1,700-image corpus
  is a meaningfully sized bill, not a rounding error).
- `is_starred`, `shared_conversations.json`, and `message_feedback.json`
  signals (if present in the export) are captured as `user_starred`,
  `user_shared`, `share_id`, `feedback_rating`, `feedback_note`, and
  `feedback_at` on each point — also reported in the `--dry-run` summary.

Raw export `.zip` files are ignored by git (`*.zip`) by default, and so is
any folder you extract an export into (`unzipped/` and similarly-named
folders) — never commit real chat history.
If you hit an embedding JSON decode error on large imports, rebuild with `./make_all.sh` to pick up the latest embedding response handling.
If OpenRouter calls succeed but Qdrant remains empty, rebuild and rerun ingest to pick up UUID-compatible Qdrant point IDs (look for `not a valid point ID` in logs when using older binaries).
As always: rebuild with `./make_all.sh` before testing ingest changes — running
against a stale `bin/frontpocket` binary looks identical to a real bug.

## 12) Explore memory in the browser (MindDrill)

With the API running, launch the MindDrill memory explorer:

```bash
frontpocket minddrill          # or: ./bin/minddrill / go run ./cmd/minddrill
```

Open the printed URL (default <http://localhost:8089>). MindDrill serves `/config.json`
from its own origin and uses it to apply the `--api` target for all browser API calls.
Ensure the MindDrill origin is listed in `CORS_ALLOW_ORIGINS`. See `docs/minddrill.md`
for options.

Current MindDrill behavior highlights:

- Search and browse support duplicate grouping (`group duplicates`, default on) with expandable
  `×N similar` groups and a `show all` path.
- Search/browse/context-pack requests show inline loading states, disable submit buttons while in
  flight, and show friendly empty states when no memories match.
- Expanded result-card full text uses the same safe Markdown renderer as chat mode, including
  fenced code blocks with per-block copy actions.
- Corpus stats for projects and memory kinds are clickable shortcuts into browse filters.

## 13) Expose MCP tools for external agents

Run the MCP stdio bridge from the same machine as FrontPocket:

```bash
frontpocket mcp

# optional overrides
frontpocket mcp --api http://localhost:8088 \
  --api-key-header X-FrontPocket-Key \
  --api-key your-real-key
```

This exposes `frontpocket_health`, `frontpocket_search`, and
`frontpocket_context_pack` to any MCP-compatible agent runtime.

## 14) Run memory loop (dry-run)

```bash
frontpocket memory-loop --batch-size 200 --dry-run
```

## 15) Write proposed canon candidates

```bash
frontpocket memory-loop --batch-size 200 --write-candidates
```

## 16) Review queue from CLI

```bash
frontpocket memory-loop list
frontpocket memory-loop approve --id cand_xxx --reviewed-by mark
frontpocket memory-loop reject --id cand_xxx --reason "insufficient evidence" --reviewed-by mark
frontpocket memory-loop merge --id cand_xxx --target canon_abc --reviewed-by mark
```

## 17) Review queue from API

```bash
curl http://localhost:8088/memory/canon/proposed
curl -X POST http://localhost:8088/memory/canon/proposed/cand_xxx/approve \
  -H 'Content-Type: application/json' \
  -d '{"reviewed_by":"mark"}'
```

## 18) Run speaker-aware cleanup + reflection

```bash
# cleanup/normalize raw memory into fp_cleaned_memory
python -m frontpocket.memory_cleanup --batch-size 200 --write-cleaned

# reflect from cleaned memory (default source)
python fp_reflect_loop.py --limit 50

# query reflection layer (vectors hidden by default; metadata still shown)
python fp_reflect_query.py "lm studio troubleshooting" --speaker assistant --limit 5

# include full vectors only when needed
python fp_reflect_query.py "lm studio troubleshooting" --speaker assistant --limit 5 --include-vectors
```

Speaker-aware behavior in this pipeline:
- Source roles normalize to `user | assistant | system | tool | mixed | unknown`.
- Assistant technical chunks are classified `domain=technical` with `phase_applicability=not_applicable`.
- Project hints can be inferred from `source_title` when project is empty; final project assignment remains manual/approved.

## 19) Run tests

> `./make_all.sh` builds both binaries into `bin/`. Always launch `./bin/frontpocket` and
> `./bin/minddrill` from there — not a binary of the same name sitting in the repo root from
> an older build — or you may be running stale code with a current backend, which is
> confusing to debug.

```bash
go test ./...
```

A matching CI check runs the same suite on GitHub Actions for pushes and pull requests (`.github/workflows/test.yml`).
