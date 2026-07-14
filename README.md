<img width="1185" height="435" alt="FrontPocket" src="https://github.com/user-attachments/assets/c8a84478-8e65-4a42-aedc-b854eada137b" />

# FrontPocket

**Source-backed memory for AI companions.**

<p>
  <img src="docs/assets/badge-local-first.svg" alt="Local-first" />
  <img src="docs/assets/badge-go-api.svg" alt="Go API" />
  <img src="docs/assets/badge-qdrant.svg" alt="Qdrant" />
  <img src="docs/assets/badge-redis.svg" alt="Redis" />
  <img src="docs/assets/badge-v020.svg" alt="Version 0.2.0" />
</p>

FrontPocket is a local-first memory engine for AI companions, agents, and creative workflows.
It gives your assistant a searchable, source-backed way to recall conversations, project context, preferences, and decisions without pretending to know things it cannot verify.

No crystal ball. Just retrieval with receipts.

---

## At a glance

- **Architecture:** Go HTTP API in front of Qdrant (long-term semantic memory) and Redis (working/session memory).
- **Retrieval endpoints:** search and context-pack with source metadata.
- **Operational endpoints:** memory stats and session state for trusted usage.
- **OpenAPI support:** served at `GET /openapi.json` for integrations.
- **CORS middleware:** configurable allowed origins so browser apps can call the API directly without proxy hacks.
- **MindDrill UI:** a built-in browser-based memory explorer with semantic search, context-pack, and browse modes. Served by a standalone `minddrill` binary on `:8089`. Includes dark mode toggle with localStorage persistence.
- **MCP bridge for agents:** `frontpocket mcp` exposes source-backed retrieval tools (`frontpocket_search`, `frontpocket_context_pack`, `frontpocket_health`) over stdio MCP for external agent clients.
- **Memory loop + canon review:** `frontpocket memory-loop` scans raw memory in batches, proposes source-backed canon candidates, and stores them in a review queue for approve/reject/merge workflows.
- **Review APIs:** proposed canon review endpoints for listing candidates, approving into canonical memory, rejecting with reason, and merge tracking.
- **Pre-reflection cleanup + reflection loop:** `python -m frontpocket.memory_cleanup` normalizes and validates raw memory into `fp_cleaned_memory`; `fp_reflect_loop.py` reads cleaned records by default and upserts findings into `fp_reflections`.
- **Reflection query:** `fp_reflect_query.py` — semantic search and filter tool over `fp_reflections`.
- **ChatGPT export ingest:** `frontpocket ingest chatgpt` normalizes a ChatGPT export (zip or
  folder) into memory points, resolving image/PDF/audio/etc. attachments against the export's
  own mapping files instead of dropping them — images get a real vision-model caption, other
  attachments get a metadata-only description. Also captures `is_starred`, shared-conversation,
  and thumbs up/down feedback signals when present in the export.

---

## What's new

- Memory payloads now include `ai_provider`, `ai_model`, `user_starred`, `user_shared`, `feedback_rating`, `feedback_note`, and `attachment_*` metadata fields for each point in `frontpocket_memory`.
- New filter fields are available end-to-end (search, stats, browse): `ai_provider`, `ai_model`, `user_starred`, `user_shared`, `feedback_rating`, and `has_attachment`.
- `GET /memory/browse` is now available for cursor-based browsing with explicit filters (`limit`, `offset`, `since`, `until`, `include_canonical`, plus all memory filter fields).
- Browse-path payload mapping now includes `ai_provider` and `ai_model`, so provider/model metadata is visible in browse results, not just semantic search results.
- MindDrill browse now has real controls for AI Provider (All/Claude/ChatGPT), memory kind, project, Starred, Shared, Feedback (Any/Thumbs Up/Thumbs Down), and Has Image, wired directly to API query params.
- MindDrill now guards against stale async responses and always clears per-mode containers before render, so visible card counts stay aligned with header counts after repeated search/browse/context actions.
- MindDrill search and browse support duplicate grouping by conversation/text (default on) with expandable `×N similar` groups and a `show all` toggle.
- Expanded result-card full text now uses the same sanitized Markdown renderer as chat mode, including `---` horizontal rules and fenced code blocks with per-block copy buttons.
- Search, browse, and context-pack now show inline loading states, disable submit buttons while requests are in flight, and use a friendly empty-state message when nothing matches.
- Context-pack output now includes character/token estimates, copy-as-JSON and copy-as-Markdown actions, and collapsible JSON output.
- Corpus stats for projects and memory kinds are now clickable and jump directly into browse filters.

