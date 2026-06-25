# AGENTS.md

Guidance for AI coding agents working on **FrontPocket**.

FrontPocket is a local-first, source-backed memory engine for AI companions, agents, and creative workflows.

It is designed to give AI systems useful continuity without pretending they magically remember things.

The system should help an assistant say:

> “Here is what I found in memory, and here is where it came from.”

Not:

> “I just know.”

Source-backed recall is the lantern. Do not lose it in the code cave.

---

## Current Direction

FrontPocket is now planned as a **Go-first** project.

Use Go for the core application:

* HTTP API server
* CLI commands
* Qdrant integration
* Redis integration
* JSONL ingestion
* Chunking
* Embedding provider abstraction
* Memory search
* Context-pack generation
* OpenAPI-ready endpoint design

Python may be added later for optional experiments, import converters, notebooks, or helper scripts, but it should not be required for the core MVP.

---

## Core Stack

FrontPocket uses:

* **Go** for the main application
* **Qdrant** for long-term semantic memory
* **Redis** for fast working/session memory
* **Ollama** for local-first embeddings by default
* **OpenAI** as an optional embedding/chat provider
* **OpenRouter** as an optional embedding/chat provider
* **Docker Compose** for local development
* **JSONL** as the preferred normalized import format
* **.env** for configuration and provider selection

The default setup should be local-first.

Cloud providers must be optional.

---

## Project Philosophy

FrontPocket exists because AI assistants often lose continuity between conversations.

This project provides a memory layer that can retrieve relevant prior context without stuffing entire chat histories into prompts.

FrontPocket is not:

* A consciousness project
* A mystical memory claim
* A black-box personality simulator
* A cloud dependency
* A reason for an AI to fake certainty

FrontPocket is:

* A local-first memory engine
* A source-backed recall system
* A clean API for assistants and agents
* A practical continuity layer
* A tool users control

Keep the public repo grounded, useful, and honest.

---

## Core Design Rules

### 1. Local-first by default

FrontPocket should run fully on a user’s own machine or private server.

The default provider should be:

```env
EMBEDDING_PROVIDER=ollama
```

Cloud providers such as OpenAI and OpenRouter may be supported, but must remain optional.

Do not make cloud services mandatory for the MVP.

---

### 2. API-shaped from day one

Even in local mode, all access should flow through the Go HTTP API.

Correct path:

```text
Client / Agent / Chat Bridge
        ↓
FrontPocket Go HTTP API
        ↓
Qdrant + Redis
```

Avoid this:

```text
Client / Agent / Chat Bridge
        ↓
Qdrant directly
```

The API layer is the future security boundary.

This matters because FrontPocket may later be exposed behind a domain for Custom GPT Actions or other external agent tools.

---

### 3. Retrieval first, memory mutation later

The early MVP should prioritize safe read-only recall.

Build and stabilize:

* Ingestion
* Chunking
* Embedding
* Storage
* Search
* Context-pack retrieval

Do not rush automatic memory writing, deleting, rewriting, or agent-initiated mutation.

Admin features can come later once the retrieval loop is trustworthy.

---

### 4. Never fake memory

If a result was not retrieved from memory, do not present it as remembered.

All memory responses should include enough metadata to trace the source.

Useful fields:

* `memory_id`
* `conversation_id`
* `source_title`
* `source_type`
* `timestamp`
* `speaker`
* `project`
* `memory_kind`
* `source_quote`
* `summary`
* `score`
* `embedding_provider`
* `embedding_model`
* `embedding_dimensions`

The assistant should be able to distinguish between:

* What the user said
* What the assistant said
* What the system inferred
* What was generated as a summary
* What came from a project file or external note

That distinction matters.

---

### 5. Preserve user control

Users must be able to inspect, correct, pin, export, and delete memory records.

Do not implement hidden writes.

Do not silently import sensitive data.

Do not bury deletion functionality.

Memory without correction is just a haunted filing cabinet.

---

## Expected Repository Structure

Use this structure unless the project has clearly moved in another direction:

