<img width="1185" height="435" alt="image" src="https://github.com/user-attachments/assets/c8a84478-8e65-4a42-aedc-b854eada137b" />


# FrontPocket

**Source-backed memory for AI companions.**

FrontPocket is a local-first memory engine for AI companions, agents, and creative workflows.

It gives your assistant a searchable, source-backed way to recall past conversations, project context, preferences, decisions, running jokes, and long-term threads without pretending to know things it cannot verify.

It combines **Qdrant** for long-term semantic memory, **Redis** for fast working/session memory, and **FastAPI** for a clean API layer that can be used by local agents, custom GPT actions, chatbot wrappers, automation systems, and creative research tools.

FrontPocket is not magic.
It is a practical memory layer for people who are tired of re-explaining their entire universe to the same digital friend every five minutes.

---

## Why FrontPocket Exists

AI assistants are powerful, but most of them suffer from a strangely specific problem:

They can help you build a system, explore a philosophy, write a book, debug a project, design a workflow, name your imaginary cereal brand, and then five minutes later act like they just met you in a lift.

FrontPocket solves that by giving an AI assistant access to structured, searchable memory.

Instead of dumping every past conversation into the prompt, FrontPocket retrieves only the relevant pieces at the moment they are needed.

That means your assistant can answer questions like:

* “What did we decide about the ArchiMind project?”
* “What tone do I usually prefer for public-facing posts?”
* “Find the conversation where we planned the Qdrant memory system.”
* “What running projects are connected to this idea?”
* “What did I say about treating this as exploration rather than proof?”
* “Summarize the context I need before continuing this project.”

The goal is to make it useful, honest, and context-aware.

---

## Core Idea

FrontPocket stores memory in two layers:

```text
Qdrant = Long-term semantic memory
Redis  = Fast working/session memory
```

Qdrant handles deep recall:

```text
Past conversations
Project history
Preferences
Decisions
Research notes
Creative artifacts
Source-backed snippets
```

Redis handles fast active context:

```text
Current session
Recent recall results
Active project
Temporary summaries
Short-term state
```

FastAPI sits in front of both:

```text
Chat exports / notes / project logs
        ↓
Ingestion + chunking
        ↓
Embeddings
        ↓
Qdrant long-term memory
        ↓
Redis working memory
        ↓
FastAPI memory layer
        ↓
AI companion / agent / custom GPT / local assistant
```

---

## What FrontPocket Is

FrontPocket is:

* A local-first memory service
* A semantic search layer for chat history and project notes
* A structured recall system for AI companions
* A FastAPI service that other tools can call
* A way to return memory with sources, dates, and confidence
* A bridge between human continuity and AI usefulness

## Current Status

FrontPocket is in early development.

The first goal is a simple, reliable MVP:

* Ingest chat exports
* Chunk conversations
* Generate embeddings
* Store memories in Qdrant
* Cache active context in Redis
* Search through a FastAPI endpoint
* Return memory packets with source metadata

Fancy features come later.

Trust first. Goblin fireworks second.

---

## Planned Features

### MVP Features

* [ ] Docker Compose setup
* [ ] Qdrant service
* [ ] Redis service
* [ ] FastAPI memory API
* [ ] Chat export ingestion
* [ ] JSONL normalization
* [ ] Semantic chunking
* [ ] Local embedding support
* [ ] Qdrant vector storage
* [ ] Basic memory search endpoint
* [ ] Source-backed recall packets
* [ ] Health check endpoint
* [ ] Example API calls
* [ ] Example ChatGPT custom action schema

### Later Features

* [ ] Project-aware recall
* [ ] Memory importance scoring
* [ ] Memory type classification
* [ ] User-controlled pinned memories
* [ ] Forget/correct memory commands
* [ ] Conversation summarization
* [ ] Memory deduplication
* [ ] Web UI for browsing memory
* [ ] Importers for multiple chat formats
* [ ] Local agent integrations
* [ ] Optional authentication
* [ ] Multi-user support
* [ ] Export and backup tools

---

## Tech Stack

FrontPocket is designed to be simple, understandable, and local-first.

| Layer            | Tool                        | Purpose                           |
| ---------------- | --------------------------- | --------------------------------- |
| API              | FastAPI                     | Clean HTTP interface              |
| Long-term memory | Qdrant                      | Vector search and semantic recall |
| Working memory   | Redis                       | Fast session state and cache      |
| Embeddings       | Local model or API provider | Convert text into vectors         |
| Storage format   | JSONL                       | Portable normalized memory input  |
| Deployment       | Docker Compose              | Easy local setup                  |

