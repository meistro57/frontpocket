# FrontPocket — Phase 2 Completion: Force Real Coverage of Untested Paths

## Context

Phase 2 core mechanism (image resolution via `asset_file_names`) is verified
end-to-end on real data, twice, including a dedup bug fix already confirmed
against live Qdrant output. Two paths remain unverified against real data —
implemented and unit-tested, but never actually observed working on a real
attachment:

1. **Non-image attachment handling** — the metadata-only path (no vision
   call) for PDFs, audio, video, or other non-`image/*` mime types.
2. **`library_files.json`-resolved attachments** — the second, richer
   attachment system (underscore-prefixed long-hex IDs like
   `file_000000008bd4720cb1f09007b2a83cf1`), distinct from the
   dash-prefixed `file-XXXX` IDs that `asset_file_names` resolves.

The 50-conversation test slice used so far only happened to contain one true
attachment (an image, via `asset_file_names`). Random conversation sampling
won't reliably hit the other two paths — need to target them directly.

## Task

### Step 1 — Find real examples of both untested paths before touching ingest

Read `unzipped/library_files.json` directly (593KB, real data, already on
disk) and find:
- At least one entry where `mime_type` does NOT start with `image/` (e.g.
  `application/pdf`, `video/mp4`, `audio/*`, etc.) — note its
  `origination_conversation_id`.
- Confirm at least one entry exists with a `mime_type` that DOES start with
  `image/`, resolved via `library_files.json` specifically (not
  `asset_file_names`) — note its `origination_conversation_id` too, so we
  also get a real `library_files`-path image as a bonus confirmation
  alongside the non-image case.

Report the actual `conversation_id`(s) found and their real mime types
before proceeding — don't guess or assume, pull them from the real file.

### Step 2 — Rebuild, then run a targeted ingest scoped to just those conversations

```bash
cd ~/frontpocket
./make_all.sh   # always rebuild before testing — see Phase 1's stale-binary lesson

QDRANT_COLLECTION=frontpocket_memory_phase2_test ./bin/frontpocket ingest chatgpt \
  ./unzipped --project FrontPocket --conversation-id <the-non-image-conversation-id>

QDRANT_COLLECTION=frontpocket_memory_phase2_test ./bin/frontpocket ingest chatgpt \
  ./unzipped --project FrontPocket --conversation-id <the-library-files-image-conversation-id>
```

If there's no existing `--conversation-id` flag to scope ingestion to a
single conversation, add one now (small, targeted CLI addition) rather than
re-running the full 50-conversation slice repeatedly to fish for a hit —
this also becomes a generally useful debugging tool going forward.

Append to the existing `frontpocket_memory_phase2_test` collection (don't
wipe it this time) — we want to end up with all three cases (image via
asset_file_names, non-image via library_files, image via library_files)
sitting in the same test collection together for one final combined check.

### Step 3 — Report back exact point IDs for both new cases

Same standard as every prior verification today: report actual point IDs
and actual field values, not a summary claim. I'll pull them directly from
Qdrant myself before this gets called done.

## Definition of done

- Real, confirmed non-image attachment point in `frontpocket_memory_phase2_test`
  showing `attachment_mime_type` (non-image, e.g. `application/pdf`),
  `attachment_source_system: "library_files"`, `attachment_filename`
  populated, and `text` containing the metadata-only description (not a
  vision caption, since no vision call should have fired for this one).
- Real, confirmed image attachment resolved via `library_files` (not
  `asset_file_names`) with a real vision caption in `text`.
- All three cases (original asset_file_names image + these two new ones)
  coexisting in the same test collection.
- Production collections still untouched.

## Then — move forward

Once both are confirmed, Phase 2 is genuinely done. Next up is your call:
full-corpus Phase 2 run for real (~1,700 images, real vision API cost — get
the dry-run cost estimate first per the original Phase 2 brief), or Phase 3
(`model_slug` / `conversation_template_id` capture), or something else.
