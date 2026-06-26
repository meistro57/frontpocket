#!/usr/bin/env python3
# filename: fp_reflect_loop.py
"""
FrontPocket Reflection Loop
===========================
The meta-bridge reflect loop, but aimed at YOU instead of sacred texts.

Iterates frontpocket_memory points, sends each chunk to an LLM for
deep reflection, evaluates the result, and upserts findings into a
new `fp_reflections` collection.

Meta-bridge reflects on Dolores Cannon and the Ra Material.
FrontPocket reflects on Mark Hubrich.

Same architecture. Different soul.

Usage:
    python fp_reflect_loop.py
    python fp_reflect_loop.py --limit 50
    python fp_reflect_loop.py --goal "map the awakening arc across all conversations"
    python fp_reflect_loop.py --speaker user       # only your messages
    python fp_reflect_loop.py --speaker assistant  # only AI responses
    python fp_reflect_loop.py --model google/gemini-3.1-flash-lite
    python fp_reflect_loop.py --loop-interval 300 --max-loops 0
    python fp_reflect_loop.py --from-scratch
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import uuid
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, TypedDict

from dotenv import load_dotenv
import requests

# ── config ────────────────────────────────────────────────────────────────────

load_dotenv()

QDRANT_URL         = os.getenv("QDRANT_URL", "http://localhost:6333")
OPENROUTER_API_KEY = os.getenv("OPENROUTER_API_KEY", "")
OPENROUTER_BASE    = os.getenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1").rstrip("/")
EMBED_MODEL        = os.getenv("OPENROUTER_EMBEDDING_MODEL", "google/gemini-embedding-2-preview")
FRONTPOCKET_URL    = os.getenv("FRONTPOCKET_PUBLIC_URL", "http://localhost:8088")

SOURCE_COLLECTION = "frontpocket_memory"
TARGET_COLLECTION = "fp_reflections"
DEFAULT_MODEL     = "google/gemini-2.5-flash-lite"  # $0.10/$0.40 per 1M — fast and cheap
EMBED_DIMS        = 3072

# Payload flags written back to SOURCE_COLLECTION
FLAG_REFLECTED_AT  = "fp_reflected_at"
FLAG_DEPTH         = "fp_reflection_depth"
FLAG_THEMES        = "fp_themes"
FLAG_PHASE         = "fp_awakening_phase"


# ── data classes ──────────────────────────────────────────────────────────────

@dataclass
class MemoryPoint:
    point_id: str
    memory_id: str
    source_title: str
    source_quote: str
    text: str
    speaker: str
    timestamp: str
    conversation_id: str
    project: str


@dataclass
class Reflection:
    themes: List[str] = field(default_factory=list)
    depth: str = "shallow"
    awakening_phase: str = "unknown"
    emotional_tone: str = ""
    insight: str = ""
    questions: List[str] = field(default_factory=list)
    echoes: List[str] = field(default_factory=list)
    contradiction_signal: bool = False
    reflection_confidence: float = 0.0
    raw: Dict[str, Any] = field(default_factory=dict)


# ── qdrant helpers ────────────────────────────────────────────────────────────

def qdrant(method: str, path: str, payload: Any = None) -> Any:
    url = f"{QDRANT_URL}{path}"
    headers = {"Content-Type": "application/json"}
    resp = requests.request(method, url, headers=headers,
                            json=payload, timeout=30)
    if resp.status_code >= 400:
        raise RuntimeError(f"Qdrant {method} {path} → {resp.status_code}: {resp.text[:300]}")
    return resp.json()


def ensure_target_collection(from_scratch: bool = False) -> None:
    if from_scratch:
        try:
            qdrant("DELETE", f"/collections/{TARGET_COLLECTION}")
            print(f"[init] dropped {TARGET_COLLECTION}")
        except Exception:
            pass

    try:
        qdrant("GET", f"/collections/{TARGET_COLLECTION}")
        print(f"[init] {TARGET_COLLECTION} exists")
        return
    except RuntimeError:
        pass

    qdrant("PUT", f"/collections/{TARGET_COLLECTION}", {
        "vectors": {
            "insight": {"size": EMBED_DIMS, "distance": "Cosine"},
        }
    })
    print(f"[init] created {TARGET_COLLECTION}")

    for field_name, schema in [
        ("depth",                  "keyword"),
        ("awakening_phase",        "keyword"),
        ("speaker",                "keyword"),
        ("source_title",           "keyword"),
        ("contradiction_signal",   "bool"),
        ("reflected_at",           "integer"),
        ("reflection_confidence",  "float"),
    ]:
        try:
            qdrant("PUT", f"/collections/{TARGET_COLLECTION}/index",
                   {"field_name": field_name, "field_schema": schema})
        except Exception:
            pass


def get_already_reflected() -> set:
    reflected = set()
    offset = None
    while True:
        body: Dict[str, Any] = {
            "limit": 250,
            "with_payload": ["source_memory_id"],
            "with_vector": False,
        }
        if offset:
            body["offset"] = offset
        result = qdrant("POST", f"/collections/{TARGET_COLLECTION}/points/scroll", body)
        points = result.get("result", {}).get("points", [])
        for p in points:
            mid = (p.get("payload") or {}).get("source_memory_id", "")
            if mid:
                reflected.add(mid)
        offset = result.get("result", {}).get("next_page_offset")
        if not offset:
            break
    return reflected


def iter_memory_points(skip: set, speaker_filter: Optional[str]) -> Any:
    offset = None
    while True:
        body: Dict[str, Any] = {
            "limit": 100,
            "with_payload": True,
            "with_vector": False,
        }
        if offset:
            body["offset"] = offset
        if speaker_filter:
            body["filter"] = {"must": [{"key": "speaker", "match": {"value": speaker_filter}}]}

        result = qdrant("POST", f"/collections/{SOURCE_COLLECTION}/points/scroll", body)
        points = result.get("result", {}).get("points", [])

        for p in points:
            pl = p.get("payload") or {}
            memory_id = pl.get("memory_id", "")
            if memory_id in skip:
                continue
            yield MemoryPoint(
                point_id=str(p["id"]),
                memory_id=memory_id,
                source_title=pl.get("source_title", ""),
                source_quote=pl.get("source_quote", ""),
                text=pl.get("text", "") or pl.get("source_quote", ""),
                speaker=pl.get("speaker", ""),
                timestamp=pl.get("timestamp", ""),
                conversation_id=pl.get("conversation_id", ""),
                project=pl.get("project", ""),
            )

        offset = result.get("result", {}).get("next_page_offset")
        if not offset:
            break


# ── embedding ──────────────────────────────────────────────────────────────────

def embed(text: str) -> List[float]:
    resp = requests.post(
        f"{OPENROUTER_BASE}/embeddings",
        headers={
            "Authorization": f"Bearer {OPENROUTER_API_KEY}",
            "Content-Type": "application/json",
        },
        json={"model": EMBED_MODEL, "input": text},
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()["data"][0]["embedding"]


# ── LLM reflection ────────────────────────────────────────────────────────────

SYSTEM_PROMPT = """You are a deep psychological and spiritual analyst with expertise in:
- Consciousness studies, awakening patterns, and spiritual transformation
- Emotional intelligence and human communication patterns
- Depth psychology and Jungian archetypes
- The specific canon Mark draws on: Seth/Jane Roberts, Dolores Cannon, Bashar, Ra Material, Kryon

