# FrontPocket — Phase 2 Codex Task Prompt: Image/Attachment Resolution

Paste this whole file as your task brief. Read it fully before writing any
code. **Read `AGENTS.md` in the repo root first** — it is the authoritative
style/design guide and overrides your own defaults.

## Context

Phase 1 (starred/shared/feedback metadata) is done and verified — confirmed
directly against real Qdrant data (`feedback_rating: "thumbs_down"`,
`feedback_note: "totally lost personality"` landed correctly on real points).
That task's one real failure mode was **testing against a stale, un-rebuilt
binary** — the source code was correct the whole time, but the compiled
`bin/frontpocket` wasn't rebuilt before the test ingest ran. Do not repeat
this: **always run `./make_all.sh` (or `go build -o bin/frontpocket
./cmd/frontpocket`) immediately before any test ingest, and verify with a
real Qdrant query afterward, not just a clean exit code.**

## What's actually in the export (confirmed by direct inspection)

The unzipped ChatGPT export (`~/frontpocket/unzipped/`) has **two separate,
incompatible attachment systems**, each needing its own resolver:

1. **Inline chat uploads** — reference IDs like `file-15GcoL75t6eDcSQs6aLGnE`
   (dash-prefixed). Resolved via `unzipped/conversation_asset_file_names.json`,
   a flat map from reference ID → original filename with extension (e.g.
   `.png`, `.jpg`, `.dng`). The actual bytes live at
   `unzipped/file-15GcoL75t6eDcSQs6aLGnE.dat`.
2. **ChatGPT "Library" persistent files** — reference IDs like
   `file_000000008bd4720cb1f09007b2a83cf1` (underscore-prefixed, longer hex).
   Resolved via `unzipped/library_files.json`, which has richer metadata per
   entry: `file_extension`, `mime_type`, `library_file_category`,
   `origination_conversation_id`, `origination_message_id`,
   `origination_thread_id`. Bytes live the same way, as `.dat` files in
   `unzipped/`.

The current importer (`internal/memory/chatgpt_import.go`) already detects
attachment references (`extractAttachmentRefs`, `detectAttachments`) but
throws them away as a text stub (`"attachment_refs: file-abc123"` or
`"attachment content: image_asset_pointer"`). That stub is the only thing
currently embedded for any attachment-bearing message — no image content,
no real filename, no caption.

## Mission

1. Resolve attachment references against **both** mapping files.
2. Locate the actual bytes on disk.
3. For image mime types, generate a real caption/description via a
   vision-capable model call.
4. Replace the current placeholder stub text with the real caption (so it
   actually gets embedded and becomes semantically searchable) and store
   resolution metadata on the point.
5. For non-image or unresolvable attachments, degrade gracefully — don't
   error the whole ingest, fall back to today's stub behavior for that one
   reference.

## Step 1 — Confirm the embedding model's actual capabilities before assuming a captioning approach

Do not assume `google/gemini-embedding-2-preview` (the configured
`EMBEDDING_PROVIDER`/model in `.env`) accepts image input directly. Check
OpenRouter's model documentation for it. Default to the safer, simpler
architecture regardless of the answer: **caption via a separate
vision-capable chat model call, then embed the caption text through the
existing text-embedding pipeline** — this works no matter what the embedding
model supports, and doesn't require re-plumbing `internal/embed/` for
multimodal input. Only pursue direct multimodal embedding if captioning
proves clearly insufficient after testing.

