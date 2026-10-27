#!/usr/bin/env python3
"""FrontPocket stable-id migration helper.

Dry-run (default, strictly read-only): scan a collection for legacy
`{conversation_id}_turn_{NNN}_chunk_{NNN}` memory_ids, replay the original
ChatGPT/Claude export archives to recover each chunk's source message id, and
report what share of legacy points can be re-keyed to the stable
`{conversation_id}_msg_{message_id}_chunk_{NNN}` form. Optionally emit an
old,new CSV map. No writes.

Apply (demo only): copy N legacy conversations WITH VECTORS from --source into
--dest (must be fp_fix_*), migrate IDs inside the copy, verify, emit map.
NEVER touches frontpocket_memory, fp_cleaned_memory or fp_reflections.
"""
import argparse
import csv
import glob
import hashlib
import json
import os
import re
import sys
import urllib.request

PROD_BLOCK = {"frontpocket_memory", "fp_cleaned_memory", "fp_reflections"}
LEGACY_RE = re.compile(r"^(.*)_turn_(\d+)_chunk_(\d+)$")


def qdrant_url() -> str:
    env = os.environ.get("QDRANT_URL", "http://localhost:6333").strip()
    return (env or "http://localhost:6333").rstrip("/")


def http_json(method: str, path: str, payload=None):
    data = None
    headers = {}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(qdrant_url() + path, data=data, method=method, headers=headers)
    with urllib.request.urlopen(req) as resp:
        body = resp.read().decode("utf-8")
    return json.loads(body) if body else {}


def qdrant_point_id(memory_id: str) -> str:
    """Mirror Go qdrantPointID: md5 -> UUID-shaped hex (version/variant nibbles)."""
    h = bytearray(hashlib.md5(memory_id.encode("utf-8")).digest())
    h[6] = (h[6] & 0x0F) | 0x50
    h[8] = (h[8] & 0x3F) | 0x80
    s = h.hex()
    return f"{s[0:8]}-{s[8:12]}-{s[12:16]}-{s[16:20]}-{s[20:32]}"


def norm(text: str) -> str:
    return " ".join((text or "").split()).strip()


def collection_dims(name: str):
    info = http_json("GET", f"/collections/{name}")
    vec = info["result"]["config"]["params"]["vectors"]
    return vec["size"], vec.get("distance", "Cosine")


def scroll_points(collection, payload, with_vector=False, page=256, offset=None, upto=None):
    out = []
    while True:
        body = {"limit": page, "with_payload": payload, "with_vector": with_vector}
        if offset is not None:
            body["offset"] = offset
        data = http_json("POST", f"/collections/{collection}/points/scroll", body)
        res = data.get("result", {})
        pts = res.get("points", [])
        out.extend(pts)
        offset = res.get("next_page_offset")
        if offset is None or (upto is not None and len(out) >= upto):
            break
    return out


def upsert_batch(collection, points):
    for i in range(0, len(points), 32):
        http_json("PUT", f"/collections/{collection}/points?wait=true", {"points": points[i:i + 32]})


def delete_batch(collection, ids):
    for i in range(0, len(ids), 64):
        http_json("POST", f"/collections/{collection}/points/delete?wait=true", {"points": ids[i:i + 64]})


def count_exact(collection):
    return http_json("POST", f"/collections/{collection}/points/count", {"exact": True})["result"]["count"]