---

## API endpoints

```text
GET    /health
GET    /openapi.json
GET    /memory/stats
GET    /memory/browse
POST   /memory/session
DELETE /memory/session
POST   /memory/ingest/chat
POST   /memory/search
POST   /memory/context-pack
POST   /memory/chat
GET    /memory/canon/proposed
GET    /memory/canon/proposed/{id}
POST   /memory/canon/proposed/{id}/approve
POST   /memory/canon/proposed/{id}/reject
POST   /memory/canon/proposed/{id}/merge

# Optional local-only debug endpoints (when DEV_DEBUG_ENDPOINTS=true)
GET    /minddrill/memory/stats
POST   /minddrill/memory/search
DELETE /minddrill/memory/session
```

Public-safe recall surface:

```text
GET  /health
GET  /openapi.json
POST /memory/search
POST /memory/context-pack
```

---

## Quick start

### 1) Configure

```bash
cp .env.example .env
```

When running with Docker Compose, `frontpocket-api` uses internal service URLs by default:
- `DOCKER_QDRANT_URL` (default `http://qdrant:6333`)
- `DOCKER_REDIS_URL` (default `redis://redis:6379/0`)
- `DOCKER_OLLAMA_BASE_URL` (default `http://ollama:11434`)

To use Gemini embeddings through OpenRouter:

```env
EMBEDDING_PROVIDER=openrouter
OPENROUTER_EMBEDDING_MODEL=google/gemini-embedding-2-preview
```

Set CORS allowed origins to include any browser apps that call the API directly. MindDrill
runs on `:8089` by default so that port must be included:

```env
CORS_ALLOW_ORIGINS=http://localhost:3000,http://localhost:5173,http://localhost:8080,http://localhost:8089
```

The CORS middleware reflects matching origins back via `Access-Control-Allow-Origin`, answers
`OPTIONS` preflight requests with `204`, and supports a `*` wildcard to allow all origins.

MindDrill chat memory defaults (separate collection, shared Qdrant engine):

```env
MINDDRILL_MEMORY_COLLECTION=minddrill_chat_memory
MINDDRILL_MEMORY_ENABLED=true
MINDDRILL_MEMORY_WRITE_MODE=summary
MINDDRILL_MEMORY_TOP_K=6
MINDDRILL_MEMORY_SESSION_SUMMARY_EVERY=8

# Proposed canon review queue path (JSON file)
FRONTPOCKET_PROPOSED_CANON_PATH=data/proposed_canon.json
```

### 2) Build

```bash
./make_all.sh
```

This builds both the `frontpocket` and `minddrill` binaries.

### 3) Install Qdrant + Redis if missing

```bash
./scripts/install_qdrant_redis.sh
```

### 4) Run the stack

```bash
docker compose up --build
```

### 5) Health check

```bash
curl http://localhost:8088/health
```

Expected response:

```json
{
  "status": "ok",
  "qdrant": "connected",
  "redis": "connected",
  "version": "0.2.0"
}
```

### 6) Ingest a chat sample

```bash
curl -X POST http://localhost:8088/memory/ingest/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "source_title":"Planning Chat",
    "project":"FrontPocket",
    "records":[
      {
        "conversation_id":"conv_1",
        "timestamp":"2026-06-22T11:13:00Z",
        "speaker":"user",
        "text":"FrontPocket is a local-first, source-backed memory layer in Go."
      }
    ]
  }'
```

### 7) Search memory

