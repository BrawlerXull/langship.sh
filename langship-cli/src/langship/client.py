"""Thin httpx wrapper around the Langship API.

The Langship server exposes its routes under /api/... (see pkg/api). All
methods here prepend nothing — callers pass the full path, e.g.
client.get("/api/agents").
"""

from __future__ import annotations

from typing import Any, Iterator, Optional

import httpx

from . import config


class APIError(Exception):
    def __init__(self, status: int, message: str, body: Any = None):
        self.status = status
        self.message = message
        self.body = body
        super().__init__(f"{status} {message}")


def _client(timeout: float = 30.0) -> httpx.Client:
    headers = {"Content-Type": "application/json"}
    tok = config.token()
    if tok:
        headers["Authorization"] = f"Bearer {tok}"
    return httpx.Client(base_url=config.api_url(), timeout=timeout, headers=headers)


def _raise_for(resp: httpx.Response) -> None:
    if resp.is_success:
        return
    try:
        body = resp.json()
        msg = body.get("error") or body.get("message") or resp.text
    except Exception:
        body = resp.text
        msg = resp.text or resp.reason_phrase
    raise APIError(resp.status_code, msg, body)


def get(path: str, **kwargs: Any) -> Any:
    with _client() as c:
        r = c.get(path, **kwargs)
    _raise_for(r)
    return r.json() if r.content else None


def post(path: str, json: Optional[dict] = None, **kwargs: Any) -> Any:
    with _client() as c:
        r = c.post(path, json=json or {}, **kwargs)
    _raise_for(r)
    return r.json() if r.content else None


def put(path: str, json: Optional[dict] = None, **kwargs: Any) -> Any:
    with _client() as c:
        r = c.put(path, json=json or {}, **kwargs)
    _raise_for(r)
    return r.json() if r.content else None


def patch(path: str, json: Optional[dict] = None, **kwargs: Any) -> Any:
    with _client() as c:
        r = c.patch(path, json=json or {}, **kwargs)
    _raise_for(r)
    return r.json() if r.content else None


def delete(path: str, **kwargs: Any) -> Any:
    with _client() as c:
        r = c.delete(path, **kwargs)
    _raise_for(r)
    return r.json() if r.content else None


def stream_sse(path: str, timeout: float = 600.0) -> Iterator[dict]:
    """Yield parsed `data:` JSON objects from an SSE endpoint until the
    server closes the stream (e.g. /api/executions/{id}/stream). Non-JSON
    data lines yield {"raw": "..."}.
    """
    headers = {"Accept": "text/event-stream"}
    tok = config.token()
    if tok:
        headers["Authorization"] = f"Bearer {tok}"
    import json as _json

    with httpx.Client(base_url=config.api_url(), timeout=timeout, headers=headers) as c:
        with c.stream("GET", path) as r:
            _raise_for(r)
            for line in r.iter_lines():
                if not line or not line.startswith("data:"):
                    continue
                payload = line[len("data:"):].strip()
                if not payload:
                    continue
                try:
                    yield _json.loads(payload)
                except Exception:
                    yield {"raw": payload}
