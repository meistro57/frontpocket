from __future__ import annotations

import argparse
import hashlib
import json
import re
import uuid
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Dict, Iterable, List, Optional, Tuple

from .qdrant_io import build_scroll_filter, ensure_collection, parse_point_vector_info, qdrant

RAW_COLLECTION = "frontpocket_memory"
CLEANED_COLLECTION = "fp_cleaned_memory"
CLEANUP_VERSION = "memory-cleanup-v1"

KNOWN_SPEAKERS = {"user", "assistant", "system", "tool", "mixed", "unknown"}

SPIRITUAL_KEYWORDS = {
    "awakening",
    "consciousness",
    "operation new earth",
    "higher self",
    "metaphysical",
    "ra material",
    "seth",
    "dolores cannon",
    "spiritual",
}

TECHNICAL_KEYWORDS = {
    "python",
    "database",
    "server",
    "cad",
    "inventory",
    "sql",
    "go",
    "docker",
    "script",
    "coding",
}

PROJECT_HINT_KEYWORDS = {
    "modular steel inventory": ("Steel Inventory", "source_title_keyword", 0.8),
    "steel inventory": ("Steel Inventory", "source_title_keyword", 0.75),
    "tts voices": ("TTS Voices", "source_title_keyword", 0.75),
    "audio": ("Audio Tools", "source_title_keyword", 0.6),
    "awakening infinite potential": ("Awakening Infinite Potential", "source_title_keyword", 0.9),
    "operation new earth": ("Operation New Earth", "source_title_keyword", 0.9),
}


@dataclass
class CleanupResult:
    payload: Dict[str, Any]
    vector: List[float]


def normalize_speaker(value: Any) -> Tuple[str, Optional[str]]:
    raw = (value or "").strip().lower()
    if raw in KNOWN_SPEAKERS:
        return raw, None
    if raw in {"ai", "model", "bot"}:
        return "assistant", "speaker_normalized_from_alias"
    if raw == "":
        return "unknown", "missing_speaker"
    return "unknown", "unknown_speaker"


def normalize_timestamp(value: Any) -> Tuple[Optional[str], Optional[str]]:
    raw = (value or "").strip()
    if raw == "":
        return None, "missing_timestamp"
    candidate = raw.replace("Z", "+00:00")
    try:
        dt = datetime.fromisoformat(candidate)
    except ValueError:
        return None, "malformed_timestamp"
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc).isoformat().replace("+00:00", "Z"), None


def infer_project_hint(payload: Dict[str, Any]) -> Tuple[Optional[str], str, float]:
    source_title = str(payload.get("source_title") or "").lower()
    tags = " ".join([str(t).lower() for t in payload.get("tags") or []])
    text = " ".join([source_title, tags, str(payload.get("project") or "").lower()])
    for keyword, value in PROJECT_HINT_KEYWORDS.items():
        if keyword in text:
            return value
    return None, "none", 0.0


def detect_quote_quality(quote: str) -> Tuple[str, List[str], bool]:
    warnings: List[str] = []
    chunk_boundary_warning = False
    trimmed = quote.strip()
    if trimmed == "":
        return "missing", ["missing_quote"], False
    if len(trimmed) < 20:
        warnings.append("very_short_quote")
    if trimmed.startswith("...") or trimmed.endswith("..."):
        warnings.append("ellipsis_truncation")
        chunk_boundary_warning = True
    if re.match(r"^[a-z]{2,}[^\s]", trimmed):
        warnings.append("possible_midword_start")
    if re.match(r"^[a-z]", trimmed) and len(trimmed.split()) > 2:
        warnings.append("starts_lower_fragment")
    if trimmed and trimmed[-1] not in ".!?\"'”’":
        warnings.append("ends_mid_sentence")
    if trimmed.count("(") != trimmed.count(")"):
        warnings.append("unmatched_parentheses")
    if len(trimmed.split()) < 4:
        warnings.append("low_semantic_content")

    if "missing_quote" in warnings:
        return "missing", warnings, chunk_boundary_warning
    if any(w in warnings for w in ["possible_midword_start", "unmatched_parentheses"]):
        return "malformed", warnings, chunk_boundary_warning
    if any(w in warnings for w in ["ellipsis_truncation", "ends_mid_sentence"]):
        return "truncated", warnings, chunk_boundary_warning
    if warnings:
        return "partial", warnings, chunk_boundary_warning
    return "complete", warnings, chunk_boundary_warning


