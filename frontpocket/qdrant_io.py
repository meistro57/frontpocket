from __future__ import annotations

import os
from typing import Any, Dict, Optional

import requests
from dotenv import load_dotenv

load_dotenv()

QDRANT_URL = os.getenv("QDRANT_URL", "http://localhost:6333").rstrip("/")


def qdrant(method: str, path: str, payload: Any = None, timeout: int = 30) -> Any:
    url = f"{QDRANT_URL}{path}"
    resp = requests.request(
        method,
        url,
        headers={"Content-Type": "application/json"},
        json=payload,
        timeout=timeout,
    )
    if resp.status_code >= 400:
        raise RuntimeError(f"Qdrant {method} {path} → {resp.status_code}: {resp.text[:300]}")
    if resp.text.strip() == "":
        return {}
    return resp.json()


def ensure_collection(
    name: str,
    *,
    vector_name: str = "",
    vector_size: int = 1,
    distance: str = "Cosine",
) -> None:
    try:
        qdrant("GET", f"/collections/{name}")
        return
    except RuntimeError:
        pass

    if vector_name:
        vectors: Dict[str, Any] = {vector_name: {"size": max(vector_size, 1), "distance": distance}}
    else:
        vectors = {"size": max(vector_size, 1), "distance": distance}

    qdrant("PUT", f"/collections/{name}", {"vectors": vectors})


def parse_point_vector_info(point: Dict[str, Any]) -> Dict[str, Any]:
    vector = point.get("vector")
    if vector is None:
        return {"vector_present": False, "vector_names": [], "vector_dimensions": 0, "vector_status": "missing"}
    if isinstance(vector, list):
        return {
            "vector_present": True,
            "vector_names": ["default"],
            "vector_dimensions": len(vector),
            "vector_status": "ok" if len(vector) > 0 else "empty",
        }
    if isinstance(vector, dict):
        names = sorted(vector.keys())
        dims = 0
        for value in vector.values():
            if isinstance(value, list):
                dims = max(dims, len(value))
        return {
            "vector_present": len(names) > 0,
            "vector_names": names,
            "vector_dimensions": dims,
            "vector_status": "ok" if dims > 0 else "invalid_shape",
        }
    return {"vector_present": True, "vector_names": [], "vector_dimensions": 0, "vector_status": "invalid_shape"}


def build_scroll_filter(
    *,
    project: Optional[str] = None,
    speaker: Optional[str] = None,
    since: Optional[str] = None,
    until: Optional[str] = None,
    must: Optional[list] = None,
) -> Optional[Dict[str, Any]]:
    clauses = list(must or [])
    if project and project.lower() != "all":
        clauses.append({"key": "project", "match": {"value": project}})
    if speaker:
        clauses.append({"key": "speaker", "match": {"value": speaker}})
    if since or until:
        rng: Dict[str, str] = {}
        if since:
            rng["gte"] = since
        if until:
            rng["lte"] = until
        clauses.append({"key": "timestamp", "range": rng})
    if not clauses:
        return None
    return {"must": clauses}
