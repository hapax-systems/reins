"""R2.3 — frontier key capture and secrets bootstrap.

The estate pattern that works (pass-backed, tmpfs-env, dependency-rooted) is hardcoded to one
operator's ~20 entries, UID, and identity strings — a stranger's kit cannot inherit it. The graph
gap is four claims, and this module is each of them as machinery:

  * PORTABLE, BACKEND-AGNOSTIC STORE. `SecretStore` is the contract; `FileStore`
    (durable, device-bound files) and `MemoryStore` (tests / stranger) are the
    backends. Nothing else in the kernel may know which is in use. `FileStore`
    (`backend_id=file`) is the only durable one. The `pass` backend was removed
    2026-09-16 on the operator's ruling that pass/gopass are never used to
    manage secrets going forward; `default_store()` is now unconditional, so no
    caller needs a "this is not pass" branch to be safe.
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
import hmac
import os
import re
import secrets
import stat
import tempfile
from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import StrEnum
from pathlib import Path
from types import MappingProxyType
from typing import Callable, Mapping, Protocol

from bootstrap_receipt import (
    BootstrapAct,
    BootstrapPhase,
    BootstrapReceipt,
    EvidenceStatus,
    append_receipt,
    load_chain,
)

from .boot_profile import PROFILES, ratified_profile
from . import provider_legality as _legality
from .egress_consent import EgressConsentError, require_egress
from .fatigue_budget import require_budget

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
    def delete(self, name: str) -> bool: ...


def _default_file_root() -> Path:
    env = os.environ.get("REINS_SECRET_STORE", "").strip()
    if env:
        return Path(env)
    return Path.home() / ".config" / "reins" / "secrets"


class SecretIntegrityError(Exception):
    """A stored blob did not authenticate: tampered, truncated, wrong key, or
    written under another name.

    DELIBERATELY NOT a ``ValueError`` subclass. The defect this type exists to
    close is that ``FileStore.get`` caught ``ValueError`` and returned ``None``,
    so a corrupted secret was indistinguishable from an absent one and every
    caller's "not found, run the put dialogue" arm fired on a tampered blob. A
    subclass would be swallowed by exactly those ``except ValueError`` arms
    again. Messages never carry the value or the key.
    """


#: Format 2 header: 3 ASCII bytes + a version byte, so a later format 3 is a
#: one-byte change rather than another undiscriminated blob shape.
_SECRET_MAGIC_V2 = b"RS2\x00"
_NONCE_LEN = 16
_TAG_LEN = 32
_V1_MIN_LEN = _NONCE_LEN + _TAG_LEN
_V2_MIN_LEN = len(_SECRET_MAGIC_V2) + _NONCE_LEN + _TAG_LEN


def _hkdf_sha256(*, ikm: bytes, salt: bytes, info: bytes, length: int) -> bytes:
    """RFC 5869 HKDF over HMAC-SHA256, stdlib only.

    WHY NOT ``cryptography``. ``scripts/hapax-secret`` execs the SYSTEM
    ``python3`` with PYTHONPATH pointed at this api directory — not the uv venv
    — because GET is the bootstrap read path that watchdogs and units take
    before any environment exists. Measured on the store host: ``cryptography``
    is not importable from that interpreter. A compiled dependency on the secret
    READ path means a broken wheel locks the operator out of the credentials
    needed to repair the wheel. Falsifier: point the launcher at an interpreter
    whose environment is guaranteed present before the first secret read, and an
    AEAD from ``cryptography`` becomes affordable and is the better primitive.
    """
    prk = hmac.new(salt, ikm, hashlib.sha256).digest()
    out = b""
    block = b""
    counter = 1
    while len(out) < length:
        block = hmac.new(prk, block + info + bytes([counter]), hashlib.sha256).digest()
        out += block
        counter += 1
    return out[:length]


def _v2_subkeys(key: bytes, nonce: bytes) -> tuple[bytes, bytes]:
    """Separate enc and mac subkeys. v1 used the SAME 32 bytes for the SHAKE
    keystream and the HMAC tag; distinct ``info`` labels under one HKDF is the
    single mechanism that separates them."""
    return (
        _hkdf_sha256(ikm=key, salt=nonce, info=b"reins-secret-v2-enc", length=32),
        _hkdf_sha256(ikm=key, salt=nonce, info=b"reins-secret-v2-mac", length=32),
    )


def _v2_aad(name: str) -> bytes:
    """Associated data: header + length-prefixed name. The length prefix makes
    the encoding unambiguous, and the name in the MAC is the single mechanism
    that stops a blob being renamed into another secret's slot."""
    raw = name.encode("utf-8")
    if len(raw) > 0xFFFF:
        raise ValueError("secret name is too long to bind into the blob")
    return _SECRET_MAGIC_V2 + len(raw).to_bytes(2, "big") + raw


