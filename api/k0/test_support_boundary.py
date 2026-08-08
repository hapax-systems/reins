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


def test_a_contact_channel_is_unrepresentable_in_the_pointer(tmp_path: Path) -> None:
    """claude r1: mailto:, URLs, chat schemes — none parse as an answer surface."""
    for channel in ("mailto:ops@example.com", "https://example.com/support", "irc://irc.example/chan"):
        with pytest.raises(ValueError, match="document path"):
            SupportBoundary(("install",), (), channel)


def test_the_refusal_branches(tmp_path: Path) -> None:
    """codex/claude r1: chain break, missing body, tampered body — each loud with its own words."""
    import json as _json

    from bootstrap_receipt import RECEIPT_CHAIN_FILENAME
    from k0.ratification import SIGNATURE_DIRNAME

    # chain break
    sub = tmp_path / "c1"
    sub.mkdir()
    root = _root(sub)
    key = _key(sub)
    present(root, BOUNDARY, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, BOUNDARY, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    chain_path = root / RECEIPT_CHAIN_FILENAME
    rows = chain_path.read_text(encoding="utf-8").splitlines()
    forged = _json.loads(rows[1])
    forged["payload_refs"] = ["stipulation:sha256:" + "f" * 64]
    rows[1] = _json.dumps(forged)
    chain_path.write_text("\n".join(rows) + "\n", encoding="utf-8")
    with pytest.raises(SupportBoundaryError, match="fails verification"):
        ratified_boundary(root, allow_unauthenticated=True)

    # missing body
    sub = tmp_path / "c2"
    sub.mkdir()
    root = _root(sub)
    key = _key(sub)
    present(root, BOUNDARY, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, BOUNDARY, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    (root / SIGNATURE_DIRNAME / f"{BOUNDARY.stipulation_id()}.body").unlink()
    with pytest.raises(SupportBoundaryError, match="cannot be read"):
        ratified_boundary(root, allow_unauthenticated=True)

    # tampered body
    sub = tmp_path / "c3"
    sub.mkdir()
    root = _root(sub)
    key = _key(sub)
    present(root, BOUNDARY, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, BOUNDARY, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    (root / SIGNATURE_DIRNAME / f"{BOUNDARY.stipulation_id()}.body").write_text(
        '{"in_scope":["install"],"out_scope":[],"answer_surface":"docs/X.md"}',
        encoding="utf-8",
    )
    with pytest.raises(SupportBoundaryError, match="not the artifact"):
        ratified_boundary(root, allow_unauthenticated=True)