Add a new env var, `VISION_MODEL`, for the captioning call — do not reuse
`CHAT_PROVIDER`'s configured model (`anthropic/claude-sonnet-4.6`) blindly
without confirming it's the right cost/quality tradeoff for ~1,700 images.
Suggest `google/gemini-2.5-flash` (vision-capable, distinct from the
reflection pipeline's cheaper `gemini-2.5-flash-lite`) as a reasonable
default, but flag this as a real design decision, not a given.

## Step 2 — New file: `internal/memory/assets.go`

```go
// LoadAssetFileNames reads conversation_asset_file_names.json and returns
// a map from reference ID (e.g. "file-15GcoL75t6eDcSQs6aLGnE") to the
// resolved original filename (with extension). Returns an empty map, no
// error, if the file doesn't exist (optional file, same pattern as Phase 1's
// LoadShareSignals/LoadFeedbackSignals).
func LoadAssetFileNames(rootDir string) (map[string]string, error)

// LoadLibraryFiles reads library_files.json and returns a map from
// reference ID to a LibraryFileEntry with mime type, category, and
// origination metadata. Same optional-file handling.
type LibraryFileEntry struct {
    FileExtension            string
    MimeType                 string
    Category                 string
    OriginationConversationID string
    OriginationMessageID     string
    OriginationThreadID      string
}
func LoadLibraryFiles(rootDir string) (map[string]LibraryFileEntry, error)

// ResolveAttachment checks a reference ID against both maps (dash-prefixed
// IDs typically match assetFileNames, underscore-prefixed typically match
// libraryFiles, but check both regardless rather than branching on prefix
// shape, since that's an inference, not a guarantee) and returns the
// resolved filename, mime type, category, and full path to the .dat file on
// disk. Returns (nil, false) if unresolvable in either map, or if the .dat
// file doesn't actually exist at the expected path (broken cross-reference,
// account for this — don't assume every referenced ID has a file on disk).
func ResolveAttachment(ref string, rootDir string, assetNames map[string]string, libraryFiles map[string]LibraryFileEntry) (*ResolvedAttachment, bool)

type ResolvedAttachment struct {
    Filename    string
    MimeType    string
    Category    string // from library_files.json, empty for asset_file_names-resolved refs
    DiskPath    string
    SourceSystem string // "asset_file_names" | "library_files" | "unresolved"
}
```

## Step 3 — New file: `internal/memory/caption.go`

```go
// CaptionImage sends image bytes to VISION_MODEL via OpenRouter and returns
// a text description suitable for embedding. Must handle non-image mime
// types by returning early with no API call — only caption when MimeType
// starts with "image/". For non-image attachments (PDFs, audio, etc.),
// return a plain metadata-only description ("PDF attachment: filename.pdf")
// without a vision call.
func CaptionImage(attachment ResolvedAttachment) (string, error)
```

Rate-limit / batch this sensibly — ~1,700 images means real API cost and
real wall-clock time. Add a `--dry-run` count of how many images would
actually trigger a captioning call (i.e. `MimeType` starts with `image/`)
before committing to a real run, so the cost is knowable upfront, the same
way Phase 1's dry-run printed starred/shared/feedback counts.

## Step 4 — Wire into `chatgpt_import.go`

Where `buildAttachmentText` currently produces the placeholder stub, call
`ResolveAttachment` first. If resolved:
- For images: call `CaptionImage`, use the returned caption as the actual
  chunked/embedded text for that point (replacing the stub).
- For non-images: use the metadata-only description as the text.
- Store `attachment_filename`, `attachment_mime_type`,
  `attachment_category`, `attachment_source_system` as new fields on
  `MemoryPoint` (same pattern as Phase 1's six new fields — add to
  `models.go`, thread through `ingest.go`, include in
  `internal/store/qdrant.go`'s `toQdrantPayload`).
- If unresolved: fall back to exactly today's placeholder behavior — do not
  regress existing behavior for references that can't be resolved.

## Step 5 — Verification (do not skip, do not trust exit code alone)

```bash
cd ~/frontpocket
go build ./...
go test ./tests/... -v

./make_all.sh   # REQUIRED — do not test against a stale binary, see Context above

# Dry run: confirm counts before spending on real vision calls
./bin/frontpocket ingest chatgpt ./unzipped --dry-run --project FrontPocket
# Should print: total attachments detected, resolved via asset_file_names,
# resolved via library_files, unresolved, and how many would trigger an
# actual image captioning API call.

# Small real test — a handful of conversations only, into a FRESH test
# collection distinct from Phase 1's (don't reuse frontpocket_memory_phase1_test)
QDRANT_COLLECTION=frontpocket_memory_phase2_test ./bin/frontpocket ingest chatgpt \
  ./unzipped --project FrontPocket --limit 50
```

Then verify directly against Qdrant — pull a handful of points that have
`attachment_source_system` set and confirm `text` actually contains a real
caption, not a placeholder stub, and that `attachment_filename` is a real
filename with a real extension:

```bash
curl -s -X POST http://localhost:6333/collections/frontpocket_memory_phase2_test/points/scroll \
  -H 'Content-Type: application/json' \
  -d '{"limit": 5, "filter": {"must": [{"key": "attachment_source_system", "match": {"any": ["asset_file_names", "library_files"]}}]}}'
```

Report back the actual JSON from that query, not just "done."

## Explicit non-goals

- No direct multimodal embedding unless captioning is proven insufficient
  first (see Step 1).
- No touching `frontpocket_memory` or `fp_reflections` (production).
- No touching Phase 1's fields or `frontpocket_memory_phase1_test`.
- No changes to `fp_reflect_loop.py` / `memory_cleanup.py`.
- No full-corpus run (~1,700 images) without an explicit go-ahead after the
  dry-run cost/count numbers are reviewed — this task's definition of done
  is a working, verified small-scale test, not a completed full ingest.

## Definition of done

- `go build ./...` and `go test ./...` pass clean on a freshly rebuilt binary.
- Dry-run prints accurate counts: total attachments, resolved via each
  system, unresolved, and estimated vision-API-call count.
- A 50-conversation test ingest into `frontpocket_memory_phase2_test` shows
  real captions (not stubs) and correct `attachment_*` metadata on
  spot-checked points, confirmed via the curl query above with actual JSON
  reported back.
- Non-image attachments get metadata-only descriptions without triggering
  vision calls (verify this explicitly — don't just assume the mime-type
  gate works).