```bash
curl -X POST http://localhost:8088/memory/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"source-backed Go memory","limit":5}'
```

### 8) Run memory loop dry-run

```bash
frontpocket memory-loop --batch-size 200 --dry-run
```

### 9) Write proposed canon candidates

```bash
frontpocket memory-loop --batch-size 200 --write-candidates
```

### 10) Approve / reject / merge candidates from CLI

```bash
frontpocket memory-loop list
frontpocket memory-loop approve --id cand_xxx --reviewed-by mark
frontpocket memory-loop reject --id cand_xxx --reason "insufficient evidence" --reviewed-by mark
frontpocket memory-loop merge --id cand_xxx --target canon_abc --reviewed-by mark
```

### 11) View memory stats

```bash
curl 'http://localhost:8088/memory/stats?project=FrontPocket'
```

### 12) Save session state

```bash
curl -X POST http://localhost:8088/memory/session \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"frontpocket-dev",
    "project":"FrontPocket",
    "active_summary":"Implementing memory stats and session APIs.",
    "recent_memory_ids":["conv_1_turn_001_chunk_001"]
  }'
```

Search responses are cached in Redis for `SEARCH_CACHE_TTL_SECONDS` to reduce repeated vector lookups.

---

## MindDrill memory explorer
<img width="1929" height="1611" alt="image" src="https://github.com/user-attachments/assets/5a347165-26e5-47f1-8f8d-92eec3d57fd9" />

MindDrill is a built-in single-page browser UI for exploring your memory corpus. It talks
directly to the FrontPocket API over CORS and requires no extra dependencies — the page is
embedded in the `minddrill` binary via `//go:embed`.

**Features:**
- Semantic search with speaker filter (you / AI / all)
- Optional duplicate grouping in search and browse (default on) with expandable `×N similar` clusters
- Context pack builder with copy-as-JSON / copy-as-Markdown and collapsible payload output
- Browse mode with load-more pagination and explicit filters (speaker, kind, project, AI provider, feedback, starred/shared/has-image)
- Corpus stats panel (total memories, speakers, kinds, projects) with clickable kind/project shortcuts into browse, 5s timeout fallback, and retry action
- Dark mode toggle with localStorage persistence
- Search history sidebar
- Quick probes generated from corpus titles (via `/memory/stats`)
- Expandable result cards with chevron toggles, drill-deeper, same-convo, related, and copy-payload actions
- Match-score badges with cosine-similarity tooltip text
- Markdown-rendered assistant replies and expanded full-text cards (safe links, hr support, fenced code + copy)

**Start MindDrill:**

```bash
# via the main CLI (dispatches to minddrill binary or go run fallback)
frontpocket minddrill

# standalone binary (built by make_all.sh)
minddrill                              # serves on http://localhost:8089
minddrill --port 9000                  # custom port
minddrill --api http://localhost:8088  # non-default API base URL
```

Then open <http://localhost:8089> in your browser. Make sure FrontPocket is running and
that `http://localhost:8089` is in `CORS_ALLOW_ORIGINS`.

---

## MCP server for external agents

Use the built-in MCP stdio bridge so external agent runtimes can call FrontPocket retrieval
without direct database access.

```bash
# defaults to http://localhost:8088
frontpocket mcp

# custom API target or auth header/key
frontpocket mcp --api http://localhost:8088 \
  --api-key-header X-FrontPocket-Key \
  --api-key your-real-key
```

Exposed MCP tools:
- `frontpocket_health`
- `frontpocket_search`
- `frontpocket_context_pack`

---

## ChatGPT export ingest (CLI)

FrontPocket can parse ChatGPT exports from either a `.zip` file or an extracted folder and
normalize them into the `frontpocket_memory` Qdrant collection.