def blob_format_of(blob: bytes) -> int:
    """2 when the header is present, else 1.

    KNOWN AND ACCEPTED LIMIT: a v1 blob opens with a 16-byte random nonce, so
    with probability 2**-32 its first four bytes are the v2 header and it is
    read as v2, fails to authenticate, and is reported as an integrity failure.
    That is a named refusal with the right next action (re-put the secret), not
    a silent wrong answer, and it is why there is no "try v1 after v2 fails"
    arm: such a fallback would make a genuine v2 tamper take a second, weaker
    path instead of refusing.
    """
    return 2 if blob.startswith(_SECRET_MAGIC_V2) else 1


def _file_wrap_v2(key: bytes, nonce: bytes, name: str, plaintext: bytes) -> bytes:
    """header || nonce || tag || ct — encrypt-then-MAC with separated subkeys
    and the name bound into the authenticated data."""
    enc_key, mac_key = _v2_subkeys(key, nonce)
    stream = hashlib.shake_256(enc_key).digest(len(plaintext))
    ct = bytes(a ^ b for a, b in zip(plaintext, stream, strict=True))
    tag = hmac.new(mac_key, _v2_aad(name) + nonce + ct, hashlib.sha256).digest()
    return _SECRET_MAGIC_V2 + nonce + tag + ct


def _file_unwrap_v2(key: bytes, blob: bytes, name: str) -> bytes:
    if len(blob) < _V2_MIN_LEN:
        raise SecretIntegrityError("secret blob is truncated below the v2 header")
    body = blob[len(_SECRET_MAGIC_V2) :]
    nonce, tag, ct = body[:_NONCE_LEN], body[_NONCE_LEN:_V1_MIN_LEN], body[_V1_MIN_LEN:]
    enc_key, mac_key = _v2_subkeys(key, nonce)
    expect = hmac.new(mac_key, _v2_aad(name) + nonce + ct, hashlib.sha256).digest()
    if not hmac.compare_digest(tag, expect):
        raise SecretIntegrityError("secret blob failed authentication")
    stream = hashlib.shake_256(enc_key).digest(len(ct))
    return bytes(a ^ b for a, b in zip(ct, stream, strict=True))


def _file_wrap(key: bytes, nonce: bytes, plaintext: bytes) -> bytes:
    """Format 1. RETAINED FOR READING ONLY — nothing writes it any more; the
    tests that pin v1 reads construct their fixtures with it."""
    stream = hashlib.shake_256(key + nonce).digest(len(plaintext))
    ct = bytes(a ^ b for a, b in zip(plaintext, stream, strict=True))
    tag = hmac.new(key, nonce + ct, hashlib.sha256).digest()
    return nonce + tag + ct


def _file_unwrap(key: bytes, blob: bytes) -> bytes:
    if len(blob) < _V1_MIN_LEN:
        raise SecretIntegrityError("secret blob is truncated below the v1 header")
    nonce, tag, ct = blob[:_NONCE_LEN], blob[_NONCE_LEN:_V1_MIN_LEN], blob[_V1_MIN_LEN:]
    expect = hmac.new(key, nonce + ct, hashlib.sha256).digest()
    if not hmac.compare_digest(tag, expect):
        raise SecretIntegrityError("secret blob failed authentication")
    stream = hashlib.shake_256(key + nonce).digest(len(ct))
    return bytes(a ^ b for a, b in zip(ct, stream, strict=True))


