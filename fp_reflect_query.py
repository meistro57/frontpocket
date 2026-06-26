#!/usr/bin/env python3
# filename: fp_reflect_query.py
"""
Query the fp_reflections collection.
Semantic search over the insight vectors — find moments that feel like a given query.

Usage:
    python fp_reflect_query.py "awakening anxiety"
    python fp_reflect_query.py "ship on standby" --limit 5
    python fp_reflect_query.py "contradiction" --phase flood
    python fp_reflect_query.py "" --depth profound --limit 20
    python fp_reflect_query.py "" --speaker user --phase integration
    python fp_reflect_query.py --stats
"""

from __future__ import annotations

import argparse
import os
import sys
import json
from typing import Any, Dict, List, Optional

import requests
from dotenv import load_dotenv

load_dotenv()

QDRANT_URL         = os.getenv("QDRANT_URL", "http://localhost:6333")
OPENROUTER_API_KEY = os.getenv("OPENROUTER_API_KEY", "")
OPENROUTER_BASE    = os.getenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1")
EMBED_MODEL        = os.getenv("OPENROUTER_EMBEDDING_MODEL", "google/gemini-embedding-2-preview")
TARGET_COLLECTION  = "fp_reflections"


def qdrant(method: str, path: str, payload: Any = None) -> Any:
    url = f"{QDRANT_URL}{path}"
    resp = requests.request(method, url, headers={"Content-Type": "application/json"},
                            json=payload, timeout=30)
    if resp.status_code >= 400:
        raise RuntimeError(f"Qdrant {resp.status_code}: {resp.text[:300]}")
    return resp.json()


def embed(text: str) -> List[float]:
    resp = requests.post(
        f"{OPENROUTER_BASE}/embeddings",
        headers={"Authorization": f"Bearer {OPENROUTER_API_KEY}", "Content-Type": "application/json"},
        json={"model": EMBED_MODEL, "input": text},
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()["data"][0]["embedding"]


def build_filter(phase: Optional[str], depth: Optional[str],
                 speaker: Optional[str], contradiction: bool) -> Optional[Dict]:
    must = []
    if phase:
        must.append({"key": "awakening_phase", "match": {"value": phase}})
    if depth:
        must.append({"key": "depth", "match": {"value": depth}})
    if speaker:
        must.append({"key": "speaker", "match": {"value": speaker}})
    if contradiction:
        must.append({"key": "contradiction_signal", "match": {"value": True}})
    return {"must": must} if must else None


def search(query: str, limit: int, filters: Optional[Dict]) -> List[Dict]:
    vector = embed(query)
    body: Dict[str, Any] = {
        "vector": {"name": "insight", "vector": vector},
        "limit": limit,
        "with_payload": True,
        "with_vector": False,
    }
    if filters:
        body["filter"] = filters
    result = qdrant("POST", f"/collections/{TARGET_COLLECTION}/points/search", body)
    return result.get("result", [])


def scroll_filter(limit: int, filters: Optional[Dict]) -> List[Dict]:
    body: Dict[str, Any] = {
        "limit": limit,
        "with_payload": True,
        "with_vector": False,
        "order_by": {"key": "reflection_confidence", "direction": "desc"},
    }
    if filters:
        body["filter"] = filters
    result = qdrant("POST", f"/collections/{TARGET_COLLECTION}/points/scroll", body)
    return result.get("result", {}).get("points", [])


def print_result(p: Dict, score: Optional[float] = None) -> None:
    pl = p.get("payload") or {}
    score_str = f"  score={score:.3f}" if score is not None else ""
    conf_str  = f"  conf={pl.get('reflection_confidence', 0):.2f}"
    print(f"\n{'─'*60}")
    print(f"  {pl.get('speaker','?'):<10} | {pl.get('awakening_phase','?'):<12} | {pl.get('depth','?')}{score_str}{conf_str}")
    print(f"  [{pl.get('timestamp','')[:10]}] {pl.get('source_title','')}")
    print()
    quote = (pl.get("source_quote") or "")[:200]
    if quote:
        print(f"  original: \"{quote}\"")
        print()
    insight = pl.get("insight") or ""
    if insight:
        print(f"  insight:  {insight}")
        print()
    themes = pl.get("themes") or []
    if themes:
        print(f"  themes:   {', '.join(themes)}")
    phase = pl.get("awakening_phase","")
    et = pl.get("emotional_tone","")
    if et:
        print(f"  tone:     {et}")
    questions = pl.get("questions") or []
    if questions:
        print(f"  raises:   {questions[0]}")
    if pl.get("contradiction_signal"):
        print(f"  ⚡ CONTRADICTION SIGNAL")


def show_stats() -> None:
    try:
        info = qdrant("GET", f"/collections/{TARGET_COLLECTION}")
        count = info.get("result", {}).get("points_count", 0)
        print(f"\n[fp_reflections] {count} reflection points\n")
    except Exception as e:
        print(f"[error] {e}")
        return

    for field_name, label in [
        ("awakening_phase", "by awakening phase"),
        ("depth",           "by depth"),
        ("speaker",         "by speaker"),
    ]:
        print(f"  {label}:")
        for val in ["dormancy","crack","flood","embodiment","teacher","integration","settled","unknown",
                    "shallow","moderate","deep","profound","user","assistant"]:
            filt = {"must": [{"key": field_name, "match": {"value": val}}]}
            try:
                r = qdrant("POST", f"/collections/{TARGET_COLLECTION}/points/count", {"filter": filt, "exact": True})
                n = r.get("result", {}).get("count", 0)
                if n > 0:
                    print(f"    {val:<20} {n}")
            except Exception:
                pass
        print()


def main() -> int:
    parser = argparse.ArgumentParser(description="Query fp_reflections — semantic search over Mark's reflected memory")
    parser.add_argument("query",    nargs="?", default="", help="Semantic search query (empty = filter-only scroll)")
    parser.add_argument("--limit",  type=int, default=10)
    parser.add_argument("--phase",  default=None, choices=["dormancy","crack","flood","embodiment","teacher","integration","settled","unknown"])
    parser.add_argument("--depth",  default=None, choices=["shallow","moderate","deep","profound"])
    parser.add_argument("--speaker",default=None, choices=["user","assistant"])
    parser.add_argument("--contradiction", action="store_true", help="Show only contradiction-flagged reflections")
    parser.add_argument("--stats",  action="store_true", help="Show collection stats and exit")
    args = parser.parse_args()

    if args.stats:
        show_stats()
        return 0

    filters = build_filter(args.phase, args.depth, args.speaker, args.contradiction)

    query = (args.query or "").strip()

    if query:
        print(f"\n[search] '{query}'  limit={args.limit}")
        if filters:
            print(f"[filter] {json.dumps(filters)}")
        results = search(query, args.limit, filters)
        for r in results:
            print_result(r, score=r.get("score"))
    else:
        print(f"\n[scroll] top {args.limit} by confidence")
        if filters:
            print(f"[filter] {json.dumps(filters)}")
        results = scroll_filter(args.limit, filters)
        for r in results:
            print_result(r)

    print(f"\n{'─'*60}")
    print(f"  {len(results)} results\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
