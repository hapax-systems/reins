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
    PROBED row — the working-key validation receipt — moves a name to VALIDATED, and the row
    pins the digest of the exact bytes proven to work: edit the stored key and it falls back to
    CAPTURED_UNVALIDATED, delete it and the answer is ABSENT. The validator is injected: this
    module never transmits, and the first transmitting call remains MEASURED_PROBE's wall,
    post-consent.
  * DECLINE IS A LEGAL ANSWER. A REFUSED row leaves the capability credential_gated: it renders
    dark and is never re-elicited. "Never nags" is machine-checked, not a tone of voice.

VALUE DISCIPLINE: secret values move exactly twice — from the operator's input into the backend,
and from the backend to an injected validator, in memory. The backend's own encrypted store is
persistence WITH the operator's consent and is the store's reason to exist; apart from it, no
value is ever logged, written to the chain, or transmitted by this module. The chain carries
refs (`k0-secret:<name>`, `key-validation:sha256:<digest>`, `key-value:sha256:<digest>`), and
BootstrapReceipt's ref grammar refuses bare-secret shapes on top of that.
"""

from __future__ import annotations

import hashlib
import re
import secrets
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

from .boot_profile import PROFILES, ratified_profile
from .egress_consent import EgressConsentError, require_egress

AUTH_PHASE = BootstrapPhase.AUTH_MATERIALIZE

#: AUTH_MATERIALIZE rows live here; the requirement DATA lives in the ratified boot profile's
#: consented bytes (boot_profile.BootProfile.secret_requirements) — the secret set is generated
#: from the ratified capability set literally, not by a config table in this module.


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

    Single-line invariant: `pass insert --echo` reads exactly one line, so multiline values are
    refused here rather than truncated by the backend. `pass show` appends exactly one newline
    on output; `get` strips exactly that byte, so put/get round-trips byte-identically.
    """

    prefix: str = "first-init/"
    backend_id: str = "pass"

    def _path(self, name: str) -> str:
        return f"{self.prefix}{name}"

    def has(self, name: str) -> bool:
        # `pass ls` lists without DECRYPTING — `pass show` would read the value just to answer
        # presence, and a presence check has no business holding the secret (codex r4 major).
        return subprocess.run(
            ["pass", "ls", self._path(name)],
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
        out = result.stdout
        if out.endswith(b"\n"):
            out = out[:-1]
        return out

    def put(self, name: str, value: bytes) -> None:
        if b"\n" in value or b"\r" in value:
            raise ValueError(
                f"{name}: pass entries here are single-line — a multiline value would be "
                "silently truncated by insert --echo, and a truncated key is a wrong key"
            )
        rc = None
        try:
            subprocess.run(
                ["pass", "insert", "--echo", "--force", self._path(name)],
                input=value + b"\n",
                capture_output=True,
                check=True,
            )
        except subprocess.CalledProcessError as exc:
            # CalledProcessError RETAINS the captured stdout/stderr, and a backend error can
            # quote what it was fed. Keep only the exit code; the exception object — and its
            # place in the raised error's __context__ chain — must not survive (codex r3 major).
            rc = exc.returncode
        if rc is not None:
            raise RuntimeError(
                f"pass insert {self._path(name)!r} failed (rc={rc}); "
                "backend output deliberately suppressed"
            )


@dataclass(frozen=True)
class ProbeEndpoint:
    """Where a provider's key is proven: host, path, auth shape — REGISTRY DATA, never caller
    input (r9/r10 criticals: a caller-selected path or host can point at an endpoint that
    cannot discriminate, and consent is then anchored to nothing)."""

    host: str
    path: str
    auth_scheme: str  # "bearer" | "x-api-key"
    key_prefix: str  # the provider's key shape — the negative control must match it
    extra_headers: tuple[tuple[str, str], ...] = ()


#: The sanctioned providers and the endpoints that actually check authorization on them. A
#: provider enters this table only when its validation endpoint is KNOWN to refuse invalid
#: keys; the per-call negative control re-proves that property on every run.
PROVIDER_PROBE_ENDPOINTS: dict[str, ProbeEndpoint] = {
    "anthropic": ProbeEndpoint(
        "api.anthropic.com",
        "/v1/models",
        "x-api-key",
        "sk-ant-",
        (("anthropic-version", "2023-06-01"),),
    ),
    "openai": ProbeEndpoint("api.openai.com", "/v1/models", "bearer", "sk-"),
}


def _negative_control_key(endpoint: ProbeEndpoint) -> bytes:
    """The deliberately-invalid control key — provider-SHAPED and random (r9/r10/r11).

    The bypass ladder ends here: a fixed control can be hardcoded around; an unmarked random
    one can be shape-selected around (accept sk-*, refuse the rest — and the control is 'the
    rest'). The terminal form is a key in the provider's own shape with a random payload: to
    the endpoint it is indistinguishable from a real key, so an endpoint that accepts it is
    caught by the indiscriminate check, and an endpoint that refuses it actually checked.
    """
    import secrets

    return (endpoint.key_prefix + secrets.token_hex(24)).encode("ascii")


@dataclass(frozen=True)
class ProbeOutcome:
    """What the wire said: evidence on success, a classified cause on failure — never both."""

    evidence: str | None
    failure: str | None


def _https_probe_transport(root: Path, endpoint: ProbeEndpoint, value: bytes) -> ProbeOutcome:
    # The wire dials ONLY registry endpoints (codex r16): a transport taking free host/path/
    # header arguments is a dial-anything primitive wearing a consent check, and caller-shaped
    # headers are caller-controlled content on the wire. Registry membership is verified here,
    # so the consented destination is the only reachable one.
    if endpoint not in PROVIDER_PROBE_ENDPOINTS.values():
        raise ValueError(
            "the kernel's wire dials sanctioned registry endpoints only — an endpoint that is "
            "not registry data is not a legal destination"
        )
    """THE KERNEL'S OWN WIRE — AND IT IS SELF-GATING (codex r15). A public transport that does
    not check consent would be the bypass around the gate: any caller could import it and dial
    without a ratified allowlist. So the transport takes the root and runs `require_egress`
    itself. Every dial this kernel can make passes through consent; the seam is exclusive by
    construction. Tests patch the stdlib boundary (http.client.HTTPSConnection), never this
    module's surface.

    Deliberately narrow: GET with the key as a bearer token, 10s timeout, no request body, the
    response body consumed and DISCARDED (never logged, stored, or returned). Failures are
    classified into a fixed vocabulary so the operator's ledger says WHAT failed — a timeout
    and a 401 are different problems with different next moves — without ever quoting the
    wire. Evidence on success: status plus the provider's request id, when it sends one.
    """
    require_egress(root, endpoint.host)
    import http.client
    import socket
    import ssl

    # Explicit, not assumed: the default context verifies certificates and hostnames
    # (CERT_REQUIRED, check_hostname). Passing it makes the verification a choice this code
    # makes, visible to a reader and capturable by a test double.
    conn = http.client.HTTPSConnection(endpoint.host, timeout=10, context=ssl.create_default_context())
    try:
        if endpoint.auth_scheme == "x-api-key":
            auth_headers = {"x-api-key": value.decode("utf-8")}
        else:
            auth_headers = {"Authorization": f"Bearer {value.decode('utf-8')}"}
        headers = {**auth_headers, **dict(endpoint.extra_headers)}
        conn.request("GET", endpoint.path, headers=headers)
        response = conn.getresponse()
        response.read()
    except socket.timeout:
        return ProbeOutcome(None, "timeout")
    except ConnectionRefusedError:
        return ProbeOutcome(None, "connection-refused")
    except ssl.SSLError:
        return ProbeOutcome(None, "tls-error")
    except UnicodeDecodeError:
        return ProbeOutcome(None, "key-not-utf8")
    except (OSError, http.client.HTTPException):
        return ProbeOutcome(None, "transport-error")
    finally:
        conn.close()
    if 200 <= response.status < 300:
        request_id = response.getheader("x-request-id") or response.getheader("server") or "unknown"
        return ProbeOutcome(f"https-status:{response.status}:server:{request_id}", None)
    if 300 <= response.status < 400:
        # A redirect is NOT a validation: the endpoint moved, and following it with a bearer
        # token is how keys leak to hosts nobody consented to (codex r7 critical).
        return ProbeOutcome(None, f"http-{response.status}-redirect")
    return ProbeOutcome(None, f"http-{response.status}")


#: Every failure class the wire can report, with the operator's next move (executive_function:
#: an error without a next action is a dead end). Renderers look the class up here; the ledger
#: row carries the class itself.
FAILURE_NEXT_MOVES: dict[str, str] = {
    "timeout": "the host did not answer in 10s — check reachability, then retry; the key is unproven, not condemned",
    "connection-refused": "the host refused the connection — check the host name and egress path, then retry",
    "tls-error": "the TLS handshake failed — do not retry blindly; establish whether the endpoint is the real one before offering the key again",
    "key-not-utf8": "the stored value is not UTF-8 — it cannot ride an Authorization header; re-capture the key",
    "transport-error": "an unclassified transport failure — inspect locally, then retry",
    "unknown": "the probe failed without a classified cause — inspect locally, then retry",
}


def failure_next_move(failure_class: str) -> str:
    """The operator's next move for a failed validation class — including the http-<status>
    classes, which are per-status data, not prose to parse."""
    if failure_class in FAILURE_NEXT_MOVES:
        return FAILURE_NEXT_MOVES[failure_class]
    if failure_class.startswith("control-inconclusive-"):
        cause = failure_class.removeprefix("control-inconclusive-")
        if cause in ("http-404", "http-429"):
            return (
                f"the negative control was answered {cause.removeprefix('http-')} — "
                + (
                    "the probe path is wrong; confirm the provider's validation endpoint"
                    if cause == "http-404"
                    else "the endpoint is rate-limiting; wait, then retry"
                )
            )
        if cause.startswith("http-"):
            return (
                f"the negative control was answered {cause.removeprefix('http-')} rather than "
                "401/403 — the endpoint's discrimination is unproven; inspect the endpoint, "
                "then retry"
            )
        inner = FAILURE_NEXT_MOVES.get(cause, FAILURE_NEXT_MOVES["unknown"])
        return (
            f"the negative control did not produce a clean refusal ({cause}) — the endpoint's "
            f"discrimination is unproven, so nothing about the real key is established; resolve "
            f"the control's own failure first: {inner}"
        )
    if failure_class == "endpoint-indiscriminate":
        return (
            "the endpoint answered success to a deliberately invalid key — this probe path "
            "cannot validate anything; choose an endpoint that requires authorization"
        )
    if failure_class.endswith("-redirect"):
        return (
            "the endpoint answered a redirect — do not follow it with a bearer token; establish "
            "the provider's real validation endpoint and update the probe path"
        )
    if failure_class.startswith("http-"):
        status = failure_class.removeprefix("http-")
        if status in ("401", "403"):
            return (
                f"the provider answered {status} — the key does not work (wrong, expired, or "
                "revoked); capture a working key, then validate again"
            )
        if status == "404":
            return "the provider answered 404 — the probe path is wrong; confirm the provider's validation endpoint"
        if status == "429":
            return "the provider answered 429 — rate-limited; wait, then retry — the key is unproven, not condemned"
        if status.startswith("4"):
            return (
                f"the provider answered {status} — a request-level rejection, not necessarily "
                "the key; inspect before recapturing (codex r14: 4xx is not automatically "
                "'bad credential')"
            )
        if status.startswith("5"):
            return (
                f"the provider answered {status} — the failure is theirs; retry later, the key "
                "is unproven, not condemned"
            )
    return FAILURE_NEXT_MOVES["unknown"]


def required_secrets(root: Path) -> tuple[str, ...]:
    """The secret set GENERATED from the ratified capability set — read off the chain.

    The requirement is a field of the ratified boot profile's consented bytes, so this is not a
    config lookup: it is the operator's signed answer to "what must be captured". No ratified
    profile — or a ratified definition that is not the current one — fails closed: requirements
    cannot be generated from a capability set whose current terms were never consented to.
    """
    ratified = ratified_profile(root)
    if ratified is None:
        raise KeyError(
            "no ratified boot profile: there is no consented capability set to generate "
            "secret requirements from"
        )
    if not ratified.current:
        raise KeyError(
            f"{ratified.profile_id}: the ratified definition is not the current one — the "
            "operator consented to older terms; re-ratify the current definition, then generate"
        )
    profile = PROFILES[ratified.profile_id]
    return profile.secret_requirements


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
    the no is later and sovereign. VALIDATED requires the PROBED row to pin the digest of the
    value IN THE STORE NOW: the receipt consents to exact bytes, so a key changed after
    validation falls back to CAPTURED_UNVALIDATED and a deleted one to ABSENT — a stale receipt
    can never keep a replaced secret reading as supply.
    """
    rows = _rows(root, name)
    if any(r.act is BootstrapAct.REFUSED for r in rows):
        return SecretSupply.CREDENTIAL_GATED
    value = store.get(name)
    if value is None:
        return SecretSupply.ABSENT
    validated_ref = f"key-value:sha256:{hashlib.sha256(value).hexdigest()[:16]}"
    if any(r.act is BootstrapAct.PROBED and validated_ref in r.payload_refs for r in rows):
        return SecretSupply.VALIDATED
    return SecretSupply.CAPTURED_UNVALIDATED


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
    provider: str,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> bool:
    """The working-key validation receipt. UNVALIDATED IS NOT SUPPLY, as a machine check.

    The probe TRANSMITS — a validation ping is the kernel's one real egress seam today — so it
    is gated: `require_egress(root, endpoint.host)` runs before the probe is ever invoked, and a
    destination the operator has not consented to is refused, never dialed. The host is read
    FROM the probe (one object, one truth — no separate argument to disagree with it). The
    probe receives the value in memory; this module never transmits itself. The probe returns
    an evidence string on success (a response id — something a stranger could re-check), which
    is DIGESTED into the row: the chain pins that validation happened against this evidence
    without carrying the evidence itself. A failed probe writes a FAILURE row (no value, no
    response body) — the name stays CAPTURED_UNVALIDATED and the failure is durable, because a
    silent failure would let a wrong key burn retries forever.

    THE WIRE IS THE KERNEL'S, WITH NO INJECTION SEAM (r4/r5/r6 rounds): the probe is a
    descriptor (host, path — data, never code), consent is checked against `endpoint.host`, and
    the dial is ALWAYS this module's own stdlib HTTPS transport with that same attribute. The transport is PRIVATE — validate_key is the only public transmitting surface, so the validation discipline (negative control, value-binding, durable classified failures) cannot be stepped around by importing the wire directly (glm/claude r18).
    `validate_key` takes no transport argument — there is no caller-supplied code anywhere on
    the transmission path, so the consented destination is the dialed destination by
    construction. Tests patch the stdlib boundary, not this module's surface. Failures land on
    the ledger with a classified cause; the evidence the wire returns remains the re-checkable
    witness. This is descriptor-shaped and consent-sanctioned — the R3.12 doctrine forbids
    ad-hoc raw clients, not the kernel's own minimal one.
    """
    endpoint = PROVIDER_PROBE_ENDPOINTS.get(provider)
    if endpoint is None:
        raise ValueError(
            f"{provider!r}: not a sanctioned provider — the probe endpoint is registry data, "
            "never caller input, and an unknown provider has none"
        )
    if supply_state(root, store, name) is SecretSupply.CREDENTIAL_GATED:
        raise ValueError(
            f"{name}: declined by the operator — validating a refused secret would be nagging "
            "by another door"
        )
    try:
        require_egress(root, endpoint.host)
    except EgressConsentError:
        # A refused attempt is durable (claude r7): an operator who never ran the ceremony can
        # see that something TRIED to validate against an unconsented host. The row names the
        # host and deliberately NOT the k0-secret ref — supply_state must never read an egress
        # refusal as a capture decline.
        _append_row(
            root,
            act=BootstrapAct.REFUSED,
            estate_id=estate_id,
            kernel_version=kernel_version,
            payload_refs=[
                f"egress-host:{endpoint.host}",
                "egress-attempt:validation-refused-by-consent-gate",
            ],
            receipt_id=f"egress-refused-{name}-{len(load_chain(root))}",
            observed_at=observed_at,
        )
        raise
    value = store.get(name)
    if value is None:
        raise ValueError(
            f"{name}: nothing captured to validate — capture first, then prove; an unvalidated "
            "key is not supply, and an absent one is not even that"
        )
    # NEGATIVE CONTROL (codex r8 critical): a 2xx proves the key works ONLY if the endpoint
    # discriminates. A deliberately invalid key must fail here first; an endpoint that answers
    # success to garbage can validate nothing, and the real probe must not run against it.
    control = _https_probe_transport(root, endpoint, _negative_control_key(endpoint))
    if control.evidence is not None:
        _append_row(
            root,
            act=BootstrapAct.PROBED,
            estate_id=estate_id,
            kernel_version=kernel_version,
            payload_refs=[
                f"k0-secret:{name}",
                f"egress-host:{endpoint.host}",
                "key-validation-failed:endpoint-indiscriminate",
            ],
            receipt_id=f"key-validation-indiscriminate-{name}-{len(_rows(root, name))}",
            observed_at=observed_at,
        )
        return False
    if control.failure != "http-401":
        # Only 401 on garbage PROVES discrimination (codex r14): a blanket 403 can be a WAF or
        # an IP block answering without ever seeing the credential, a 404 is a wrong path, a
        # 429 is a rate limit, a timeout is the network. Anything but the authentication
        # refusal is inconclusive, and the control's own classified cause is carried so the
        # operator fixes that first.
        #
        # SEAT SPLIT, recorded: glm r17 would also accept 403. The disagreement is real; the
        # safe direction is never-false-validate — a provider whose only refusal is 403 reads
        # INCONCLUSIVE here, and inconclusive never mints supply. Tightening later (if a
        # sanctioned provider proves 403-only) is one line; loosening after a false validation
        # is a credential leak.
        cause = control.failure or "unknown"
        _append_row(
            root,
            act=BootstrapAct.PROBED,
            estate_id=estate_id,
            kernel_version=kernel_version,
            payload_refs=[
                f"k0-secret:{name}",
                f"egress-host:{endpoint.host}",
                f"key-validation-failed:control-inconclusive-{cause}",
            ],
            receipt_id=f"key-validation-inconclusive-{name}-{len(_rows(root, name))}",
            observed_at=observed_at,
        )
        return False
    outcome = _https_probe_transport(root, endpoint, value)
    if outcome.evidence is None:
        # A failed probe is NOT silent: the failure row carries the classified cause (timeout
        # and a 401 are different problems with different next moves), the consented host, and
        # never a value or a response body.
        _append_row(
            root,
            act=BootstrapAct.PROBED,
            estate_id=estate_id,
            kernel_version=kernel_version,
            payload_refs=[
                f"k0-secret:{name}",
                f"egress-host:{endpoint.host}",
                f"key-validation-failed:{outcome.failure or 'unknown'}",
            ],
            receipt_id=f"key-validation-failed-{name}-{len(_rows(root, name))}",
            observed_at=observed_at,
        )
        return False
    evidence = outcome.evidence
    _append_row(
        root,
        act=BootstrapAct.PROBED,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[
            f"k0-secret:{name}",
            f"egress-host:{endpoint.host}",
            f"key-validation:sha256:{hashlib.sha256(evidence.encode('utf-8')).hexdigest()[:16]}",
            # The receipt binds the EXACT bytes proven to work. sha256 over a provider key is
            # identification, not disclosure — the value is high-entropy, so its digest is not
            # a brute-force oracle. Without this, editing the stored key afterwards would leave
            # the old receipt attesting to a value nobody validated (codex r1 critical).
            f"key-value:sha256:{hashlib.sha256(value).hexdigest()[:16]}",
        ],
        receipt_id=f"key-validation-{name}-{len(_rows(root, name))}",
        observed_at=observed_at,
    )
    return True


def pass_backend_available() -> bool:
    """The pass backend is offered only when the host can actually serve it."""
    return shutil.which("pass") is not None
