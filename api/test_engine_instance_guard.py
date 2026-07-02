"""Engine/instance separation guard (G9, benchmark child cc-task-reins-engine-instance-guard).

The ENGINE (internal/, cmd/, api/, scripts/, deck/) must carry no baked operator-absolute home paths —
instance location comes from config/env (config.toml, REINS_* env). A `/home/<user>` literal in engine
source couples the engine to one operator's filesystem and breaks the kit/packaging telos ("ship a kit";
engine vs instance is a hard invariant per the directional-signals digest B1/B4).

Machine-checkable, fail-closed: this test FAILS on any new leak. Docs, fixtures, and tests are exempt
(they may cite instance examples); the example config is checked separately for ABSOLUTE (non-~) paths.
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
