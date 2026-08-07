"""R2.3 — key capture, tested at the laws: generated sets, unvalidated-is-not-supply, never-nags."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import (
    RECEIPT_CHAIN_FILENAME,
    BootstrapPhase,
    append_receipt,
    genesis_self_attest,
    verify_chain_at,
)
from k0.boot_profile import PROFILES
from k0.egress_consent import EgressAllowlist
from k0.egress_consent import accept as accept_egress
from k0.egress_consent import elicit_allowlist
from k0.key_capture import (
    MemoryStore,
    SecretSupply,
    decline_capture,
    elicit_capture,
    needs_elicitation,
    required_secrets,
    supply_state,
    validate_key,
)

ESTATE = "estate-0000000000000000"
KERNEL = "k0-test"
NAME = "frontier-provider-key"


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


HOST = "api.anthropic.com"


def _consent_egress(root: Path, key: Path) -> None:
    """The consent that makes a validation ping legal: the allowlist naming the provider host."""
    elicit_allowlist(root, EgressAllowlist(hosts=(HOST,)), estate_id=ESTATE, kernel_version=KERNEL)
    accept_egress(
        root, EgressAllowlist(hosts=(HOST,)), key_path=key, estate_id=ESTATE,
        kernel_version=KERNEL,
    )


class _FakeResponse:
    def __init__(self, status: int, headers: dict | None = None) -> None:
        self.status = status
        self._headers = headers or {}

    def read(self) -> bytes:
        return b""

    def getheader(self, name: str):
        return self._headers.get(name)


def _patch_wire(monkeypatch: pytest.MonkeyPatch, *, valid_keys: tuple[bytes, ...] = (b"sk-canary-value", b"sk-first-value", b"sk-replaced-value", b"sk-real-key", b"sk-bearer-value", b"sk-x"), status: int = 200, error: Exception | None = None, record: list | None = None) -> None:
    """Patch the STDLIB boundary — never the module's surface. The module has no injection seam,
    so the tests meet it at http.client.HTTPSConnection, where production behavior lives."""
    import http.client

    class _FakeConn:
        """A DISCRIMINATING endpoint: the negative-control key is refused, everything else gets
        the canned status — so the control actually controls."""

        def __init__(self, host: str, timeout: int = 10, context=None) -> None:
            self._host = host
            self._auth = ""
            if record is not None:
                record.append(("connect", host))

        def request(self, method: str, path: str, headers: dict | None = None) -> None:
            raw = (headers or {}).get("Authorization") or (headers or {}).get("x-api-key", "")
            self._auth = raw.removeprefix("Bearer ")
            if record is not None:
                record.append(("request", self._host, path, self._auth))
            if error is not None:
                raise error

        def getresponse(self):
            # The honest endpoint: it holds REGISTERED keys and refuses everything else —
            # including any shape-shaped garbage. A shape rule here would be the very bypass
            # the negative control exists to catch (claude r11).
            if self._auth.encode() not in valid_keys:
                return _FakeResponse(401)
            return _FakeResponse(status)

        def close(self) -> None:
            pass

    monkeypatch.setattr(http.client, "HTTPSConnection", _FakeConn)


def test_the_secret_set_is_generated_from_the_ratified_profile(tmp_path: Path) -> None:
    """The requirement is IN the ratified artifact, not in a config table (codex r2 critical).

    The harness profile needs NOTHING captured — the sanctioned harness is the secret store
    (access-bootstrap amendment). The hosted profile needs exactly one frontier key. No ratified
    profile — or a stale one — fails closed: there is no consented capability set to read.
    """
    from dataclasses import replace

    from k0.boot_profile import present
    from k0.ratification import ratify

    root = _root(tmp_path)
    key = _key(tmp_path)

    with pytest.raises(KeyError, match="no consented capability set"):
        required_secrets(root)

    harness = PROFILES["existing-agent-harness"]
    present(root, harness, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, harness.stipulation(), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert required_secrets(root) == (), (
        "the harness IS the secret store for entitlement auth — nothing to capture"
    )

    hosted = PROFILES["hosted-model-kit-minimal"]
    present(root, hosted, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, hosted.stipulation(), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert required_secrets(root) == ("frontier-provider-key",), (
        "supersession by recency: the hosted floor's one frontier key is the requirement now"
    )

    stale = replace(hosted, tradeoffs=hosted.tradeoffs + ("a new term",))
    present(root, stale, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, stale.stipulation(), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(KeyError, match="not the current one"):
        required_secrets(root)


def test_the_requirement_rides_inside_the_consented_bytes() -> None:
    """The generation claim is literal: the secret set is a field of the ratified body."""
    import json

    for profile in PROFILES.values():
        body = json.loads(profile.body())
        assert body["secret_requirements"] == sorted(profile.secret_requirements), (
            f"{profile.profile_id}: the requirement must be inside the bytes the operator signs"
        )


def test_the_supply_ladder_absent_to_validated(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    root = _root(tmp_path)
    store = MemoryStore()
    _consent_egress(root, _key(tmp_path))
    _patch_wire(monkeypatch)

    assert supply_state(root, store, NAME) is SecretSupply.ABSENT
    assert needs_elicitation(root, store, NAME), "absent and unasked is the one askable state"

    elicit_capture(root, NAME, estate_id=ESTATE, kernel_version=KERNEL)
    assert not needs_elicitation(root, store, NAME), (
        "a pending elicitation is the ceremony in flight — re-asking is the nag"
    )
    assert supply_state(root, store, NAME) is SecretSupply.ABSENT, (
        "an elicitation is not supply"
    )

    store.put(NAME, b"sk-canary-value")
    assert supply_state(root, store, NAME) is SecretSupply.CAPTURED_UNVALIDATED, (
        "presence in the store is capture, not capability"
    )
    assert not needs_elicitation(root, store, NAME)

    ok = validate_key(
        root,
        store,
        NAME,
        provider="anthropic",
        estate_id=ESTATE,
        kernel_version=KERNEL,
    )
    assert ok
    assert supply_state(root, store, NAME) is SecretSupply.VALIDATED
    assert verify_chain_at(root).ok, "the ceremony rows must leave the chain valid"
    phases = {r.phase for r in _chain(root)}
    assert BootstrapPhase.AUTH_MATERIALIZE in phases


def _chain(root: Path):
    from bootstrap_receipt import load_chain

    return load_chain(root)


def test_a_failed_validation_writes_no_row_and_changes_nothing(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    root = _root(tmp_path)
    store = MemoryStore()
    _consent_egress(root, _key(tmp_path))
    _patch_wire(monkeypatch, status=401)
    store.put(NAME, b"sk-canary-value")

    ok = validate_key(
        root,
        store,
        NAME,
        provider="anthropic",
        estate_id=ESTATE,
        kernel_version=KERNEL,
    )
    assert not ok
    assert supply_state(root, store, NAME) is SecretSupply.CAPTURED_UNVALIDATED, (
        "a failed validation is not supply — the name stays unvalidated, and retry is legal"
    )
    failures = [
        r for r in _chain(root) if any(ref.startswith("key-validation-failed:") for ref in r.payload_refs)
    ]
    assert len(failures) == 1, (
        "but the failure is DURABLE: silent retries would let a wrong key burn quota forever "
        "(codex r4). The row carries no value and no response body."
    )
    assert any("key-validation-failed:http-401" in r.payload_refs for r in failures), (
        "and the cause is CLASSIFIED — a 401 and a timeout are different problems with "
        "different next moves, and the ledger says which"
    )
    assert b"sk-canary-value" not in (root / RECEIPT_CHAIN_FILENAME).read_bytes()


def test_validation_without_capture_and_validation_of_nothing_are_refused(tmp_path: Path) -> None:
    root = _root(tmp_path)
    store = MemoryStore()
    _consent_egress(root, _key(tmp_path))
    with pytest.raises(ValueError, match="nothing captured"):
        validate_key(
            root,
            store,
            NAME,
            provider="anthropic",
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )


def test_the_decline_path_is_dark_and_never_nags(tmp_path: Path) -> None:
    root = _root(tmp_path)
    store = MemoryStore()

    elicit_capture(root, NAME, estate_id=ESTATE, kernel_version=KERNEL)
    decline_capture(root, NAME, estate_id=ESTATE, kernel_version=KERNEL)

    assert supply_state(root, store, NAME) is SecretSupply.CREDENTIAL_GATED
    assert not needs_elicitation(root, store, NAME), "a declined name is never re-asked"

    store.put(NAME, b"sk-canary-value")
    assert supply_state(root, store, NAME) is SecretSupply.CREDENTIAL_GATED, (
        "the no is later and sovereign — a value appearing afterward does not undo it"
    )
    with pytest.raises(ValueError, match="nagging by another door"):
        validate_key(
            root,
            store,
            NAME,
            provider="anthropic",
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )
    assert verify_chain_at(root).ok


def test_no_secret_value_ever_touches_the_chain(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The canary test: run the whole ceremony with a distinctive value, then scan the ledger."""
    root = _root(tmp_path)
    store = MemoryStore()
    _consent_egress(root, _key(tmp_path))
    _patch_wire(monkeypatch)
    canary = b"sk-canary-7f3c9a1b-never-on-disk"

    elicit_capture(root, NAME, estate_id=ESTATE, kernel_version=KERNEL)
    store.put(NAME, canary)
    validate_key(
        root,
        store,
        NAME,
        provider="anthropic",
        estate_id=ESTATE,
        kernel_version=KERNEL,
    )

    chain_bytes = (root / RECEIPT_CHAIN_FILENAME).read_bytes()
    assert canary not in chain_bytes
    assert b"sk-canary" not in chain_bytes
    assert b"probe-receipt:ok-1" not in chain_bytes, (
        "the validation evidence is digested into the row; the evidence itself stays off the ledger"
    )