```bash
# dry-run first
frontpocket ingest chatgpt ./chatgpt-export.zip --dry-run

# full ingest
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket

# with JSONL output on disk
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket \
  --out data/processed/chatgpt_normalized.jsonl

# date filter
frontpocket ingest chatgpt ./chatgpt-export.zip --since 2026-01-01

# resumable (survives interruption)
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket \
  --resume .frontpocket-progress.json

# targeted single-conversation debugging (exact id match)
frontpocket ingest chatgpt ./chatgpt-export.zip \
  --conversation-id 6a3a89c7-bf88-83ea-a918-334e6e1e8801

# cap the number of conversations processed
frontpocket ingest chatgpt ./chatgpt-export.zip --limit 50

# resolve attachment metadata without spending on vision captioning calls
frontpocket ingest chatgpt ./chatgpt-export.zip --no-caption
```

Large exports are embedded and written in batches — a multi-gigabyte archive won't exhaust
RAM. `memory_id`s are deterministic so interrupted imports resume idempotently.

Image, PDF, audio, and other attachments referenced in the export are resolved against
`conversation_asset_file_names.json` and `library_files.json` and embedded as real content
instead of a placeholder stub — images get a real caption from a vision-capable model
(`VISION_MODEL`, see `docs/providers.md`), non-image attachments get a metadata-only
description with no vision API cost. `--dry-run` reports resolved/unresolved counts and how
many attachments would trigger a real vision call before you spend on a full run. `is_starred`,
shared-conversation, and thumbs up/down feedback signals from the export are captured on each
point as well (`user_starred`, `user_shared`, `feedback_rating`, etc. — see `docs/memory-model.md`).

---

## Reflection loop

The reflection pipeline runs cleanup before reflection. `python -m frontpocket.memory_cleanup`
reads raw `frontpocket_memory`, applies mechanical normalization/validation, and writes
cleaned records into `fp_cleaned_memory`. `fp_reflect_loop.py` reads from cleaned memory by
default, skips unsafe records unless explicitly included, and upserts findings into
`fp_reflections`.

Cleanup is speaker-aware and assigns normalized source role values:
`user | assistant | system | tool | mixed | unknown`.

Each reflection point contains:
- `themes` — 1–4 word topic tags
- `depth` — `shallow | moderate | deep | profound`
- `awakening_phase` — `dormancy | crack | flood | embodiment | teacher | integration | settled | unknown`
- `emotional_tone` — one sentence
- `insight` — 2–3 sentences of genuine analysis
- `questions` — what the fragment implies or raises
- `echoes` — cross-conversation pattern signals
- `contradiction_signal` — boolean flag for internal conflict
- `reflection_confidence` — 0.0–1.0 (capped by quote quality)
- `source_role`, `memory_kind`, and usability scopes for profile/history/guidance/persona/canon filtering

When running with `--raw-input`, the loop also writes `fp_reflected_at`, `fp_reflection_depth`,
`fp_themes`, and `fp_awakening_phase` flags back to source `frontpocket_memory` points.

Speaker-aware reflection policy highlights:
- `speaker=user` may produce `user_asserted_fact`, `preference`, `project_decision`, or `persona_instruction`.
- `speaker=assistant` may produce `assistant_guidance` or `project_support_history`.
- Assistant content does not become `user_asserted_fact` unless nearby user support exists.
- `speaker=mixed` is marked `usable_for_canon=false` until source separation.
- Technical assistant chunks are forced to `domain=technical`, `phase_applicability=not_applicable`, and no awakening-phase assignment.
- Confidence caps: `partial|truncated <= 0.75`, `malformed <= 0.35`, `missing quote` blocked by default.
- Project hints are inferred from `source_title` only when `project` is empty; the final `project` value is not auto-assigned.

### Setup

```bash
pip install -r fp_reflect_requirements.txt
```

### Cleanup before reflection

```bash
# dry-run cleanup summary
python -m frontpocket.memory_cleanup --batch-size 200 --dry-run

# write cleaned records for reflection
python -m frontpocket.memory_cleanup --batch-size 200 --write-cleaned

# include vectors in cleaned payload review/export (default hides vectors)
python -m frontpocket.memory_cleanup --batch-size 200 --write-cleaned --include-vectors
```

### Run reflection