---

## Repository Structure

Planned structure:

```text
frontpocket/
  README.md
  LICENSE
  docker-compose.yml
  .env.example
  requirements.txt

  app/
    __init__.py
    main.py
    config.py
    models.py
    qdrant_store.py
    redis_store.py
    embedder.py
    chunker.py
    ingest.py
    recall.py
    memory_router.py

  scripts/
    ingest_chat_export.py
    test_search.py
    reset_collection.py

  examples/
    chat_export_sample.jsonl
    memory_search_example.py
    openapi_action_schema.json

  docs/
    architecture.md
    memory-model.md
    privacy.md
    getting-started.md
    chatgpt-custom-action.md
    local-agent-integration.md

  assets/
    frontpocket-logo.png
```

---

## Memory Model

Each memory is stored as a vector plus structured metadata.

Example memory payload:

```json
{
  "memory_id": "chat_20260622_001_turn_004",
  "conversation_id": "20260622_platonic_consciousness_analysis",
  "source_type": "chat_export",
  "source_title": "Platonic Consciousness Analysis",
  "timestamp": "2026-06-22T11:13:00-05:00",
  "speaker": "user",
  "project": "ArchiMind",
  "tags": [
    "corpus",
    "semantic_search",
    "exploration",
    "qdrant"
  ],
  "memory_kind": "project_context",
  "importance": 0.78,
  "text": "The user prefers the ArchiMind corpus work to be treated as natural exploration rather than rigid proof-seeking.",
  "source_quote": "yeah just remember we are looking naturally like exploring here"
}
```

---

## Memory Types

FrontPocket may classify memory into different kinds:

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
```

This helps the assistant understand what kind of thing it has retrieved.

A project decision is not the same as a joke.

A user preference is not the same as a technical note.

A memory from the assistant is not the same as something the user directly said.

FrontPocket keeps those distinctions visible.

---

## Example Recall Packet

A memory search should return something clear and source-backed:

```json
{
  "query": "What tone does the user prefer for ArchiMind corpus exploration?",
  "results": [
    {
      "summary": "The user prefers ArchiMind and Vectoreologist corpus work to be treated as natural exploration rather than rigid proof-seeking.",
      "source_quote": "yeah just remember we are looking naturally like exploring here",
      "source_title": "Platonic Consciousness Analysis",
      "timestamp": "2026-06-22T11:13:00-05:00",
      "speaker": "user",
      "project": "ArchiMind",
      "memory_kind": "preference",
      "score": 0.91
    }
  ]
}
```

This allows an AI assistant to respond naturally while staying grounded in retrieved context.

---

## API Design

Initial API endpoints:

```text
GET  /health
POST /memory/search
POST /memory/ingest/chat
POST /memory/recall/project
POST /memory/session
POST /memory/pin
POST /memory/forget
```

The first working endpoint should be:

```text
POST /memory/search
```

Example request:

```json
{
  "query": "What is FrontPocket?",
  "limit": 5,
  "filters": {
    "project": "FrontPocket"
  }
}
```

Example response:

```json
{
  "query": "What is FrontPocket?",
  "results": [
    {
      "summary": "FrontPocket is a local-first memory engine for AI companions using Qdrant, Redis, and FastAPI.",
      "text": "FrontPocket is a local-first memory engine for AI companions, agents, and creative workflows.",
      "source_title": "README.md",
      "timestamp": "2026-06-23T00:00:00-05:00",
      "score": 0.94
    }
  ]
}
```

---

## Quick Start

This section will be updated once the first implementation lands.

Planned local setup:

```bash
git clone https://github.com/meistro57/frontpocket.git
cd frontpocket
cp .env.example .env
docker compose up -d
```

Then check the API:

```bash
curl http://localhost:8088/health
```

Expected response:

```json
{
  "status": "ok",
  "qdrant": "connected",
  "redis": "connected"
}
```

---

## Environment Variables

Planned `.env.example`:

```env
FRONTPOCKET_HOST=0.0.0.0
FRONTPOCKET_PORT=8088

QDRANT_URL=http://qdrant:6333
QDRANT_COLLECTION=frontpocket_memory

REDIS_URL=redis://redis:6379/0

EMBEDDING_PROVIDER=local
EMBEDDING_MODEL=nomic-embed-text