def classify_phase_applicability(payload: Dict[str, Any]) -> Tuple[str, Optional[str]]:
    blob = " ".join(
        [
            str(payload.get("source_title") or "").lower(),
            str(payload.get("project") or "").lower(),
            str(payload.get("text") or "").lower(),
            " ".join([str(x).lower() for x in payload.get("themes") or []]),
        ]
    )
    has_spiritual = any(k in blob for k in SPIRITUAL_KEYWORDS)
    has_technical = any(k in blob for k in TECHNICAL_KEYWORDS)
    if has_spiritual and has_technical:
        return "uncertain", "mixed_spiritual_technical_signals"
    if has_spiritual:
        return "applicable", None
    if has_technical:
        return "not_applicable", "technical_content_detected"
    return "uncertain", "insufficient_phase_signal"


def compute_evidence_and_scope(quote: str, text: str, quote_quality: str) -> Tuple[str, str]:
    total_len = len((quote or "").strip()) + len((text or "").strip())
    if quote_quality in {"missing", "malformed"}:
        return "none", "not_enough_context"
    if total_len < 80:
        return "weak", "quote_only"
    if total_len < 240:
        return "medium", "quote_plus_context"
    return "strong", "cluster_context"


def detect_embedded_vectors(payload: Dict[str, Any]) -> Tuple[bool, int, List[str], bool]:
    vector_names: List[str] = []
    dimensions = 0
    removed = False
    for key in ["vector", "vectors"]:
        value = payload.get(key)
        if isinstance(value, list):
            vector_names.append(key)
            dimensions = max(dimensions, len(value))
            removed = True
        elif isinstance(value, dict):
            vector_names.append(key)
            for nested in value.values():
                if isinstance(nested, list):
                    dimensions = max(dimensions, len(nested))
            removed = True
    return len(vector_names) > 0, dimensions, vector_names, removed


def sanitize_payload_for_review(payload: Dict[str, Any], include_vectors: bool) -> Tuple[Dict[str, Any], bool]:
    clone = dict(payload)
    removed = False
    if include_vectors:
        return clone, removed
    for key in ["vector", "vectors"]:
        if key in clone:
            clone.pop(key, None)
            removed = True
    return clone, removed


def cleanup_status_for(blockers: List[str], quote_quality: str, repaired: bool, duplicate_of: Optional[str]) -> str:
    if duplicate_of:
        return "skipped_duplicate"
    if blockers:
        return "failed" if "corrupted_json" in blockers else "needs_review"
    if repaired:
        return "repaired"
    if quote_quality in {"partial", "truncated"}:
        return "needs_review"
    return "clean"