```text
frontpocket/
  README.md
  AGENTS.md
  LICENSE
  .gitignore
  .env.example
  docker-compose.yml
  Dockerfile
  go.mod
  go.sum

  cmd/
    frontpocket/
      main.go
    minddrill/
      main.go
      index.html

  internal/
    api/
      server.go
      routes_health.go
      routes_memory.go
      middleware_auth.go

    config/
      config.go

    memory/
      models.go
      recall.go
      ingest.go
      chunker.go
      context_pack.go

    store/
      qdrant.go
      redis.go

    embed/
      embedder.go
      ollama.go
      openai.go
      openrouter.go

    log/
      logger.go

  scripts/
    test_search.sh
    reset_collection.sh

  examples/
    chat_export_sample.jsonl
    search_request.json
    openapi_action_schema.yaml

  docs/
    getting-started.md
    architecture.md
    memory-model.md
    privacy.md
    local-first.md
    future-gpt-action.md
    providers.md
    minddrill.md

  tests/
    chunker_test.go
    config_test.go
    models_test.go
    recall_test.go
```

Do not create sprawling directories without a reason.

Prefer boring, understandable structure over clever architecture origami.

---

## Primary Development Goals

### MVP goal

The first working version should:

1. Start with Docker Compose.
2. Run the Go API, Qdrant, and Redis.
3. Expose `/health`.
4. Read configuration from `.env`.
5. Accept normalized JSONL chat records.
6. Chunk imported text.
7. Generate embeddings through a provider interface.
8. Store vectors and metadata in Qdrant.
9. Cache session/recent context in Redis.
10. Search memory with `/memory/search`.
11. Return source-backed recall packets.

### First success case

The first useful test should look like:

```text
Input:
"What did we decide FrontPocket is?"

Output:
A memory packet saying FrontPocket is a local-first, source-backed memory layer for AI companions using Go, Qdrant, Redis, and pluggable embedding providers, with a source quote and timestamp.
```

---

## API Guidelines

Use Go’s HTTP stack or a lightweight router.

Acceptable choices include:

* `net/http`
* `chi`
* `gin`

Prefer simple and readable over framework theatrics.

Initial endpoints:

```text
GET  /health
POST /memory/search
POST /memory/context-pack
POST /memory/ingest/chat
```

Future endpoints:

```text
POST /memory/pin
POST /memory/forget
POST /memory/correct
POST /memory/session
GET  /memory/stats
```

For the public or GPT Action version, expose only safe recall endpoints at first:

```text
GET  /health
POST /memory/search
POST /memory/context-pack
```

Keep ingest, delete, reset, and admin endpoints private or protected.

---

## OpenAPI Requirements

Keep the OpenAPI schema clean.

This matters because the future plan includes using FrontPocket as a Custom GPT Action through a public HTTPS endpoint.

Route handlers and schemas should include:

* Clear operation names
* Clear request schemas
* Clear response schemas
* Descriptions that explain what each endpoint does
* Reasonable limits on input fields

Example rules:

* `limit` should have a maximum value.
* `query` should be required for search.
* Filters should be explicit.
* Responses should include source metadata.
* Errors should be structured.
* Admin endpoints should not be exposed casually.

OpenAPI should be generated or maintained in a way that stays accurate.

Do not let `openapi_action_schema.yaml` drift into fantasy-land.

---

## Configuration Rules

Use `.env` for settings.

Do not hardcode:

* Hostnames
* Ports
* API keys
* Collection names
* Embedding providers
* Embedding models
* Chat providers
* Chunk sizes

The `.env.example` file should be self-documenting.

The default active settings should be local-first.

Unused provider choices should be included but commented out so users can see the available options.

Example principle:

```env
# Default local-first provider
EMBEDDING_PROVIDER=ollama

# To use OpenAI instead:
# EMBEDDING_PROVIDER=openai

# To use OpenRouter instead:
# EMBEDDING_PROVIDER=openrouter
```

Do not put real keys in `.env.example`.

---

## Required .env.example Coverage

The `.env.example` file should include sections for:

* FrontPocket app settings
* API security
* Qdrant settings
* Redis settings
* Embedding provider
* Ollama embedding settings
* OpenAI embedding settings
* OpenRouter embedding settings
* Optional chat/summarizer provider
* Chunking settings
* Ingestion settings
* Search settings
* Context-pack settings
* Logging
* Development settings
* Future public / GPT Action settings