def test_a_key_changed_after_validation_falls_off_the_supply_rung(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The receipt consents to EXACT BYTES (codex r1 critical). Replace the stored value after
    validation and the name must drop to CAPTURED_UNVALIDATED; delete it and the answer is
    ABSENT. A stale receipt can never keep a replaced secret reading as supply."""
    root = _root(tmp_path)
    store = MemoryStore()
    _consent_egress(root, _key(tmp_path))
    _patch_wire(monkeypatch)
    store.put(NAME, b"sk-first-value")
    assert validate_key(
        root, store, NAME, provider="anthropic",
        estate_id=ESTATE, kernel_version=KERNEL,
    )
    assert supply_state(root, store, NAME) is SecretSupply.VALIDATED

    store.put(NAME, b"sk-replaced-value")
    assert supply_state(root, store, NAME) is SecretSupply.CAPTURED_UNVALIDATED

    store._values.clear()
    assert supply_state(root, store, NAME) is SecretSupply.ABSENT


FAKE_PASS = """#!/bin/sh
# Minimal pass(1) contract double: show prints the stored line (with its newline), insert
# --echo reads exactly one line from stdin. Values arrive on stdin, never argv — this double
# would not see them otherwise, which is the point of the contract.
store="$FAKE_PASS_DIR"
cmd="$1"; shift
case "$cmd" in
  show)
    f="$store/$1"
    if [ -f "$f" ]; then cat "$f"; exit 0; else exit 1; fi
    ;;
  ls)
    # presence without decryption: prints the name, never the value
    if [ -f "$store/$1" ]; then echo "$1"; exit 0; else exit 1; fi
    ;;
  insert)
    p=""
    while [ $# -gt 0 ]; do
      case "$1" in --*) shift ;; *) p="$1"; shift ;; esac
    done
    [ -n "$p" ] || exit 2
    mkdir -p "$store/$(dirname -- "$p" 2>/dev/null || echo .)" 2>/dev/null
    IFS= read -r line
    printf '%s\\n' "$line" > "$store/$p"
    ;;
