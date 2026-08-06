"""identity-seed: non-PII by construction, and never silently re-minted."""

from __future__ import annotations

import json
import os
import socket

import pytest

from k0 import RefusalError
from k0.identity import (
    SEED_FILENAME,
    EstateIdentity,
    IdentitySeedError,
    assert_non_pii,
    load_or_mint,
    mint_estate_id,
)


def test_minted_ids_are_random_not_derived():
    ids = {mint_estate_id() for _ in range(200)}
    assert len(ids) == 200, "collision in 200 draws — this is not a CSPRNG"
    assert all(len(i) == 32 for i in ids)


def test_a_minted_id_encodes_no_host_fact():
    """The regression this guards: making minting 'deterministic' by seeding from the machine."""
    for _ in range(20):
        assert_non_pii(mint_estate_id())


def test_an_id_containing_the_hostname_is_refused():
    host = socket.gethostname()
    if len(host) < 4:
        pytest.skip("hostname too short to be a meaningful fingerprint")
    with pytest.raises(IdentitySeedError, match="host fact"):
        assert_non_pii(f"estate-{host}-001")


def test_an_id_containing_the_username_is_refused():
    user = os.environ.get("USER") or os.environ.get("LOGNAME") or ""
    if len(user) < 4:
        pytest.skip("username too short")
    with pytest.raises(IdentitySeedError, match="host fact"):
        assert_non_pii(f"{user}-estate")


# --- persistence ---------------------------------------------------------------------------
def test_first_run_mints_and_persists(tmp_path):
    ident = load_or_mint(tmp_path, chain_exists=False)
    assert (tmp_path / SEED_FILENAME).is_file()
    again = load_or_mint(tmp_path, chain_exists=False)
    assert again.estate_id == ident.estate_id, "load must not re-mint"


def test_a_missing_seed_beside_an_existing_chain_refuses(tmp_path):
    """Re-minting here would orphan every receipt already chained under the old identity."""
    with pytest.raises(RefusalError) as e:
        load_or_mint(tmp_path, chain_exists=True)
    assert "orphan" in e.value.refusal.why
    assert e.value.refusal.legal_next
    assert not (tmp_path / SEED_FILENAME).exists(), "must not have minted"


def test_unknown_chain_state_denies(tmp_path):
    """An unevaluable predicate DENIES — the K0 law, applied here."""
    with pytest.raises(RefusalError) as e:
        load_or_mint(tmp_path, chain_exists=None)
    assert "could not determine" in e.value.refusal.why
    assert not (tmp_path / SEED_FILENAME).exists()


def test_a_non_durable_root_refuses(tmp_path):
    with pytest.raises(IdentitySeedError, match="not a directory"):
        load_or_mint(tmp_path / "does-not-exist", chain_exists=False)


def test_a_corrupt_seed_is_refused_not_replaced(tmp_path):
    (tmp_path / SEED_FILENAME).write_text("{not json", encoding="utf-8")
    with pytest.raises(IdentitySeedError, match="not readable JSON"):
        load_or_mint(tmp_path, chain_exists=False)


def test_an_unknown_schema_is_refused(tmp_path):
    (tmp_path / SEED_FILENAME).write_text(json.dumps({"seed_schema": 99}), encoding="utf-8")
    with pytest.raises(IdentitySeedError, match="unknown seed_schema"):
        load_or_mint(tmp_path, chain_exists=False)


def test_a_seed_missing_its_id_is_refused(tmp_path):
    (tmp_path / SEED_FILENAME).write_text(
        json.dumps({"seed_schema": 1, "minted_at": "2026-08-01T00:00:00+00:00"}), encoding="utf-8"
    )
    with pytest.raises(IdentitySeedError, match="missing estate_id"):
        load_or_mint(tmp_path, chain_exists=False)


def test_roundtrip(tmp_path):
    ident = load_or_mint(tmp_path, chain_exists=False)
    assert EstateIdentity.from_json(ident.to_json()).estate_id == ident.estate_id


def test_the_seed_closes_the_genesis_chain(tmp_path):
    """The seam that was missing, end to end: nothing minted an estate_id, so the genesis receipt
    had no identity to attribute the kernel's self-attestation to. Now it does — and the receipt
    carries the ratified K0 manifest pin, so the first chain link states which kernel armed."""
    import sys, os as _os
    sys.path.insert(0, _os.path.dirname(_os.path.dirname(_os.path.abspath(__file__))))
    import bootstrap_receipt as br

    from k0.manifest import kernel_identity

    ident = load_or_mint(tmp_path, chain_exists=False)
    # The kernel attests the manifest it CARRIES, not one the caller hands it.
    receipt = br.genesis_self_attest(estate_id=ident.estate_id, **kernel_identity())
    assert receipt.estate_id == ident.estate_id
    assert receipt.phase is br.BootstrapPhase.K0_ACTIVE
    assert receipt.prev_receipt_hash is None, "genesis is the first link"
    from k0.manifest import RATIFIED_PIN
    assert any(RATIFIED_PIN in r for r in receipt.payload_refs)
