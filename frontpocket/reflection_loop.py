from __future__ import annotations

import argparse
import json
import os
import time
import uuid
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, TypedDict

import requests
from dotenv import load_dotenv

from .qdrant_io import qdrant

load_dotenv()

OPENROUTER_API_KEY = os.getenv("OPENROUTER_API_KEY", "")
OPENROUTER_BASE = os.getenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1").rstrip("/")
EMBED_MODEL = os.getenv("OPENROUTER_EMBEDDING_MODEL", "google/gemini-embedding-2-preview")

RAW_SOURCE_COLLECTION = "frontpocket_memory"
CLEANED_SOURCE_COLLECTION = "fp_cleaned_memory"
TARGET_COLLECTION = "fp_reflections"
DEFAULT_MODEL = "google/gemini-2.5-flash-lite"
EMBED_DIMS = 3072

FLAG_REFLECTED_AT = "fp_reflected_at"
FLAG_DEPTH = "fp_reflection_depth"
FLAG_THEMES = "fp_themes"
FLAG_PHASE = "fp_awakening_phase"


@dataclass
class MemoryPoint:
    point_id: str
    memory_id: str
    source_title: str
    source_quote: str
    text: str
    speaker: str
    source_role: str
    timestamp: str
    conversation_id: str
    project: str
    project_hint: str = ""
    project_hint_basis: str = "none"
    project_confidence: float = 0.0
    quote_quality: str = "unknown"
    phase_applicability: str = "uncertain"
    evidence_strength: str = "weak"
    reflection_scope: str = "quote_only"
    domain: str = "general"
    memory_kind: str = "unknown"
    usable_for_user_profile: bool = False
    usable_for_project_history: bool = False
    usable_for_assistant_guidance: bool = False
    usable_for_persona_memory: bool = False
    usable_for_canon: bool = False
    safe_for_reflection: bool = True


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

    qdrant(
        "PUT",
        f"/collections/{TARGET_COLLECTION}",
        {"vectors": {"insight": {"size": EMBED_DIMS, "distance": "Cosine"}}},
    )
    print(f"[init] created {TARGET_COLLECTION}")


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


def iter_raw_memory_points(skip: set, speaker_filter: Optional[str]) -> Any:
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

        result = qdrant("POST", f"/collections/{RAW_SOURCE_COLLECTION}/points/scroll", body)
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
                source_role=pl.get("speaker", "unknown") or "unknown",
                timestamp=pl.get("timestamp", ""),
                conversation_id=pl.get("conversation_id", ""),
                project=pl.get("project", ""),
            )

        offset = result.get("result", {}).get("next_page_offset")
        if not offset:
            break


def iter_cleaned_memory_points(skip: set, speaker_filter: Optional[str], include_needs_review: bool) -> Any:
    offset = None
    while True:
        must: List[Dict[str, Any]] = []
        if not include_needs_review:
            must.append({"key": "safe_for_reflection", "match": {"value": True}})
        if speaker_filter:
            must.append({"key": "speaker_normalized", "match": {"value": speaker_filter}})

        body: Dict[str, Any] = {
            "limit": 100,
            "with_payload": True,
            "with_vector": False,
        }
        if must:
            body["filter"] = {"must": must}
        if offset:
            body["offset"] = offset

        result = qdrant("POST", f"/collections/{CLEANED_SOURCE_COLLECTION}/points/scroll", body)
        points = result.get("result", {}).get("points", [])

        for p in points:
            pl = p.get("payload") or {}
            memory_id = str(pl.get("raw_memory_id") or pl.get("source_memory_id") or "")
            if not memory_id or memory_id in skip:
                continue
            cleaned_payload = pl.get("payload_cleaned") or {}
            text = str(cleaned_payload.get("text") or "")
            source_quote = str(pl.get("source_quote_cleaned") or cleaned_payload.get("source_quote") or "")
            yield MemoryPoint(
                point_id=str(pl.get("source_point_id") or p.get("id") or ""),
                memory_id=memory_id,
                source_title=str(pl.get("source_title") or cleaned_payload.get("source_title") or ""),
                source_quote=source_quote,
                text=text or source_quote,
                speaker=str(pl.get("speaker_normalized") or cleaned_payload.get("speaker") or "unknown"),
                source_role=str(pl.get("source_role") or pl.get("speaker_normalized") or cleaned_payload.get("speaker") or "unknown"),
                timestamp=str(pl.get("timestamp_normalized") or cleaned_payload.get("timestamp") or ""),
                conversation_id=str(pl.get("conversation_id") or cleaned_payload.get("conversation_id") or ""),
                project=str(cleaned_payload.get("project") or ""),
                project_hint=str(pl.get("project_hint") or ""),
                project_hint_basis=str(pl.get("project_hint_basis") or "none"),
                project_confidence=float(pl.get("project_confidence") or 0.0),
                quote_quality=str(pl.get("quote_quality") or "unknown"),
                phase_applicability=str(pl.get("phase_applicability") or "uncertain"),
                evidence_strength=str(pl.get("evidence_strength") or "weak"),
                reflection_scope=str(pl.get("reflection_scope") or "quote_only"),
                domain=str(pl.get("domain") or "general"),
                memory_kind=str(pl.get("memory_kind") or "unknown"),
                usable_for_user_profile=bool(pl.get("usable_for_user_profile", False)),
                usable_for_project_history=bool(pl.get("usable_for_project_history", False)),
                usable_for_assistant_guidance=bool(pl.get("usable_for_assistant_guidance", False)),
                usable_for_persona_memory=bool(pl.get("usable_for_persona_memory", False)),
                usable_for_canon=bool(pl.get("usable_for_canon", False)),
                safe_for_reflection=bool(pl.get("safe_for_reflection", False)),
            )

        offset = result.get("result", {}).get("next_page_offset")
        if not offset:
            break