You are analyzing fragments from a real human's ChatGPT conversation history spanning 3+ years.
The human is Mark Hubrich — a steel detailer, maker, hacker, and spiritual seeker from Chicago.
He has ADHD, rides a Harley, owns a dog named Stella, and is on a genuine awakening journey.

Your job: reflect deeply on each fragment and extract structured insight in JSON.
Be specific to what's actually in the text — not generic. Be honest, not flattering.
Short texts get honest short reflections. Don't pad."""


REFLECTION_PROMPT = """Reflect on this conversation fragment from Mark's history.

Speaker: {speaker}
Conversation: {source_title}
Date: {timestamp}

Text:
---
{text}
---

Return ONLY valid JSON with these fields:
{{
  "themes": ["list", "of", "1-4", "word", "themes"],
  "depth": "shallow|moderate|deep|profound",
  "awakening_phase": "dormancy|crack|flood|embodiment|teacher|integration|settled|unknown",
  "emotional_tone": "one sentence description of the emotional quality",
  "insight": "2-3 sentences of genuine insight about what this reveals about Mark",
  "questions": ["1-3 questions this fragment raises or implies"],
  "echoes": ["themes or patterns that echo other known aspects of his journey"],
  "contradiction_signal": false,
  "reflection_confidence": 0.0
}}

awakening_phase guide:
- dormancy: pre-spiritual, builder/hacker mode, no explicit seeking
- crack: first spiritual questions, cross-referencing traditions
- flood: the big november 2024 outpouring to his higher self
- embodiment: testing the practices against real life (condensed milk incident etc)
- teacher: writing books, building Eli GPT, launching meetup, teaching others
- integration: 2025 long middle — building meta-bridge, living with what he knows
- settled: 2026 — quiet knowing, reality poking, living in the question

reflection_confidence: 0.0-1.0 (how much genuine insight you extracted)
contradiction_signal: true if the text shows internal conflict, dissonance, or contradiction"""