def clean_point(
    point: Dict[str, Any],
    *,
    seen_ids: set,
    seen_hashes: Dict[str, str],
    include_vectors: bool,
    repair_quotes: bool,
) -> CleanupResult:
    warnings: List[str] = []
    blockers: List[str] = []

    point_id = str(point.get("id") or "")
    payload = point.get("payload")
    if not isinstance(payload, dict):
        payload = {}
        blockers.append("corrupted_json")

    raw_memory_id = str(payload.get("memory_id") or point_id or "")
    source_memory_id = str(payload.get("memory_id") or raw_memory_id)
    source_point_id = point_id

    if raw_memory_id == "":
        blockers.append("missing_source_id")
    if raw_memory_id in seen_ids:
        blockers.append("duplicate_record")
    seen_ids.add(raw_memory_id)

    speaker_original = str(payload.get("speaker") or "")
    speaker_normalized, speaker_warning = normalize_speaker(speaker_original)
    if speaker_warning:
        warnings.append(speaker_warning)

    timestamp_original = str(payload.get("timestamp") or "")
    timestamp_normalized, ts_warning = normalize_timestamp(timestamp_original)
    if ts_warning:
        warnings.append(ts_warning)
        if ts_warning == "malformed_timestamp":
            blockers.append("unsupported_schema")

    source_quote_original = str(payload.get("source_quote") or "")
    source_quote_cleaned = source_quote_original.strip()
    quote_quality, quote_warnings, chunk_boundary_warning = detect_quote_quality(source_quote_cleaned)
    warnings.extend(quote_warnings)

    quote_repair_method = "none"
    quote_repair_confidence = 0.0
    repaired = False
    if repair_quotes and quote_quality in {"partial", "truncated"} and len(source_quote_cleaned) >= 20:
        source_quote_cleaned = source_quote_cleaned.replace("\n", " ").strip()
        quote_repair_method = "whitespace_normalization"
        quote_repair_confidence = 0.35
        quote_quality = "repaired"
        repaired = True

    if quote_quality in {"missing", "malformed"}:
        blockers.append("missing_quote" if quote_quality == "missing" else "malformed_quote")

    project_original = payload.get("project")
    project_original_string = str(project_original or "").strip()
    if project_original_string == "":
        project_original = None
        warnings.append("empty_project")

    source_title = str(payload.get("source_title") or "").strip() or None
    if source_title is None:
        warnings.append("missing_source_title")

    conversation_id = str(payload.get("conversation_id") or "").strip() or None
    if conversation_id is None:
        warnings.append("missing_conversation_id")

    project_hint, project_hint_basis, project_confidence = infer_project_hint(payload)

    embedded_vector_present, embedded_dims, embedded_names, embedded_removed = detect_embedded_vectors(payload)
    point_vector_info = parse_point_vector_info(point)
    vector_present = bool(embedded_vector_present or point_vector_info["vector_present"])
    vector_dimensions = max(int(embedded_dims), int(point_vector_info["vector_dimensions"]))
    vector_names = sorted(set(embedded_names + point_vector_info["vector_names"]))
    vector_status = point_vector_info["vector_status"]

    if embedded_vector_present:
        warnings.append("embedded_vectors_in_payload")

    text = str(payload.get("text") or "")
    if len(text.strip()) < 20 and len(source_quote_cleaned.strip()) < 20:
        blockers.append("insufficient_context")
    if text.strip() == "" and source_quote_cleaned.strip() == "":
        blockers.append("vector_only_payload")

    content_hash = hashlib.sha256(
        json.dumps(
            {
                "source_memory_id": source_memory_id,
                "quote": source_quote_cleaned,
                "text": text.strip(),
            },
            sort_keys=True,
        ).encode("utf-8")
    ).hexdigest()

    duplicate_of = seen_hashes.get(content_hash)
    duplicate_confidence = 0.0
    if duplicate_of:
        duplicate_confidence = 0.99
    else:
        seen_hashes[content_hash] = raw_memory_id

    dedupe_key = f"{source_memory_id}:{content_hash[:16]}"

    if duplicate_of:
        blockers.append("duplicate_record")

    phase_applicability, phase_warning = classify_phase_applicability(payload)
    if phase_warning:
        warnings.append(phase_warning)

    evidence_strength, reflection_scope = compute_evidence_and_scope(source_quote_cleaned, text, quote_quality)

    payload_cleaned, removed_for_review = sanitize_payload_for_review(payload, include_vectors)

    safe_for_reflection = len(blockers) == 0 and quote_quality not in {"missing", "malformed"}

    cleanup_id = str(uuid.uuid4())
    cleaned_id = str(uuid.uuid5(uuid.NAMESPACE_URL, f"cleaned:{raw_memory_id}:{content_hash}"))

    if any(len(str(payload.get(k, ""))) > 16000 for k in ["source_quote", "text", "summary"]):
        warnings.append("oversized_fields")

    if vector_status == "invalid_shape":
        warnings.append("invalid_vector_shape")

    cleaned_payload = {
        "id": cleaned_id,
        "cleanup_id": cleanup_id,
        "raw_memory_id": raw_memory_id,
        "source_memory_id": source_memory_id,
        "source_point_id": source_point_id,
        "conversation_id": conversation_id,
        "source_title": source_title,
        "speaker_original": speaker_original,
        "speaker_normalized": speaker_normalized,
        "timestamp_original": timestamp_original,
        "timestamp_normalized": timestamp_normalized,
        "project_original": project_original,
        "project_hint": project_hint,
        "project_hint_basis": project_hint_basis,
        "project_confidence": project_confidence,
        "source_quote_original": source_quote_original,
        "source_quote_cleaned": source_quote_cleaned,
        "source_context_before": None,
        "source_context_after": None,
        "quote_quality": quote_quality,
        "quote_repair_method": quote_repair_method,
        "quote_repair_confidence": quote_repair_confidence,
        "chunk_boundary_warning": chunk_boundary_warning,
        "payload_cleaned": payload_cleaned,
        "vector_present": vector_present,
        "vector_names": vector_names,
        "vector_dimensions": vector_dimensions,
        "vector_status": vector_status,
        "vector_removed_from_payload": bool(embedded_removed or removed_for_review),
        "content_hash": content_hash,
        "dedupe_key": dedupe_key,
        "duplicate_of": duplicate_of,
        "duplicate_confidence": duplicate_confidence,
        "awakening_phase_original": payload.get("fp_awakening_phase") or payload.get("awakening_phase"),
        "awakening_phase_cleaned": payload.get("fp_awakening_phase") or payload.get("awakening_phase"),
        "phase_applicability": phase_applicability,
        "phase_warning": phase_warning,
        "evidence_strength": evidence_strength,
        "reflection_scope": reflection_scope,
        "safe_for_reflection": safe_for_reflection,
        "reflection_blockers": sorted(set(blockers)),
        "cleanup_status": cleanup_status_for(sorted(set(blockers)), quote_quality, repaired, duplicate_of),
        "cleanup_warnings": sorted(set(warnings)),
        "cleaned_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "cleaned_by_version": CLEANUP_VERSION,
    }

    return CleanupResult(payload=cleaned_payload, vector=[0.0])