def embed(text: str) -> List[float]:
    resp = requests.post(
        f"{OPENROUTER_BASE}/embeddings",
        headers={"Authorization": f"Bearer {OPENROUTER_API_KEY}", "Content-Type": "application/json"},
        json={"model": EMBED_MODEL, "input": text},
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()["data"][0]["embedding"]


SYSTEM_PROMPT = """You are a source-grounded reflection analyst.
Only infer what the provided text supports. Keep reflections concise and specific.
Do not force spiritual framing when evidence is weak or technical-only."""


REFLECTION_PROMPT = """Reflect on this conversation fragment.

Speaker: {speaker}
Source role: {source_role}
Conversation: {source_title}
Date: {timestamp}
Quote quality: {quote_quality}
Evidence strength: {evidence_strength}
Reflection scope: {reflection_scope}
Phase applicability: {phase_applicability}
Domain: {domain}
Memory kind target: {memory_kind}

Text:
---
{text}
---

Rules:
- If evidence_strength is weak, keep insight narrow and cautious.
- If reflection_scope is quote_only, avoid broad personality/project claims.
- If phase_applicability is not_applicable, avoid spiritual framing unless directly present in text.
- Assistant/source_role=assistant content must not be reframed as direct user facts.

Return ONLY valid JSON with:
{{
  "themes": ["list", "of", "1-4", "word", "themes"],
  "depth": "shallow|moderate|deep|profound",
  "awakening_phase": "dormancy|crack|flood|embodiment|teacher|integration|settled|unknown",
  "emotional_tone": "one sentence emotional quality",
  "insight": "1-3 short sentences",
  "questions": ["0-3 grounded questions"],
  "echoes": ["0-3 grounded echoes"],
  "contradiction_signal": false,
  "reflection_confidence": 0.0
}}"""


def apply_confidence_cap(quote_quality: str, reflection_confidence: float) -> float:
    if quote_quality in {"partial", "truncated"}:
        return min(reflection_confidence, 0.75)
    if quote_quality == "malformed":
        return min(reflection_confidence, 0.35)
    return reflection_confidence


def reflect_on_point(point: MemoryPoint, model: str) -> Reflection:
    text = (point.text or point.source_quote or "").strip()
    if point.quote_quality == "missing":
        return Reflection(depth="shallow", reflection_confidence=0.0)
    if not text or len(text) < 20:
        return Reflection(depth="shallow", reflection_confidence=0.0)

    prompt = REFLECTION_PROMPT.format(
        speaker=point.speaker,
        source_role=point.source_role,
        source_title=point.source_title,
        timestamp=point.timestamp[:10] if point.timestamp else "unknown",
        text=text[:2000],
        quote_quality=point.quote_quality,
        evidence_strength=point.evidence_strength,
        reflection_scope=point.reflection_scope,
        phase_applicability=point.phase_applicability,
        domain=point.domain,
        memory_kind=point.memory_kind,
    )

    resp = requests.post(
        f"{OPENROUTER_BASE}/chat/completions",
        headers={"Authorization": f"Bearer {OPENROUTER_API_KEY}", "Content-Type": "application/json"},
        json={
            "model": model,
            "messages": [
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": prompt},
            ],
            "max_tokens": 500,
            "temperature": 0.3,
        },
        timeout=45,
    )
    resp.raise_for_status()

    raw_text = resp.json()["choices"][0]["message"]["content"].strip()
    if raw_text.startswith("```"):
        raw_text = raw_text.split("```")[1]
        if raw_text.startswith("json"):
            raw_text = raw_text[4:]

    try:
        data = json.loads(raw_text)
    except json.JSONDecodeError:
        return Reflection(depth="shallow", reflection_confidence=0.0)

    awakening_phase = data.get("awakening_phase") or "unknown"
    if point.phase_applicability == "not_applicable":
        awakening_phase = "unknown"

    reflection_confidence = float(data.get("reflection_confidence") or 0.0)
    reflection_confidence = apply_confidence_cap(point.quote_quality, reflection_confidence)

    return Reflection(
        themes=data.get("themes") or [],
        depth=data.get("depth") or "shallow",
        awakening_phase=awakening_phase,
        emotional_tone=data.get("emotional_tone") or "",
        insight=data.get("insight") or "",
        questions=data.get("questions") or [],
        echoes=data.get("echoes") or [],
        contradiction_signal=bool(data.get("contradiction_signal", False)),
        reflection_confidence=reflection_confidence,
        raw=data,
    )


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


def upsert_reflection(point: MemoryPoint, reflection: Reflection, model: str) -> str:
    insight_text = reflection.insight or reflection.emotional_tone or " ".join(reflection.themes)
    if not insight_text.strip():
        insight_text = "no meaningful insight extracted"

    vector = embed(insight_text)

    point_id = str(uuid.uuid5(uuid.NAMESPACE_URL, f"fp_reflection:{point.memory_id}"))

    payload = {
        "source_memory_id": point.memory_id,
        "source_point_id": point.point_id,
        "source_title": point.source_title,
        "source_quote": (point.source_quote or "")[:500],
        "speaker": point.speaker,
        "source_role": point.source_role,
        "timestamp": point.timestamp,
        "conversation_id": point.conversation_id,
        "project": point.project,
        "project_hint": point.project_hint,
        "project_hint_basis": point.project_hint_basis,
        "project_confidence": point.project_confidence,
        "quote_quality": point.quote_quality,
        "phase_applicability": point.phase_applicability,
        "evidence_strength": point.evidence_strength,
        "reflection_scope": point.reflection_scope,
        "domain": point.domain,
        "memory_kind": point.memory_kind,
        "usable_for_user_profile": point.usable_for_user_profile,
        "usable_for_project_history": point.usable_for_project_history,
        "usable_for_assistant_guidance": point.usable_for_assistant_guidance,
        "usable_for_persona_memory": point.usable_for_persona_memory,
        "usable_for_canon": point.usable_for_canon,
        "vector_present": True,
        "vector_names": ["insight"],
        "vector_dimensions": len(vector),
        "themes": reflection.themes,
        "depth": reflection.depth,
        "awakening_phase": reflection.awakening_phase,
        "emotional_tone": reflection.emotional_tone,
        "insight": reflection.insight,
        "questions": reflection.questions,
        "echoes": reflection.echoes,
        "contradiction_signal": reflection.contradiction_signal,
        "reflection_confidence": reflection.reflection_confidence,
        "model": model,
        "reflected_at": int(time.time()),
    }

    qdrant(
        "PUT",
        f"/collections/{TARGET_COLLECTION}/points",
        {"points": [{"id": point_id, "vector": {"insight": vector}, "payload": payload}]},
    )

    return point_id


def flag_source_point(point: MemoryPoint, reflection: Reflection) -> None:
    if not point.point_id:
        return
    try:
        qdrant(
            "POST",
            f"/collections/{RAW_SOURCE_COLLECTION}/points/payload?wait=false",
            {
                "payload": {
                    FLAG_REFLECTED_AT: int(time.time()),
                    FLAG_DEPTH: reflection.depth,
                    FLAG_THEMES: reflection.themes[:5],
                    FLAG_PHASE: reflection.awakening_phase,
                },
                "points": [point.point_id],
            },
        )
    except Exception:
        return


class LoopState(TypedDict, total=False):
    processed: int
    skipped: int
    errors: int
    interesting: int
    contradictions: int
    by_phase: Dict[str, int]
    by_depth: Dict[str, int]
    last_error: str


def run_once(args: argparse.Namespace, model: str) -> LoopState:
    source = CLEANED_SOURCE_COLLECTION if args.from_cleaned else RAW_SOURCE_COLLECTION

    print("\n[config]")
    print(f"  source:    {source}")
    print(f"  target:    {TARGET_COLLECTION}")
    print(f"  model:     {model}")
    print(f"  limit:     {args.limit}")
    print(f"  speaker:   {args.speaker or 'all'}")
    goal = getattr(args, "goal", "")
    if goal:
        print(f"  goal:      {goal}")
    print()

    ensure_target_collection(getattr(args, "from_scratch", False))

    print("[init] scanning already-reflected points...")
    already = get_already_reflected()
    print(f"[init] {len(already)} already reflected — skipping")

    state: LoopState = {
        "processed": 0,
        "skipped": 0,
        "errors": 0,
        "interesting": 0,
        "contradictions": 0,
        "by_phase": {},
        "by_depth": {},
        "last_error": "",
    }

    t0 = time.time()

    iterator = (
        iter_cleaned_memory_points(already, args.speaker, args.include_needs_review)
        if args.from_cleaned
        else iter_raw_memory_points(already, args.speaker)
    )

    for point in iterator:
        if state["processed"] >= args.limit:
            break

        if args.from_cleaned and not args.include_needs_review and not point.safe_for_reflection:
            state["skipped"] += 1
            continue

        text = (point.text or point.source_quote or "").strip()
        if len(text) < 20:
            state["skipped"] += 1
            continue

        try:
            reflection = reflect_on_point(point, model)
            upsert_reflection(point, reflection, model)
            if not args.from_cleaned:
                flag_source_point(point, reflection)

            ev = evaluate(reflection)
            decision = decide(ev)

            state["processed"] += 1
            state["by_phase"][reflection.awakening_phase] = state["by_phase"].get(reflection.awakening_phase, 0) + 1
            state["by_depth"][reflection.depth] = state["by_depth"].get(reflection.depth, 0) + 1

            if decision == "store_interesting":
                state["interesting"] += 1
            elif decision == "track_contradiction":
                state["contradictions"] += 1

            if not args.quiet:
                icon = {"store_interesting": "★", "track_contradiction": "⚡", "continue_scan": "·"}.get(decision, "?")
                phase = reflection.awakening_phase[:8].ljust(8)
                depth = reflection.depth[:8].ljust(8)
                conf = f"{reflection.reflection_confidence:.2f}"
                print(f"[{state['processed']:>4}] {icon} {point.speaker[:4]:<4} | {phase} | {depth} | conf={conf} | {point.source_title[:40]}")

        except Exception as e:
            state["errors"] += 1
            state["last_error"] = f"{type(e).__name__}: {e}"
            if not args.quiet:
                print(f"[{state['processed']:>4}] ✗ {state['last_error'][:80]}")

    elapsed = time.time() - t0

    print("\n[done]")
    print(f"  processed:      {state['processed']}")
    print(f"  skipped:        {state['skipped']}")
    print(f"  errors:         {state['errors']}")
    print(f"  interesting:    {state['interesting']}")
    print(f"  contradictions: {state['contradictions']}")
    print(f"  elapsed:        {elapsed/60:.1f}m")

    return state


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="FrontPocket Reflection Loop")
    parser.add_argument("--model", default=DEFAULT_MODEL, help=f"OpenRouter model. Default: {DEFAULT_MODEL}")
    parser.add_argument("--limit", type=int, default=50, help="Max points to process per run")
    parser.add_argument("--speaker", default=None, choices=["user", "assistant", "system", "tool", "mixed", "unknown"], help="Filter by speaker")
    parser.add_argument("--loop-interval", type=float, default=0.0, help="Seconds between runs (0 = single run)")
    parser.add_argument("--max-loops", type=int, default=1, help="How many runs (0 = infinite)")
    parser.add_argument("--from-scratch", action="store_true", help="Wipe fp_reflections and start fresh")
    parser.add_argument("--quiet", action="store_true", help="Suppress per-point output")
    parser.add_argument("--goal", default="", help="Optional reflection goal text (legacy compatibility)")
    parser.add_argument("--from-cleaned", dest="from_cleaned", action="store_true", default=True, help="Read from fp_cleaned_memory")
    parser.add_argument("--raw-input", dest="from_cleaned", action="store_false", help="Use frontpocket_memory directly")
    parser.add_argument("--include-needs-review", action="store_true", help="Include cleaned records marked needs_review/unsafe")
    return parser


def run(argv: Optional[List[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not OPENROUTER_API_KEY:
        print("[error] OPENROUTER_API_KEY not set in .env")
        return 1

    model = args.model.strip()

    if args.loop_interval <= 0:
        state = run_once(args, model)
        return 0 if state["errors"] == 0 else 1

    run_count = 0
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


def main() -> int:
    return run()


if __name__ == "__main__":
    raise SystemExit(main())
