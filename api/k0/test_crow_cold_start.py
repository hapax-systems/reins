"""R2.15 — the crow seat cold-start, pinned.

Each law of the terminal act has a test that violates it and asserts refusal
(the R0.4 form), plus the integration witness: a full ceremony, then the
cold-start, then the chain read back.
"""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import (
    BootstrapAct,
    BootstrapPhase,
    EvidenceStatus,
    append_receipt,
    genesis_self_attest,
    load_chain,
    verify_chain_at,
)
from k0.boot_profile import PROFILES
from k0.ceremony import ratify_genesis_stipulations
from k0.crow_cold_start import (
    CAPABILITYIO_FLIP_TARGET,
    LEARNER_SIGNALS,
    TERMINAL_ACT_ID,
    CrowBootstrapChannel,
    CrowColdStartError,
    cold_start,
    crow_store_path,
    default_channel,
)
from k0.egress_consent import EgressAllowlist
from k0.forge_choice import FORGE_PROFILES, ForgeChoice
from k0.refusal import RefusalError
from k0.support_boundary import SupportBoundary

ESTATE = "estate:test"
KERNEL = "k0:test"


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


def _ceremony_complete_root(tmp_path: Path) -> Path:
    root = _root(tmp_path)
    ratify_genesis_stipulations(
        root,
        principal="operator@estate-0",
        roles=("alpha",),
        boot_profile=PROFILES["existing-agent-harness"],
        allowlist=EgressAllowlist(hosts=("api.anthropic.com",)),
        forge_profile=FORGE_PROFILES[ForgeChoice.GITHUB_ONLY],
        key_path=_key(tmp_path),
        support_boundary=SupportBoundary(
            in_scope=("install",),
            out_scope=("custom-consulting",),
            answer_surface="docs/SUPPORT.md",
        ),
        estate_id=ESTATE,
        kernel_version=KERNEL,
    )
    return root


# ── the integration witness ──────────────────────────────────────────────


def test_cold_start_after_ceremony_mints_store_and_stipulated_admission(
    tmp_path: Path,
) -> None:
    root = _ceremony_complete_root(tmp_path)
    result = cold_start(root, estate_id=ESTATE, kernel_version=KERNEL)

    # the store: created empty-but-valid, on disk before its receipt (write order)
    store = crow_store_path(root)
    assert (store / "README.json").exists()
    assert result.store_receipt == "crow-store-created"

    chain = load_chain(root)
    store_row = next(r for r in chain if r.receipt_id == "crow-store-created")
    admission = next(r for r in chain if r.receipt_id == "crow-bootstrap-admission")

    # both at the first post-ceremony rung, both MINTED, both local_only by construction
    for row in (store_row, admission):
        assert row.phase is BootstrapPhase.SURFACE_OBSERVE
        assert row.act is BootstrapAct.MINTED
        assert row.transmit_class == "local_only"
        assert row.may_authorize is False

    # the store's existence is witnessed; the admission is STIPULATED — asserted, never observed
    assert store_row.evidence_status is EvidenceStatus.OBSERVED
    assert admission.evidence_status is EvidenceStatus.ASSERTED

    # the UNMEASURED mark and the channel binding are on the admission's refs
    refs = set(admission.payload_refs)
    assert "crow-bootstrap-admission:unmeasured" in refs
    assert "crow-bootstrap-admission:stipulated" in refs
    assert any(ref.startswith("crow-channel-descriptor:sha256:") for ref in refs)
    assert f"capabilityio-flip-target:{CAPABILITYIO_FLIP_TARGET}" in refs

    assert verify_chain_at(root).ok


# ── the laws, each violated ─────────────────────────────────────────────


def test_cold_start_refuses_before_the_ceremony(tmp_path: Path) -> None:
    root = _root(tmp_path)  # genesis only; no ceremony
    with pytest.raises(RefusalError) as excinfo:
        cold_start(root, estate_id=ESTATE, kernel_version=KERNEL)
    assert excinfo.value.refusal.gate == "k0.crow-cold-start"
    assert excinfo.value.refusal.legal_next  # a refusal without a next step is a dead end