esac
"""


def test_pass_store_contract_round_trips_single_line_values(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The production backend against a pass(1) contract double (claude r1 major).

    What is pinned here is the COMMAND SHAPE: insert with --echo (one line, no confirmation
    prompt — the default form deadlocks a noninteractive caller), value on stdin, and show's
    trailing-newline convention, which get() must strip for the round-trip to be exact.
    """
    import os

    from k0.key_capture import PassStore

    fake_dir = tmp_path / "fake-store"
    fake_dir.mkdir()
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    script = bin_dir / "pass"
    script.write_text(FAKE_PASS, encoding="utf-8")
    script.chmod(0o755)
    monkeypatch.setenv("FAKE_PASS_DIR", str(fake_dir))
    monkeypatch.setenv("PATH", f"{bin_dir}{os.pathsep}{os.environ['PATH']}")

    store = PassStore()
    assert not store.has(NAME)
    store.put(NAME, b"sk-round-trip-123")
    assert store.has(NAME)
    assert store.get(NAME) == b"sk-round-trip-123", (
        "put/get must round-trip byte-identically — show appends one newline, get strips one"
    )
    with pytest.raises(ValueError, match="single-line"):
        store.put(NAME, b"two\nlines")


FAILING_PASS = """#!/bin/sh
# A pass(1) double whose insert FAILS and quotes its stdin on stderr — the leak case.
cmd="$1"; shift
case "$cmd" in
  show) exit 1 ;;
  ls) exit 1 ;;
  insert)
    IFS= read -r line
    echo "gpg: cannot encrypt for $line: no key" >&2
    exit 1
    ;;
esac
"""


