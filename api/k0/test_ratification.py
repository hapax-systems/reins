"""R2.8 — performing a ratification, and proving it later.

Every test here corresponds to a way the act can be faked, lost, or replayed. Two of them exist
because the first implementation had exactly that bug and it was caught before the suite ran:

  * `verify_ratifications` reconstructed the signed payload from the receipt, but a receipt carries
    refs and not values, so the subject was unrecoverable and verification could never pass.
  * the artifact digest and the signed-bytes digest were conflated; comparing one to the other
    fails always, and "always fails" and "detects tampering" are indistinguishable from the outside.
"""

from __future__ import annotations

import hashlib
import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from bootstrap_receipt import (
    BootstrapAct,
    append_receipt,
    genesis_self_attest,
    load_chain,
    verify_chain_at,
)
from k0.ratification import (
    RatificationError,
    Stipulation,
    pending,
    propose,
    ratify,
    verify_ratifications,
)
from k0.refusal import RefusalError
from k0.ratifier import (
    RatifierError,
    sign_ratification,
    verify_ratification,
    write_allowed_signers,
)
from k0.recovery import SignerEntry, write_signers

ESTATE = "estate-0000000000000000"
KERNEL = "k0-test"


def _keypair(tmp_path: Path) -> tuple[Path, Path, str]:
    """A real ed25519 keypair. The ceremony is not mocked: a fake signer would let every test pass
    against a module that never obtains consent, which is the defect this whole member exists to
    remove."""
    key = tmp_path / "ratifier_ed25519"
    subprocess.run(
        ["ssh-keygen", "-t", "ed25519", "-N", "", "-C", "ratifier@test", "-f", str(key)],
        check=True,
        capture_output=True,
    )
    principal = "ratifier@test"
    allowed = tmp_path / "allowed_signers"
    write_allowed_signers(allowed, principal, (key.with_suffix(".pub")).read_text().strip())
    return key, allowed, principal


def _root(tmp_path: Path) -> Path:
    """A durable root whose chain is opened by the kernel's genesis self-attest.

    `genesis_self_attest` BUILDS the receipt; `append_receipt` writes it. Keeping those separate is
    deliberate in the primitive — constructing the claim and committing it are different acts — so
    the fixture does both explicitly rather than hiding the seam.
    """
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


def _stip(body: bytes = b"axiom: single_user\n") -> Stipulation:
    return Stipulation.over("axioms.v1", "the estate's axiom set, v1", body)


def test_ratifying_lands_a_verifiable_act(tmp_path: Path) -> None:
    """The whole point: perform the act, then have a stranger prove it happened."""
    root = _root(tmp_path)
    key, allowed, principal = _keypair(tmp_path)
    s = _stip()

    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)
    assert pending(root) == ("axioms.v1",), "a proposed stipulation must show as pending"

    ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert pending(root) == (), "a ratified stipulation is no longer pending"

    verdict = verify_ratifications(
        root, allowed_signers=allowed, principal=principal, scratch_dir=tmp_path
    )
    assert verdict.ok, f"a ratification we just performed did not verify: {verdict.unverified}"
    assert verdict.verified == ("axioms.v1",)

    acts = [r.act for r in load_chain(root)]
    assert BootstrapAct.RATIFIED in acts, "the act must be witnessed in the chain"
    assert verify_chain_at(root).ok, "ratifying must leave the receipt chain valid"


def test_a_ratification_cannot_be_invented_without_a_proposal(tmp_path: Path) -> None:
    """Consent to a question never posed is unauditable, so it is refused."""
    root = _root(tmp_path)
    key, _, _ = _keypair(tmp_path)
    with pytest.raises(RatificationError) as exc:
        ratify(root, _stip(), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert exc.value.refusal is not None, "a governance refusal must be data, not a bare message"
    assert exc.value.refusal.legal_next, "INV-3: every refusal leaves a legal next move"


def test_consent_is_given_once(tmp_path: Path) -> None:
    """Two consents for one act make the ledger ambiguous about which one binds."""
    root = _root(tmp_path)
    key, _, _ = _keypair(tmp_path)
    s = _stip()
    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(RatificationError):
        ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(RatificationError):
        propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)


