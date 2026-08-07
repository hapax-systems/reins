"""R2.3 — frontier key capture and secrets bootstrap.

The estate pattern that works (pass-backed, tmpfs-env, dependency-rooted) is hardcoded to one
operator's ~20 entries, UID, and identity strings — a stranger's kit cannot inherit it. The graph
gap is four claims, and this module is each of them as machinery:

  * PORTABLE, BACKEND-AGNOSTIC STORE. `SecretStore` is the contract; `PassStore` and
    `MemoryStore` are two backends. Nothing else in the kernel may know which is in use.
  * THE SECRET SET IS GENERATED, never enumerated. `required_secrets` derives the set from the
    RATIFIED boot profile (R3.6): the existing-agent-harness profile needs nothing (the sanctioned
    harness IS the secret store for entitlement auth — access-bootstrap amendment, 2026-07-09);
    the hosted-model profile needs exactly one frontier provider key. A capability that was not
    ratified cannot have a secret requirement — that is what "generated from the ratified set"
    means, and it is why this module reads the chain rather than a config list.
  * UNVALIDATED KEY IS NOT SUPPLY. Presence in the store is capture, not capability. Only a
    PROBED row — the working-key validation receipt — moves a name to VALIDATED. The validator is
    injected: this module never transmits, and the first transmitting call remains
    MEASURED_PROBE's wall, post-consent.
  * DECLINE IS A LEGAL ANSWER. A REFUSED row leaves the capability credential_gated: it renders
    dark and is never re-elicited. "Never nags" is machine-checked, not a tone of voice.

VALUE DISCIPLINE: secret values move exactly once — from the operator's input into the backend —
and exist afterward only in process memory, handed to an injected validator. The chain carries
refs (`k0-secret:<name>`, `key-validation:sha256:<digest>`), and BootstrapReceipt's ref grammar
refuses bare-secret shapes on top of that. No value is ever logged, persisted, or transmitted
by this module.
"""

from __future__ import annotations

import hashlib
import shutil
import subprocess
from dataclasses import dataclass
from datetime import UTC, datetime
from enum import StrEnum
from pathlib import Path
from typing import Callable, Protocol

from bootstrap_receipt import (
    BootstrapAct,
    BootstrapPhase,
    BootstrapReceipt,
    EvidenceStatus,
    append_receipt,
    load_chain,
)

AUTH_PHASE = BootstrapPhase.AUTH_MATERIALIZE

#: What each ratified boot profile requires of the secret store. The harness profile needs
#: NOTHING captured — the sanctioned harness holds its own provider sign-in, and the kit never
#: touches it. The hosted profile needs exactly one frontier key. Adding a profile without
#: extending this table fails closed in `required_secrets`.
PROFILE_SECRET_REQUIREMENTS: dict[str, tuple[str, ...]] = {
    "existing-agent-harness": (),
    "hosted-model-kit-minimal": ("frontier-provider-key",),
}


class SecretSupply(StrEnum):
    """The supply ladder. Order is derivation, never storage."""

    ABSENT = "absent"  # nothing captured, nothing declined — elicitation is legal
    CAPTURED_UNVALIDATED = "captured_unvalidated"  # in the store, unproven — NOT supply
    VALIDATED = "validated"  # a working-key validation receipt exists — supply
    CREDENTIAL_GATED = "credential_gated"  # declined — renders dark, never nags


class SecretStore(Protocol):
    """The backend contract. Values cross this boundary in memory only."""

    backend_id: str

    def has(self, name: str) -> bool: ...
    def get(self, name: str) -> bytes | None: ...
    def put(self, name: str, value: bytes) -> None: ...


@dataclass
class MemoryStore:
    """A stranger's backend and the test fixture: process memory, nothing durable."""

    backend_id: str = "memory"

    def __post_init__(self) -> None:
        self._values: dict[str, bytes] = {}

    def has(self, name: str) -> bool:
        return name in self._values

    def get(self, name: str) -> bytes | None:
        return self._values.get(name)

    def put(self, name: str, value: bytes) -> None:
        self._values[name] = value


@dataclass(frozen=True)
class PassStore:
    """The estate-pattern backend: `pass`, values via stdin/stdout pipes, never argv.

    argv is world-readable (`ps`); stdin is not. A value must never appear on a command line,
    in an environment block, or in an error message — subprocess errors are re-raised without
    captured output attached.
    """

    prefix: str = "first-init/"
    backend_id: str = "pass"

    def _path(self, name: str) -> str:
        return f"{self.prefix}{name}"

    def has(self, name: str) -> bool:
        return subprocess.run(
            ["pass", "show", self._path(name)],
            capture_output=True,
            check=False,
        ).returncode == 0

    def get(self, name: str) -> bytes | None:
        result = subprocess.run(
            ["pass", "show", self._path(name)],
            capture_output=True,
            check=False,
        )
        if result.returncode != 0:
            return None
        return result.stdout

    def put(self, name: str, value: bytes) -> None:
        subprocess.run(
            ["pass", "insert", "--force", self._path(name)],
            input=value,
            capture_output=True,
            check=True,
        )


