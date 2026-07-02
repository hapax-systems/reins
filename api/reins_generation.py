"""reins_generation — Python-side reader/verifier for the hot-plug generation store (U6b-stage).

Mirrors internal/generation.Store.Verify so the governed `stage` verb can validate a target generation
against its manifest with the SAME byte-binding contract the Go cockpit uses. READ-ONLY: never writes the
store, never flips a pointer (staging != swapping — the swap is U6b-swap).

The api-tree hash is over the canonical {top-level *.py} set, length-framed, sorted by name — identical
construction to internal/generation.APITreeHash and reins_serve.api_tree_sha (all three pinned to one
cross-language parity constant by their tests). _framed is duplicated here (3 lines) rather than imported
from reins_serve to avoid an import cycle; the parity constant is the actual guarantee.
"""

from __future__ import annotations

import hashlib
import json
import os


def _framed(h, b: bytes) -> None:
    # 8-byte big-endian length prefix + bytes — MUST match reins_serve._framed and Go writeFramed.
    h.update(len(b).to_bytes(8, "big"))
    h.update(b)


def store_root() -> str:
    env = os.environ.get("REINS_GENERATION_ROOT", "").strip()
    if env:
        return env
    return os.path.join(os.path.expanduser("~"), ".local", "share", "reins")


def _gen_dir(root: str, sha: str) -> str:
    return os.path.join(root, "generations", sha)


def _api_tree_hash(api_dir: str) -> str:
    """Full framed digest over {top-level *.py} (non-recursive; __pycache__/.pyc/lockfiles ignored).
    Skips DIRECTORIES (mirrors Go readAPITree's e.IsDir() skip) so a `*.py`-named subdir cannot raise."""
    h = hashlib.sha256()
    for name in sorted(os.listdir(api_dir)):
        full = os.path.join(api_dir, name)
        if not name.endswith(".py") or os.path.isdir(full):
            continue
        with open(full, "rb") as f:
            _framed(h, name.encode())
            _framed(h, f.read())
    return h.hexdigest()


def verify_generation(sha: str, root: str | None = None) -> tuple[bool, str]:
    """Return (ok, reason). ok=True iff the generation exists, is not quarantined, has a manifest, and its
    on-disk bytes (binary sha256 + api-tree framed hash) recompute to the manifest — the Go Verify contract.
    A bare/empty sha, a missing/tampered generation, or a quarantined one => (False, honest reason)."""
    if not sha or not sha.strip():
        return False, "empty generation sha"
    root = root or store_root()
    if os.path.exists(os.path.join(root, "quarantine", sha)):
        return False, f"generation {sha} is quarantined"
    d = _gen_dir(root, sha)
    mp = os.path.join(d, "manifest.json")
    if not os.path.exists(mp):
        return False, f"generation {sha} absent (no manifest)"
    try:
        with open(mp) as f:
            m = json.load(f)
    except (OSError, ValueError) as e:
        return False, f"generation {sha} manifest corrupt: {e}"
    if not isinstance(m, dict):
        # valid JSON but not an object (null/true/42/"s"/[...]) — a typed refusal, never an
        # uncaught AttributeError on m.get -> 500 + an un-witnessed verdict.
        return False, f"generation {sha} manifest is not an object"
    try:
        with open(os.path.join(d, "reins"), "rb") as f:
            binary = f.read()
    except OSError:
        return False, f"generation {sha} binary unreadable"
    if hashlib.sha256(binary).hexdigest() != m.get("binary_sha256"):
        return False, f"generation {sha} binary hash mismatch"
    api_dir = os.path.join(d, "api")
    if not os.path.isdir(api_dir):
        return False, f"generation {sha} api tree missing"
    try:
        tree_hash = _api_tree_hash(api_dir)
    except OSError as e:
        # any read error recomputing the tree is a typed refusal, never an uncaught 500.
        return False, f"generation {sha} api tree unreadable: {e}"
    if tree_hash != m.get("api_tree_sha256"):
        return False, f"generation {sha} api-tree hash mismatch"
    return True, "verified"
