# MindDrill Roadmap

MindDrill is FrontPocket's local memory explorer and chat surface. Its job is to make retrieval visible, source-backed, and user-controlled rather than spooky. This roadmap keeps the UI boring to run, honest about provenance, and safe for local-first use.

## Current delivery — DeepDrill provenance tracing

Delivered in this branch:

- Added `minddrill deepdrill provenance <thought_id>` with optional `--source <source_id>`, `--session`, `--collection`, `--limit`, and `--json`.
- Added provenance report sections for source type, upstream/downstream/related edges, chronology signals, weaknesses, and confidence.
- Added scheduler routing so `PROVENANCE_GAP`, `CHRONOLOGY_GAP`, and `SOURCE_QUALITY` can auto-select `PROVENANCE_TRACE`.

## Sprint 1 — Trust and Control

Status: completed in this branch.

Goals:

- Make chat session deletion available through a normal, non-debug API route.
- Improve browser error handling so structured API errors are shown instead of plain HTTP numbers.
- Show richer provenance for memories used by chat replies.
- Persist search history locally and provide an explicit clear action.
- Keep debug-only endpoints out of the default UI path.

Delivered:

- `DELETE /memory/chat/session?session_id=...` clears MindDrill chat session state and MindDrill chat memory without requiring debug endpoints.
- MindDrill chat, search, context-pack, and browse calls use a shared JSON fetch helper that surfaces structured error codes, messages, and details.
- Chat evidence now reuses the richer memory result cards, including score, source metadata, speaker, project, memory kind, model, copy, related, same-conversation, and drill-deeper actions.
- Search history is stored in browser local storage and can be cleared from the sidebar.

## Sprint 2 — Better Exploration

Goals:

- Replace the hardcoded browse query with real browse controls.
- Add first-class filters for project, memory kind, source type, speaker, tags, date range, AI provider/model, starred/shared state, feedback rating, and attachment presence.
- Add memory lens presets such as decisions, preferences, corrections, high-importance memories, source-backed only, and user-authored only.
- Add a memory inspector drawer for full payload review and copy/export actions.

Delivered in this branch:

- `/memory/browse` now supports filter query params for project, memory kind, source type, speaker, tags, date bounds, ai provider/model, starred/shared state, feedback rating, and attachment presence.
- MindDrill browse now uses explicit filter controls (speaker, memory kind, project, AI provider, starred/shared state, feedback, has image) instead of random seeded similarity search.
- Search and browse now support duplicate grouping (default on) with expandable `×N similar` clusters and a `show all` path.
- Search/browse/context-pack now use inline loading indicators, disable submit buttons while requests are active, and show friendly empty-state feedback.
- Context-pack output now has character/token estimates, copy-as-JSON / copy-as-Markdown actions, and collapsible JSON display.
- Result cards now use chevron expand/collapse controls and tooltip text on score badges.
- Expanded search/browse full text now uses the same sanitized Markdown renderer as chat mode, including fenced code block copy actions.
- Rendering now clears per-mode containers and ignores stale async responses to prevent duplicate/stale card accumulation.

Definition of done:

- Browse can be used without project-specific baked-in prompts.
- Filter controls map directly to explicit API filters.
- The inspector shows raw text, source quote, summary, timestamps, embedding metadata, tags, and used-memory IDs.

## Sprint 3 — Better Memory Writes

Goals:

- Split chat actions into send only, remember this, and do not store.
- Add a preview before explicit memory writes.
- Add editable memory kind, importance, tags, and project fields for explicit memories.
- Add optional summariser-assisted memory creation while keeping retrieval-only mode as the default.

Definition of done:

- Users can see and edit what MindDrill intends to remember before it is stored.
- Generated summaries are labelled as generated, not raw user statements.
- Basic chat and search still work without a cloud provider or chat model.

## Sprint 4 — Maintainability and Accessibility

Goals:

- Split the embedded UI into `index.html`, `static/minddrill.css`, and `static/minddrill.js` while keeping Go embedding and no required frontend build step.
- Replace inline handlers with registered JavaScript event listeners.
- Improve keyboard navigation, focus states, ARIA labels, and live status/error regions.
- Add tests for embedded assets, route usage, and capability-aware UI behaviour.

Definition of done:

- MindDrill remains a single local binary experience.
- UI files are easier to review and test.
- Keyboard-heavy use feels deliberate rather than bolted on with chewing gum.