def _file_unwrap_any(key: bytes, blob: bytes, name: str) -> bytes:
    """Read either format. Writers only ever emit v2."""
    if blob_format_of(blob) == 2:
        return _file_unwrap_v2(key, blob, name)
    return _file_unwrap(key, blob)


#: Byte shapes a stored value can carry. ``--audit`` reports them; put refuses
#: the ones in REFUSABLE_VALUE_FLAGS. Flag names, never values.
_UTF8_BOM = b"\xef\xbb\xbf"

#: THE CLOSED VOCABULARY. value_shape_flags returns a subset of exactly these
#: literals and nothing derived from the value — that is what makes it safe to
#: print a flag list beside a secret's name. Pinned by
#: test_value_shape_flags_only_ever_returns_the_closed_vocabulary, which fuzzes
#: the function and asserts no returned string is absent from this set.
#:
#: Named for what it is — the byte shapes a value can carry — and not
#: "SECRET_*": CodeQL classifies identifiers as secret material by NAME, and
#: under its former name this tuple of flag literals was reported as a secret
#: flowing to stdout for four commits running (py/clear-text-logging-
#: sensitive-data; the SARIF named this tuple, the function below and
#: blob_format_of as its three sources). The property that matters is the
#: closed set, and the fuzz test holds it.
VALUE_SHAPE_FLAGS = (
    "empty",
    "bom",
    "nul",
    "cr",
    "trailing-newline",
    "trailing-blank-line",
    "multiline",
    "leading-whitespace",
    "trailing-whitespace",
    "non-utf8",
)

#: INFORMATIONAL, not defects — reported by --audit, never refused at put.
#:
#: `multiline` was refusable in the first cut of this change and the ruling that
#: corrected it was measured, not argued: 7 of the 184 live values carry
#: interior newlines and every one is a document rather than a credential
#: string — an ssh private key, a GitHub App private key, two Google
#: service-account JSONs, an rclone config, a BitLocker recovery blob, a GPG
#: passphrase file. Refusing interior LF would have failed all seven at their
#: next put. `non-utf8` is here for the same reason: a binary secret is
#: legitimate.
INFORMATIONAL_VALUE_FLAGS = frozenset({"multiline", "non-utf8"})

#: Everything else has been measured to break a consumer.
REFUSABLE_VALUE_FLAGS = frozenset(set(VALUE_SHAPE_FLAGS) - INFORMATIONAL_VALUE_FLAGS)


def value_shape_flags(value: bytes) -> tuple[str, ...]:
    """The byte shapes of one value, in a stable order. Never returns the value.

    The BOM flag exists because it happened: 17 of 112 FileStore values began
    ``ef bb bf`` (carried in from the pass store), and every consumer of those
    keys authenticated with three junk bytes on the front and got a 401 that
    named the provider, not the store.

    ``multiline`` exists because the opposite happened: refusing interior
    newlines would have refused seven live values that are documents rather than
    credential strings. A flag vocabulary that cannot separate "this byte broke
    a consumer" from "this value is a file" refuses the wrong half.

    THE TRAILING-NEWLINE POLICY, stated once, here:

      * a single-line value ends at its last non-whitespace byte;
      * a multi-line value may end in exactly one LF;
      * two trailing LFs are a defect either way.

    The refusal exists to catch the single-line key pasted with its Enter, not
    to reject files for being files.
    """
    flags: list[str] = []
    if not value:
        flags.append("empty")
    if value.startswith(_UTF8_BOM):
        flags.append("bom")
    if b"\x00" in value:
        flags.append("nul")
    if b"\r" in value:
        flags.append("cr")
    # Multi-line means CONTENT on more than one line, so strip every trailing
    # newline before looking. Stripping only one made "sk-key\n\n" — a
    # single-line value with a blank line pasted onto it — read as a document
    # and slip past the refusal it exists for.
    body = value.rstrip(b"\n")
    multiline = b"\n" in body
    if multiline:
        # INFORMATION. An interior newline means the value is a document, not a
        # credential string, and documents are legitimate here.
        flags.append("multiline")
    trailing = len(value) - len(body)
    if trailing >= 2:
        # Refusable whatever the value is. One newline ends a file; two mean a
        # blank line nobody intended, and for a document that is as much a
        # transcription artefact as a BOM.
        flags.append("trailing-blank-line")
    elif trailing == 1 and not multiline:
        # Refusable. A bearer token sent as "sk-abc\n" gets a 401 that names the
        # provider rather than the store — the same failure the BOM produced.
        # A DOCUMENT ending in one newline is not this: a PEM key, a
        # service-account JSON and an rclone config are files, and a file ends
        # in a newline. Refusing those refuses the values that matter most.
        flags.append("trailing-newline")
    # Look for edge whitespace PAST a BOM and PAST the trailing newlines, so one
    # put reports every shape it carries. Checking value[:1] meant a value of
    # BOM-then-space reported only `bom`, the operator fixed that, and the space
    # came back as a second refusal on the retry. A refusal that reveals one
    # problem at a time is a refusal the operator meets several times.
    unbommed = body[len(_UTF8_BOM) :] if body.startswith(_UTF8_BOM) else body
    if unbommed[:1] in (b" ", b"\t"):
        flags.append("leading-whitespace")
    if unbommed[-1:] in (b" ", b"\t"):
        flags.append("trailing-whitespace")
    try:
        value.decode("utf-8")
    except UnicodeDecodeError:
        flags.append("non-utf8")
    return tuple(flags)