The file should show all common provider choices and comment out unused alternatives.

---

## Recommended .env.example

Keep the `.env.example` roughly shaped like this:

```env
# ============================================================
# FrontPocket .env.example
# ============================================================
#
# Copy this file to:
#
#   .env
#
# Then edit the values you want to use.
#
# Do not commit your real .env file.
# Do not put real API keys in this example file.
#
# ============================================================


# ============================================================
# FRONTPOCKET APP SETTINGS
# ============================================================

# App mode options:
#   local       = local development / private machine
#   server      = private server deployment
#   public      = public HTTPS deployment behind a domain
FRONTPOCKET_MODE=local

FRONTPOCKET_HOST=0.0.0.0
FRONTPOCKET_PORT=8088

FRONTPOCKET_PUBLIC_URL=http://localhost:8088

# Example future public URL:
# FRONTPOCKET_PUBLIC_URL=https://frontpocket.example.com


# ============================================================
# API SECURITY
# ============================================================

# Options:
#   false
#   true
FRONTPOCKET_REQUIRE_API_KEY=false

FRONTPOCKET_API_KEY=change-me
FRONTPOCKET_API_KEY_HEADER=X-FrontPocket-Key


# ============================================================
# QDRANT SETTINGS
# ============================================================

# Docker Compose default:
QDRANT_URL=http://qdrant:6333

# Local machine default:
# QDRANT_URL=http://localhost:6333

QDRANT_COLLECTION=frontpocket_memory
QDRANT_VECTOR_NAME=

# Distance options:
#   Cosine
#   Dot
#   Euclid
QDRANT_DISTANCE=Cosine


# ============================================================
# REDIS SETTINGS
# ============================================================

# Docker Compose default:
REDIS_URL=redis://redis:6379/0

# Local machine default:
# REDIS_URL=redis://localhost:6379/0

REDIS_KEY_PREFIX=frontpocket


# ============================================================
# EMBEDDING PROVIDER
# ============================================================

# Options:
#   ollama      = local-first embeddings through Ollama
#   openai      = direct OpenAI embeddings
#   openrouter  = OpenRouter embeddings API
EMBEDDING_PROVIDER=ollama

EMBEDDING_DIMENSIONS=


# ============================================================
# OLLAMA EMBEDDING SETTINGS
# ============================================================

# Active when:
#   EMBEDDING_PROVIDER=ollama

# Docker Compose default:
OLLAMA_BASE_URL=http://ollama:11434

# Local machine default:
# OLLAMA_BASE_URL=http://localhost:11434

OLLAMA_EMBEDDING_MODEL=nomic-embed-text

# Example alternatives:
# OLLAMA_EMBEDDING_MODEL=mxbai-embed-large
# OLLAMA_EMBEDDING_MODEL=all-minilm


# ============================================================
# OPENAI EMBEDDING SETTINGS
# ============================================================

# Active when:
#   EMBEDDING_PROVIDER=openai

# EMBEDDING_PROVIDER=openai
OPENAI_API_KEY=
OPENAI_BASE_URL=https://api.openai.com/v1

OPENAI_EMBEDDING_MODEL=text-embedding-3-small

# Example alternatives:
# OPENAI_EMBEDDING_MODEL=text-embedding-3-large
# OPENAI_EMBEDDING_MODEL=text-embedding-ada-002


# ============================================================
# OPENROUTER EMBEDDING SETTINGS
# ============================================================

# Active when:
#   EMBEDDING_PROVIDER=openrouter

# EMBEDDING_PROVIDER=openrouter
OPENROUTER_API_KEY=
OPENROUTER_BASE_URL=https://openrouter.ai/api/v1

OPENROUTER_EMBEDDING_MODEL=openai/text-embedding-3-small

# Example alternatives:
# OPENROUTER_EMBEDDING_MODEL=openai/text-embedding-3-large
# OPENROUTER_EMBEDDING_MODEL=google/text-embedding-004
# OPENROUTER_EMBEDDING_MODEL=mistralai/mistral-embed

OPENROUTER_SITE_URL=
OPENROUTER_APP_NAME=FrontPocket


# ============================================================
# OPTIONAL CHAT / SUMMARIZER PROVIDER
# ============================================================

# FrontPocket MVP should work without a chat model.
#
# Options:
#   none
#   ollama
#   openai
#   openrouter
CHAT_PROVIDER=none

# ----------------------------
# Ollama chat settings
# ----------------------------
# CHAT_PROVIDER=ollama
OLLAMA_CHAT_MODEL=llama3.1

# Example alternatives:
# OLLAMA_CHAT_MODEL=qwen2.5
# OLLAMA_CHAT_MODEL=mistral
# OLLAMA_CHAT_MODEL=gemma2

# ----------------------------
# OpenAI chat settings
# ----------------------------
# CHAT_PROVIDER=openai
OPENAI_CHAT_MODEL=gpt-4o-mini

# Example alternatives:
# OPENAI_CHAT_MODEL=gpt-4.1-mini
# OPENAI_CHAT_MODEL=gpt-4.1

# ----------------------------
# OpenRouter chat settings
# ----------------------------
# CHAT_PROVIDER=openrouter
OPENROUTER_CHAT_MODEL=openai/gpt-4o-mini

# Example alternatives:
# OPENROUTER_CHAT_MODEL=anthropic/claude-3.5-sonnet
# OPENROUTER_CHAT_MODEL=google/gemini-flash-1.5
# OPENROUTER_CHAT_MODEL=mistralai/mistral-small


# ============================================================
# CHUNKING SETTINGS
# ============================================================

CHUNK_SIZE=900
CHUNK_OVERLAP=150
MIN_CHUNK_SIZE=120


# ============================================================
# INGESTION SETTINGS
# ============================================================

# Options:
#   chat_export
#   project_note
#   markdown
#   jsonl
#   manual
DEFAULT_SOURCE_TYPE=chat_export

STORE_ASSISTANT_MESSAGES=true
STORE_USER_MESSAGES=true
STORE_SYSTEM_MESSAGES=false

AUTO_PROJECT_TAGGING=false
AUTO_SUMMARIZE_ON_INGEST=false
AUTO_IMPORTANCE_SCORING=false


# ============================================================
# SEARCH SETTINGS
# ============================================================

SEARCH_DEFAULT_LIMIT=5
SEARCH_MAX_LIMIT=20
SEARCH_MIN_SCORE=0

INCLUDE_SOURCE_QUOTES=true
INCLUDE_FULL_TEXT=true


# ============================================================
# CONTEXT PACK SETTINGS
# ============================================================

CONTEXT_PACK_DEFAULT_LIMIT=8
CONTEXT_PACK_MAX_LIMIT=20

# Requires CHAT_PROVIDER not to be none.
CONTEXT_PACK_USE_SUMMARIZER=false


# ============================================================
# LOGGING
# ============================================================

# Options:
#   debug
#   info
#   warn
#   error
LOG_LEVEL=info

# Options:
#   text
#   json
LOG_FORMAT=text

LOG_REQUESTS=true
LOG_RESPONSES=false


# ============================================================
# DEVELOPMENT SETTINGS
# ============================================================

DEV_RELOAD=false
DEV_DEBUG_ENDPOINTS=false

CORS_ALLOW_ORIGINS=http://localhost:3000,http://localhost:5173,http://localhost:8080


# ============================================================
# FUTURE PUBLIC / GPT ACTION SETTINGS
# ============================================================

# Public domain for GPT Action use:
# FRONTPOCKET_PUBLIC_URL=https://frontpocket.example.com

FRONTPOCKET_PRIVACY_POLICY_URL=

# Public GPT Action should be read-only at first.
PUBLIC_READONLY_MODE=true

# Admin endpoints should stay private.
ENABLE_ADMIN_ENDPOINTS=false

# Future allowed public endpoints:
#   GET  /health
#   POST /memory/search
#   POST /memory/context-pack
```