def test_tampering_with_the_ratified_bytes_is_detected(tmp_path: Path) -> None:
    """The chain pins the signed bytes. Swapping them must not survive verification."""
    root = _root(tmp_path)
    key, allowed, principal = _keypair(tmp_path)
    s = _stip()
    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    payload = root / "ratifications" / "axioms.v1.payload"
    payload.write_bytes(b"axioms.v1\nsomething the operator never saw\n" + b"0" * 64 + b"\n")

    verdict = verify_ratifications(
        root, allowed_signers=allowed, principal=principal, scratch_dir=tmp_path
    )
    assert not verdict.ok, "altered ratified bytes verified clean — a false green on consent"
    assert "axioms.v1" in dict(verdict.unverified)


def test_a_missing_signature_is_not_a_pass(tmp_path: Path) -> None:
    """A chain claiming consent whose signature is gone must report unverified, never verified."""
    root = _root(tmp_path)
    key, allowed, principal = _keypair(tmp_path)
    s = _stip()
    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    (root / "ratifications" / "axioms.v1.sig").unlink()

    verdict = verify_ratifications(
        root, allowed_signers=allowed, principal=principal, scratch_dir=tmp_path
    )
    assert not verdict.ok, "a missing signature must never read as a verified ratification"


def test_history_verifies_at_the_moment_consent_was_given(tmp_path: Path) -> None:
    """THE TRAP `ratifier.verify_ratification` WARNS ABOUT.

    ssh-keygen validates against the current time by default. A ratification recorded in the past
    must still verify — verifying history at "now" is the wrong question, and getting it wrong is
    indistinguishable from detecting tampering.

    The receipt is written with an observed_at well in the past; verification must use THAT.
    """
    root = _root(tmp_path)
    key, allowed, principal = _keypair(tmp_path)
    s = _stip()
    past = datetime.now(UTC) - timedelta(days=120)
    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL, observed_at=past)
    ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL, observed_at=past)

    verdict = verify_ratifications(
        root, allowed_signers=allowed, principal=principal, scratch_dir=tmp_path
    )
    assert verdict.ok, (
        "a ratification made in the past failed to verify. If this fails because the signature was "
        f"checked at 'now' rather than at observed_at, history is unprovable: {verdict.unverified}"
    )


def test_pending_is_derived_and_survives_a_copied_chain(tmp_path: Path) -> None:
    """ANTI-WIZARDRY. Progress is a fold over the ledger, so it holds no state to lose.

    The chain is copied to a fresh root with no other context. If progress were a wizard cursor,
    the copy would not know where it was.
    """
    root = _root(tmp_path)
    for i in (1, 2, 3):
        propose(
            root,
            Stipulation.over(f"stip.{i}", f"subject {i}", f"body {i}".encode()),
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )
    assert pending(root) == ("stip.1", "stip.2", "stip.3")

    elsewhere = tmp_path / "copied"
    elsewhere.mkdir()
    (elsewhere / "bootstrap-receipts.jsonl").write_bytes(
        (root / "bootstrap-receipts.jsonl").read_bytes()
    )
    assert pending(elsewhere) == ("stip.1", "stip.2", "stip.3"), (
        "ceremony progress did not survive being carried to another root — that is a wizard cursor, "
        "which R2.8 forbids"
    )


def test_an_unreadable_chain_refuses_rather_than_reading_as_empty(tmp_path: Path) -> None:
    """FAIL-CLOSED. A chain we cannot parse is not a chain with nothing in it.

    Treating it as empty would let a ratified stipulation reappear as pending and be consented to
    twice — absence-into-zero, in the ledger built to prevent it.
    """
    root = _root(tmp_path)
    (root / "bootstrap-receipts.jsonl").write_text("{not json\n", encoding="utf-8")
    with pytest.raises(RatificationError):
        pending(root)


def test_the_signed_payload_binds_the_identifier(tmp_path: Path) -> None:
    """A signature over a bare digest is replayable onto any artifact with that digest.

    The payload carries id and subject as well, so a signature for one stipulation cannot be moved
    onto another.
    """
    a = Stipulation.over("stip.a", "subject a", b"same bytes")
    b = Stipulation.over("stip.b", "subject b", b"same bytes")
    assert a.digest == b.digest, "fixture precondition: same artifact"
    assert a.payload() != b.payload(), (
        "two different stipulations over the same artifact produced identical signed bytes — a "
        "signature for one would verify for the other"
    )