def load_archive_for(archive_paths, wanted_cids):
    """Parse ChatGPT/Claude export files; return conv_id -> ordered [(msg_id, text)].

    Streams file-by-file and stops early once every wanted conversation is found.
    wanted_cids=None loads everything (slower; used only for capped dry-runs).
    """
    found = {}
    wanted = None if wanted_cids is None else set(wanted_cids)
    for path in sorted(archive_paths):
        try:
            with open(path, "r", encoding="utf-8") as f:
                data = json.load(f)
        except (json.JSONDecodeError, OSError):
            continue
        convs = []
        if isinstance(data, list):
            convs = data
        elif isinstance(data, dict):
            if "mapping" in data:
                convs = [data]
            elif isinstance(data.get("conversations"), list):
                convs = data["conversations"]
            elif isinstance(data.get("sessions"), list):
                for sess in data["sessions"]:
                    cid = sess.get("id") or sess.get("session_id") or os.path.basename(path)
                    msgs = []
                    leaves = sess.get("leaves") or []
                    for leaf in leaves:
                        chain = leaf.get("message_chain") or leaf.get("messages") or []
                        for m in chain:
                            mid = m.get("uuid") or m.get("id") or ""
                            content = m.get("content") or m.get("text") or ""
                            if isinstance(content, dict):
                                content = content.get("text", "")
                            msgs.append((str(mid), str(content)))
                    if cid:
                        convs.append({"id": cid, "_claude_msgs": msgs})
        for conv in convs:
            cid = str(conv.get("conversation_id") or conv.get("id") or "").strip()
            if not cid:
                continue
            if wanted is not None and cid in found:
                continue
            if "_claude_msgs" in conv:
                found[cid] = conv["_claude_msgs"]
                continue
            mapping = conv.get("mapping")
            if not isinstance(mapping, dict):
                continue
            # DFS order over the conversation tree; message id = node.id == message.id
            nodes = {k: v for k, v in mapping.items() if isinstance(v, dict)}
            parent_map, children = {}, {}
            for nid, node in nodes.items():
                parent = node.get("parent")
                parent_map[nid] = parent
                if parent:
                    children.setdefault(parent, []).append(nid)
            roots = [nid for nid, p in parent_map.items() if not p or p not in nodes]
            ordered, seen, stack = [], set(), list(roots)
            while stack:
                nid = stack.pop(0)
                if nid in seen:
                    continue
                seen.add(nid)
                node = nodes.get(nid, {})
                msg = node.get("message") or {}
                if isinstance(msg, dict):
                    mid = str(msg.get("id") or nid)
                    content = msg.get("content") or {}
                    text = ""
                    if isinstance(content, dict):
                        parts = content.get("parts") or []
                        text = "\n".join(str(p) for p in parts if isinstance(p, str))
                    elif isinstance(content, str):
                        text = content
                    if text.strip():
                        ordered.append((mid, text))
                stack = children.get(nid, []) + stack
            if ordered:
                found[cid] = ordered
        if wanted is not None and wanted.issubset(found.keys()):
            break
    return found


def build_msgs_index(ordered):
    idx = {}
    for mid, text in ordered:
        n = norm(text)
        if not n:
            continue
        idx.setdefault(n, mid)
        idx.setdefault("P::" + n[:100], mid)
        idx.setdefault("T::" + n[-100:], mid)
        if len(n) > 1400:  # chunked body: index every 1400-char slice start
            start = 0
            while start < len(n):
                idx.setdefault("C::" + n[start:start + 100], mid)
                start += 1400
    return idx


def match_chunk(chunk_norm, idx, ordered_norm=None):
    if not chunk_norm:
        return ""
    for key in (chunk_norm, "P::" + chunk_norm[:100], "C::" + chunk_norm[:100]):
        mid = idx.get(key)
        if mid:
            return mid
    for key, mid in idx.items():
        if key.startswith(("P::", "C::")) and chunk_norm.startswith(key[3:]):
            return mid
    # fallback: middle/end chunks are slices of a message body — containment scan
    if ordered_norm:
        for mid, body in ordered_norm:
            if len(chunk_norm) >= 40 and chunk_norm in body:
                return mid
    return ""


def scan_legacy(collection, max_points=None):
    payload = ["memory_id", "conversation_id", "text"]
    pts = scroll_points(collection, payload, upto=max_points)
    legacy = []
    for p in pts:
        pl = p.get("payload") or {}
        mid = str(pl.get("memory_id", ""))
        m = LEGACY_RE.match(mid)
        if not m or "_msg_" in mid:
            continue
        legacy.append({"point_id": p.get("id"), "memory_id": mid, "conv": m.group(1),
                       "turn": int(m.group(2)), "chunk": int(m.group(3)),
                       "text": str(pl.get("text", ""))})
    by_conv = {}
    for row in legacy:
        by_conv.setdefault(row["conv"], []).append(row)
    return legacy, by_conv