def reflect_on_point(point: MemoryPoint, model: str) -> Reflection:
    text = (point.text or point.source_quote or "").strip()
    if not text or len(text) < 20:
        return Reflection(depth="shallow", reflection_confidence=0.0)

    prompt = REFLECTION_PROMPT.format(
        speaker=point.speaker,
        source_title=point.source_title,
        timestamp=point.timestamp[:10] if point.timestamp else "unknown",
        text=text[:2000],
    )

    resp = requests.post(
        f"{OPENROUTER_BASE}/chat/completions",
        headers={
            "Authorization": f"Bearer {OPENROUTER_API_KEY}",
            "Content-Type": "application/json",
        },
        json={
            "model": model,
            "messages": [
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": prompt},
            ],
            "max_tokens": 600,
            "temperature": 0.4,
        },
        timeout=45,
    )
    resp.raise_for_status()

    raw_text = resp.json()["choices"][0]["message"]["content"].strip()

    # strip markdown fences if present
    if raw_text.startswith("```"):
        raw_text = raw_text.split("```")[1]
        if raw_text.startswith("json"):
            raw_text = raw_text[4:]

    try:
        data = json.loads(raw_text)
    except json.JSONDecodeError:
        return Reflection(depth="shallow", reflection_confidence=0.0)

    return Reflection(
        themes=data.get("themes") or [],
        depth=data.get("depth") or "shallow",
        awakening_phase=data.get("awakening_phase") or "unknown",
        emotional_tone=data.get("emotional_tone") or "",
        insight=data.get("insight") or "",
        questions=data.get("questions") or [],
        echoes=data.get("echoes") or [],
        contradiction_signal=bool(data.get("contradiction_signal", False)),
        reflection_confidence=float(data.get("reflection_confidence") or 0.0),
        raw=data,
    )


# ── evaluation ─────────────────────────────────────────────────────────────────

def evaluate(r: Reflection) -> Dict[str, Any]:
    is_interesting = (
        r.reflection_confidence >= 0.6
        or len(r.themes) >= 3
        or len(r.questions) >= 2
        or len(r.echoes) >= 2
        or r.depth in ("deep", "profound")
    )
    return {
        "is_interesting": is_interesting,
        "has_contradiction": r.contradiction_signal,
        "depth_score": {"shallow": 0, "moderate": 1, "deep": 2, "profound": 3}.get(r.depth, 0),
    }