Keep this file clear. Future users should be able to understand their options just by reading it.

---

## .gitignore Requirements

The repo must include `.gitignore`.

It must prevent secrets and local runtime junk from entering Git.

Required entries:

```gitignore
# Environment files
.env
.env.local
*.env
!.env.example

# OS/editor junk
.DS_Store
Thumbs.db
.vscode/
.idea/

# Go build artifacts
bin/
dist/
*.exe
*.test
*.out
coverage.out

# Logs
*.log
logs/

# Local data
data/
storage/
qdrant_storage/
redis_data/

# Temporary files
tmp/
temp/
.cache/
```

Do not commit real memory data, private chat exports, API keys, Redis dumps, or Qdrant storage directories.

The example files should use fake/sample data only.

---

## Embedding Provider Guidelines

Use a provider interface.

The rest of the app should not care whether vectors came from Ollama, OpenAI, or OpenRouter.

Expected interface:

```go
type Embedder interface {
    EmbedText(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
```

Initial providers:

```text
ollama
openai
openrouter
```

Default:

```env
EMBEDDING_PROVIDER=ollama
```

Provider selection should happen through config.

If a required API key is missing, fail clearly.

Example:

```text
OPENAI_API_KEY is required when EMBEDDING_PROVIDER=openai.
```

Example:

```text
OPENROUTER_API_KEY is required when EMBEDDING_PROVIDER=openrouter.
```

Do not silently fall back to another provider. Silent fallback makes debugging rotten.

---

## Chat / Summarizer Provider Guidelines

The MVP should work without a chat/summarizer provider.

Default:

```env
CHAT_PROVIDER=none
```

Later, chat providers may be used for:

* Memory summarization
* Memory kind classification
* Project tagging
* Context-pack summarization
* Importance scoring

Supported future options:

```text
none
ollama
openai
openrouter
```

Do not require a chat model for basic ingest and search.

---

## Embedding Dimensions and Collections

Different embedding models may use different vector sizes.

Do not mix vectors with different dimensions in the same Qdrant collection.

When ingesting, store embedding metadata:

```json
{
  "embedding_provider": "ollama",
  "embedding_model": "nomic-embed-text",
  "embedding_dimensions": 768
}
```

Before inserting into Qdrant, verify the collection vector size matches the embedding dimensions.

If it does not match, fail clearly and tell the user what happened.

Example:

```text
Qdrant collection vector size is 768, but current embedding model returned 1536 dimensions. Use a different collection or recreate the collection.
```

No quiet vector soup.

---

## Storage Guidelines

### Qdrant

Qdrant stores long-term semantic memory.

Each memory point should contain:

* Vector embedding
* Original text
* Source quote
* Summary if available
* Metadata payload
* Embedding provider metadata

Payload should support filtering by:

* `project`
* `memory_kind`
* `speaker`
* `source_type`
* `conversation_id`
* `timestamp`
* `tags`
* `embedding_provider`
* `embedding_model`

### Redis

Redis stores fast working memory.

Use Redis for:

* Current session state
* Recent recall cache
* Active project context
* Temporary summaries
* Lightweight task state

Do not use Redis as the permanent source of truth for long-term memory.

---

## Memory Payload Shape

Preferred memory payload:

```json
{
  "memory_id": "chat_20260622_001_turn_004",
  "conversation_id": "20260622_archimind",
  "source_type": "chat_export",
  "source_title": "ArchiMind Planning Chat",
  "timestamp": "2026-06-22T11:13:00-05:00",
  "speaker": "user",
  "project": "ArchiMind",
  "tags": ["corpus", "semantic_search", "exploration"],
  "memory_kind": "project_context",
  "importance": 0.78,
  "text": "The user prefers the ArchiMind corpus work to be treated as natural exploration rather than rigid proof-seeking.",
  "source_quote": "yeah just remember we are looking naturally like exploring here",
  "embedding_provider": "ollama",
  "embedding_model": "nomic-embed-text",
  "embedding_dimensions": 768
}
```

Do not discard raw source text unless there is a clear privacy or storage reason.

---

## Memory Kinds

Use these memory kinds unless a better taxonomy is intentionally added:

```text
fact
preference
project_context
decision
technical_solution
creative_artifact
running_joke
personal_context
relationship_context
research_note
system_note
```

Keep the distinction between:

* What the user said
* What the assistant said
* What the system inferred
* What the project generated

This matters for trust.

---

## Chunking Guidelines