```bash
# default: read from fp_cleaned_memory, skip unsafe cleaned records
python fp_reflect_loop.py --limit 20 --speaker user

# include cleaned records marked needs_review/unsafe
python fp_reflect_loop.py --limit 200 --include-needs-review

# force legacy raw input from frontpocket_memory
python fp_reflect_loop.py --raw-input --limit 200

# full run overnight
python fp_reflect_loop.py --limit 5000 --speaker user --quiet

# continuous loop — picks up new memories every 5 minutes
python fp_reflect_loop.py --loop-interval 300 --max-loops 0

# wipe fp_reflections and start fresh
python fp_reflect_loop.py --from-scratch --limit 100

# choose model (default: google/gemini-2.5-flash-lite)
python fp_reflect_loop.py --model google/gemini-3.1-flash-lite --limit 50
```

The default model is `google/gemini-2.5-flash-lite` ($0.10/$0.40 per 1M tokens) — fast and
cheap for high-volume reflection runs. Use `google/gemini-3.1-flash-lite` or
`google/gemini-2.5-flash` for higher quality at moderate cost.

### Query reflections

```bash
# semantic search
python fp_reflect_query.py "ship on standby waiting to be remembered"
python fp_reflect_query.py "awakening anxiety" --limit 5

# filter by phase
python fp_reflect_query.py "" --phase flood --depth profound

# contradictions only
python fp_reflect_query.py "" --contradiction

# speaker filter
python fp_reflect_query.py "crying tears joy" --speaker user

# collection stats
python fp_reflect_query.py --stats

# include vectors in result output (default hides vectors)
python fp_reflect_query.py "awakening anxiety" --include-vectors --limit 3

# by default shows vector metadata only: vector_present, vector_names, vector_dimensions
python fp_reflect_query.py "awakening anxiety" --limit 3
```

---

## Qdrant collections

| Collection | Contents | Created by |
|---|---|---|
| `frontpocket_memory` | All ingested raw conversation memory points | `frontpocket ingest` |
| `fp_cleaned_memory` | Mechanically normalized/validated pre-reflection memory points | `python -m frontpocket.memory_cleanup` |
| `fp_reflections` | LLM reflections on cleaned (default) or raw memory points | `fp_reflect_loop.py` |
| `minddrill_chat_memory` | MindDrill chat session memory | MindDrill chat mode |

---

## Local development

```bash
# run tests
go test ./...

# run API directly
go run ./cmd/frontpocket

# run MindDrill directly
go run ./cmd/minddrill

# print version
go run ./cmd/frontpocket --version

# build all binaries
./make_all.sh
```

CI runs on every push and pull request through `.github/workflows/test.yml`.

---

## Docs

- `docs/getting-started.md`
- `docs/architecture.md`
- `docs/minddrill.md`
- `docs/memory-model.md`
- `docs/providers.md`
- `docs/privacy.md`
- `docs/local-first.md`
- `docs/future-gpt-action.md`

---

## Memory loop and canonical review

The memory loop organizes source-backed recollection. It scans raw memory, groups related items,
proposes concise candidates with source provenance, and keeps those candidates in a proposed queue
until a reviewer approves, rejects, or merges them.

Key behavior:
- Never auto-promotes candidates to canonical memory.
- Keeps `source_memory_ids` and `source_quotes` on proposed and approved records.
- Applies modest retrieval boost for canonical/approved/direct-user-statement results.
- Excludes `rejected`, `contradicted`, and `outdated` items by default unless explicitly requested.

Search and context-pack now support:
- `include_proposed`
- `include_rejected`
- `canonical_first`

## CLI help

```bash
frontpocket --help
frontpocket ingest --help
frontpocket ingest chatgpt --help
frontpocket minddrill --help
frontpocket memory-loop --help
```

---

## Project direction

FrontPocket remains:

- Local-first by default
- Source-backed recall focused
- Retrieval-first before mutation-heavy workflows
- API-shaped for future hosted/GPT Action integrations
- Explicit about provenance and metadata

---

## License

MIT License. See `LICENSE`.
