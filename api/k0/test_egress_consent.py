"""R2.4 — egress consent: the allowlist is a ratified stipulation; the gate is default-deny."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import (
    BootstrapAct,
    BootstrapPhase,
    BootstrapReceipt,
    EvidenceStatus,
    append_receipt,
    genesis_self_attest,
    load_chain,
    verify_chain_at,
)
from k0.egress_consent import (
    EgressAllowlist,
    EgressConsentError,
    accept,
    egress_decision,
    elicit_allowlist,
    ratified_allowlist,
)
from k0.ratification import SIGNATURE_DIRNAME

ESTATE = "estate-0000000000000000"
KERNEL = "k0-test"
ALLOWED = EgressAllowlist(hosts=("api.anthropic.com", "api.z.ai"))


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


def _consented(root: Path, key: Path) -> None:
    elicit_allowlist(root, ALLOWED, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, ALLOWED, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)


def test_the_allowlist_is_a_stipulation_through_the_ceremony(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    _consented(root, key)

    got = ratified_allowlist(root)
    assert got is not None and set(got.hosts) == set(ALLOWED.hosts)
    assert egress_decision(root, "api.anthropic.com")
    assert egress_decision(root, "api.z.ai")
    assert not egress_decision(root, "api.anthropic.com.evil.example"), (
        "exact hosts, never suffixes — a lookalike is a denial"
    )
    assert not egress_decision(root, "example.com"), "an unnamed host is denied"
    assert verify_chain_at(root).ok


def test_default_deny_without_any_ratification(tmp_path: Path) -> None:
    root = _root(tmp_path)
    assert ratified_allowlist(root) is None, "no consent reads as no allowlist — dark, not empty"
    assert not egress_decision(root, "api.anthropic.com")


def test_elicitation_without_consent_still_denies(tmp_path: Path) -> None:
    root = _root(tmp_path)
    elicit_allowlist(root, ALLOWED, estate_id=ESTATE, kernel_version=KERNEL)
    assert not egress_decision(root, "api.anthropic.com"), (
        "an elicitation is a question, not a consent"
    )


def test_a_tampered_allowlist_body_refuses_loudly(tmp_path: Path) -> None:
    """Silent deny would hide tampering inside the safe-looking answer — corruption is loud."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    _consented(root, key)

    body = root / SIGNATURE_DIRNAME / f"{ALLOWED.stipulation_id()}.body"
    body.write_text('{"hosts":["api.attacker.example"]}', encoding="utf-8")
    with pytest.raises(EgressConsentError, match="changed after consent") as exc:
        egress_decision(root, "api.anthropic.com")
    assert exc.value.refusal is not None and exc.value.refusal.legal_next.strip()

    body.unlink()
    with pytest.raises(EgressConsentError, match="cannot be read"):
        ratified_allowlist(root)


def test_the_well_ordering_law_a_probe_before_consent_closes_the_gate(tmp_path: Path) -> None:
    """The first model call may not precede the consent that governs it. The receipt spine makes
    such a chain ILLEGAL outright (phases never regress: a probe row before the ratification is
    'MEASURED_PROBE before AUTH_MATERIALIZE' or a phase regression), and the gate verifies the
    chain before trusting it — so the violation surfaces as a verification refusal, loud."""
    root = _root(tmp_path)
    key = _key(tmp_path)

    # A probe row appended BEFORE the elicitation/consent — the violation.
    chain = load_chain(root)
    append_receipt(
        root,
        BootstrapReceipt(
            receipt_id="probe-before-consent-0",
            estate_id=ESTATE,
            kernel_version=KERNEL,
            phase=BootstrapPhase.MEASURED_PROBE,
            act=BootstrapAct.PROBED,
            payload_refs=["probe:sha256:" + "0" * 16],
            evidence_status=EvidenceStatus.OBSERVED,
            prev_receipt_hash=chain[-1].receipt_hash(),
            observed_at=datetime.now(UTC),
        ),
    )

    _consented(root, key)
    with pytest.raises(EgressConsentError, match="fails verification"):
        egress_decision(root, "api.anthropic.com")


def test_bad_allowlist_shapes_are_refused_at_construction() -> None:
    with pytest.raises(ValueError, match="consents to nothing"):
        EgressAllowlist(hosts=())
    with pytest.raises(ValueError, match="never patterns"):
        EgressAllowlist(hosts=("*.anthropic.com",))
    with pytest.raises(ValueError, match="never patterns"):
        EgressAllowlist(hosts=("API.Anthropic.com",))  # case is data; lowercase or refuse