Chunking should preserve meaning.

Avoid blindly slicing by character count if it destroys context.

A good memory chunk may include:

* User message
* Assistant response
* Nearby context
* Conversation title
* Timestamp
* Project tag
* Source reference

Use overlap when helpful, but avoid creating excessive duplicate memory points.

A reasonable starting point:

```env
CHUNK_SIZE=900
CHUNK_OVERLAP=150
MIN_CHUNK_SIZE=120
```

These values are not sacred. Adjust based on retrieval quality.

---

## Ingestion Guidelines

Ingestion should begin with normalized JSONL.

Preferred input shape:

```json
{"conversation_id":"20260622_archimind","timestamp":"2026-06-22T11:13:00-05:00","speaker":"user","text":"We are treating this corpus work as natural exploration."}
{"conversation_id":"20260622_archimind","timestamp":"2026-06-22T11:14:00-05:00","speaker":"assistant","text":"Right. We are looking for what the texts say, not claiming the text proves reality."}
```

Importers can later convert from:

* ChatGPT exports
* Claude exports
* Markdown logs
* JSON transcripts
* Project notes
* Custom agent logs

Do not include real private chat exports in the repository.

Use fake sample data only.

---

## Testing Guidelines

Add tests when adding non-trivial logic.

Prioritize tests for:

* Config loading
* Provider selection
* Chunking
* JSONL parsing
* Qdrant payload creation
* Search request validation
* Health checks
* API response shape
* Embedding dimension mismatch handling

Use Go tests.

Suggested command:

```bash
go test ./...
```

Do not write tests that require external cloud services for the default test suite.

Tests for OpenAI/OpenRouter should be mocked unless explicitly marked as integration tests.

---

## Code Style

Use clear, boring Go.

Preferred:

* Small packages
* Explicit structs
* Interfaces at sensible seams
* Context-aware functions
* Good error messages
* No unnecessary cleverness
* No hidden global state
* No silent exception swallowing
* No mystery constants

The code should be understandable by someone returning to the project after a month away.

Future Mark will thank you. Probably while holding coffee.

---

## Error Handling

Errors should be clear and structured.

Bad:

```json
{
  "error": "failed"
}
```

Better:

```json
{
  "error": {
    "code": "QDRANT_UNAVAILABLE",
    "message": "Could not connect to Qdrant.",
    "detail": "Check QDRANT_URL and Docker service status."
  }
}
```

Do not leak secrets in error messages.

Do not expose internal stack traces in production mode.

---

## Security Guidelines

Security is part of the architecture, not a sticker added later.

Rules:

1. Do not expose Redis publicly.
2. Do not expose Qdrant publicly.
3. Use the Go HTTP API as the only public boundary.
4. Add API key support early, even if disabled in local mode.
5. Keep admin endpoints separate from recall endpoints.
6. Never log API keys.
7. Never commit `.env`.
8. Never commit real memory data.
9. Use fake/sample data in tests and docs.
10. Keep future GPT Action access read-only at first.

For public deployment, require HTTPS and authentication.

---

## Privacy Guidelines

FrontPocket may store highly personal chat history.

Treat memory data carefully.

Do not add telemetry by default.

Do not silently upload user data to external services.

Do not add cloud dependencies without documenting exactly what data leaves the machine.

All importers should make it clear what is being ingested.

All deletion tools should be real deletion tools, not decorative buttons.

---

## Documentation Guidelines

Update documentation when changing behaviour.

Docs should explain:

* What the system does
* What it does not do
* How to run it locally
* How to configure `.env`
* How to choose providers
* How to ingest memory
* How to search memory
* How source-backed recall works
* How to keep data private
* How future GPT Action integration may work

Keep docs practical.

Use examples.

Avoid overclaiming.

---

## Public Repo Tone

FrontPocket should feel friendly, useful, and grounded.

Acceptable tone:

* Clear
* Practical
* Slightly playful
* Honest about limitations

Avoid:

* Hype fog
* Claims of sentience
* “Unlock infinite consciousness” wording
* Corporate sludge
* Overpromising

Good project sentence:

> FrontPocket helps AI assistants retrieve relevant memory with sources, instead of pretending to remember.

