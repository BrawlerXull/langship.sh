"""Local config persisted to ~/.langship/config.toml.

There is no default API URL — the CLI requires `langship login` (or the
LANGSHIP_API_URL env var) so it never silently talks to the wrong server.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Any, Optional

if sys.version_info >= (3, 11):
    import tomllib
else:  # pragma: no cover
    import tomli as tomllib  # type: ignore

import tomli_w

CONFIG_DIR = Path.home() / ".langship"
CONFIG_PATH = CONFIG_DIR / "config.toml"


def load() -> dict[str, Any]:
    if not CONFIG_PATH.exists():
        return {}
    with CONFIG_PATH.open("rb") as f:
        return tomllib.load(f)


def save(cfg: dict[str, Any]) -> None:
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)
    with CONFIG_PATH.open("wb") as f:
        tomli_w.dump(cfg, f)


def api_url_or_none() -> Optional[str]:
    """Resolve API URL: env var > config file > None."""
    return os.environ.get("LANGSHIP_API_URL") or load().get("api_url")


def api_url() -> str:
    """Resolve API URL or raise — used everywhere except `login`."""
    url = api_url_or_none()
    if not url:
        raise RuntimeError(
            "no API URL configured — run `langship login --api-url <url>` "
            "or set LANGSHIP_API_URL"
        )
    return url.rstrip("/")


def token() -> Optional[str]:
    return os.environ.get("LANGSHIP_TOKEN") or load().get("token")