def test_receipts_never_carry_the_signature_itself(tmp_path: Path) -> None:
    """Receipts carry references, never values. The signature lives beside the chain."""
    root = _root(tmp_path)
    key, _, _ = _keypair(tmp_path)
    s = _stip()
    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    signature = (root / "ratifications" / "axioms.v1.sig").read_text(encoding="utf-8")
    raw = (root / "bootstrap-receipts.jsonl").read_text(encoding="utf-8")
    body = signature.replace("-----BEGIN SSH SIGNATURE-----", "").strip().splitlines()
    assert body, "fixture precondition: the signature has content"
    assert body[0] not in raw, "the signature leaked into the receipt chain; receipts carry refs"


def test_a_ratification_never_grants_authority(tmp_path: Path) -> None:
    """never-mint: the row witnesses consent; it does not confer it."""
    root = _root(tmp_path)
    key, _, _ = _keypair(tmp_path)
    s = _stip()
    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    for receipt in load_chain(root):
        assert receipt.may_authorize is False


def test_signing_failure_leaves_the_stipulation_pending(tmp_path: Path) -> None:
    """If consent was not obtained, the ledger must not claim it was.

    The signature is taken BEFORE the row is appended precisely so a failure here writes nothing.
    """
    root = _root(tmp_path)
    s = _stip()
    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(RatificationError):
        ratify(
            root, s, key_path=tmp_path / "no-such-key", estate_id=ESTATE, kernel_version=KERNEL
        )
    assert pending(root) == ("axioms.v1",), (
        "signing failed but the stipulation stopped being pending — the ledger now implies a "
        "consent that was never given"
    )
    assert not any(r.act is BootstrapAct.RATIFIED for r in load_chain(root))


def test_stipulation_digest_is_over_the_artifact_not_the_subject(tmp_path: Path) -> None:
    """Ratifying a description rather than the artifact is how a ceremony attests to nothing."""
    body = b"the actual bytes"
    s = Stipulation.over("stip.x", "a human-facing description", body)
    assert s.digest == hashlib.sha256(body).hexdigest()


# --- The two cases the first pass of these tests failed to cover ---------------------------------
# Both were found by mutation testing, not by reading: mutants that verified at "now" instead of
# observed_at, and that dropped the signed-bytes pin entirely, BOTH SURVIVED the suite above.
# A test that cannot fail when the thing it names is removed is not testing that thing.


def test_a_rotated_key_still_proves_the_ratifications_it_made(tmp_path: Path) -> None:
    """The historical-verification trap, with a key whose window is actually closed.

    The earlier version of this test used a past `observed_at` but an allowed-signers file with NO
    validity window — so ssh-keygen accepted any instant and verifying at "now" passed too. It
    could not distinguish correct behaviour from the bug.

    Here the key is genuinely rotated: its window closes a day ago. Verification at "now" MUST fail
    and verification at the receipt's own `observed_at` MUST succeed. That asymmetry is the point —
    a ratification is permanent even though the key that made it is retired.
    """
    root = _root(tmp_path)
    key, allowed, principal = _keypair(tmp_path)
    s = _stip()
    past = datetime.now(UTC) - timedelta(days=120)
    propose(root, s, estate_id=ESTATE, kernel_version=KERNEL, observed_at=past)
    ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL, observed_at=past)

    # Close the key's window: valid until yesterday, i.e. it can no longer bind anything new.
    write_signers(
        allowed,
        [
            SignerEntry(
                principal=principal,
                public_key=key.with_suffix(".pub").read_text().strip(),
                valid_after=past - timedelta(days=1),
                valid_before=datetime.now(UTC) - timedelta(days=1),
            )
        ],
    )

    verdict = verify_ratifications(
        root, allowed_signers=allowed, principal=principal, scratch_dir=tmp_path
    )
    assert verdict.ok, (
        "a ratification made before the key was retired no longer verifies. History must remain "
        f"provable after rotation, or every past consent evaporates on key change: {verdict.unverified}"
    )

    # And the control: at "now" the retired key must NOT verify. If this passes, the test above is
    # not evidence of anything.
    sig = (root / "ratifications" / "axioms.v1.sig").read_text(encoding="utf-8")
    # k0 refuses as DATA: verify_ratification routes through fail_closed.decide, which
    # raises RefusalError carrying a Refusal — not a bare RuntimeError.
    with pytest.raises(RefusalError):
        verify_ratification(
            s.payload(),
            sig,
            allowed_signers=allowed,
            principal=principal,
            scratch_dir=tmp_path,
            verify_time=None,
        )