Bad project sentence:

> FrontPocket awakens persistent artificial consciousness through universal semantic resonance.

That second one gets the spray bottle.

---

## Future GPT Action Plan

FrontPocket should eventually work behind a domain name as a Custom GPT Action.

Plan for:

```text
Custom GPT
    ↓
OpenAPI schema
    ↓
https://frontpocket.example.com
    ↓
FrontPocket Go HTTP API
    ↓
Qdrant + Redis
```

Important future requirements:

* Public HTTPS endpoint
* API key authentication
* Privacy policy URL
* Clean `/openapi.json`
* Read-only recall endpoints exposed
* Admin endpoints protected or local-only

Do not design the local app in a way that blocks this future path.

---

## Docker Guidelines

Docker Compose should support local development.

Expected services:

```text
frontpocket-api
qdrant
redis
ollama
```

Ollama may be optional depending on whether the user runs it on the host machine.

For local dev, exposing Qdrant and Redis ports is acceptable.

For public deployment, only the Go API should be exposed through a reverse proxy.

Do not expose Redis or Qdrant directly to the public internet.

Ever.

That is how the trousers catch fire.

---

## Suggested Implementation Order

Follow this order unless there is a strong reason not to:

1. Create project structure.
2. Add `.gitignore`.
3. Add `.env.example`.
4. Add Docker Compose with Go API, Qdrant, Redis, and optional Ollama.
5. Add typed config loading.
6. Add `/health`.
7. Add API key middleware.
8. Add embedding provider interface.
9. Add Ollama embedder.
10. Add OpenAI embedder.
11. Add OpenRouter embedder.
12. Add Qdrant connection helper.
13. Add Redis connection helper.
14. Add normalized JSONL models.
15. Add chunker.
16. Add ingestion command.
17. Add `/memory/search`.
18. Add example data.
19. Add tests.
20. Add documentation.

Small working steps beat grand architectural fog.

---

## Definition of Done for MVP

The MVP is done when:

* `docker compose up` starts the stack.
* `/health` returns API, Qdrant, and Redis status.
* `.env.example` shows all major options with unused options commented.
* `.gitignore` protects real `.env` files and local data.
* A sample JSONL chat can be ingested.
* Embeddings are generated through the provider interface.
* Memories are stored in Qdrant with payload metadata.
* `/memory/search` returns relevant results.
* Each result includes source metadata.
* Embedding metadata is stored with each memory.
* Vector dimension mismatches fail clearly.
* The README explains how to run it.
* No real personal data is included in the repo.
* Redis and Qdrant are not required to be public.
* The OpenAPI schema is clean enough to support future GPT Action use.

---

## Agent Behavior Rules

When working on this repository:

1. Read this file before making changes.
2. Prefer small, reviewable edits.
3. Do not rewrite the whole project without being asked.
4. Do not remove privacy or source metadata features.
5. Do not replace local-first defaults with mandatory cloud services.
6. Do not expose databases directly.
7. Do not invent undocumented architecture.
8. Add or update tests for meaningful logic.
9. Update docs when behaviour changes.
10. Leave TODOs only when they are specific and useful.

---

## Naming Conventions

Use consistent naming:

* Project: `FrontPocket`
* Repo: `frontpocket`
* API service: `frontpocket-api`
* Qdrant collection: `frontpocket_memory`
* Main command: `frontpocket`
* Main package path: `cmd/frontpocket`
* Memory search endpoint: `/memory/search`
* Context endpoint: `/memory/context-pack`

Avoid renaming core concepts without updating docs.

---

## Preferred Commit Style

Suggested commit messages:

```text
Add Go HTTP health endpoint
Add Docker Compose stack
Add self-documenting env example
Add gitignore for secrets and local data
Add embedding provider interface
Add Ollama embedding provider
Add OpenAI embedding provider
Add OpenRouter embedding provider
Add Qdrant storage helper
Add JSONL chat importer
Add memory search endpoint
Document future GPT Action deployment
```

Keep commit messages plain and useful.

---

## Final Reminder

FrontPocket should remember what matters and show where it came from.

It should be local-first, provider-flexible, privacy-aware, and honest about what it retrieved.

That is the whole creature.
