"""Engine/instance separation guard (G9, benchmark child cc-task-reins-engine-instance-guard).

The ENGINE (internal/, cmd/, api/, scripts/, deck/) must carry no baked operator-absolute home paths —
instance location comes from config/env (config.toml, REINS_* env). A `/home/<user>` literal in engine
source couples the engine to one operator's filesystem and breaks the kit/packaging telos ("ship a kit";
engine vs instance is a hard invariant per the directional-signals digest B1/B4).

Machine-checkable: the engine guard exempts docs, fixtures, and tests. The separate public-document
guard covers Markdown everywhere and text under docs/, including witness records; unreadable inputs
fail the check. The example config is checked separately for ABSOLUTE (non-~) paths.

NOTE (org-architecture scout wf_0ad18193, 2026-07-02): the path-literal guard is NOT the whole story. The
scout verified reins/api is NOT actually shippable — it sys.path.insert(council_root)s and imports council's
`shared.*` substrate IN-PROCESS while pyproject declares only fastapi+uvicorn. A path-portable
`from shared.X` passes the leak check clean, so the guard was BLIND to the real packagability blocker. The
coupling-ledger test below makes it honest: it pins the council coupling to the known set (fails on any NEW
cross-repo import) until the fix — extracting those modules as a published `hapax-spine` wheel that reins
declares as a real versioned dependency (Phase 1 of the repository-architecture report).
"""
from __future__ import annotations

import os
import re
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parent.parent
ENGINE_DIRS = ("internal", "cmd", "api", "scripts", "deck")
EXEMPT = re.compile(r"(_test\.go|test_[^/]+\.py|\.md)$")
LEAK = re.compile(r"/home/[a-z_][a-z0-9_-]*")


def _public_text_files():
    excluded = {".git", ".venv", "node_modules", "__pycache__", ".ruff_cache", ".pytest_cache"}
    extensions = {".md", ".json", ".jsonl", ".toml", ".txt", ".go", ".py", ".sh"}

    def refuse(error):
        raise error

    for directory, dirs, names in os.walk(REPO, onerror=refuse):
        dirs[:] = [name for name in dirs if name not in excluded]
        for name in names:
            path = Path(directory) / name
            relative = path.relative_to(REPO)
            suffix = path.suffix.lower()
            if suffix == ".md" or (relative.parts[0] == "docs" and suffix in extensions):
                yield path


def test_repo_docs_and_markdown_carry_no_doxxing_home_paths():
    leaks = []
    for path in _public_text_files():
        for line, text in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            if LEAK.search(text):
                leaks.append(f"{path.relative_to(REPO)}:{line}")
    assert not leaks, "public document home path(s); use portable placeholders:\n" + "\n".join(leaks)


def _engine_files():
    for d in ENGINE_DIRS:
        root = REPO / d
        if not root.is_dir():
            continue
        for p in root.rglob("*"):
            rel = p.relative_to(REPO)
            if any(part.startswith(".") for part in rel.parts):
                continue  # build artifacts (.venv, .ruff_cache, …) are not engine source
            if p.is_file() and p.suffix in (".go", ".py", ".sh", ".toml", ".json", "") and not EXEMPT.search(str(p)):
                yield p


def test_engine_carries_no_baked_home_paths():
    leaks = []
    for p in _engine_files():
        try:
            text = p.read_text(errors="ignore")
        except OSError:
            continue
        for i, line in enumerate(text.splitlines(), 1):
            if LEAK.search(line):
                leaks.append(f"{p.relative_to(REPO)}:{i}: {line.strip()[:100]}")
    assert not leaks, "engine/instance leak — baked home path(s) in engine source:\n" + "\n".join(leaks)


def test_engine_carries_no_baked_axiom_vocabulary():
    """Invariant I5 (engine/instance clean + axiom-agnostic): the ENGINE carries ZERO hardcoded instance
    axiom vocabulary. Instance-specific axiom identifiers belong to INSTANCE config, not engine source,
    so separate deployments can declare their own axiom sets without engine branches.
    Docs/fixtures/tests are exempt; engine SOURCE must not branch on instance axiom names.
    Fail-closed on any new leak — the engine is clean today; this LOCKS it."""
    legacy_axiom = re.compile("single[_-]" + "user", re.IGNORECASE)
    leaks = []
    for p in _engine_files():
        try:
            text = p.read_text(errors="ignore")
        except OSError:
            continue
        for i, line in enumerate(text.splitlines(), 1):
            if legacy_axiom.search(line):
                leaks.append(f"{p.relative_to(REPO)}:{i}: {line.strip()[:100]}")
    assert not leaks, (
        "engine/instance leak — hardcoded instance axiom identifier in engine source. The engine must "
        "be axiom-agnostic; axiom identifiers belong to instance config:\n" + "\n".join(leaks)
    )


