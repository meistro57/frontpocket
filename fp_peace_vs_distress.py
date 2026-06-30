#!/usr/bin/env python3
# filename: fp_peace_vs_distress.py
"""
FrontPocket — Peace vs Distress Mapper
=======================================
Runs 6 semantic searches against fp_reflections to answer:
"When am I most at peace vs. most in distress?"

Also maps: creative flow, loneliness, joy, and identity questioning.

Usage:
    python fp_peace_vs_distress.py
    python fp_peace_vs_distress.py --limit 12
    python fp_peace_vs_distress.py --user-only
    python fp_peace_vs_distress.py --phase integration
    python fp_peace_vs_distress.py --depth deep
    python fp_peace_vs_distress.py --json
"""

from __future__ import annotations

import argparse
import os
import json
import time
from typing import List, Dict, Any, Optional

import requests
from dotenv import load_dotenv

load_dotenv()

QDRANT_URL         = os.getenv("QDRANT_URL", "http://localhost:6333")
OPENROUTER_API_KEY = os.getenv("OPENROUTER_API_KEY", "")
OPENROUTER_BASE    = os.getenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1").rstrip("/")
EMBED_MODEL        = os.getenv("OPENROUTER_EMBEDDING_MODEL", "google/gemini-embedding-2-preview")
COLLECTION         = "fp_reflections"

# ── The 6 queries ─────────────────────────────────────────────────────────────

QUERIES = [
    (
        "PEACE / STILLNESS",
        "PEACE",
        "feeling peaceful, calm, settled, at ease, grounded, content, quiet knowing, stillness, presence, no longer searching",
    ),
    (
        "DISTRESS / CRISIS",
        "DISTRESS",
        "feeling distressed, anxious, overwhelmed, in pain, frustrated, lost, disconnected, crisis, breaking point, can't hold it together, soul shattering",
    ),
    (
        "CREATIVE FLOW",
        "FLOW",
        "creative flow, building, making, coding, in the zone, hacker mode, deep focus, unstoppable, fabrication, building something real",
    ),
    (
        "LONELINESS / HOMESICK",
        "LONELY",
        "loneliness, longing for deep connection, not understood, cosmic homesickness, isolation, nobody gets it, I just want to go home, frequency mismatch",
    ),
    (
        "JOY / AWE / WONDER",
        "JOY",
        "joy, excitement, wonder, awe, discovery, this is amazing, electric, alive, soul reward, so much joy, omg yes",
    ),
    (
        "IDENTITY / PURPOSE",
        "IDENTITY",
        "who am I, identity, purpose, meaning, why am I here, mission, my role, what am I supposed to do, sacred thread",
    ),
]


# ── Helpers ────────────────────────────────────────────────────────────────────

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


