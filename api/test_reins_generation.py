"""U6b-stage — the Python generation verifier (parity with Go internal/generation.Verify)."""

import hashlib
import json
import os


def _stage(root, sha, binary=b"BIN", api_files=None):
    """Write a minimal generation the way internal/generation.Store.Stage does (binary + api/*.py +
    a manifest binding the framed byte-hashes), so verify_generation can validate it."""
    import reins_generation as rg

    api_files = api_files or {"reins_serve.py": b"serve", "reins_read.py": b"read"}
    d = os.path.join(root, "generations", sha)
    api_dir = os.path.join(d, "api")
    os.makedirs(api_dir, exist_ok=True)
    with open(os.path.join(d, "reins"), "wb") as f:
        f.write(binary)
    for name, content in api_files.items():
        with open(os.path.join(api_dir, name), "wb") as f:
            f.write(content)
    h = hashlib.sha256()
    for name in sorted(api_files):
        rg._framed(h, name.encode())
        rg._framed(h, api_files[name])
    manifest = {"sha": sha, "binary_sha256": hashlib.sha256(binary).hexdigest(),
                "api_tree_sha256": h.hexdigest(), "created": "t", "prev": ""}
    with open(os.path.join(d, "manifest.json"), "w") as f:
        json.dump(manifest, f)


def test_verify_generation_ok(tmp_path):
    import reins_generation as rg

    _stage(str(tmp_path), "sha-a")
    ok, reason = rg.verify_generation("sha-a", root=str(tmp_path))
    assert ok is True and reason == "verified"


def test_verify_missing_generation_refused(tmp_path):
    import reins_generation as rg

    ok, reason = rg.verify_generation("nope", root=str(tmp_path))
    assert ok is False and "absent" in reason


def test_verify_empty_sha_refused(tmp_path):
    import reins_generation as rg

    ok, reason = rg.verify_generation("", root=str(tmp_path))
    assert ok is False and "empty" in reason


def test_verify_tampered_binary_refused(tmp_path):
    import reins_generation as rg

    _stage(str(tmp_path), "sha-a")
    with open(os.path.join(str(tmp_path), "generations", "sha-a", "reins"), "wb") as f:
        f.write(b"EVIL")
    ok, reason = rg.verify_generation("sha-a", root=str(tmp_path))
    assert ok is False and "binary hash mismatch" in reason


def test_verify_tampered_api_tree_refused(tmp_path):
    import reins_generation as rg

    _stage(str(tmp_path), "sha-a")
    with open(os.path.join(str(tmp_path), "generations", "sha-a", "api", "reins_serve.py"), "wb") as f:
        f.write(b"mutated")
    ok, reason = rg.verify_generation("sha-a", root=str(tmp_path))
    assert ok is False and "api-tree hash mismatch" in reason


def test_verify_quarantined_refused(tmp_path):
    import reins_generation as rg

    _stage(str(tmp_path), "sha-a")
    qdir = os.path.join(str(tmp_path), "quarantine")
    os.makedirs(qdir, exist_ok=True)
    with open(os.path.join(qdir, "sha-a"), "w") as f:
        f.write("bad")
    ok, reason = rg.verify_generation("sha-a", root=str(tmp_path))
    assert ok is False and "quarantined" in reason


def test_verify_ignores_pycache_pollution(tmp_path):
    # parity with the Go fix: __pycache__/.pyc/lockfiles are NOT part of the canonical .py set, so
    # interpreter pollution must not fail verify.
    import reins_generation as rg

    _stage(str(tmp_path), "sha-a")
    api_dir = os.path.join(str(tmp_path), "generations", "sha-a", "api")
    os.makedirs(os.path.join(api_dir, "__pycache__"), exist_ok=True)
    with open(os.path.join(api_dir, "__pycache__", "x.pyc"), "wb") as f:
        f.write(b"bytecode")
    with open(os.path.join(api_dir, "uv.lock"), "w") as f:
        f.write("lock")
    ok, reason = rg.verify_generation("sha-a", root=str(tmp_path))
    assert ok is True, f"pycache/lockfile pollution must not fail verify: {reason}"