def validate_secret_value(value: bytes) -> None:
    """Refuse a value whose bytes are known to break consumers. ONE definition:
    the command surface calls it before any write, and the TTY dialogue calls
    the same function so the operator sees the same sentence without a round
    trip. No silent stripping — a store that quietly edits the operator's bytes
    cannot be reasoned about, and the BOM incident was undetectable precisely
    because nothing ever said anything.
    """
    bad = [f for f in value_shape_flags(value) if f in REFUSABLE_VALUE_FLAGS]
    if bad:
        raise ValueError(
            "secret value refused ("
            + ", ".join(bad)
            + "). Next action: re-enter the value with no byte-order mark, no "
            "carriage return, and no leading or trailing space or tab. A "
            "single-line value must end at its last non-whitespace byte; a "
            "multi-line value may end in exactly one newline, never two. Run "
            "hapax-secret --audit to see which stored names carry the same shapes"
        )


def _default_key_file() -> Path | None:
    """An out-of-tree key location, or None for the in-root default.

    The key sits beside the blobs it protects, inside a home directory that
    restic and the vault snapshotter both walk. Co-located key and ciphertext
    means one backup set carries both halves. REINS_SECRET_KEY_FILE (and
    ``hapax-secret --key-file``) move the key off that path; the exclusion
    measurement and the boundary are written up in docs/secret-store-key.md.
    """
    env = os.environ.get("REINS_SECRET_KEY_FILE", "").strip()
    return Path(env) if env else None


#: How many superseded blobs a name keeps. Bounded because history is
#: ciphertext of real credentials: unbounded history is an unbounded liability.
_HISTORY_KEEP = 5
_HISTORY_DIR = ".history"
#: The stamp _archive and delete write — %Y%m%dT%H%M%S%fZ — plus the "-N" that
#: _archive appends when two supersessions of one name land in one microsecond.
_HISTORY_STAMP_RE = re.compile(r"\d{8}T\d{12}Z(?:-\d+)?")


def _history_entries(hdir: Path, name: str, suffix: str) -> list[tuple[Path, str]]:
    """THIS name's history files, with their stamps, in stamp order.

    A name may contain dots, so ``api`` and ``api.openai`` are both legal and
    the glob ``api.*.bin`` matches ``api.openai.<stamp>.bin`` as well as
    ``api.<stamp>.bin``. Found in review of PR 44: without the stamp check,
    history("api") reported api.openai's stamps, put("api") counted
    api.openai's archives toward api's cap and pruned them, and delete("api")
    destroyed api.openai's archived ciphertext. Only a segment that IS a stamp
    belongs to this name.
    """
    entries = []
    for path in hdir.glob(f"{name}.*{suffix}"):
        stamp = path.name[len(name) + 1 : -len(suffix)]
        if _HISTORY_STAMP_RE.fullmatch(stamp):
            entries.append((path, stamp))
    return sorted(entries, key=lambda entry: entry[1])