def iter_raw_points(
    *,
    batch_size: int,
    limit: int,
    project: Optional[str],
    speaker: Optional[str],
    since: Optional[str],
    until: Optional[str],
    include_vectors: bool,
) -> Iterable[Dict[str, Any]]:
    scanned = 0
    offset: Optional[Any] = None
    while True:
        body: Dict[str, Any] = {
            "limit": batch_size,
            "with_payload": True,
            "with_vector": include_vectors,
        }
        filt = build_scroll_filter(project=project, speaker=speaker, since=since, until=until)
        if filt:
            body["filter"] = filt
        if offset is not None:
            body["offset"] = offset

        result = qdrant("POST", f"/collections/{RAW_COLLECTION}/points/scroll", body)
        points = result.get("result", {}).get("points", [])
        for point in points:
            if limit > 0 and scanned >= limit:
                return
            scanned += 1
            yield point
        offset = result.get("result", {}).get("next_page_offset")
        if not offset or not points:
            return


def write_cleaned_batch(records: List[CleanupResult]) -> None:
    if not records:
        return
    points = []
    for record in records:
        points.append({"id": record.payload["id"], "vector": record.vector, "payload": record.payload})
    qdrant("PUT", f"/collections/{CLEANED_COLLECTION}/points?wait=true", {"points": points})


def summarize(results: List[CleanupResult]) -> Dict[str, int]:
    stats = {
        "total_scanned": len(results),
        "clean": 0,
        "repaired": 0,
        "needs_review": 0,
        "failed": 0,
        "skipped_duplicate": 0,
        "malformed_quotes": 0,
        "empty_project": 0,
        "vector_payloads_detected": 0,
        "safe_for_reflection": 0,
        "blocked_from_reflection": 0,
    }
    for item in results:
        payload = item.payload
        status = payload.get("cleanup_status")
        if status in stats:
            stats[status] += 1
        if payload.get("quote_quality") in {"malformed", "missing", "truncated"}:
            stats["malformed_quotes"] += 1
        if "empty_project" in (payload.get("cleanup_warnings") or []):
            stats["empty_project"] += 1
        if "embedded_vectors_in_payload" in (payload.get("cleanup_warnings") or []):
            stats["vector_payloads_detected"] += 1
        if payload.get("safe_for_reflection"):
            stats["safe_for_reflection"] += 1
        else:
            stats["blocked_from_reflection"] += 1
    return stats


def parse_args(argv: Optional[List[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="FrontPocket memory cleanup preflight loop")
    parser.add_argument("--batch-size", type=int, default=200)
    parser.add_argument("--limit", type=int, default=0)
    parser.add_argument("--project", default=None)
    parser.add_argument("--speaker", default=None)
    parser.add_argument("--since", default=None)
    parser.add_argument("--until", default=None)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--write-cleaned", action="store_true")
    parser.add_argument("--repair-quotes", action="store_true")
    parser.add_argument("--include-vectors", action="store_true")
    parser.add_argument("--include-needs-review", action="store_true")
    parser.add_argument("--only-needs-review", action="store_true")
    parser.add_argument("--verbose", action="store_true")
    parser.add_argument("--no-llm", action="store_true")
    parser.add_argument("--force", action="store_true")
    return parser.parse_args(argv)


def run(argv: Optional[List[str]] = None) -> int:
    args = parse_args(argv)
    if not args.dry_run and not args.write_cleaned:
        args.dry_run = True

    ensure_collection(CLEANED_COLLECTION, vector_size=1, distance="Cosine")

    seen_ids: set = set()
    seen_hashes: Dict[str, str] = {}
    results: List[CleanupResult] = []
    write_buffer: List[CleanupResult] = []

    for point in iter_raw_points(
        batch_size=max(1, args.batch_size),
        limit=max(0, args.limit),
        project=args.project,
        speaker=args.speaker,
        since=args.since,
        until=args.until,
        include_vectors=args.include_vectors,
    ):
        cleaned = clean_point(
            point,
            seen_ids=seen_ids,
            seen_hashes=seen_hashes,
            include_vectors=args.include_vectors,
            repair_quotes=args.repair_quotes,
        )

        if args.only_needs_review and cleaned.payload["cleanup_status"] != "needs_review":
            continue
        if not args.include_needs_review and cleaned.payload["cleanup_status"] == "needs_review":
            pass

        results.append(cleaned)
        if args.write_cleaned and not args.dry_run:
            write_buffer.append(cleaned)
            if len(write_buffer) >= max(1, args.batch_size):
                write_cleaned_batch(write_buffer)
                write_buffer = []

    if args.write_cleaned and not args.dry_run and write_buffer:
        write_cleaned_batch(write_buffer)

    stats = summarize(results)
    print(json.dumps(stats, indent=2))

    samples = [r.payload for r in results[: min(3, len(results))]]
    if samples:
        print(json.dumps(samples, indent=2))

    return 0


def main() -> int:
    return run()


if __name__ == "__main__":
    raise SystemExit(main())
