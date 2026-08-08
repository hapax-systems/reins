"""R2.12 — the support boundary: ratified closing act, refusal-shaped dead-ends."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import append_receipt, genesis_self_attest, verify_chain_at
from k0.support_boundary import (
    RefusalCard,
    SupportBoundary,
    SupportBoundaryError,
    accept,
    dead_end,
    present,
    ratified_boundary,
)

ESTATE = "estate-0000000000000000"
KERNEL = "k0-test"
BOUNDARY = SupportBoundary(
    in_scope=("install", "ceremony", "degradation-ledger"),
    out_scope=("custom-consulting",),
    answer_surface="docs/SUPPORT.md",
)


def _key(tmp_path: Path) -> Path:
    key = tmp_path / "ratifier_ed25519"
    subprocess.run(
        ["ssh-keygen", "-t", "ed25519", "-N", "", "-C", "ratifier@test", "-f", str(key)],
        check=True,
        capture_output=True,
    )
    return key


def _root(tmp_path: Path) -> Path:
    root = tmp_path / "root"
    root.mkdir()
    append_receipt(
        root,
        genesis_self_attest(
            estate_id=ESTATE,
            kernel_version=KERNEL,
            kernel_manifest_sha256="a" * 64,
            observed_at=datetime.now(UTC) - timedelta(days=365),
        ),
    )
    return root


def test_the_boundary_is_ratified_and_read_back(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    assert ratified_boundary(root, allow_unauthenticated=True) is None

    present(root, BOUNDARY, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, BOUNDARY, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    got = ratified_boundary(root, allow_unauthenticated=True)
    assert set(got.in_scope) == set(BOUNDARY.in_scope)
    assert got.answer_surface == "docs/SUPPORT.md"
    assert verify_chain_at(root).ok


def test_the_hash_only_read_requires_the_opt_in(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    present(root, BOUNDARY, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, BOUNDARY, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(SupportBoundaryError, match="allow_unauthenticated"):
        ratified_boundary(root)


def test_dead_ends_are_refusal_shaped_and_contactless(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    present(root, BOUNDARY, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, BOUNDARY, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    card = dead_end(root, "custom-consulting")
    assert card.determination == "out-of-scope"
    assert card.pointer == "docs/SUPPORT.md"
    assert "verify_chain" in card.self_diagnosis

    assert dead_end(root, "unlisted-thing").determination == "unenumerated"
    assert dead_end(root, "ceremony").determination == "in-scope"

    # The shape has no contact channel: no field names one, and the card carries none.
    import dataclasses

    fields = {f.name for f in dataclasses.fields(RefusalCard)}
    assert not any(
        word in name for name in fields for word in ("contact", "email", "chat", "channel", "issue")
    ), "a contact channel in the shape would make 'never' unenforceable"


def test_dead_end_before_any_boundary_is_honest_dark(tmp_path: Path) -> None:
    root = _root(tmp_path)
    card = dead_end(root, "anything")
    assert card.determination == "unenumerated"
    assert "dark" in card.pointer


def test_the_boundary_shape_laws(tmp_path: Path) -> None:
    with pytest.raises(ValueError, match="nothing in scope"):
        SupportBoundary((), ("xa",), "docs/SUPPORT.md")
    with pytest.raises(ValueError, match="both in and out"):
        SupportBoundary(("xa",), ("xa",), "docs/SUPPORT.md")
    with pytest.raises(ValueError, match="lowercase kebab"):
        SupportBoundary(("Not A Topic",), (), "docs/SUPPORT.md")