def required_secrets(profile_id: str) -> tuple[str, ...]:
    """The secret set GENERATED from a ratified boot profile. Unknown profiles fail closed."""
    if profile_id not in PROFILE_SECRET_REQUIREMENTS:
        raise KeyError(
            f"{profile_id}: no ratified profile by this id — a secret requirement cannot be "
            "generated for a capability set that was never consented to"
        )
    return PROFILE_SECRET_REQUIREMENTS[profile_id]


def _append_row(
    root: Path,
    *,
    act: BootstrapAct,
    estate_id: str,
    kernel_version: str,
    payload_refs: list[str],
    receipt_id: str,
    observed_at: datetime | None,
) -> Path:
    chain = load_chain(root)
    if not chain:
        raise ValueError(
            "no genesis self-attest: there is no ceremony to record key capture within"
        )
    receipt = BootstrapReceipt(
        receipt_id=receipt_id,
        estate_id=estate_id,
        kernel_version=kernel_version,
        phase=AUTH_PHASE,
        act=act,
        payload_refs=payload_refs,
        evidence_status=EvidenceStatus.OBSERVED,
        prev_receipt_hash=chain[-1].receipt_hash(),
        observed_at=observed_at or datetime.now(UTC),
    )
    return append_receipt(root, receipt)


def _rows(root: Path, name: str) -> list[BootstrapReceipt]:
    ref = f"k0-secret:{name}"
    return [
        receipt
        for receipt in load_chain(root)
        if receipt.phase == AUTH_PHASE and ref in receipt.payload_refs
    ]


def elicit_capture(
    root: Path,
    name: str,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """Record the ask. An elicitation is not supply and changes no supply state."""
    return _append_row(
        root,
        act=BootstrapAct.ELICITED,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[f"k0-secret:{name}"],
        receipt_id=f"key-capture-elicited-{name}-{len(_rows(root, name))}",
        observed_at=observed_at,
    )


def decline_capture(
    root: Path,
    name: str,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """Record the sovereign's no. After this the name renders dark and is never re-asked."""
    return _append_row(
        root,
        act=BootstrapAct.REFUSED,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[f"k0-secret:{name}"],
        receipt_id=f"key-capture-declined-{name}-{len(_rows(root, name))}",
        observed_at=observed_at,
    )


def supply_state(root: Path, store: SecretStore, name: str) -> SecretSupply:
    """Derive the ladder rung from the chain plus the store — there is no cursor to drift.

    REFUSED wins over presence: a key captured and then declined reads credential_gated, because
    the no is later and sovereign. A PROBED success wins over everything below it: validation is
    the only rung that is supply.
    """
    rows = _rows(root, name)
    if any(r.act is BootstrapAct.REFUSED for r in rows):
        return SecretSupply.CREDENTIAL_GATED
    if any(r.act is BootstrapAct.PROBED for r in rows):
        return SecretSupply.VALIDATED
    if store.has(name):
        return SecretSupply.CAPTURED_UNVALIDATED
    return SecretSupply.ABSENT


def needs_elicitation(root: Path, store: SecretStore, name: str) -> bool:
    """May the ceremony ask for this name? NEVER-NAGS, as a machine check.

    Only ABSENT-and-never-asked is askable. A pending elicitation is not re-asked (the ceremony
    is in flight, not forgotten); a captured, validated, or declined name is never re-asked.
    """
    if supply_state(root, store, name) is not SecretSupply.ABSENT:
        return False
    return not any(r.act is BootstrapAct.ELICITED for r in _rows(root, name))


def validate_key(
    root: Path,
    store: SecretStore,
    name: str,
    *,
    validator: Callable[[bytes], str | None],
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> bool:
    """The working-key validation receipt. UNVALIDATED IS NOT SUPPLY, as a machine check.

    The validator is injected and receives the value in memory; this module never transmits.
    It returns an evidence string on success (a probe-receipt ref, a response id — something a
    stranger could re-check), which is DIGESTED into the row: the chain pins that validation
    happened against this evidence without carrying the evidence itself. Failure writes no row:
    a failed validation is not a disposition, and the name stays CAPTURED_UNVALIDATED.
    """
    if supply_state(root, store, name) is SecretSupply.CREDENTIAL_GATED:
        raise ValueError(
            f"{name}: declined by the operator — validating a refused secret would be nagging "
            "by another door"
        )
    value = store.get(name)
    if value is None:
        raise ValueError(
            f"{name}: nothing captured to validate — capture first, then prove; an unvalidated "
            "key is not supply, and an absent one is not even that"
        )
    evidence = validator(value)
    if evidence is None:
        return False
    _append_row(
        root,
        act=BootstrapAct.PROBED,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[
            f"k0-secret:{name}",
            f"key-validation:sha256:{hashlib.sha256(evidence.encode('utf-8')).hexdigest()[:16]}",
        ],
        receipt_id=f"key-validation-{name}-{len(_rows(root, name))}",
        observed_at=observed_at,
    )
    return True


def pass_backend_available() -> bool:
    """The pass backend is offered only when the host can actually serve it."""
    return shutil.which("pass") is not None