def test_reins_api_is_decoupled_from_council_substrate():
    """RESOLVED (was the coupling-debt ledger): reins/api now imports the SDLC-runtime mechanism from the
    published `hapax-spine` WHEEL (`from hapax.spine.*`), NOT council's `shared.*` in-process. The
    sys.path.insert(council_root) hack is gone; reins is a shippable kit. This test now GUARDS the decoupling
    — any regression back to `from shared.X` or the sys.path hack fails."""
    src = (REPO / "api" / "reins_read.py").read_text()
    imported = set(re.findall(r"from shared\.(\w+) import", src))
    assert not imported, (
        "reins/api must import NOTHING from council's shared.* — it depends on the hapax-spine wheel now. "
        "Regressed imports: " + ", ".join(sorted(imported))
    )
    assert "sys.path.insert" not in src, "the council_root sys.path.insert hack must stay deleted"
    reqs = (REPO / "api" / "requirements.txt")
    if reqs.exists():
        assert "hapax-spine" in reqs.read_text().lower(), (
            "reins/api/requirements.txt must declare the hapax-spine wheel dependency"
        )


def test_example_config_paths_are_portable():
    # the EXAMPLE instance config may name instance paths, but only in portable ~-form — an absolute
    # /home/<user> path in the example ships one operator's filesystem as the default.
    example = REPO / "config.example.toml"
    if not example.exists():
        return
    bad = [f"line {i}: {ln.strip()[:100]}"
           for i, ln in enumerate(example.read_text().splitlines(), 1)
           if LEAK.search(ln)]
    assert not bad, "config.example.toml carries absolute home paths (use ~-form):\n" + "\n".join(bad)


@pytest.mark.parametrize("relative", [
    "README.md", "nested/NOTES.md", ".github/HELP.md",
    *["docs/releases/witness" + ext for ext in
      (".md", ".json", ".jsonl", ".toml", ".txt", ".go", ".py", ".sh")],
])
def test_doxxing_scan_catches_planted_home_paths(tmp_path, monkeypatch, relative):
    monkeypatch.setitem(globals(), "REPO", tmp_path)
    target = tmp_path / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text("safe first line\n" + "/home/" + "synthetic_user/file\n")
    with pytest.raises(AssertionError) as caught:
        test_repo_docs_and_markdown_carry_no_doxxing_home_paths()
    assert relative + ":2" in str(caught.value)
    assert "synthetic_user" not in str(caught.value)


def test_doxxing_scan_spares_placeholders_and_build_artifacts(tmp_path, monkeypatch):
    monkeypatch.setitem(globals(), "REPO", tmp_path)
    (tmp_path / "README.md").write_text("Use /home/<user>/files or ~/files.\n")
    for directory in (".git", ".venv", "node_modules", "__pycache__", ".ruff_cache", ".pytest_cache"):
        target = tmp_path / "docs" / directory / "generated.md"
        target.parent.mkdir(parents=True)
        target.write_text("/home/" + "synthetic_build/path")
    test_repo_docs_and_markdown_carry_no_doxxing_home_paths()


def test_doxxing_scan_unreadable_document_refuses(tmp_path, monkeypatch):
    monkeypatch.setitem(globals(), "REPO", tmp_path)
    target = tmp_path / "README.md"
    target.write_text("safe")
    read = Path.read_text
    def unreadable(path, *args, **kwargs):
        if path == target:
            raise PermissionError("synthetic unreadable document")
        return read(path, *args, **kwargs)
    monkeypatch.setattr(Path, "read_text", unreadable)
    with pytest.raises(PermissionError, match="synthetic unreadable"):
        test_repo_docs_and_markdown_carry_no_doxxing_home_paths()


def test_doxxing_scan_unreadable_directory_refuses(tmp_path, monkeypatch):
    monkeypatch.setitem(globals(), "REPO", tmp_path)
    directory = tmp_path / "docs"
    directory.mkdir()
    scan = os.scandir

    def unreadable(path):
        if Path(path) == directory:
            raise PermissionError("synthetic unreadable directory")
        return scan(path)

    monkeypatch.setattr(os, "scandir", unreadable)
    with pytest.raises(PermissionError, match="synthetic unreadable directory"):
        test_repo_docs_and_markdown_carry_no_doxxing_home_paths()
