"""Engine/instance separation guard (G9, benchmark child cc-task-reins-engine-instance-guard).

The ENGINE (internal/, cmd/, api/, scripts/, deck/) must carry no baked operator-absolute home paths —
instance location comes from config/env (config.toml, REINS_* env). A `/home/<user>` literal in engine
source couples the engine to one operator's filesystem and breaks the kit/packaging telos ("ship a kit";
engine vs instance is a hard invariant per the directional-signals digest B1/B4).

Machine-checkable, fail-closed: this test FAILS on any new leak. Docs, fixtures, and tests are exempt
(they may cite instance examples); the example config is checked separately for ABSOLUTE (non-~) paths.

NOTE (org-architecture scout wf_0ad18193, 2026-07-02): the path-literal guard is NOT the whole story. The
scout verified reins/api is NOT actually shippable — it sys.path.insert(council_root)s and imports council's
`shared.*` substrate IN-PROCESS while pyproject declares only fastapi+uvicorn. A path-portable
`from shared.X` passes the leak check clean, so the guard was BLIND to the real packagability blocker. The
coupling-ledger test below makes it honest: it pins the council coupling to the known set (fails on any NEW
cross-repo import) until the fix — extracting those modules as a published `hapax-spine` wheel that reins
declares as a real versioned dependency (Phase 1 of the repository-architecture report).
"""
from __future__ import annotations

import re
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
ENGINE_DIRS = ("internal", "cmd", "api", "scripts", "deck")
EXEMPT = re.compile(r"(_test\.go|test_[^/]+\.py|\.md)$")
LEAK = re.compile(r"/home/[a-z_][a-z0-9_-]*")


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


def test_reins_api_council_coupling_is_pinned_not_growing():
    """HONEST coupling ledger: reins/api imports council's shared.* substrate in-process (the reins-
    packagability BLOCKER the leak-guard was blind to). This does NOT fail on the documented debt, but PINS
    it — a NEW cross-repo `from shared.X` fails, so the blocker cannot silently grow. The fix is to extract
    these modules as a published `hapax-spine` wheel and declare it in reins/api/pyproject.toml, then delete
    the sys.path.insert(council_root) hack (repository-architecture report §7, the ONE first step)."""
    src = (REPO / "api" / "reins_read.py").read_text()
    imported = set(re.findall(r"from shared\.(\w+) import", src))
    KNOWN_BLOCKER = {  # council-substrate modules reins imports in-process -> the hapax-spine extraction set
        "coord_event_log", "coord_projection", "dispatcher_policy", "langfuse_client",
        "platform_capability_receipts", "platform_capability_registry", "quota_spend_ledger",
    }
    grew = imported - KNOWN_BLOCKER
    assert not grew, (
        "NEW cross-repo council coupling in reins/api extends the packagability blocker: "
        + ", ".join(sorted(grew))
        + " — do NOT add `from shared.X`; declare a published hapax-spine wheel dependency instead."
    )
    # requirements.txt must keep disclosing the substrate coupling honestly (never claim standalone).
    reqs = (REPO / "api" / "requirements.txt")
    if reqs.exists():
        assert "shared" in reqs.read_text().lower(), (
            "reins/api/requirements.txt must disclose the council-substrate coupling (it is not standalone)"
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