def plan_migration(by_conv, archive_index, emit=None):
    rows, matched, unmatched = [], 0, 0
    for cid, pts in by_conv.items():
        ordered = archive_index.get(cid)
        idx = build_msgs_index(ordered) if ordered else {}
        ordered_norm = [(mid, norm(text)) for mid, text in (ordered or [])] if ordered else None
        for row in sorted(pts, key=lambda r: (r["turn"], r["chunk"])):
            mid = match_chunk(norm(row["text"]), idx, ordered_norm) if idx else ""
            new = f"{cid}_msg_{mid}_chunk_{row['chunk']:03d}" if mid else ""
            if mid:
                matched += 1
            else:
                unmatched += 1
            rows.append({"old": row["memory_id"], "new": new, "point_id": row["point_id"],
                         "conv": cid, "match": bool(mid)})
    if emit:
        with open(emit, "w", newline="", encoding="utf-8") as f:
            w = csv.writer(f)
            w.writerow(["old_memory_id", "new_memory_id"])
            for r in rows:
                if r["new"]:
                    w.writerow([r["old"], r["new"]])
    return rows, matched, unmatched


def run_dry(args):
    legacy, by_conv = scan_legacy(args.source, max_points=args.max_points)
    total = count_exact(args.source)
    print(f"source={args.source} points={total} legacy_points={len(legacy)} "
          f"legacy_conversations={len(by_conv)} scanned<= {args.max_points} pts")
    convs = sorted(by_conv.keys())
    if args.sample_conversations:
        convs = [c for c in convs if len(by_conv[c]) <= 60][: args.sample_conversations]
        by_conv = {c: by_conv[c] for c in convs}
    archive = discover_archives(args.archive)
    index = load_archive_for(archive, wanted_cids=set(convs))
    rows, matched, unmatched = plan_migration(by_conv, index, emit=args.emit_map)
    pct = (100.0 * matched / len(rows)) if rows else 0.0
    print(f"sampled_conversations={len(convs)} legacy_chunks={len(rows)} "
          f"msg_resolvable={matched} ({pct:.1f}%) unmatched={unmatched}")
    print("resolvable=per-sampled; full share requires uncapped --sample-conversations 0 run")
    if args.emit_map:
        print(f"map_written={args.emit_map} rows={matched}")


def discover_archives(patterns):
    files = []
    for pat in patterns:
        if os.path.isdir(pat):
            for root, _dirs, names in os.walk(pat):
                files.extend(os.path.join(root, n) for n in names if n.endswith(".json"))
        else:
            files.extend(glob.glob(pat))
    return files