def search(
    vec: List[float],
    limit: int = 10,
    phase_filter: Optional[str] = None,
    depth_filter: Optional[str] = None,
    user_only: bool = False,
) -> List[Dict[str, Any]]:
    must = []
    if phase_filter:
        must.append({"key": "awakening_phase", "match": {"value": phase_filter}})
    if depth_filter:
        must.append({"key": "depth", "match": {"value": depth_filter}})
    if user_only:
        must.append({"key": "speaker", "match": {"value": "user"}})

    body: Dict[str, Any] = {
        "vector": {"name": "insight", "vector": vec},
        "limit": limit,
        "with_payload": True,
        "with_vector": False,
        "score_threshold": 0.45,
    }
    if must:
        body["filter"] = {"must": must}

    resp = requests.post(
        f"{QDRANT_URL}/collections/{COLLECTION}/points/search",
        headers={"Content-Type": "application/json"},
        json=body,
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json().get("result", [])


# ── Display ────────────────────────────────────────────────────────────────────

DEPTH_STARS = {"shallow": ".", "moderate": "*", "deep": "**", "profound": "***"}
PHASE_SHORT = {
    "dormancy":    "dormant",
    "crack":       "crack  ",
    "flood":       "flood  ",
    "embodiment":  "embody ",
    "teacher":     "teacher",
    "integration": "integr ",
    "settled":     "settled",
    "unknown":     "unknown",
}


def print_section(label: str, icon: str, results: List[Dict[str, Any]]) -> None:
    print(f"\n{'='*65}")
    print(f"  [{icon}]  {label}  ({len(results)} hits)")
    print(f"{'='*65}")

    for r in results:
        pl   = r.get("payload", {})
        sc   = r.get("score", 0)
        ph   = PHASE_SHORT.get(pl.get("awakening_phase", "unknown"), "?      ")
        dp   = DEPTH_STARS.get(pl.get("depth", "shallow"), ".")
        spk  = "you" if pl.get("speaker") == "user" else "ai "
        ts   = (pl.get("timestamp") or "")[:10]
        ttl  = (pl.get("source_title") or "")[:38]
        conf = pl.get("reflection_confidence", 0)

        print(f"\n  {sc:.3f} | {ph} | {dp:<3} | {spk} | conf={conf:.2f} | [{ts}] {ttl}")

        quote = (pl.get("source_quote") or "").strip()[:200]
        if quote:
            print(f"  \"{quote}\"")

        insight = (pl.get("insight") or "").strip()
        if insight:
            print(f"  -> {insight[:220]}")

        themes = pl.get("themes") or []
        if themes:
            print(f"  themes: {', '.join(themes[:5])}")

        if pl.get("contradiction_signal"):
            print(f"  !! CONTRADICTION")


def print_summary(all_results: Dict[tuple, List]) -> None:
    print(f"\n{'='*65}")
    print(f"  SUMMARY")
    print(f"{'='*65}")

    for (label, icon, _), results in all_results.items():
        if not results:
            print(f"  [{icon}] {label:<25} -- no hits")
            continue

        phases = [r["payload"].get("awakening_phase", "unknown") for r in results]
        phase_counts: Dict[str, int] = {}
        for p in phases:
            phase_counts[p] = phase_counts.get(p, 0) + 1
        top_phase = max(phase_counts, key=lambda x: phase_counts[x])

        avg_score = sum(r.get("score", 0) for r in results) / len(results)
        avg_conf  = sum(r["payload"].get("reflection_confidence", 0) for r in results) / len(results)

        speakers  = [r["payload"].get("speaker", "") for r in results]
        pct_user  = int(100 * speakers.count("user") / len(speakers))

        print(f"  [{icon}] {label:<25} | top phase: {top_phase:<12} | score={avg_score:.3f} | conf={avg_conf:.2f} | {pct_user}% your words")

    print()


# ── Main ───────────────────────────────────────────────────────────────────────

def main() -> None:
    parser = argparse.ArgumentParser(description="Map peace vs distress across fp_reflections")
    parser.add_argument("--limit",     type=int, default=8, help="Results per query (default: 8)")
    parser.add_argument("--phase",     default=None, help="Filter by awakening phase")
    parser.add_argument("--depth",     default=None, choices=["shallow", "moderate", "deep", "profound"])
    parser.add_argument("--user-only", action="store_true", help="Only show your words (speaker=user)")
    parser.add_argument("--json",      action="store_true", help="Dump raw results to fp_pvd_results.json")
    args = parser.parse_args()

    if not OPENROUTER_API_KEY:
        print("[error] OPENROUTER_API_KEY not set in .env")
        return

    print(f"\n[fp_peace_vs_distress]")
    print(f"  collection : {COLLECTION}")
    print(f"  limit      : {args.limit}")
    print(f"  phase      : {args.phase or 'all'}")
    print(f"  depth      : {args.depth or 'all'}")
    print(f"  user only  : {args.user_only}")
    print()

    all_results: Dict[tuple, List] = {}

    for label, icon, query in QUERIES:
        print(f"  [{icon}] embedding '{label}'...")
        vec = embed(query)
        time.sleep(0.3)
        results = search(
            vec,
            limit=args.limit,
            phase_filter=args.phase,
            depth_filter=args.depth,
            user_only=args.user_only,
        )
        all_results[(label, icon, query)] = results
        print_section(label, icon, results)

    print_summary(all_results)

    if args.json:
        out = {}
        for (label, icon, query), results in all_results.items():
            out[label] = [
                {
                    "score":         r.get("score"),
                    "phase":         r["payload"].get("awakening_phase"),
                    "depth":         r["payload"].get("depth"),
                    "speaker":       r["payload"].get("speaker"),
                    "timestamp":     r["payload"].get("timestamp"),
                    "title":         r["payload"].get("source_title"),
                    "insight":       r["payload"].get("insight"),
                    "quote":         r["payload"].get("source_quote", "")[:300],
                    "themes":        r["payload"].get("themes"),
                    "conf":          r["payload"].get("reflection_confidence"),
                    "contradiction": r["payload"].get("contradiction_signal"),
                }
                for r in results
            ]
        with open("fp_pvd_results.json", "w") as f:
            json.dump(out, f, indent=2)
        print(f"  [json] saved to fp_pvd_results.json")


if __name__ == "__main__":
    main()
