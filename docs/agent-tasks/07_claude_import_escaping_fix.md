# FrontPocket — Claude Importer: Quick Follow-Up (Escaping Fix)

Small, scoped fix — not a new investigation. The Claude importer
(`internal/memory/claude_import.go`) works correctly overall, verified
against real Qdrant data (real `speaker: "user"`/`"assistant"` chat turns,
real `memories.json`/`projects.json` ingestion). Two small things to clean
up before this is fully done.

## Issue 1 — Double-escaped newlines in `text`/`summary`/`source_quote`

Spot-checked real points in `frontpocket_claude_test` and found some
`tool_use`/`tool_result`-derived text contains literal `\n` (backslash + n,
two characters) instead of actual line breaks. Example, verbatim from a real
point's `text` field:

```
SELECT * FROM [{self.current_table}]\n                    {order_clause}\n                    OFFSET ? ROWS
```

That's a real newline that got double-escaped somewhere — likely because the
`tool_use`/`tool_result` content block's inner value is itself a JSON string
(tool input/output are commonly JSON-encoded blobs inside Claude's export
format), and it's being concatenated into the message text without a second
unescape pass. Find where `tool_use`/`tool_result` content gets converted to
plain text in `claude_import.go` and confirm: if the block's value is itself
JSON, decode it properly (extract the actual code/text field, not the raw
JSON string) rather than embedding the JSON-with-escapes as-is.

Check plain `text`-type content blocks too — confirm they're NOT affected
(the samples pulled so far suggest this is isolated to `tool_use`/
`tool_result`, but confirm rather than assume).

## Issue 2 — Confirm `tool_use`/`tool_result` structure is actually distinguishable

Every point sampled so far reads as plain prose/code, indistinguishable from
a plain `text` block. Pull a few real `tool_use` and `tool_result` points
specifically and report back: does the ingested text preserve any signal
that this was a tool call (tool name, e.g.) or tool result, or does it
collapse into indistinguishable plain text? If Claude used tools mid-conversation
(likely, given `tool_use=8955` and `tool_result=8820` occurrences in the
Step 1 findings), losing that signal means downstream reflection/search can't
distinguish "Claude wrote this code" from "Claude's tool executed and
returned this output" — worth knowing whether that distinction is preserved
in the payload (even just a `content_block_type` tag) or intentionally
collapsed for simplicity.

## Verification

```bash
cd ~/frontpocket
go build ./... && go test ./tests/... -v
./make_all.sh

QDRANT_COLLECTION=frontpocket_claude_test ./bin/frontpocket ingest claude \
  ./Claude-Marks-chat_history_7-1-26 --project FrontPocket --limit 20
```

Pull a real `tool_use`/`tool_result`-sourced point after the fix and confirm
`text` contains actual newlines (not `\n` literal) and report the real JSON,
same standard as every other check today.

## Non-goals

- No changes to the ChatGPT importer.
- No full-corpus Claude run — this is a quality fix on the existing test
  collection scope.
- Do not touch `frontpocket_memory` (the GPT full ingest is still running).