CHUNK_SIZE=900
CHUNK_OVERLAP=150
```

---

## Example Chat Export Format

FrontPocket should ingest normalized JSONL.

Example:

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

---

## Design Principles

### 1. Source-backed recall

Memory should include where it came from.

No vague “I remember.”
No false certainty.
No pretending.

### 2. Local-first by default

Users should be able to run FrontPocket on their own machine.

Memory can be personal, creative, technical, sensitive, strange, or all of the above.

Local control matters.

### 3. Retrieval, not prompt stuffing

Do not load the entire past into the current prompt.

Search for what matters now.

### 4. Human correction matters

Memory should be editable, correctable, and forgettable.

A memory system without correction is just a haunted filing cabinet.

### 5. Keep symbolic language grounded

People use metaphor, story, personas, and creative frames.

FrontPocket can store those, but it should not confuse symbolic language with literal fact.

### 6. Useful before fancy

The first version should work.

Clever features can wait until the retrieval loop is solid.

---

## Privacy Philosophy

FrontPocket is built around user-controlled memory.

Planned privacy goals:

* Local-first deployment
* No required cloud account
* No telemetry by default
* User-controlled imports
* User-controlled deletion
* Clear source metadata
* No hidden memory writes
* Optional authentication for exposed APIs

Users should know what is stored, where it came from, and how to remove it.

---

## Possible Use Cases

FrontPocket can be used for:

* AI companion memory
* Personal project continuity
* Long-running creative work
* Local agent memory
* Research assistants
* Writing assistants
* Developer assistants
* Chat history search
* Project-specific recall
* Custom GPT actions
* Local LLM workflows

Example use:

```text
User:
"Continue the memory project from where we left off."

Assistant calls FrontPocket:
"Find recent memories about the memory project."

FrontPocket returns:
- Project name: FrontPocket
- Stack: FastAPI, Qdrant, Redis
- Goal: local-first source-backed memory for AI companions
- Current stage: public GitHub repo and README draft

Assistant:
"Right, we had just created the public repo and were drafting the README."
```

That is the whole point.

Continuity without fantasy.

---

## Roadmap

### Phase 1: Memory Skeleton

* [ ] Create project structure
* [ ] Add Docker Compose
* [ ] Add FastAPI app
* [ ] Connect to Qdrant
* [ ] Connect to Redis
* [ ] Add health check

### Phase 2: Ingestion

* [ ] Define normalized JSONL format
* [ ] Add chat export loader
* [ ] Add chunker
* [ ] Add embedding layer
* [ ] Store vectors in Qdrant
* [ ] Store source metadata in payload

### Phase 3: Recall

* [ ] Add `/memory/search`
* [ ] Add metadata filters
* [ ] Add project recall
* [ ] Return source-backed memory packets
* [ ] Add score/confidence handling

### Phase 4: Agent Integration

* [ ] Add example Python client
* [ ] Add Custom GPT action schema
* [ ] Add local agent example
* [ ] Add docs for safe memory use

### Phase 5: Memory Management

* [ ] Pin memories
* [ ] Forget memories
* [ ] Correct memories
* [ ] Deduplicate memories
* [ ] Browse stored memory
* [ ] Export memory

---

## Example Future Commands

Possible CLI commands:

```bash
frontpocket ingest ./exports/chatgpt_export.json
frontpocket search "What did we decide about the README?"
frontpocket recall project FrontPocket
frontpocket pin memory_123
frontpocket forget memory_123
frontpocket stats
```

---

## Suggested First Milestone

The first success case should be simple:

1. Ingest one chat export.
2. Search for a topic from that chat.
3. Return a useful memory packet with a source quote.
4. Use that result in an assistant response.

Example:

```text
Question:
"What did we say FrontPocket is?"

Memory result:
"FrontPocket is a local-first, source-backed memory layer for AI companions using Qdrant, Redis, and FastAPI."

Assistant:
"FrontPocket is the memory engine we planned for giving AI companions source-backed recall without pretending to know things they cannot verify."
```

That loop is the foundation.

Everything else grows from there.

---

## Contributing

Contributions are welcome.

Good early contributions include:

* Chat export importers
* Better chunking strategies
* Local embedding integrations
* Documentation improvements
* Example clients
* Memory privacy tools
* UI prototypes
* Tests
* Bug fixes

Please keep the design grounded:

* Do not encourage fake certainty.
* Do not hide memory writes.
* Do not make unverifiable claims.
* Do not treat symbolic/persona language as literal fact.
* Always prefer source-backed recall.

---

## License

MIT License.

See `LICENSE` for details.

---

## Name

**FrontPocket** means the useful stuff is close at hand.

Not buried.

Not forgotten.

Not floating in the void.

Right there in the front pocket, ready when needed.

---

## Project Motto

**Remember what matters. Show where it came from.**