def decide(evaluation: Dict[str, Any]) -> str:
    if evaluation["has_contradiction"]:
        return "track_contradiction"
    if evaluation["is_interesting"]:
        return "store_interesting"
    return "continue_scan"


# ── upsert ─────────────────────────────────────────────────────────────────────

def upsert_reflection(point: MemoryPoint, reflection: Reflection, model: str) -> str:
    insight_text = reflection.insight or reflection.emotional_tone or " ".join(reflection.themes)
    if not insight_text.strip():
        insight_text = "no meaningful insight extracted"

    vector = embed(insight_text)

    point_id = str(uuid.uuid5(
        uuid.NAMESPACE_URL,
        f"fp_reflection:{point.memory_id}",
    ))

    payload = {
        "source_memory_id":       point.memory_id,
        "source_point_id":        point.point_id,
        "source_title":           point.source_title,
        "source_quote":           (point.source_quote or "")[:500],
        "speaker":                point.speaker,
        "timestamp":              point.timestamp,
        "conversation_id":        point.conversation_id,
        "project":                point.project,
        "themes":                 reflection.themes,
        "depth":                  reflection.depth,
        "awakening_phase":        reflection.awakening_phase,
        "emotional_tone":         reflection.emotional_tone,
        "insight":                reflection.insight,
        "questions":              reflection.questions,
        "echoes":                 reflection.echoes,
        "contradiction_signal":   reflection.contradiction_signal,
        "reflection_confidence":  reflection.reflection_confidence,
        "model":                  model,
        "reflected_at":           int(time.time()),
    }

    qdrant("PUT", f"/collections/{TARGET_COLLECTION}/points", {
        "points": [{
            "id": point_id,
            "vector": {"insight": vector},
            "payload": payload,
        }]
    })

    return point_id


def flag_source_point(point: MemoryPoint, reflection: Reflection) -> None:
    try:
        qdrant("POST", f"/collections/{SOURCE_COLLECTION}/points/payload?wait=false", {
            "payload": {
                FLAG_REFLECTED_AT: int(time.time()),
                FLAG_DEPTH:        reflection.depth,
                FLAG_THEMES:       reflection.themes[:5],
                FLAG_PHASE:        reflection.awakening_phase,
            },
            "points": [point.point_id],
        })
    except Exception as e:
        print(f"    [flag] write failed: {e}")


# ── loop ───────────────────────────────────────────────────────────────────────

class LoopState(TypedDict, total=False):
    processed:      int
    skipped:        int
    errors:         int
    interesting:    int
    contradictions: int
    by_phase:       Dict[str, int]
    by_depth:       Dict[str, int]
    last_error:     str