def run_apply(args):
    if args.source in PROD_BLOCK and not args.make_copy:
        raise SystemExit("apply against a production source requires --make-copy (demo-on-copy mode)")
    if not args.dest.startswith("fp_fix_"):
        raise SystemExit(f"--dest must be an fp_fix_* scratch collection (got {args.dest!r})")
    if not args.make_copy or args.make_copy < 1:
        raise SystemExit("--apply demo requires --make-copy N (N conversations copied to --dest)")

    # pick N legacy conversations that ARE archive-resolvable (prefer small ones)
    legacy, by_conv = scan_legacy(args.source, max_points=args.max_points)
    archive_patterns = discover_archives(args.archive)
    small = [c for c in sorted(by_conv.keys(), key=lambda c: len(by_conv[c])) if len(by_conv[c]) <= 60][:40]
    index = load_archive_for(archive_patterns, wanted_cids=set(small))
    picked = [c for c in small if c in index][: args.make_copy]
    index = {c: index[c] for c in picked}
    if not picked:
        raise SystemExit("no archive-resolvable legacy conversations found in scan window")
    print(f"picked conversations: {picked}")

    size, distance = collection_dims(args.source)
    http_json("PUT", f"/collections/{args.dest}", {"vectors": {"size": size, "distance": distance}})
    print(f"created {args.dest} ({size},{distance})")

    copied = 0
    for cid in picked:
        pts = scroll_with_filter(args.source, cid)
        upsert_batch(args.dest, [{"id": p["id"], "vector": p.get("vector"), "payload": p.get("payload", {})}
                                 for p in pts])
        copied += len(pts)
        print(f"copied conversation {cid}: {len(pts)} points (with vectors)")
    pre = count_exact(args.dest)
    print(f"copy verification: expected {copied}, collection {args.dest} count={pre}")

    # migrate inside the copy
    rows, matched, unmatched = plan_migration({c: [
        {"memory_id": p["payload"]["memory_id"], "conv": c,
         "turn": int(LEGACY_RE.match(p["payload"]["memory_id"]).group(2)) if LEGACY_RE.match(p["payload"]["memory_id"]) else 0,
         "chunk": int(LEGACY_RE.match(p["payload"]["memory_id"]).group(3)) if LEGACY_RE.match(p["payload"]["memory_id"]) else 0,
         "text": p["payload"].get("text", ""), "point_id": p["id"]}
        for p in scroll_with_filter(args.dest, c)] for c in picked}, index, emit=args.emit_map)

    # build new points from copy's stored vectors
    new_points, old_ids, collisions = [], [], 0
    seen_new = set()
    by_conv_full = {c: scroll_with_filter(args.dest, c) for c in picked}
    lookup = {p["id"]: p for c in picked for p in by_conv_full[c]}
    for r in rows:
        if not r["new"]:
            continue
        src = lookup.get(r["point_id"])
        if src is None:
            continue
        pl = dict(src.get("payload") or {})
        pl["memory_id"] = r["new"]
        mid = r["new"].split("_msg_", 1)[1].rsplit("_chunk_", 1)[0]
        pl["message_id"] = mid
        pid = qdrant_point_id(r["new"])
        if pid in seen_new:
            collisions += 1
            continue
        seen_new.add(pid)
        new_points.append({"id": pid, "vector": src.get("vector"), "payload": pl})
        old_ids.append(src["id"])
    if collisions:
        print(f"WARNING: {collisions} id collisions skipped (duplicate old->new mappings)")
    upsert_batch(args.dest, new_points)
    delete_batch(args.dest, old_ids)

    post = count_exact(args.dest)
    msg_count = sum(1 for c in picked for p in scroll_with_filter(args.dest, c)
                    if "_msg_" in str(p.get("payload", {}).get("memory_id", "")))
    print(f"migrate: matched={matched} unmatched_left_legacy={unmatched}")
    print(f"final count={post} (pre={pre}) msg_shaped={msg_count}")
    if args.emit_map:
        print(f"map_written={args.emit_map}")
    print(f"sample-verify: {verify_texts(args.dest, rows, matched)}")


def scroll_with_filter(collection, cid):
    out, offset = [], None
    while True:
        body = {"limit": 256, "with_payload": True, "with_vector": True,
                "filter": {"must": [{"key": "conversation_id", "match": {"value": cid}}]}}
        if offset is not None:
            body["offset"] = offset
        data = http_json("POST", f"/collections/{collection}/points/scroll", body)
        res = data.get("result", {})
        out.extend(res.get("points", []))
        offset = res.get("next_page_offset")
        if offset is None:
            break
    return out


def verify_texts(collection, rows, matched):
    checked = 0
    for r in rows:
        if not r["new"] or checked >= 5:
            continue
        pid = qdrant_point_id(r["new"])
        try:
            data = http_json("GET", f"/collections/{collection}/points/{pid}")
            pl = data.get("result", {}).get("payload") or {}
            if pl.get("memory_id") == r["new"] and str(pl.get("text", "")).strip():
                checked += 1
        except Exception:
            return f"FAILED at {r['new']}"
    return f"OK ({checked}/5 sampled migrated points readable with intact text)"


def parse_args():
    p = argparse.ArgumentParser(description="FrontPocket stable-id migration helper")
    p.add_argument("--source", default="frontpocket_memory")
    p.add_argument("--dest", default="fp_fix_mig_copy")
    p.add_argument("--archive", action="append",
                   default=["/home/mark/frontpocket/GPT/conversations-*.json"],
                   help="archive dir or glob (repeatable)")
    p.add_argument("--emit-map", default="")
    p.add_argument("--max-points", type=int, default=30000, help="legacy scan window cap (points)")
    p.add_argument("--sample-conversations", type=int, default=25, help="0 = all scanned")
    p.add_argument("--apply", action="store_true")
    p.add_argument("--make-copy", type=int, default=0)
    return p.parse_args()


def main():
    args = parse_args()
    if args.source in PROD_BLOCK and args.apply:
        # source prod is fine read-only; the guard is on writes (dest must be fp_fix_*)
        pass
    if args.apply:
        run_apply(args)
    else:
        run_dry(args)


if __name__ == "__main__":
    main()