def test_cold_start_refuses_a_second_genesis(tmp_path: Path) -> None:
    root = _ceremony_complete_root(tmp_path)
    cold_start(root, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(CrowColdStartError) as excinfo:
        cold_start(root, estate_id=ESTATE, kernel_version=KERNEL)
    assert excinfo.value.refusal is not None
    assert "hydrate" in excinfo.value.refusal.legal_next


def test_channel_refuses_a_non_stipulated_admission_class() -> None:
    channel = default_channel()
    forged = CrowBootstrapChannel(
        channel_id=channel.channel_id,
        descriptor_digest=channel.descriptor_digest,
        transport=channel.transport,
        admission_class="measured",
        flip_target=channel.flip_target,
        learner_signals=channel.learner_signals,
    )
    with pytest.raises(CrowColdStartError):
        forged.validate()


def test_channel_refuses_an_adhoc_transport() -> None:
    channel = default_channel()
    forged = CrowBootstrapChannel(
        channel_id=channel.channel_id,
        descriptor_digest=channel.descriptor_digest,
        transport="direct-websocket",
        admission_class=channel.admission_class,
        flip_target=channel.flip_target,
        learner_signals=channel.learner_signals,
    )
    with pytest.raises(CrowColdStartError):
        forged.validate()


def test_channel_refuses_free_text_learner_signals() -> None:
    channel = default_channel()
    forged = CrowBootstrapChannel(
        channel_id=channel.channel_id,
        descriptor_digest=channel.descriptor_digest,
        transport=channel.transport,
        admission_class=channel.admission_class,
        flip_target=channel.flip_target,
        learner_signals=(*LEARNER_SIGNALS, "free_text_grade"),
    )
    with pytest.raises(CrowColdStartError):
        forged.validate()


def test_cold_start_refuses_a_broken_chain(tmp_path: Path) -> None:
    root = _ceremony_complete_root(tmp_path)
    chain_path = root / "bootstrap-receipts.jsonl"
    lines = chain_path.read_text(encoding="utf-8").splitlines()
    # tamper with a mid-chain row: the hash link to its successor breaks
    lines[2] = lines[2].replace('"observed"', '"0bserved"', 1)
    chain_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    # the cold-start must refuse fail-closed. Which layer catches it (the ceremony
    # completeness readers or the cold-start's own chain verification) is the
    # estate's business — every path raises a typed refusal with a legal_next.
    from k0.role_registry import RoleRegistryError

    with pytest.raises((CrowColdStartError, RoleRegistryError, RefusalError)) as excinfo:
        cold_start(root, estate_id=ESTATE, kernel_version=KERNEL)
    refusal = getattr(excinfo.value, "refusal", None)
    assert refusal is not None and refusal.legal_next


# ── the segment binding and the no-model law ─────────────────────────────


def test_terminal_act_binding_agrees_with_the_segment() -> None:
    import deterministic_segment

    assert deterministic_segment.TERMINAL_ACT_ID == TERMINAL_ACT_ID == "R2.15-crow-cold-start"


def test_the_module_imports_no_model_or_network_client() -> None:
    """The no-LLM source scan (the segment's acceptance form): the terminal act is
    model-INVOLVING downstream, but this module itself makes no transmitting call."""
    import ast

    source = Path(__file__).with_name("crow_cold_start.py").read_text(encoding="utf-8")
    tree = ast.parse(source)
    imported = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            imported.update(alias.name.split(".")[0] for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            imported.add(node.module.split(".")[0])
    forbidden = {
        "socket",
        "urllib",
        "http",
        "requests",
        "httpx",
        "openai",
        "anthropic",
        "litellm",
        "websockets",
        "grpc",
    }
    assert not (imported & forbidden), f"the cold-start imports transmitting clients: {imported & forbidden}"