def test_pass_backend_errors_never_carry_the_value(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A failing backend may quote what it was fed; the caller must never see it (r3 majors).

    Also pins the get()-on-missing arm: an absent entry reads None, not an error.
    """
    import os

    from k0.key_capture import PassStore

    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    script = bin_dir / "pass"
    script.write_text(FAILING_PASS, encoding="utf-8")
    script.chmod(0o755)
    monkeypatch.setenv("PATH", f"{bin_dir}{os.pathsep}{os.environ['PATH']}")

    store = PassStore()
    assert store.get(NAME) is None, "a missing entry is None, not an exception"

    with pytest.raises(RuntimeError, match="rc=1") as exc:
        store.put(NAME, b"sk-leak-canary-99")
    assert "sk-leak-canary-99" not in str(exc.value)
    assert "cannot encrypt" not in str(exc.value), "backend stderr is suppressed entirely"
    assert exc.value.__cause__ is None and exc.value.__context__ is None, (
        "the suppressed error must not leak through the exception chain either"
    )


def test_validation_against_an_unconsented_host_never_reaches_the_validator(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The gate on the ACTUAL egress path (codex r2 critical): the validator is the wire, so an
    unconsented host must refuse before the callable is invoked — not after, not with a warning."""
    from k0.egress_consent import EgressConsentError

    root = _root(tmp_path)
    store = MemoryStore()
    store.put(NAME, b"sk-canary-value")

    dialed: list[tuple] = []
    _patch_wire(monkeypatch, record=dialed)
    with pytest.raises(EgressConsentError, match="no egress allowlist is ratified"):
        validate_key(
            root, store, NAME, provider="openai",
            estate_id=ESTATE, kernel_version=KERNEL,
        )
    assert dialed == [], "the wire — the transmitting act — was never touched"
    refused = [
        r for r in _chain(root)
        if r.act.value == "refused" and "egress-host:api.openai.com" in r.payload_refs
    ]
    assert len(refused) == 1, (
        "the refused attempt is durable (claude r7): the ledger shows something tried to "
        "validate against an unconsented host"
    )
    assert supply_state(root, store, NAME) is SecretSupply.CAPTURED_UNVALIDATED, (
        "a refused validation is not a disposition; the name stays unvalidated"
    )


def test_the_consented_host_is_what_REACHES_the_transport(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The binding, proven: validate_key passes probe.host — the attribute it checked consent
    against — to the transport itself. A recording transport witnesses that the consented host
    is the dialed host; there is no second channel for a caller to whisper a different one."""
    root = _root(tmp_path)
    store = MemoryStore()
    _consent_egress(root, _key(tmp_path))
    store.put(NAME, b"sk-canary-value")

    dialed: list[tuple] = []
    _patch_wire(monkeypatch, record=dialed)
    ok = validate_key(
        root, store, NAME,
        provider="anthropic",
        estate_id=ESTATE, kernel_version=KERNEL,
    )
    assert ok
    assert ("connect", HOST) in dialed and ("request", HOST, "/v1/models", "sk-canary-value") in dialed, (
        "the host the consent check evaluated is the host the kernel's transport dialed"
    )
    requests = [r for r in dialed if r[0] == "request"]
    assert requests[0][3].startswith("sk-ant-") and requests[0][3] != "sk-canary-value", (
        "the negative control ran first — provider-shaped, random-payload, indistinguishable "
        "from a real key to the endpoint (codex/claude r11)"
    )
    assert requests[1][3] == "sk-canary-value"


def test_the_wire_has_no_injection_seam() -> None:
    """There is no transport parameter to substitute and no callable on the probe (r4/r5
    criticals): validate_key always runs the module's own HTTPS transport."""
    import inspect

    from k0.key_capture import https_probe_transport

    assert "transport" not in inspect.signature(validate_key).parameters, (
        "no injection seam: there is no transport parameter to substitute"
    )
    assert callable(https_probe_transport)
    import dataclasses

    from k0.key_capture import PROVIDER_PROBE_ENDPOINTS, ProbeEndpoint

    assert set(PROVIDER_PROBE_ENDPOINTS) == {"anthropic", "openai"}, (
        "the sanctioned-provider table changed — a deliberate act, not a drive-by"
    )
    assert all(
        {f.name for f in dataclasses.fields(e)} == {"host", "path", "auth_scheme", "key_prefix", "extra_headers"}
        for e in PROVIDER_PROBE_ENDPOINTS.values()
    ), "endpoints are data — a callable field would be caller code on the wire"


def test_the_kernels_transport_behavior_against_the_stdlib_boundary(monkeypatch: pytest.MonkeyPatch) -> None:
    """The production wire code, exercised (codex r5): header shape, evidence format, classified
    failures, and the response body consumed-and-discarded."""
    import socket

    from k0.key_capture import https_probe_transport

    record: list[tuple] = []
    _patch_wire(monkeypatch, status=200, record=record)
    # _FakeResponse carries no request-id header -> server token "unknown"
    out = https_probe_transport(HOST, "/v1/models", b"sk-bearer-value")
    assert out.evidence == "https-status:200:server:unknown" and out.failure is None
    assert ("request", HOST, "/v1/models", "sk-bearer-value") in record, (
        "the key rides the provider's auth header and nowhere else"
    )

    out = https_probe_transport.__wrapped__ if hasattr(https_probe_transport, "__wrapped__") else None
    _patch_wire(monkeypatch, status=401)
    out = https_probe_transport(HOST, "/v1/models", b"sk-bearer-value")
    assert out.evidence is None and out.failure == "http-401", "a refused key is its own class"

    _patch_wire(monkeypatch, error=socket.timeout())
    out = https_probe_transport(HOST, "/v1/models", b"sk-bearer-value")
    assert out.failure == "timeout", "an unreachable host says so — the operator's next move differs"

    _patch_wire(monkeypatch, error=ConnectionRefusedError())
    assert https_probe_transport(HOST, "/v1/models", b"x").failure == "connection-refused"


def test_remaining_transport_and_descriptor_branches(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The rest of the wire vocabulary (codex r6): each failure class classifies, and the
    descriptor refuses bad shapes at construction."""
    import ssl

    from k0.key_capture import failure_next_move, https_probe_transport

    _patch_wire(monkeypatch, error=ssl.SSLError("handshake"))
    assert https_probe_transport(HOST, "/v1/models", b"sk-x").failure == "tls-error"

    _patch_wire(monkeypatch, error=OSError("reset"))
    assert https_probe_transport(HOST, "/v1/models", b"sk-x").failure == "transport-error"

    _patch_wire(monkeypatch, status=503)
    assert https_probe_transport(HOST, "/v1/models", b"sk-x").failure == "http-503"

    assert https_probe_transport(HOST, "/v1/models", b"\xff\xfe").failure == "key-not-utf8"

    with pytest.raises(ValueError, match="not a sanctioned provider"):
        validate_key(
            _root(tmp_path), MemoryStore(), NAME, provider="some-random-provider",
            estate_id=ESTATE, kernel_version=KERNEL,
        )

    assert "expired" in failure_next_move("http-401")
    assert "theirs" in failure_next_move("http-503")
    assert failure_next_move("tls-error") != failure_next_move("timeout"), (
        "each class carries its own next move — one generic string would be the dead end "
        "executive_function forbids"
    )


def test_a_redirect_never_validates(monkeypatch: pytest.MonkeyPatch) -> None:
    """3xx is not success (codex r7 critical): a redirect followed with a bearer token is the
    classic key-leak, and a login-page 302 would otherwise 'validate' any garbage."""
    from k0.key_capture import failure_next_move, https_probe_transport

    _patch_wire(monkeypatch, status=302)
    out = https_probe_transport(HOST, "/v1/models", b"sk-x")
    assert out.evidence is None and out.failure == "http-302-redirect"
    assert "do not follow" in failure_next_move(out.failure)


def test_the_tls_context_verifies(monkeypatch: pytest.MonkeyPatch) -> None:
    """glm r7: certificate verification is explicit and capturable, not assumed."""
    import http.client
    import ssl

    captured: dict = {}

    class _CtxConn:
        def __init__(self, host: str, timeout: int = 10, context=None) -> None:
            captured["context"] = context

        def request(self, *a, **k) -> None:
            raise OSError("stop here — the context is what is under test")

        def close(self) -> None:
            pass

    monkeypatch.setattr(http.client, "HTTPSConnection", _CtxConn)
    from k0.key_capture import https_probe_transport

    https_probe_transport(HOST, "/v1/models", b"x")
    ctx = captured.get("context")
    assert isinstance(ctx, ssl.SSLContext)
    assert ctx.verify_mode == ssl.CERT_REQUIRED
    assert ctx.check_hostname


def test_an_indiscriminate_endpoint_can_validate_nothing(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """If garbage gets 2xx, the real key's 2xx is noise (codex r8 critical): the validation
    fails as endpoint-indiscriminate, the row is durable, and the name stays unvalidated."""
    import http.client

    from k0.key_capture import failure_next_move

    root = _root(tmp_path)
    store = MemoryStore()
    _consent_egress(root, _key(tmp_path))
    store.put(NAME, b"sk-real-key")

    class _LaxResponse:
        status = 200

        def read(self) -> bytes:
            return b"{}"

        def getheader(self, name: str):
            return None

    class _LaxConn:
        def __init__(self, host: str, timeout: int = 10, context=None) -> None:
            pass

        def request(self, *a, **k) -> None:
            pass

        def getresponse(self):
            return _LaxResponse()

        def close(self) -> None:
            pass

    monkeypatch.setattr(http.client, "HTTPSConnection", _LaxConn)
    ok = validate_key(
        root, store, NAME, provider="anthropic",
        estate_id=ESTATE, kernel_version=KERNEL,
    )
    assert not ok
    assert supply_state(root, store, NAME) is SecretSupply.CAPTURED_UNVALIDATED
    rows = [r for r in _chain(root) if any("endpoint-indiscriminate" in ref for ref in r.payload_refs)]
    assert len(rows) == 1
    assert "requires authorization" in failure_next_move("endpoint-indiscriminate")


def test_an_inconclusive_control_proves_nothing(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Only a clean 4xx on garbage proves discrimination (codex r12): a redirect, a timeout, a
    5xx on the control leave the endpoint's behavior UNPROVEN — validation must not proceed,
    and the row carries the control's own classified cause."""
    import http.client
    import socket

    from k0.key_capture import failure_next_move

    def run_case(error, status, expected_class):
        sub = tmp_path / expected_class.replace(":", "_")
        sub.mkdir()
        root = _root(sub)
        store = MemoryStore()
        _consent_egress(root, _key(sub))
        store.put(NAME, b"sk-real-key")

        class _Conn:
            def __init__(self, host, timeout=10, context=None):
                self._auth = ""

            def request(self, method, path, headers=None):
                self._auth = (headers or {}).get("x-api-key", "")
                if error is not None:
                    raise error

            def getresponse(self):
                if self._auth.encode() not in (b"sk-real-key",):
                    return _FakeResponse(401 if status is None else status)
                return _FakeResponse(200)

            def close(self):
                pass

        monkeypatch.setattr(http.client, "HTTPSConnection", _Conn)
        ok = validate_key(
            root, store, NAME, provider="anthropic",
            estate_id=ESTATE, kernel_version=KERNEL,
        )
        assert not ok
        rows = [r for r in _chain(root) if any(expected_class in ref for ref in r.payload_refs)]
        assert len(rows) == 1, f"{expected_class}: the inconclusive control must be durable"

    run_case(None, 302, "control-inconclusive-http-302-redirect")
    run_case(socket.timeout(), None, "control-inconclusive-timeout")
    assert "discrimination is unproven" in failure_next_move("control-inconclusive-timeout")
    assert "401/403" in failure_next_move("control-inconclusive-http-502")


def test_a_non_auth_4xx_control_proves_nothing_either(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A 404/429 on garbage is not an authentication refusal (codex r13): the endpoint may not
    be checking keys at all — wrong path, rate limiter fronting it, anything."""
    import http.client

    root = _root(tmp_path)
    store = MemoryStore()
    _consent_egress(root, _key(tmp_path))
    store.put(NAME, b"sk-real-key")

    class _FourOhFour:
        def __init__(self, host, timeout=10, context=None):
            pass

        def request(self, *a, **k) -> None:
            pass

        def getresponse(self):
            return _FakeResponse(404)

        def close(self) -> None:
            pass

    monkeypatch.setattr(http.client, "HTTPSConnection", _FourOhFour)
    ok = validate_key(
        root, store, NAME, provider="anthropic",
        estate_id=ESTATE, kernel_version=KERNEL,
    )
    assert not ok
    rows = [r for r in _chain(root) if any("control-inconclusive-http-404" in ref for ref in r.payload_refs)]
    assert len(rows) == 1, "a 404 on the control is inconclusive, durable, and classified"