def test_a_valid_signature_for_a_different_stipulation_is_refused(tmp_path: Path) -> None:
    """What the signed-bytes pin is actually for.

    Corrupting the payload is caught by the signature alone, so the earlier tamper test did not
    exercise the pin — the mutant that deleted it survived. The real attack is a SWAP: move a
    genuine payload+signature pair from one ratified stipulation onto another. Both files are
    internally consistent and the signature verifies; only the digest the CHAIN pins reveals that
    the row now points at bytes it never consented to.
    """
    root = _root(tmp_path)
    key, allowed, principal = _keypair(tmp_path)
    a = Stipulation.over("stip.alpha", "alpha subject", b"alpha body")
    b = Stipulation.over("stip.beta", "beta subject", b"beta body")
    for s in (a, b):
        propose(root, s, estate_id=ESTATE, kernel_version=KERNEL)
        ratify(root, s, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    d = root / "ratifications"
    # Move beta's genuine, self-consistent pair onto alpha's slot.
    (d / "stip.alpha.payload").write_bytes((d / "stip.beta.payload").read_bytes())
    (d / "stip.alpha.sig").write_text((d / "stip.beta.sig").read_text(), encoding="utf-8")

    verdict = verify_ratifications(
        root, allowed_signers=allowed, principal=principal, scratch_dir=tmp_path
    )
    failed = dict(verdict.unverified)
    assert "stip.alpha" in failed, (
        "a genuine signature over a DIFFERENT stipulation was accepted for this row. The signature "
        "verifies — only the chain's digest pin can catch this, and it did not."
    )
    assert verdict.verified == ("stip.beta",), "beta's own ratification must still verify"


def test_resigning_a_different_subject_over_the_same_artifact_is_refused(tmp_path: Path) -> None:
    """The case ONLY the signed-bytes pin can catch — established by mutation testing.

    Deleting the byte pin and deleting the artifact-digest binding BOTH survived the suite, because
    for a swapped pair each check catches what the other misses. Redundant checks that are never
    independently exercised are coverage theater: the suite cannot tell you which one is load-bearing.

    This is the case where only the pin applies. Consent was given to subject A over artifact X.
    Someone then re-signs subject B over the SAME artifact X with a still-valid key, and installs
    that pair. The signature verifies (genuine). The artifact-digest line still matches (X is
    unchanged). Only the exact bytes the chain pinned reveal that what the operator consented to is
    not what is now on disk.

    This is not hypothetical prose: the subject is the human-facing statement of what is being
    agreed to. Changing it while keeping the artifact is precisely how a ceremony comes to attest to
    a sentence nobody read.
    """
    root = _root(tmp_path)
    key, allowed, principal = _keypair(tmp_path)
    artifact = b"the artifact bytes, unchanged throughout"
    consented = Stipulation.over("stip.subject", "PERMIT: read-only telemetry", artifact)
    propose(root, consented, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, consented, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    # Same id, same artifact digest — different subject. Signed genuinely with the live key.
    substituted = Stipulation.over("stip.subject", "PERMIT: full estate mutation", artifact)
    assert substituted.digest == consented.digest, "fixture: the artifact must be unchanged"
    assert substituted.payload() != consented.payload(), "fixture: the signed bytes must differ"

    d = root / "ratifications"
    (d / "stip.subject.payload").write_bytes(substituted.payload())
    (d / "stip.subject.sig").write_text(
        sign_ratification(substituted.payload(), key), encoding="utf-8"
    )

    verdict = verify_ratifications(
        root, allowed_signers=allowed, principal=principal, scratch_dir=tmp_path
    )
    assert not verdict.ok, (
        "a genuinely-signed substitution of the SUBJECT over an unchanged artifact was accepted. "
        "The signature verifies and the artifact digest matches — only the chain's pin on the exact "
        "consented bytes can catch this, and it did not."
    )
    assert "stip.subject" in dict(verdict.unverified)