@dataclass
class FileStore:
    """Durable device-bound store. backend_id is `file`.

    Layout: ``root/.key`` (32 random bytes, 0600, or ``key_file`` elsewhere),
    ``root/<safe-name>.bin`` (format 2 blobs), and ``root/.history/`` holding
    superseded blobs and delete tombstones. Override root with
    REINS_SECRET_STORE. Names are path-safe; values never appear on argv, in
    env, or in raised messages.
    """

    root: Path = field(default_factory=_default_file_root)
    backend_id: str = "file"
    key_file: Path | None = field(default_factory=_default_key_file)

    @property
    def key_path(self) -> Path:
        return self.key_file if self.key_file is not None else self.root / ".key"

    def _key(self) -> bytes:
        self.root.mkdir(mode=0o700, parents=True, exist_ok=True)
        path = self.key_path
        path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        flags = os.O_CREAT | os.O_EXCL | os.O_WRONLY
        try:
            fd = os.open(path, flags, 0o600)
            try:
                os.write(fd, os.urandom(32))
            finally:
                os.close(fd)
        except FileExistsError:
            pass
        return path.read_bytes()

    def _blob_path(self, name: str) -> Path:
        if not name or name in (".", "..") or "/" in name or "\\" in name:
            raise ValueError("secret name must be a single path segment")
        if re.fullmatch(r"[A-Za-z0-9._-]+", name) is None:
            raise ValueError(
                "secret name must match [A-Za-z0-9._-]+ (no normalization, no collisions)"
            )
        return self.root / f"{name}.bin"

    def _history_dir(self) -> Path:
        return self.root / _HISTORY_DIR

    def has(self, name: str) -> bool:
        return self._blob_path(name).is_file()

    def get(self, name: str) -> bytes | None:
        """None means ABSENT and nothing else.

        A blob that is present but does not authenticate raises
        SecretIntegrityError. It used to return None here, which told every
        caller "not found" about a file it had just read.
        """
        path = self._blob_path(name)
        if not path.is_file():
            return None
        return _file_unwrap_any(self._key(), path.read_bytes(), name)

    def blob_format(self, name: str) -> int | None:
        """1 or 2 for a present blob, None when absent. Reads the header only."""
        path = self._blob_path(name)
        if not path.is_file():
            return None
        with path.open("rb") as fh:
            return blob_format_of(fh.read(len(_SECRET_MAGIC_V2)))

    def _write_blob(self, path: Path, blob: bytes) -> None:
        fd, tmp = tempfile.mkstemp(prefix=f".{path.stem}.", suffix=".tmp", dir=path.parent)
        try:
            os.write(fd, blob)
            os.fchmod(fd, 0o600)
            os.close(fd)
            fd = -1
            os.replace(tmp, path)
        except Exception:
            if fd >= 0:
                os.close(fd)
            try:
                os.unlink(tmp)
            except FileNotFoundError:
                pass
            raise

    def _archive(self, name: str, blob: bytes) -> None:
        """Keep the superseded ciphertext under a fresh timestamp, then prune."""
        hdir = self._history_dir()
        hdir.mkdir(mode=0o700, parents=True, exist_ok=True)
        stamp = datetime.now(UTC).strftime("%Y%m%dT%H%M%S%fZ")
        for attempt in range(1000):
            suffix = "" if attempt == 0 else f"-{attempt}"
            path = hdir / f"{name}.{stamp}{suffix}.bin"
            try:
                fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
            except FileExistsError:
                continue
            try:
                os.write(fd, blob)
            finally:
                os.close(fd)
            break
        else:  # pragma: no cover - 1000 puts inside one microsecond
            raise RuntimeError("could not allocate a history slot for the superseded blob")
        entries = _history_entries(hdir, name, ".bin")
        for stale, _stamp in entries[:-_HISTORY_KEEP]:
            stale.unlink(missing_ok=True)

    def put(self, name: str, value: bytes) -> None:
        """Always writes format 2. A v1 blob is therefore migrated by its first
        put and by nothing else — get stays a reader, because GET runs from
        watchdogs and units and a getter that writes is a getter that can fail
        on a read-only mount.
        """
        path = self._blob_path(name)
        blob = _file_wrap_v2(self._key(), os.urandom(_NONCE_LEN), name, value)
        if path.is_file():
            self._archive(name, path.read_bytes())
        self._write_blob(path, blob)

    def history(self, name: str) -> tuple[str, ...]:
        """Timestamps of superseded versions and of the delete tombstone, in
        order. Timestamps only — never a value, never a digest of one."""
        self._blob_path(name)  # validate the name before it reaches a glob
        hdir = self._history_dir()
        if not hdir.is_dir():
            return ()
        stamps = [stamp for _path, stamp in _history_entries(hdir, name, ".bin")]
        stamps += [
            stamp + " (deleted)" for _path, stamp in _history_entries(hdir, name, ".tombstone")
        ]
        return tuple(sorted(stamps))

    def delete(self, name: str) -> bool:
        """Delete means the material is gone: the live blob AND this name's
        superseded blobs. A tombstone records that it happened.

        Keeping history through a delete would defeat the one operation whose
        whole purpose is that a revoked credential stops existing — the estate
        deletes names precisely because the value must not survive.
        """
        path = self._blob_path(name)
        try:
            path.unlink()
        except FileNotFoundError:
            return False
        hdir = self._history_dir()
        if hdir.is_dir():
            for stale, _stamp in _history_entries(hdir, name, ".bin"):
                stale.unlink(missing_ok=True)
        hdir.mkdir(mode=0o700, parents=True, exist_ok=True)
        stamp = datetime.now(UTC).strftime("%Y%m%dT%H%M%S%fZ")
        tombstone = hdir / f"{name}.{stamp}.tombstone"
        try:
            os.close(os.open(tombstone, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600))
        except FileExistsError:
            pass
        return True

    def names(self) -> tuple[str, ...]:
        """Stored names, sorted. Reads no values."""
        if not self.root.is_dir():
            return ()
        return tuple(sorted(p.name[: -len(".bin")] for p in self.root.glob("*.bin")))


