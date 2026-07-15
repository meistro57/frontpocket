# FrontPocket — Fix: Parse-Checkpoint Skip Breaks Embed Resume Entirely

## Context — confirmed from real terminal output, this is a hard blocker

Tonight's full ChatGPT ingest hit a genuine OpenRouter account limit (403,
"Key limit exceeded (total limit)") at record 30,360 — that's resolved now,
Mark raised the key's limit. Not a code problem.

But the restart attempt revealed a second, much more serious bug:

```
parse checkpoint: continuing with 2585 completed conversations (parse_checkpoint.json)
...
messages found: 0
messages accepted: 0
...
storage write: skipped (no accepted messages)
```

**Every conversation was already marked complete in the parse checkpoint
(from the earlier successful parse phase, before the crash), so the
restart produced zero message records at all** — and with zero records,
the embed/write resume loop had nothing to work with, so it skipped
storage entirely and exited having done no new work.

**This is not a one-off — it's a permanent block.** Once the parse
checkpoint says "everything's parsed," every future restart will hit this
same wall, forever, regardless of how much embedding actually completed
before a crash. The embed-loop's own separate resume journal
(`ingest_progress.json`, tracking `last_record_index`) becomes irrelevant
if it never receives any records to check against.

## Real root cause to confirm (read the actual skip logic, don't assume)

Read `internal/memory/chatgpt_parse_checkpoint.go`'s skip path (the part
that decides whether to re-process a conversation already marked complete
in `parse_checkpoint.json`) together with wherever `ParseChatGPTExport`
consumes that checkpoint. The bug is almost certainly here: skipping a
checkpointed conversation is skipping **too much** — it should skip the
*expensive* work already done (vision captioning, which is separately
cached and cheap to reuse via `caption_cache.json` anyway) but must still
**produce the conversation's message records** so they reach the
embed/write loop, which has its own independent resume logic
(`ingest_progress.json`) for deciding which specific records still need
embedding vs. already written.

In other words: parse-phase checkpointing and embed-phase resume are two
separate, independently-resumable stages, and the fix must make sure
skipping stage 1's expensive work doesn't also skip feeding stage 2 the
records it needs to do its own (separate, already-correct) resume check.

Confirm this diagnosis by reading the actual code before fixing it —
report what the skip path currently does versus what it needs to do.

## Fix

- When a conversation is already checkpointed, reconstruct its message
  records cheaply (text extraction + attachment metadata/reference
  resolution — NOT re-running vision captioning, which should transparently
  hit the existing caption cache and cost nothing if genuinely reused,
  confirm this is the case) and pass them through to the embed/write loop
  exactly as if freshly parsed.
- The embed/write loop's own `ResumeJournal` (`last_record_index`) already
  correctly decides which of those reconstructed records still need
  embedding — don't change that logic, just make sure it actually receives
  records to check against.
- Add a real test: mark all conversations as checkpointed (simulating this
  exact scenario), run the importer, and confirm it still produces the
  correct message records and correctly resumes embedding from the
  embed-journal's last position — not zero records.

## Verification

```bash
cd ~/frontpocket
go build ./... && go test ./tests/... -v
./make_all.sh

# Real reproduction of tonight's exact scenario: a fully-parse-checkpointed
# export, with an embed journal that stopped partway through. Confirm the
# restart actually continues embedding from the correct record, not skips
# everything.
./bin/frontpocket ingest chatgpt ./unzipped --project FrontPocket --ai-provider chatgpt \
  --resume ./ingest_progress.json --caption-cache ./caption_cache.json
```

Report the actual `embedding progress: X/45169 records` line from a real
restart — it must show continuing meaningfully past wherever the embed
journal left off (record 30,360's territory), not `messages accepted: 0`.

## Non-goals

- No changes to the embed-loop's own resume journal logic — it's correct,
  it just needs records to operate on.
- No changes to caption caching — confirm it's being hit correctly as part
  of this fix, don't rebuild it.
- Don't touch the Claude importer or any Phase 1/2 field logic.

## Definition of done

- Real root cause confirmed by reading the actual skip-path code.
- Fix applied so a fully-checkpointed restart still produces message
  records and correctly resumes embedding from the true last position.
- A real restart test reported with actual progress-line output proving
  it continues past record 30,360, not stuck at zero.