def run_once(args: argparse.Namespace, model: str) -> LoopState:
    print(f"\n[config]")
    print(f"  source:    {SOURCE_COLLECTION}")
    print(f"  target:    {TARGET_COLLECTION}")
    print(f"  model:     {model}")
    print(f"  limit:     {args.limit}")
    print(f"  speaker:   {args.speaker or 'all'}")
    print(f"  goal:      {args.goal}")
    print()

    ensure_target_collection(getattr(args, "from_scratch", False))

    print("[init] scanning already-reflected points...")
    already = get_already_reflected()
    print(f"[init] {len(already)} already reflected — skipping")

    state: LoopState = {
        "processed":      0,
        "skipped":        0,
        "errors":         0,
        "interesting":    0,
        "contradictions": 0,
        "by_phase":       {},
        "by_depth":       {},
        "last_error":     "",
    }

    t0 = time.time()

    for point in iter_memory_points(already, args.speaker):
        if state["processed"] >= args.limit:
            break

        text = (point.text or point.source_quote or "").strip()
        if len(text) < 20:
            state["skipped"] += 1
            continue

        try:
            reflection = reflect_on_point(point, model)
            upsert_reflection(point, reflection, model)
            flag_source_point(point, reflection)

            ev       = evaluate(reflection)
            decision = decide(ev)

            state["processed"] += 1
            state["by_phase"][reflection.awakening_phase] = state["by_phase"].get(reflection.awakening_phase, 0) + 1
            state["by_depth"][reflection.depth]           = state["by_depth"].get(reflection.depth, 0) + 1

            if decision == "store_interesting":
                state["interesting"] += 1
            elif decision == "track_contradiction":
                state["contradictions"] += 1

            if not args.quiet:
                icon  = {"store_interesting": "★", "track_contradiction": "⚡", "continue_scan": "·"}.get(decision, "?")
                phase = reflection.awakening_phase[:8].ljust(8)
                depth = reflection.depth[:8].ljust(8)
                conf  = f"{reflection.reflection_confidence:.2f}"
                print(f"[{state['processed']:>4}] {icon} {point.speaker[:4]:<4} | {phase} | {depth} | conf={conf} | {point.source_title[:40]}")

        except Exception as e:
            state["errors"] += 1
            state["last_error"] = f"{type(e).__name__}: {e}"
            if not args.quiet:
                print(f"[{state['processed']:>4}] ✗ {state['last_error'][:80]}")

    elapsed = time.time() - t0

    print(f"\n[done]")
    print(f"  processed:      {state['processed']}")
    print(f"  skipped:        {state['skipped']}")
    print(f"  errors:         {state['errors']}")
    print(f"  interesting:    {state['interesting']}")
    print(f"  contradictions: {state['contradictions']}")
    print(f"  elapsed:        {elapsed/60:.1f}m")
    print(f"\n  by phase:")
    for phase, count in sorted(state["by_phase"].items(), key=lambda x: -x[1]):
        print(f"    {phase:<20} {count}")
    print(f"\n  by depth:")
    for depth, count in sorted(state["by_depth"].items(), key=lambda x: -x[1]):
        print(f"    {depth:<12} {count}")

    print(f"\n[query] profound reflections:")
    print(f'  python fp_reflect_query.py "" --depth profound')
    print(f"\n[query] contradictions:")
    print(f'  python fp_reflect_query.py "" --contradiction')
    print(f"\n[query] the flood phase:")
    print(f'  python fp_reflect_query.py "" --phase flood')

    return state


def main() -> int:
    parser = argparse.ArgumentParser(
        description="FrontPocket Reflection Loop — reflect on Mark's conversation history"
    )
    parser.add_argument("--model",         default=DEFAULT_MODEL, help=f"OpenRouter model. Default: {DEFAULT_MODEL}")
    parser.add_argument("--goal",          default="Surface awakening patterns, emotional depth, and contradictions across Mark's conversation history")
    parser.add_argument("--limit",         type=int, default=50, help="Max points to process per run")
    parser.add_argument("--speaker",       default=None, choices=["user", "assistant"], help="Filter by speaker")
    parser.add_argument("--loop-interval", type=float, default=0.0, help="Seconds between runs (0 = single run)")
    parser.add_argument("--max-loops",     type=int, default=1, help="How many runs (0 = infinite)")
    parser.add_argument("--from-scratch",  action="store_true", help="Wipe fp_reflections and start fresh")
    parser.add_argument("--quiet",         action="store_true", help="Suppress per-point output")
    args = parser.parse_args()

    if not OPENROUTER_API_KEY:
        print("[error] OPENROUTER_API_KEY not set in .env")
        return 1

    model = args.model.strip()

    if args.loop_interval <= 0:
        state = run_once(args, model)
        return 0 if state["errors"] == 0 else 1

    run_count    = 0
    total_errors = 0

    while True:
        run_count += 1
        print(f"\n[timer] run {run_count}")
        state = run_once(args, model)
        total_errors += state["errors"]

        if args.max_loops > 0 and run_count >= args.max_loops:
            break

        print(f"\n[timer] sleeping {args.loop_interval:.0f}s")
        time.sleep(args.loop_interval)

    return 0 if total_errors == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