def default_store() -> SecretStore:
    """Unconditional since the pass backend was removed: always the FileStore."""
    return FileStore()


#: The header the secret command surface authenticates. A header rather than a
#: body field so the credential never lands in a request body that a proxy, a
#: log line, or a witnessed envelope might copy.
SECRET_COMMAND_TOKEN_HEADER = "x-reins-secret-token"
_TOKEN_REL_PATH = ("reins", "secret-command.token")


class SecretCommandTokenError(Exception):
    """The capability token could not be located or read. Carries a next action;
    never carries the token."""


def _runtime_dir() -> Path:
    """$XDG_RUNTIME_DIR, or the systemd default computed from our own uid.

    Computing /run/user/<uid> is not a widening fallback: it is the SAME
    location the variable normally holds, and the safety precondition is
    checked here, at the moment of use — the directory must exist, be a real
    directory, be owned by this uid, and not be group- or world-accessible. If
    that cannot be established we refuse. /tmp is never used: a world-writable
    directory would let any local account plant the token file first.
    """
    env = os.environ.get("XDG_RUNTIME_DIR", "").strip()
    candidate = Path(env) if env else Path(f"/run/user/{os.getuid()}")
    try:
        st = os.stat(candidate)
    except OSError as exc:
        raise SecretCommandTokenError(
            f"runtime directory {candidate} is unusable ({type(exc).__name__}). Next action: "
            "run the secret command surface in a session with XDG_RUNTIME_DIR set to a "
            "0700 directory you own"
        ) from None
    if not stat.S_ISDIR(st.st_mode):
        raise SecretCommandTokenError(
            f"runtime directory {candidate} is not a directory. Next action: point "
            "XDG_RUNTIME_DIR at a 0700 directory you own"
        )
    if st.st_uid != os.getuid():
        raise SecretCommandTokenError(
            f"runtime directory {candidate} is not owned by this uid. Next action: point "
            "XDG_RUNTIME_DIR at a 0700 directory you own"
        )
    if st.st_mode & 0o077:
        raise SecretCommandTokenError(
            f"runtime directory {candidate} is group- or world-accessible. Next action: "
            "chmod 700 it, or point XDG_RUNTIME_DIR at a 0700 directory you own"
        )
    return candidate


