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
    FileStore,
    SecretIntegrityError,
    MemoryStore,
    SecretSupply,
    decline_capture,
    default_store,
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


def _materials(tmp_path: Path, key: Path) -> dict:
    """The sovereign's verification materials — mandatory on the enforcement path (r19)."""
    from k0.ratifier import write_allowed_signers

    allowed = tmp_path / "allowed_signers"
    write_allowed_signers(allowed, "ratifier@test", key.with_suffix(".pub").read_text().strip())
    scratch = tmp_path / "scratch"
    scratch.mkdir(exist_ok=True)
    return {"allowed_signers": allowed, "principal": "ratifier@test", "scratch_dir": scratch}

from k0.key_capture import PROVIDER_PROBE_ENDPOINTS as _ENDPOINTS

ANTHROPIC = _ENDPOINTS["anthropic"]


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
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
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
        provider="anthropic", **materials,
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


def test_a_failed_validation_writes_a_classified_row_and_is_not_supply(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    root = _root(tmp_path)
    store = MemoryStore()
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    _patch_wire(monkeypatch, status=401)
    store.put(NAME, b"sk-canary-value")

    ok = validate_key(
        root,
        store,
        NAME,
        provider="anthropic", **materials,
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
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    with pytest.raises(ValueError, match="nothing captured"):
        validate_key(
            root,
            store,
            NAME,
            provider="anthropic", **materials,
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )


def test_the_decline_path_is_dark_and_never_nags(tmp_path: Path) -> None:
    root = _root(tmp_path)
    store = MemoryStore()
    key = _key(tmp_path)
    materials = _materials(tmp_path, key)

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
            provider="anthropic", **materials,
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )
    assert verify_chain_at(root).ok


def test_no_secret_value_ever_touches_the_chain(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The canary test: run the whole ceremony with a distinctive value, then scan the ledger."""
    root = _root(tmp_path)
    store = MemoryStore()
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    _patch_wire(monkeypatch)
    canary = b"sk-canary-7f3c9a1b-never-on-disk"

    elicit_capture(root, NAME, estate_id=ESTATE, kernel_version=KERNEL)
    store.put(NAME, canary)
    validate_key(
        root,
        store,
        NAME,
        provider="anthropic", **materials,
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
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    _patch_wire(monkeypatch)
    store.put(NAME, b"sk-first-value")
    assert validate_key(
        root, store, NAME, provider="anthropic", **materials,
        estate_id=ESTATE, kernel_version=KERNEL,
    )
    assert supply_state(root, store, NAME) is SecretSupply.VALIDATED

    store.put(NAME, b"sk-replaced-value")
    assert supply_state(root, store, NAME) is SecretSupply.CAPTURED_UNVALIDATED

    store._values.clear()
    assert supply_state(root, store, NAME) is SecretSupply.ABSENT


def test_file_store_round_trip_and_is_the_only_durable_backend(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """FileStore is the durable backend: round-trip, missing is None, no subprocess."""
    monkeypatch.setenv("REINS_SECRET_STORE", str(tmp_path / "store"))
    monkeypatch.setenv("PATH", "/usr/bin")
    store = FileStore(root=tmp_path / "store")
    assert store.backend_id == "file"
    assert default_store().backend_id == "file"
    assert not store.has(NAME)
    assert store.get(NAME) is None
    store.put(NAME, b"sk-file-round-trip")
    assert store.has(NAME)
    assert store.get(NAME) == b"sk-file-round-trip"
    store.put(NAME, b"sk-file-overwritten")
    assert store.get(NAME) == b"sk-file-overwritten"
    assert store.delete(NAME) is True
    assert not store.has(NAME)
    assert store.delete(NAME) is False
    with pytest.raises(ValueError, match="path segment"):
        store.put("a/b", b"x")
    with pytest.raises(ValueError, match="no collisions"):
        store.put("a:b", b"x")
    with pytest.raises(ValueError, match="no collisions"):
        store.put("a?b", b"x")


def test_the_pass_backend_is_gone(tmp_path: Path) -> None:
    """Operator ruling 2026-09-16: pass/gopass are never used to manage secrets.

    The class, the availability probe, and the subprocess import that only it
    needed are all removed — not deprecated. A backend that still exists is a
    backend something can still be pointed at.
    """
    import k0.key_capture as kc

    assert not hasattr(kc, "PassStore")
    assert not hasattr(kc, "pass_backend_available")
    assert not hasattr(kc, "subprocess"), "nothing here shells out any more"
    source = (Path(kc.__file__)).read_text(encoding="utf-8")
    assert "pass insert" not in source
    assert "pass show" not in source
    # default_store is unconditional now, which is what lets every caller drop
    # its "this had better not be pass" branch.
    assert default_store().backend_id == "file"


# ---------------------------------------------------------------------------
# Blob format 2: subkey separation, name binding, versioning, migration.
# ---------------------------------------------------------------------------


def test_put_writes_format_2_and_get_still_reads_a_format_1_blob(tmp_path: Path) -> None:
    """A v1 store keeps working through the rollout, and the first put migrates."""
    from k0.key_capture import _file_wrap, blob_format_of

    store = FileStore(root=tmp_path / "store")
    key = store._key()
    blob_path = store._blob_path(NAME)
    blob_path.write_bytes(_file_wrap(key, b"\x00" * 16, b"sk-legacy-v1"))
    assert blob_format_of(blob_path.read_bytes()) == 1
    assert store.blob_format(NAME) == 1
    assert store.get(NAME) == b"sk-legacy-v1", "a v1 blob must still read"

    store.put(NAME, b"sk-rewritten")
    assert store.blob_format(NAME) == 2, "the first put migrates the blob"
    assert store.get(NAME) == b"sk-rewritten"


def test_get_does_not_rewrite_a_v1_blob(tmp_path: Path) -> None:
    """Migration is on PUT and nothing else. GET runs from watchdogs and units;
    a getter that writes is a getter that fails on a read-only mount."""
    from k0.key_capture import _file_wrap

    store = FileStore(root=tmp_path / "store")
    blob_path = store._blob_path(NAME)
    blob_path.write_bytes(_file_wrap(store._key(), b"\x01" * 16, b"sk-legacy-v1"))
    before = blob_path.read_bytes()
    for _ in range(3):
        assert store.get(NAME) == b"sk-legacy-v1"
    assert blob_path.read_bytes() == before
    assert store.blob_format(NAME) == 1


def test_v2_derives_separate_enc_and_mac_subkeys(tmp_path: Path) -> None:
    """v1 used the SAME 32 bytes for the SHAKE keystream and the HMAC tag."""
    from k0.key_capture import _v2_subkeys

    key = b"k" * 32
    nonce = b"n" * 16
    enc, mac = _v2_subkeys(key, nonce)
    assert enc != mac
    assert enc != key and mac != key
    assert len(enc) == len(mac) == 32
    # nonce-bound: a different nonce yields different subkeys
    other_enc, other_mac = _v2_subkeys(key, b"m" * 16)
    assert other_enc != enc and other_mac != mac


@pytest.mark.parametrize(
    "corrupt",
    [
        pytest.param(lambda b: b[:-1] + bytes([b[-1] ^ 1]), id="last-ciphertext-byte"),
        pytest.param(lambda b: b[:20] + bytes([b[20] ^ 1]) + b[21:], id="tag-byte"),
        pytest.param(lambda b: b[:6] + bytes([b[6] ^ 1]) + b[7:], id="nonce-byte"),
        pytest.param(lambda b: b[:-1], id="truncated"),
        pytest.param(lambda b: b[:40], id="truncated-below-header"),
    ],
)
def test_a_tampered_blob_raises_a_typed_integrity_error_not_none(
    tmp_path: Path, corrupt
) -> None:
    """THE defect this whole format change exists for.

    get() caught ValueError and returned None, so a tampered blob was
    indistinguishable from an absent one and every caller's "not found, run the
    put dialogue" arm fired on a secret that was sitting right there.
    """
    store = FileStore(root=tmp_path / "store")
    store.put(NAME, b"sk-file-canary")
    blob_path = store._blob_path(NAME)
    blob_path.write_bytes(corrupt(blob_path.read_bytes()))

    with pytest.raises(SecretIntegrityError) as exc:
        store.get(NAME)
    assert "sk-file-canary" not in str(exc.value), "the error must not carry the value"
    assert not isinstance(exc.value, ValueError), (
        "a ValueError subclass would be swallowed by the same except arms again"
    )


def test_a_blob_cannot_be_renamed_into_another_secrets_slot(tmp_path: Path) -> None:
    """The name is in the authenticated data, so moving the file does not move
    the secret. Without it, `cp litellm-master-key.bin api-openai.bin` silently
    makes one credential answer for the other.

    THE TWO NAMES ARE THE SAME LENGTH ON PURPOSE. The AAD is a length prefix
    followed by the name bytes; with names of different lengths this test
    passes even when the name itself is dropped from the AAD, because the
    LENGTH alone still differs. Measured: with api-openai (10) against
    api-mistral (11), removing the name bytes left the test green. Same-length
    names make the name bytes the only thing that varies.
    """
    store = FileStore(root=tmp_path / "store")
    assert len("api-openai") == len("api-cohere"), "the discriminator must be the name, not its length"
    store.put("api-openai", b"sk-openai-real")
    store.put("api-cohere", b"sk-cohere-real")
    hijacked = store._blob_path("api-cohere")
    hijacked.write_bytes(store._blob_path("api-openai").read_bytes())

    with pytest.raises(SecretIntegrityError):
        store.get("api-cohere")
    assert store.get("api-openai") == b"sk-openai-real"


def test_a_v1_blob_can_be_renamed_which_is_why_v2_binds_the_name(tmp_path: Path) -> None:
    """A deliberate NEGATIVE CONTROL outside the supported set: format 1 has no
    name binding, so the rename attack works against it. It is pinned here so
    the v2 property above is shown to be doing the work rather than being an
    accident of the fixture."""
    from k0.key_capture import _file_wrap

    store = FileStore(root=tmp_path / "store")
    key = store._key()
    store._blob_path("api-openai").write_bytes(_file_wrap(key, b"\x02" * 16, b"sk-openai-real"))
    store._blob_path("api-cohere").write_bytes(store._blob_path("api-openai").read_bytes())
    assert store.get("api-cohere") == b"sk-openai-real", (
        "v1 read the hijacked blob happily — that is the defect v2 closes"
    )


def test_a_v1_blob_whose_nonce_opens_with_the_v2_header_refuses_rather_than_lying(
    tmp_path: Path,
) -> None:
    """The documented 2**-32 limit of header-based discrimination.

    Such a blob is read as v2, fails to authenticate, and is REPORTED. That is
    a named refusal with the right next action, not a silent wrong answer — and
    it is why there is no "retry as v1" arm: that fallback would give a genuine
    v2 tamper a second, weaker path instead of refusing.
    """
    from k0.key_capture import _SECRET_MAGIC_V2, _file_wrap, blob_format_of

    store = FileStore(root=tmp_path / "store")
    key = store._key()
    collide_nonce = _SECRET_MAGIC_V2 + b"\x07" * 12
    blob = _file_wrap(key, collide_nonce, b"sk-unlucky")
    assert blob_format_of(blob) == 2, "by construction it looks like v2"
    store._blob_path(NAME).write_bytes(blob)

    with pytest.raises(SecretIntegrityError):
        store.get(NAME)


def test_the_stored_blob_is_not_the_plaintext(tmp_path: Path) -> None:
    store = FileStore(root=tmp_path / "store")
    store.put(NAME, b"sk-file-canary")
    assert b"sk-file-canary" not in store._blob_path(NAME).read_bytes()
    assert b"sk-file-canary" not in store.key_path.read_bytes()


# ---------------------------------------------------------------------------
# History and tombstones.
# ---------------------------------------------------------------------------


def test_put_archives_the_superseded_blob_and_bounds_the_history(tmp_path: Path) -> None:
    store = FileStore(root=tmp_path / "store")
    for i in range(8):
        store.put(NAME, f"sk-v{i}".encode())
    history = store.history(NAME)
    assert len(history) == 5, "history is bounded: it is ciphertext of real credentials"
    assert store.get(NAME) == b"sk-v7"
    assert sorted(history) == list(history), "timestamps come back in order"
    # timestamps only — never a value, never a digest of one
    for stamp in history:
        assert "sk-v" not in stamp


def test_delete_purges_the_history_and_leaves_a_tombstone(tmp_path: Path) -> None:
    """Delete is the one operation whose whole purpose is that the value stops
    existing. Keeping superseded blobs through it would defeat exactly that."""
    store = FileStore(root=tmp_path / "store")
    for i in range(3):
        store.put(NAME, f"sk-v{i}".encode())
    assert store.history(NAME)

    assert store.delete(NAME) is True
    history = store.history(NAME)
    assert len(history) == 1 and history[0].endswith("(deleted)")
    hdir = tmp_path / "store" / ".history"
    assert list(hdir.glob(f"{NAME}.*.bin")) == []
    for leftover in hdir.iterdir():
        assert b"sk-v" not in leftover.read_bytes()


def test_history_of_an_unknown_name_is_empty_and_the_name_is_still_validated(
    tmp_path: Path,
) -> None:
    store = FileStore(root=tmp_path / "store")
    assert store.history("never-stored") == ()
    with pytest.raises(ValueError, match="path segment"):
        store.history("../escape")


def test_history_files_are_not_listed_as_secrets(tmp_path: Path) -> None:
    store = FileStore(root=tmp_path / "store")
    store.put(NAME, b"sk-a")
    store.put(NAME, b"sk-b")
    assert store.names() == (NAME,)


def test_a_dotted_name_is_not_a_prefix_of_another_names_history(tmp_path: Path) -> None:
    """Names may contain dots, so ``api`` and ``api.openai`` are both legal and
    the glob ``api.*.bin`` matches the longer name's archives too. Found in
    review of PR 44: history("api") reported api.openai's stamps, put("api")
    pruned api.openai's archives toward api's cap, and delete("api") destroyed
    api.openai's archived ciphertext. Each of the three call sites is pinned
    here; the live store has no dotted names today, so this is the grammar's
    hazard rather than a measured incident."""
    store = FileStore(root=tmp_path / "store")
    for i in range(3):
        store.put("api", f"sk-short-{i}".encode())
    for i in range(3):
        store.put("api.openai", f"sk-long-{i}".encode())
    hdir = tmp_path / "store" / ".history"
    long_archives = sorted(hdir.glob("api.openai.*.bin"))
    assert len(long_archives) == 2, "the longer name has exactly its own two supersessions"

    # history(): the short name reports only its own stamps.
    assert len(store.history("api")) == 2
    assert all("openai" not in stamp for stamp in store.history("api"))
    assert len(store.history("api.openai")) == 2

    # _archive() pruning: the short name's cap counts only its own archives.
    for i in range(3, 10):
        store.put("api", f"sk-short-{i}".encode())
    assert len(store.history("api")) == 5, "bounded by ITS OWN supersessions"
    assert sorted(hdir.glob("api.openai.*.bin")) == long_archives, "prune touched another name"

    # delete(): purging the short name's history leaves the longer name whole.
    assert store.delete("api") is True
    assert sorted(hdir.glob("api.openai.*.bin")) == long_archives, "delete purged another name"
    assert store.get("api.openai") == b"sk-long-2"
    assert len(store.history("api.openai")) == 2
    short = store.history("api")
    assert len(short) == 1 and short[0].endswith("(deleted)")


# ---------------------------------------------------------------------------
# The key boundary.
# ---------------------------------------------------------------------------


def test_the_key_can_live_outside_the_store_root(tmp_path: Path) -> None:
    """The key sat beside the blobs it protects, inside the backed-up home, so
    one backup set carried both halves."""
    key_file = tmp_path / "elsewhere" / "secret.key"
    store = FileStore(root=tmp_path / "store", key_file=key_file)
    store.put(NAME, b"sk-out-of-tree")
    assert key_file.is_file()
    assert store.key_path == key_file
    assert not (tmp_path / "store" / ".key").exists()
    assert store.get(NAME) == b"sk-out-of-tree"
    assert oct(key_file.stat().st_mode & 0o777) == "0o600"
    # The blobs alone are not enough, and the refusal is TYPED: a backup set that
    # captured the ciphertext without the key cannot tell itself apart from a
    # tampered blob, and both are integrity failures rather than wrong plaintext.
    with pytest.raises(SecretIntegrityError):
        FileStore(root=tmp_path / "store", key_file=tmp_path / "other.key").get(NAME)


def test_the_key_file_env_var_is_honoured(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    key_file = tmp_path / "elsewhere" / "secret.key"
    monkeypatch.setenv("REINS_SECRET_KEY_FILE", str(key_file))
    monkeypatch.setenv("REINS_SECRET_STORE", str(tmp_path / "store"))
    store = default_store()
    assert store.key_path == key_file


# ---------------------------------------------------------------------------
# Put-time value validation.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("value", "flag"),
    [
        pytest.param(b"\xef\xbb\xbfsk-abc", "bom", id="utf8-bom"),
        pytest.param(b"sk-abc\r\n", "cr", id="crlf"),
        pytest.param(b"sk-abc\n", "trailing-newline", id="trailing-newline"),
        pytest.param(b"line-one\nline-two\n\n", "trailing-blank-line", id="multiline-blank-line"),
        pytest.param(b" sk-abc", "leading-whitespace", id="leading-space"),
        pytest.param(b"sk-abc ", "trailing-whitespace", id="trailing-space"),
        pytest.param(b"sk-abc\t", "trailing-whitespace", id="trailing-tab"),
        pytest.param(b"sk\x00abc", "nul", id="nul"),
        pytest.param(b"", "empty", id="empty"),
    ],
)
def test_a_value_with_a_known_breaking_byte_shape_is_refused(value: bytes, flag: str) -> None:
    from k0.key_capture import validate_secret_value, value_shape_flags

    assert flag in value_shape_flags(value)
    with pytest.raises(ValueError) as exc:
        validate_secret_value(value)
    assert flag in str(exc.value)
    assert "Next action:" in str(exc.value), "a refusal without a next move is a dead end"
    assert value.decode("utf-8", "replace").strip() not in str(exc.value) or not value.strip()


def test_a_clean_value_is_accepted_and_binary_is_only_flagged(tmp_path: Path) -> None:
    from k0.key_capture import validate_secret_value, value_shape_flags

    validate_secret_value(b"sk-proj-abcdef0123456789")
    assert value_shape_flags(b"sk-proj-abcdef0123456789") == ()
    # non-utf8 is INFORMATIONAL: a binary secret is legitimate
    assert value_shape_flags(b"\xff\xfe\x01") == ("non-utf8",)
    validate_secret_value(b"\xff\xfe\x01")


def test_an_interior_newline_is_information_and_a_trailing_one_is_a_refusal() -> None:
    """The ruling that corrected the first cut of this change, pinned in both
    directions as it was stated: a two-line value is ACCEPTED, a value ending in
    a newline is REFUSED.

    Measured on the 184 live values: 7 carry interior newlines and all 7 are
    documents — an ssh private key, a GitHub App private key, two Google
    service-account JSONs, an rclone config, a BitLocker recovery blob, a GPG
    passphrase file. Refusing interior LF would have failed every one at its
    next put, which is the replica push failing on exactly the values that
    matter most.
    """
    from k0.key_capture import (
        INFORMATIONAL_VALUE_FLAGS,
        REFUSABLE_VALUE_FLAGS,
        validate_secret_value,
        value_shape_flags,
    )

    assert "multiline" in INFORMATIONAL_VALUE_FLAGS
    assert "multiline" not in REFUSABLE_VALUE_FLAGS
    assert "trailing-newline" in REFUSABLE_VALUE_FLAGS

    two_line = b"-----BEGIN PRIVATE KEY-----\nc3ludGhldGlj"
    assert value_shape_flags(two_line) == ("multiline",)
    validate_secret_value(two_line)  # accepted

    with pytest.raises(ValueError, match="trailing-newline"):
        validate_secret_value(b"sk-single-line\n")


def test_the_store_itself_does_not_validate_so_migration_can_rewrite_anything(
    tmp_path: Path,
) -> None:
    """Validation lives at the command surface — the single place a value has to
    get past. FileStore.put stays a byte writer so the v1 blobs already holding
    a BOM can be read and rewritten rather than becoming unrewritable."""
    store = FileStore(root=tmp_path / "store")
    store.put(NAME, b"\xef\xbb\xbfsk-already-stored")
    assert store.get(NAME) == b"\xef\xbb\xbfsk-already-stored"


# ---------------------------------------------------------------------------
# The loopback capability token.
# ---------------------------------------------------------------------------


def test_the_capability_token_is_0600_in_the_runtime_dir_and_is_stable(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    from k0.key_capture import mint_secret_command_token, secret_command_token_path

    runtime = tmp_path / "runtime"
    runtime.mkdir(mode=0o700)
    monkeypatch.setenv("XDG_RUNTIME_DIR", str(runtime))
    token = mint_secret_command_token()
    assert token and mint_secret_command_token() == token, "minting is idempotent per boot"
    path = secret_command_token_path()
    assert oct(path.stat().st_mode & 0o777) == "0o600"
    assert path.read_text(encoding="ascii").strip() == token


@pytest.mark.parametrize("presented", [None, "", "not-the-token"])
def test_a_wrong_or_absent_token_never_matches(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, presented
) -> None:
    from k0.key_capture import mint_secret_command_token, secret_command_token_matches

    runtime = tmp_path / "runtime"
    runtime.mkdir(mode=0o700)
    monkeypatch.setenv("XDG_RUNTIME_DIR", str(runtime))
    real = mint_secret_command_token()
    assert secret_command_token_matches(real) is True
    assert secret_command_token_matches(presented) is False


def test_a_group_or_world_accessible_runtime_dir_is_refused(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The precondition is checked at the moment of use, not asserted in a
    comment: a runtime dir another account can reach would let it plant the
    token file first."""
    from k0.key_capture import SecretCommandTokenError, mint_secret_command_token

    runtime = tmp_path / "runtime"
    runtime.mkdir(mode=0o755)
    monkeypatch.setenv("XDG_RUNTIME_DIR", str(runtime))
    with pytest.raises(SecretCommandTokenError) as exc:
        mint_secret_command_token()
    assert "Next action:" in str(exc.value)


def test_a_group_or_world_readable_token_file_is_refused(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The runtime dir check and the token FILE check are two different guards.

    Found by mutation: dropping this one left every token test green, because
    they all exercised the directory guard. A token another account can read is
    a token another account can present.
    """
    from k0.key_capture import (
        SecretCommandTokenError,
        mint_secret_command_token,
        secret_command_token_path,
    )

    runtime = tmp_path / "runtime"
    runtime.mkdir(mode=0o700)
    monkeypatch.setenv("XDG_RUNTIME_DIR", str(runtime))
    mint_secret_command_token()
    secret_command_token_path().chmod(0o644)

    with pytest.raises(SecretCommandTokenError) as exc:
        mint_secret_command_token()
    assert "Next action:" in str(exc.value)


def test_an_empty_token_file_is_refused(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    from k0.key_capture import (
        SecretCommandTokenError,
        mint_secret_command_token,
        secret_command_token_path,
    )

    runtime = tmp_path / "runtime"
    runtime.mkdir(mode=0o700)
    monkeypatch.setenv("XDG_RUNTIME_DIR", str(runtime))
    mint_secret_command_token()
    secret_command_token_path().write_text("", encoding="ascii")

    with pytest.raises(SecretCommandTokenError):
        mint_secret_command_token()


def test_an_absent_runtime_dir_denies_rather_than_falling_back_to_tmp(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    from k0.key_capture import SecretCommandTokenError, secret_command_token_matches

    monkeypatch.setenv("XDG_RUNTIME_DIR", str(tmp_path / "does-not-exist"))
    with pytest.raises(SecretCommandTokenError):
        from k0.key_capture import mint_secret_command_token

        mint_secret_command_token()
    # and the matcher denies rather than passing for want of something to check
    assert secret_command_token_matches("anything") is False


def test_validation_against_an_unconsented_host_never_reaches_the_validator(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The gate on the ACTUAL egress path (codex r2 critical): the validator is the wire, so an
    unconsented host must refuse before the callable is invoked — not after, not with a warning."""
    from k0.egress_consent import EgressConsentError

    root = _root(tmp_path)
    store = MemoryStore()
    key = _key(tmp_path)
    materials = _materials(tmp_path, key)
    store.put(NAME, b"sk-canary-value")

    dialed: list[tuple] = []
    _patch_wire(monkeypatch, record=dialed)
    with pytest.raises(EgressConsentError, match="no egress allowlist is ratified"):
        validate_key(
            root, store, NAME, provider="openai", **materials,
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
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    store.put(NAME, b"sk-canary-value")

    dialed: list[tuple] = []
    _patch_wire(monkeypatch, record=dialed)
    ok = validate_key(
        root, store, NAME,
        provider="anthropic", **materials,
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

    from k0.key_capture import _https_probe_transport

    assert "transport" not in inspect.signature(validate_key).parameters, (
        "no injection seam: there is no transport parameter to substitute"
    )
    assert callable(_https_probe_transport)
    import dataclasses

    from k0.key_capture import PROVIDER_PROBE_ENDPOINTS, ProbeEndpoint

    assert set(PROVIDER_PROBE_ENDPOINTS) == {"anthropic", "openai"}, (
        "the sanctioned-provider table changed — a deliberate act, not a drive-by"
    )
    assert all(
        {f.name for f in dataclasses.fields(e)} == {"host", "path", "auth_scheme", "key_prefix", "extra_headers"}
        for e in PROVIDER_PROBE_ENDPOINTS.values()
    ), "endpoints are data — a callable field would be caller code on the wire"


def test_the_kernels_transport_behavior_against_the_stdlib_boundary(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The production wire code, exercised (codex r5): header shape, evidence format, classified
    failures, and the response body consumed-and-discarded."""
    import socket

    from k0.key_capture import _https_probe_transport

    root = _root(tmp_path)
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    record: list[tuple] = []
    _patch_wire(monkeypatch, status=200, record=record)
    # _FakeResponse carries no request-id header -> server token "unknown"
    out = _https_probe_transport(root, ANTHROPIC, b"sk-bearer-value", **materials)
    assert out.evidence == "https-status:200:server:unknown" and out.failure is None
    assert ("request", HOST, "/v1/models", "sk-bearer-value") in record, (
        "the key rides the provider's auth header and nowhere else"
    )

    _patch_wire(monkeypatch, status=401)
    out = _https_probe_transport(root, ANTHROPIC, b"sk-bearer-value", **materials)
    assert out.evidence is None and out.failure == "http-401", "a refused key is its own class"

    _patch_wire(monkeypatch, error=socket.timeout())
    out = _https_probe_transport(root, ANTHROPIC, b"sk-bearer-value", **materials)
    assert out.failure == "timeout", "an unreachable host says so — the operator's next move differs"

    _patch_wire(monkeypatch, error=ConnectionRefusedError())
    assert _https_probe_transport(root, ANTHROPIC, b"sk-x", **materials).failure == "connection-refused"


def test_remaining_transport_and_descriptor_branches(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The rest of the wire vocabulary (codex r6): each failure class classifies, and the
    descriptor refuses bad shapes at construction."""
    import ssl

    from k0.key_capture import failure_next_move, _https_probe_transport

    root = _root(tmp_path)
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    _patch_wire(monkeypatch, error=ssl.SSLError("handshake"))
    assert _https_probe_transport(root, ANTHROPIC, b"sk-x", **materials).failure == "tls-error"

    _patch_wire(monkeypatch, error=OSError("reset"))
    assert _https_probe_transport(root, ANTHROPIC, b"sk-x", **materials).failure == "transport-error"

    _patch_wire(monkeypatch, status=503)
    assert _https_probe_transport(root, ANTHROPIC, b"sk-x", **materials).failure == "http-503"

    assert _https_probe_transport(root, ANTHROPIC, b"\xff\xfe", **materials).failure == "key-not-utf8"

    with pytest.raises(ValueError, match="not a sanctioned provider"):
        validate_key(
            root, MemoryStore(), NAME, provider="some-random-provider", **materials,
            estate_id=ESTATE, kernel_version=KERNEL,
        )

    assert "expired" in failure_next_move("http-401")
    assert "theirs" in failure_next_move("http-503")
    assert failure_next_move("tls-error") != failure_next_move("timeout"), (
        "each class carries its own next move — one generic string would be the dead end "
        "executive_function forbids"
    )


def test_a_redirect_never_validates(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """3xx is not success (codex r7 critical): a redirect followed with a bearer token is the
    classic key-leak, and a login-page 302 would otherwise 'validate' any garbage."""
    from k0.key_capture import failure_next_move, _https_probe_transport

    root = _root(tmp_path)
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    _patch_wire(monkeypatch, status=302)
    out = _https_probe_transport(root, ANTHROPIC, b"sk-x", **materials)
    assert out.evidence is None and out.failure == "http-302-redirect"
    assert "do not follow" in failure_next_move(out.failure)


def test_the_tls_context_verifies(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
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
    from k0.key_capture import _https_probe_transport

    root = _root(tmp_path)
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    _https_probe_transport(root, ANTHROPIC, b"sk-x", **materials)
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
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
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
        root, store, NAME, provider="anthropic", **materials,
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
        key = _key(sub)
        _consent_egress(root, key)
        materials = _materials(sub, key)
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
            root, store, NAME, provider="anthropic", **materials,
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
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
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
        root, store, NAME, provider="anthropic", **materials,
        estate_id=ESTATE, kernel_version=KERNEL,
    )
    assert not ok
    rows = [r for r in _chain(root) if any("control-inconclusive-http-404" in ref for ref in r.payload_refs)]
    assert len(rows) == 1, "a 404 on the control is inconclusive, durable, and classified"


def test_a_blanket_403_control_proves_nothing(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A 403 can be a WAF answering before the credential is even read (codex r14): only 401 is
    the unambiguous authentication refusal."""
    import http.client

    root = _root(tmp_path)
    store = MemoryStore()
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    store.put(NAME, b"sk-real-key")

    class _FourOhThree:
        def __init__(self, host, timeout=10, context=None):
            pass

        def request(self, *a, **k) -> None:
            pass

        def getresponse(self):
            return _FakeResponse(403)

        def close(self) -> None:
            pass

    monkeypatch.setattr(http.client, "HTTPSConnection", _FourOhThree)
    ok = validate_key(
        root, store, NAME, provider="anthropic", **materials,
        estate_id=ESTATE, kernel_version=KERNEL,
    )
    assert not ok
    rows = [r for r in _chain(root) if any("control-inconclusive-http-403" in ref for ref in r.payload_refs)]
    assert len(rows) == 1, "a blanket 403 is inconclusive, durable, classified"


def test_the_wire_refuses_a_non_registry_endpoint(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The membership guard, tested (claude r17): a caller-minted endpoint — even one naming a
    consented host — is not a legal destination. Only registry data reaches the wire."""
    from k0.key_capture import ProbeEndpoint, _https_probe_transport

    root = _root(tmp_path)
    key = _key(tmp_path)
    _consent_egress(root, key)
    materials = _materials(tmp_path, key)
    dialed: list[tuple] = []
    _patch_wire(monkeypatch, record=dialed)

    from k0.key_capture import PROVIDER_PROBE_ENDPOINTS

    forged = ProbeEndpoint("api.anthropic.com", "/v1/models", "x-api-key", "sk-ant-", (("anthropic-version", "2023-06-01"),))
    assert forged == PROVIDER_PROBE_ENDPOINTS["anthropic"], "fixture premise: fields match"
    with pytest.raises(ValueError, match="registry"):
        _https_probe_transport(root, forged, b"sk-canary-value", **materials)
    # — an EQUAL-but-rebuilt endpoint is refused: the guard compares identity, so the caller
    # must pass the registry's own object (claude r20).
    registered = PROVIDER_PROBE_ENDPOINTS["anthropic"]
    _https_probe_transport(root, registered, b"sk-canary-value", **materials)
    assert ("connect", "api.anthropic.com") in dialed, "the registry's own object dials"
    custom = ProbeEndpoint("api.anthropic.com", "/v1/messages", "x-api-key", "sk-ant-", ())
    with pytest.raises(ValueError, match="registry"):
        _https_probe_transport(root, custom, b"sk-canary-value", **materials)


def test_the_bearer_scheme_and_extra_headers_reach_the_wire(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Branch coverage on the transport's scheme/header handling (codex r17)."""
    import http.client

    from k0.key_capture import PROVIDER_PROBE_ENDPOINTS, _https_probe_transport

    root = _root(tmp_path)
    # consent both provider hosts
    from k0.egress_consent import EgressAllowlist
    from k0.egress_consent import accept as accept_egress
    from k0.egress_consent import elicit_allowlist

    key = _key(tmp_path)
    materials = _materials(tmp_path, key)
    both = EgressAllowlist(hosts=("api.anthropic.com", "api.openai.com"))
    elicit_allowlist(root, both, estate_id=ESTATE, kernel_version=KERNEL)
    accept_egress(root, both, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    seen: list[dict] = []

    class _Conn:
        def __init__(self, host, timeout=10, context=None):
            self._host = host

        def request(self, method, path, headers=None) -> None:
            seen.append({"host": self._host, "path": path, "headers": dict(headers or {})})

        def getresponse(self):
            return _FakeResponse(401)

        def close(self) -> None:
            pass

    monkeypatch.setattr(http.client, "HTTPSConnection", _Conn)
    _https_probe_transport(root, PROVIDER_PROBE_ENDPOINTS["openai"], b"sk-x", **materials)
    assert seen[0]["headers"].get("Authorization") == "Bearer sk-x"
    _https_probe_transport(root, PROVIDER_PROBE_ENDPOINTS["anthropic"], b"sk-ant-x", **materials)
    assert seen[1]["headers"].get("x-api-key") == "sk-ant-x"
    assert seen[1]["headers"].get("anthropic-version") == "2023-06-01", (
        "the provider's extra headers ride the request"
    )


def test_the_wire_gate_authenticates_the_consent_row(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The REAL wire path, with wrong-principal materials (codex r19): hash links alone cannot
    catch an appended forged row, so validate_key must refuse before dialing when the consent
    row does not authenticate."""
    from k0.egress_consent import EgressConsentError

    root = _root(tmp_path)
    store = MemoryStore()
    key = _key(tmp_path)
    _consent_egress(root, key)
    store.put(NAME, b"sk-real-key")

    dialed: list[tuple] = []
    _patch_wire(monkeypatch, record=dialed)
    bad = {**_materials(tmp_path, key), "principal": "somebody-else@test"}
    with pytest.raises(EgressConsentError, match="does not verify"):
        validate_key(
            root, store, NAME, provider="anthropic", **bad,
            estate_id=ESTATE, kernel_version=KERNEL,
        )
    assert dialed == [], "an unauthenticated consent row closes the wire before any dial"


def test_the_registry_cannot_be_amended_by_a_caller() -> None:
    """codex r20: a mutable registry would let a caller register its own endpoint and pass the
    membership guard. The proxy must refuse every write."""
    from k0.key_capture import PROVIDER_PROBE_ENDPOINTS, ProbeEndpoint

    with pytest.raises(TypeError):
        PROVIDER_PROBE_ENDPOINTS["evil"] = ProbeEndpoint("api.anthropic.com", "/x", "bearer", "sk-")
    with pytest.raises(AttributeError):
        PROVIDER_PROBE_ENDPOINTS.pop("openai")
    with pytest.raises(AttributeError):
        PROVIDER_PROBE_ENDPOINTS.clear()


def test_value_shape_flags_only_ever_returns_the_closed_vocabulary() -> None:
    """What makes it safe to print flags beside a secret's name.

    The flags are computed FROM the value, so a taint analyser flags the print.
    They are not derived from it: every element comes from one closed literal
    tuple. Fuzzed rather than asserted, so a future flag built by formatting
    some of the value into a string is caught here rather than in review.
    """
    import random

    from k0.key_capture import (
        REFUSABLE_VALUE_FLAGS,
        VALUE_SHAPE_FLAGS,
        value_shape_flags,
    )

    allowed = set(VALUE_SHAPE_FLAGS)
    assert REFUSABLE_VALUE_FLAGS <= allowed

    rng = random.Random(20260916)
    corpus = [b"", b"sk-plain", b"\xef\xbb\xbfsk", b" sk ", b"sk\r\n", b"sk\x00", b"\xff\xfe"]
    corpus += [bytes(rng.randrange(256) for _ in range(rng.randrange(0, 40))) for _ in range(500)]
    seen = set()
    for value in corpus:
        flags = value_shape_flags(value)
        assert set(flags) <= allowed, f"undeclared flag from {len(value)} bytes"
        seen |= set(flags)
    assert seen >= {"empty", "bom", "cr", "nul", "leading-whitespace"}, (
        "the corpus must actually exercise the vocabulary, or the check is vacuous"
    )


@pytest.mark.parametrize(
    ("value", "accepted", "expected"),
    [
        pytest.param(b"sk-single", True, (), id="single-line-clean"),
        pytest.param(b"sk-single\n", False, ("trailing-newline",), id="single-line-one-lf"),
        pytest.param(b"sk-single\n\n", False, ("trailing-blank-line",), id="single-line-two-lf"),
        pytest.param(b"line\nline", True, ("multiline",), id="multi-line-no-lf"),
        pytest.param(b"line\nline\n", True, ("multiline",), id="multi-line-one-lf"),
        pytest.param(
            b"line\nline\n\n", False, ("multiline", "trailing-blank-line"), id="multi-line-two-lf"
        ),
        pytest.param(b"\n", False, ("trailing-newline",), id="bare-lf"),
    ],
)
def test_the_trailing_newline_policy(value: bytes, accepted: bool, expected: tuple) -> None:
    """Stated once in value_shape_flags' docstring, pinned once here:

      * a single-line value ends at its last non-whitespace byte;
      * a multi-line value may end in exactly one LF;
      * two trailing LFs are a defect either way.

    The refusal catches the single-line key pasted with its Enter. It is not for
    rejecting files for being files — measured, three live values are a GitHub
    App private key, a Google service-account JSON and an rclone config, and all
    three end in the newline their format ends in.
    """
    from k0.key_capture import validate_secret_value, value_shape_flags

    assert value_shape_flags(value) == expected
    if accepted:
        validate_secret_value(value)
    else:
        with pytest.raises(ValueError):
            validate_secret_value(value)


def test_a_document_that_ends_in_its_newline_round_trips_through_the_store(
    tmp_path: Path,
) -> None:
    """The end-to-end the ruling is about: a PEM-shaped value stores and reads
    back byte-identically, trailing newline included."""
    from k0.key_capture import validate_secret_value

    pem = b"-----BEGIN PRIVATE KEY-----\nc3ludGhldGljLW5vdC1hLWtleQ==\n-----END PRIVATE KEY-----\n"
    validate_secret_value(pem)
    store = FileStore(root=tmp_path / "store")
    store.put("ssh-id-synthetic", pem)
    assert store.get("ssh-id-synthetic") == pem


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        pytest.param(b"\xef\xbb\xbf sk", ("bom", "leading-whitespace"), id="bom-then-space"),
        pytest.param(b"\xef\xbb\xbfsk ", ("bom", "trailing-whitespace"), id="bom-then-trailing"),
        pytest.param(b"sk \n\n", ("trailing-blank-line", "trailing-whitespace"), id="space-then-blank-line"),
        pytest.param(b"sk \n", ("trailing-newline", "trailing-whitespace"), id="space-then-newline"),
    ],
)
def test_every_shape_a_value_carries_is_reported_in_one_pass(value: bytes, expected) -> None:
    """A refusal that reveals one problem at a time is a refusal the operator
    meets several times.

    Edge whitespace used to be checked against the raw first byte and against
    the value minus ONE trailing newline, so a BOM hid a leading space behind it
    and a blank line hid a trailing one. The operator would fix the reported
    shape and meet the next on the retry. Both checks now look past the BOM and
    past every trailing newline.
    """
    from k0.key_capture import value_shape_flags

    assert set(value_shape_flags(value)) == set(expected)