def _probe_row(root: Path, receipt_id: str) -> None:
    chain = load_chain(root)
    append_receipt(
        root,
        BootstrapReceipt(
            receipt_id=receipt_id,
            estate_id=ESTATE,
            kernel_version=KERNEL,
            phase=BootstrapPhase.MEASURED_PROBE,
            act=BootstrapAct.PROBED,
            payload_refs=["probe:sha256:" + "0" * 16],
            evidence_status=EvidenceStatus.OBSERVED,
            prev_receipt_hash=chain[-1].receipt_hash(),
            observed_at=datetime.now(UTC),
        ),
    )


def test_rotation_after_a_probe_is_chain_illegal(tmp_path: Path) -> None:
    """Egress consent is EXACTLY-ONCE per ceremony (codex/claude r1 majors, resolved by the spine
    rather than by this module): a second ratification after a MEASURED_PROBE row is a phase
    regression, the chain fails verification, and the gate is loud — it never answers from an
    illegal chain, in either direction."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    _consented(root, key)
    # A chain-legal probe needs AUTH_MATERIALIZE first (spine law).
    chain = load_chain(root)
    append_receipt(
        root,
        BootstrapReceipt(
            receipt_id="auth-materialize-0",
            estate_id=ESTATE,
            kernel_version=KERNEL,
            phase=BootstrapPhase.AUTH_MATERIALIZE,
            act=BootstrapAct.ELICITED,
            payload_refs=["k0-secret:frontier-provider-key"],
            evidence_status=EvidenceStatus.OBSERVED,
            prev_receipt_hash=chain[-1].receipt_hash(),
            observed_at=datetime.now(UTC),
        ),
    )
    _probe_row(root, "probe-after-consent-0")
    assert egress_decision(root, "api.anthropic.com"), "the consented gate still answers"

    rotated = EgressAllowlist(hosts=("api.anthropic.com", "api.openai.com"))
    elicit_allowlist(root, rotated, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, rotated, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    with pytest.raises(EgressConsentError, match="fails verification"):
        egress_decision(root, "api.openai.com")


def test_a_broken_chain_closes_the_gate_loudly(tmp_path: Path) -> None:
    """load_chain does not verify hashes; the GATE must (claude r1 major)."""
    import json as _json

    from bootstrap_receipt import RECEIPT_CHAIN_FILENAME

    root = _root(tmp_path)
    key = _key(tmp_path)
    _consented(root, key)

    chain_path = root / RECEIPT_CHAIN_FILENAME
    rows = chain_path.read_text(encoding="utf-8").splitlines()
    # Forge a MIDDLE row: the next row's prev_receipt_hash no longer matches, and the hash
    # link — not any content check — is what must catch it.
    forged = _json.loads(rows[1])
    forged["payload_refs"] = ["stipulation:sha256:" + "f" * 64]
    rows[1] = _json.dumps(forged)
    chain_path.write_text("\n".join(rows) + "\n", encoding="utf-8")

    with pytest.raises(EgressConsentError, match="fails verification"):
        ratified_allowlist(root)


def test_a_structurally_wrong_body_is_a_refusal_not_a_crash(tmp_path: Path) -> None:
    """The pinned body could decode to the wrong SHAPE (codex r1 major): a hosts string, a
    list of non-strings, a non-dict. The digest check passes (the bytes ARE the consented ones);
    only the shape validation keeps the gate from crashing or reading garbage."""
    from k0.ratification import Stipulation, propose, ratify

    for i, bad in enumerate(
        (
            b'{"hosts":"api.anthropic.com"}',
            b'{"hosts":[1,2]}',
            b'["api.anthropic.com"]',
        )
    ):
        sub = tmp_path / f"case{i}"
        sub.mkdir()
        root = _root(sub)
        key = _key(sub)
        import hashlib as _hashlib

        sid = f"egress-allowlist.{_hashlib.sha256(bad).hexdigest()[:8]}"
        stip = Stipulation.over(sid, "EGRESS CONSENT: shape test", bad)
        (root / SIGNATURE_DIRNAME).mkdir(exist_ok=True)
        (root / SIGNATURE_DIRNAME / f"{sid}.body").write_bytes(bad)
        propose(root, stip, estate_id=ESTATE, kernel_version=KERNEL)
        ratify(root, stip, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

        with pytest.raises(EgressConsentError, match="not decodable"):
            ratified_allowlist(root)
