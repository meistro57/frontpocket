#!/usr/bin/env python3
"""
TurboQuant recall benchmark, extended to all quantized collections.
Pulls a real point's vector for each target, runs quantized vs exact search,
compares top-10 overlap. Run from anywhere with network access to Qdrant.
"""
import json
import urllib.request

QDRANT = "http://localhost:6333"

# (collection, vector_name or None for flat, a real point id to seed the query)
TARGETS = [
    ("misfit_reports", "claims_vec", "00011b11-9834-58af-b86a-de13573c6239"),
    ("misfit_reports", "summary_vec", "00011b11-9834-58af-b86a-de13573c6239"),
    ("meta_reflections", "claims_vec", None),
    ("meta_reflections", "summary_vec", None),
    ("mb_claims", None, None),
    ("mb_chunks", None, None),
]


def post(path, body):
    req = urllib.request.Request(
        f"{QDRANT}{path}",
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read())


def get(path):
    with urllib.request.urlopen(f"{QDRANT}{path}") as resp:
        return json.loads(resp.read())


def pick_seed_point(collection):
    """Grab any real point id from the collection to use as the query seed."""
    body = {"limit": 1, "with_payload": False, "with_vector": False}
    result = post(f"/collections/{collection}/points/scroll", body)
    return result["result"]["points"][0]["id"]


def run_benchmark(collection, vector_name, point_id):
    if point_id is None:
        point_id = pick_seed_point(collection)

    with_vector = f"?with_vector=true"
    point = get(f"/collections/{collection}/points/{point_id}{with_vector}")
    vec = point["result"]["vector"]
    vector = vec[vector_name] if vector_name else vec

    base = {"limit": 10, "with_payload": False}
    if vector_name:
        base["vector"] = {"name": vector_name, "vector": vector}
    else:
        base["vector"] = vector

    quantized = post(f"/collections/{collection}/points/search", base)
    exact = post(
        f"/collections/{collection}/points/search",
        {**base, "params": {"quantization": {"ignore": True}}, "exact": True},
    )

    q_ids = [r["id"] for r in quantized["result"]]
    e_ids = [r["id"] for r in exact["result"]]
    overlap = len(set(q_ids) & set(e_ids))
    rank_matches = sum(1 for a, b in zip(q_ids, e_ids) if a == b)

    label = f"{collection}" + (f".{vector_name}" if vector_name else "")
    print(f"\n=== {label} ===")
    print(f"seed point: {point_id}")
    print(f"overlap: {overlap}/10   exact rank matches: {rank_matches}/10")
    print(f"quantized top score: {quantized['result'][0]['score']:.4f}   exact top score: {exact['result'][0]['score']:.4f}")


def main():
    for collection, vector_name, point_id in TARGETS:
        try:
            run_benchmark(collection, vector_name, point_id)
        except Exception as e:
            print(f"\n=== {collection}.{vector_name} === FAILED: {e}")


if __name__ == "__main__":
    main()