def secret_command_token_path() -> Path:
    return _runtime_dir().joinpath(*_TOKEN_REL_PATH)


def mint_secret_command_token() -> str:
    """Read the per-boot capability token, creating it if this is the first call.

    WHAT THIS DOES AND DOES NOT BUY. It excludes a process running as ANOTHER
    uid, which loopback HTTP did not: :8799 is reachable by every account on the
    host, and before this a container or a service user could put or delete any
    secret. It does NOT exclude a process running as the operator — such a
    process can read the token file, exactly as it could read the store's .key.
    An SO_PEERCRED check on a Unix socket would buy the same set and no more,
    for a larger change; that is why this is a token and not a socket. Same-uid
    isolation needs a different trust domain and is not claimed here.

    Per-boot by construction: XDG_RUNTIME_DIR is tmpfs cleared on logout.
    """
    path = secret_command_token_path()
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    try:
        fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    except FileExistsError:
        pass
    else:
        try:
            os.write(fd, secrets.token_hex(32).encode("ascii"))
        finally:
            os.close(fd)
    try:
        st = os.stat(path)
        if st.st_uid != os.getuid() or st.st_mode & 0o077:
            raise SecretCommandTokenError(
                f"secret command token {path} is not 0600 and owned by this uid. Next action: "
                "delete it and let the surface mint a fresh one"
            )
        token = path.read_text(encoding="ascii").strip()
    except OSError as exc:
        raise SecretCommandTokenError(
            f"secret command token at {path} is unreadable ({type(exc).__name__}). Next action: "
            "delete it and rerun so the surface mints a fresh one"
        ) from None
    if not token:
        raise SecretCommandTokenError(
            f"secret command token at {path} is empty. Next action: delete it and rerun so "
            "the surface mints a fresh one"
        )
    return token


def secret_command_token_matches(presented: str | None) -> bool:
    """Constant-time comparison against the on-disk token. A missing or
    unreadable token denies — it never passes for want of something to check."""
    if not presented:
        return False
    try:
        expected = mint_secret_command_token()
    except SecretCommandTokenError:
        return False
    return hmac.compare_digest(presented, expected)


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

    def delete(self, name: str) -> bool:
        return self._values.pop(name, None) is not None


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
#: keys; the per-call negative control re-proves that property on every run. IMMUTABLE
#: (MappingProxyType, codex r20): a mutable registry would let a caller register its own
#: endpoint and pass the membership guard with a destination nobody sanctioned.
PROVIDER_PROBE_ENDPOINTS: Mapping[str, ProbeEndpoint] = MappingProxyType({
    "anthropic": ProbeEndpoint(
        "api.anthropic.com",
        "/v1/models",
        "x-api-key",
        "sk-ant-",
        (("anthropic-version", "2023-06-01"),),
    ),
    "openai": ProbeEndpoint("api.openai.com", "/v1/models", "bearer", "sk-"),
})

#: Captured AT IMPORT (codex r21): the guard reads this, never the public name — rebinding
#: `PROVIDER_PROBE_ENDPOINTS` on the module would redirect readers but cannot redirect the
#: wire. THE THREAT BOUNDARY, STATED: a hostile IN-PROCESS caller can monkeypatch anything,
#: including require_egress itself; no Python structure prevents that, and none is claimed.
#: These controls make the legal path the only natural one and every divergence deliberate.
_SANCTIONED_ENDPOINTS: tuple[ProbeEndpoint, ...] = tuple(PROVIDER_PROBE_ENDPOINTS.values())


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


def _https_probe_transport(
    root: Path,
    endpoint: ProbeEndpoint,
    value: bytes,
    *,
    allowed_signers: Path,
    principal: str,
    scratch_dir: Path,
) -> ProbeOutcome:
    # The wire dials ONLY registry endpoints (codex r16): a transport taking free host/path/
    # header arguments is a dial-anything primitive wearing a consent check, and caller-shaped
    # headers are caller-controlled content on the wire. Registry membership is verified here,
    # so the consented destination is the only reachable one.
    if all(endpoint is not registered for registered in _SANCTIONED_ENDPOINTS):
        # IDENTITY, not equality (claude r20): with the registry immutable, the endpoint must BE
        # the registry's object — a caller rebuilding an equal one is shaping their own
        # destination, and that is not consent.
        # SEAT SPLIT, recorded: glm r23 holds that `is` is the wrong comparison for dataclass
        # instances. Identity is the point: equality proves only that the caller can construct
        # a lookalike, and the guard exists to make that insufficient.
        raise ValueError(
            "the kernel's wire dials the registry's own endpoint objects only — an endpoint "
            "that is not the registry's is not a legal destination"
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
    require_egress(
        root, endpoint.host, allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
    )
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


def _append_row_phased(
    root: Path,
    *,
    act: BootstrapAct,
    phase: BootstrapPhase,
    estate_id: str,
    kernel_version: str,
    payload_refs: list[str],
    receipt_id: str,
    observed_at: datetime | None,
) -> Path:
    chain = load_chain(root)
    if not chain:
        raise ValueError("no genesis self-attest: there is no ceremony to record within")
    receipt = BootstrapReceipt(
        receipt_id=receipt_id,
        estate_id=estate_id,
        kernel_version=kernel_version,
        phase=phase,
        act=act,
        payload_refs=payload_refs,
        evidence_status=EvidenceStatus.OBSERVED,
        prev_receipt_hash=chain[-1].receipt_hash(),
        observed_at=observed_at or datetime.now(UTC),
    )
    return append_receipt(root, receipt)


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
    """Record the ask. An elicitation is not supply and changes no supply state — and it is
    never free: the ratified fatigue budget guards this path (R2.11), so an exhausted budget
    refuses the ask before any row is written."""
    require_budget(root)
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
    allowed_signers: Path,
    principal: str,
    scratch_dir: Path,
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
    if not _legality.key_capture_legal(provider):
        raise ValueError(
            f"{provider!r}: key capture is not a legal acquisition path for this provider "
            "(R2.16) — its legal modes are "
            f"{sorted(m.value for m in _legality.legal_acquisition_modes(provider))}; use one of those "
            "instead of capturing a key"
        )
    if supply_state(root, store, name) is SecretSupply.CREDENTIAL_GATED:
        raise ValueError(
            f"{name}: declined by the operator — validating a refused secret would be nagging "
            "by another door"
        )
    try:
        require_egress(
            root, endpoint.host, allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
        )
    except EgressConsentError as refusal:
        # A refused attempt is durable (claude r7) — but only a CLEAN denial is recorded, and
        # never into a suspect chain (codex r23): an integrity or signature refusal means the
        # ledger itself is in question, and writing to it would mutate state we just declared
        # untrustworthy. The row rides at the chain's CURRENT tail phase: planting it at
        # AUTH_MATERIALIZE would make any later STIPULATION_RATIFY consent a phase regression
        # and brick the documented recovery path.
        if getattr(refusal.refusal, "gate", None) == "egress.default-deny":
            chain = load_chain(root)
            _append_row_phased(
                root,
                act=BootstrapAct.REFUSED,
                phase=chain[-1].phase,
                estate_id=estate_id,
                kernel_version=kernel_version,
                payload_refs=[
                    f"egress-host:{endpoint.host}",
                    "egress-attempt:validation-refused-by-consent-gate",
                ],
                receipt_id=f"egress-refused-{name}-{len(chain)}",
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
    control = _https_probe_transport(
        root, endpoint, _negative_control_key(endpoint), allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
    )
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
    outcome = _https_probe_transport(
        root, endpoint, value, allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
    )
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

