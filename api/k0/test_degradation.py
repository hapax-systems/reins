"""R2.6 — the degradation ledger, tested against the case that motivated it."""

from __future__ import annotations

import hashlib
import json
import subprocess
from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from bootstrap_receipt import RECEIPT_CHAIN_FILENAME, append_receipt, genesis_self_attest
from k0 import degradation as deg
from k0.degradation import (
    Degradation,
    DegradationError,
    Lifecycle,
    accept,
    declare,
    lift,
    rank,
    render,
    state,
)
from k0.ratifier import write_allowed_signers

ESTATE = "estate-0000000000000000"
KERNEL = "k0-test"


def _keypair(tmp_path: Path) -> Path:
    key = tmp_path / "ratifier_ed25519"
    subprocess.run(
        [
            "ssh-keygen",
            "-t",
            "ed25519",
            "-N",
            "",
            "-C",
            "ratifier@test",
            "-f",
            str(key),
        ],
        check=True,
        capture_output=True,
    )
    write_allowed_signers(
        tmp_path / "allowed_signers",
        "ratifier@test",
        key.with_suffix(".pub").read_text().strip(),
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


#: Drawn from a real incident: a review floor wanting two model families, on a host that could
#: seat one. It was reported as a quota problem and the deficit was recorded nowhere.
#: NO HOST IS NAMED — test_k0.py enforces estate-independence across the whole k0 package, and
#: that guard is right: a stranger's kernel must not carry this estate's hostnames, not even in a
#: fixture. The incident belongs in the estate's records.
REVIEW_FLOOR = Degradation(
    subject="review-floor",
    level=Lifecycle.DEGRADED,
    why="only one model family is seatable on this host; the t1_critical floor is two",
    tradeoff="reviews carry a single-family perspective; correlated blind spots go undetected",
    lift_condition="a second family becomes seatable and its capability receipt observes it",
)


def test_a_degradation_is_not_in_effect_until_the_sovereign_consents(
    tmp_path: Path,
) -> None:
    """Declaring is asking. Only ratification puts the estate into a degraded mode.

    This is the difference between a system that notices it is degraded and a system that has been
    told it may keep going anyway.
    """
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    assert state(root) == {}, (
        "a declared-but-unratified degradation must not be in effect"
    )

    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert "review-floor" in state(root)
    assert state(root)["review-floor"].level is Lifecycle.DEGRADED


def test_a_degradation_must_state_its_cost_and_its_exit(tmp_path: Path) -> None:
    """An unnamed loss cannot be consented to; an unliftable degradation is a dead end."""
    with pytest.raises(ValueError, match="TRADE-OFF"):
        Degradation(
            subject="x",
            level=Lifecycle.DEGRADED,
            why="w",
            tradeoff="  ",
            lift_condition="l",
        )
    with pytest.raises(ValueError, match="LIFT CONDITION"):
        Degradation(
            subject="x",
            level=Lifecycle.DEGRADED,
            why="w",
            tradeoff="t",
            lift_condition="   ",
        )
    with pytest.raises(ValueError, match="FULL is not a degradation"):
        Degradation(
            subject="x", level=Lifecycle.FULL, why="w", tradeoff="t", lift_condition="l"
        )


def test_nothing_decays_back_to_full(tmp_path: Path) -> None:
    """A deficit does not heal by being ignored.

    The receipt is written a year in the past. Age must not restore capability — only a ratified
    lift does.
    """
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    long_ago = datetime.now(UTC) - timedelta(days=300)
    declare(
        root,
        REVIEW_FLOOR,
        estate_id=ESTATE,
        kernel_version=KERNEL,
        observed_at=long_ago,
    )
    accept(
        root,
        REVIEW_FLOOR,
        key_path=key,
        estate_id=ESTATE,
        kernel_version=KERNEL,
        observed_at=long_ago,
    )
    assert "review-floor" in state(root), (
        "a degradation accepted 300 days ago silently expired. Capability that returns by itself "
        "was never given up in the first place."
    )


def test_a_lift_is_itself_consented_and_requires_evidence(tmp_path: Path) -> None:
    """Restoring capability is as consequential as giving it up, so it is signed too."""
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    with pytest.raises(DegradationError) as exc:
        lift(
            root,
            "review-floor",
            evidence="  ",
            key_path=key,
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )
    assert exc.value.refusal is not None and exc.value.refusal.legal_next

    lift(
        root,
        "review-floor",
        evidence="a second family became seatable; its capability receipt observed it",
        key_path=key,
        estate_id=ESTATE,
        kernel_version=KERNEL,
    )
    assert state(root) == {}, "a ratified lift must clear the degradation"


def test_lifting_something_that_is_not_degraded_is_refused(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    with pytest.raises(DegradationError):
        lift(
            root,
            "nothing",
            evidence="e",
            key_path=key,
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )


def test_a_re_degradation_after_a_lift_is_in_effect(tmp_path: Path) -> None:
    """CHAIN ORDER. Degrade, lift, degrade again — the latest ratified act wins.

    Reading ratified rows in arbitrary order would let the old lift mask the new degradation, and
    the estate would believe itself FULL while running below its floor. That is the exact class of
    silent-full error this ledger exists to remove.
    """
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    lift(
        root,
        "review-floor",
        evidence="second family seated",
        key_path=key,
        estate_id=ESTATE,
        kernel_version=KERNEL,
    )
    assert state(root) == {}

    again = Degradation(
        subject="review-floor",
        level=Lifecycle.HELD,
        why="the second family's credential expired",
        tradeoff="no reviews can be seated at all",
        lift_condition="refresh the credential and re-observe the receipt",
    )
    declare(root, again, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, again, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    current = state(root)
    assert "review-floor" in current, (
        "a re-degradation after a lift was masked by the older lift — the estate would report "
        "itself FULL while held"
    )
    assert current["review-floor"].level is Lifecycle.HELD


def test_render_is_honest_dark_and_carries_the_exit(tmp_path: Path) -> None:
    """The kernel law forbids showing a degraded subject as a bare value.

    Every rendered line must carry the deficit AND what would lift it, so the surface teaches from
    the row instead of displaying a number that is quietly wrong.
    """
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    lines = render(root)
    assert lines, "a degraded estate rendered nothing at all"
    blob = "\n".join(lines)
    assert "DEGRADED" in blob
    assert REVIEW_FLOOR.why in blob, "the deficit must be visible"
    assert REVIEW_FLOOR.tradeoff in blob, "the cost must be visible"
    assert REVIEW_FLOOR.lift_condition in blob, "a rendering with no exit is a dead end"


def test_a_body_that_is_not_the_pinned_artifact_is_refused(tmp_path: Path) -> None:
    """The stored body must be the artifact the chain pins, or the ledger proves nothing."""
    from k0.degradation import _accept_body

    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(DegradationError, match="not the artifact"):
        _accept_body(
            root,
            REVIEW_FLOOR.stipulation(),
            b"a different body entirely",
            key_path=key,
            estate_id=ESTATE,
            kernel_version=KERNEL,
            observed_at=None,
        )


def test_the_lattice_is_ordered_as_data(tmp_path: Path) -> None:
    assert (
        rank(Lifecycle.FULL)
        < rank(Lifecycle.DEGRADED)
        < rank(Lifecycle.HELD)
        < rank(Lifecycle.REFUSED)
    )


def test_a_body_present_without_ratification_still_does_not_take_effect(
    tmp_path: Path,
) -> None:
    """The RATIFIED check must be load-bearing, not incidental.

    Found by mutation testing: deleting the `act is RATIFIED` guard in state() survived the whole
    suite. It survived because unratified rows happen to have no `.body` file yet, so they were
    skipped for an unrelated reason. That is a guard resting on a coincidence — the moment anything
    writes a body earlier (a draft flow, a migration, a restore from backup), an unratified
    degradation would silently take effect and the estate would believe the operator had consented.

    So the coincidence is removed here: the body is written for a merely-DECLARED degradation, and
    state() must still refuse to honour it. Consent is what puts a degradation in effect; the
    presence of a file is not consent.
    """
    root = _root(tmp_path)
    _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)

    (root / "ratifications").mkdir(parents=True, exist_ok=True)
    (root / "ratifications" / f"{REVIEW_FLOOR.stipulation_id()}.body").write_bytes(
        REVIEW_FLOOR.body()
    )

    assert state(root) == {}, (
        "a DECLARED degradation with a body on disk took effect without the sovereign's signature. "
        "Consent is the act that degrades the estate — never the presence of a file."
    )


def test_a_ratified_body_edited_after_consent_is_refused_not_reported(
    tmp_path: Path,
) -> None:
    """THE LEDGER MUST NOT PROVE CONSENT WAS GIVEN WHILE LYING ABOUT WHAT IT WAS GIVEN TO.

    `_accept_body` hashes the artifact at WRITE time against the digest the chain pins. Until this
    was also checked at READ time, editing the `.body` afterwards changed what the estate believed
    had been accepted — a different level, a different tradeoff, a different lift condition — while
    the chain still said `ratified` and still pointed at the original digest. Found in review.

    AND IT REFUSES RATHER THAN DROPPING THE SUBJECT. Returning None on mismatch would remove the
    subject from `state()`, so a silently-edited degradation would read as FULL: the estate would
    report itself healthy precisely because its record of being unhealthy had been tampered with.
    That is absence-into-zero aimed at the worst available answer.
    """
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert "review-floor" in state(root), (
        "fixture precondition: it must be in effect first"
    )

    # THE TAMPER LEAVES A STRUCTURALLY VALID DEGRADATION, deliberately.
    #
    # An earlier version of this test set level=FULL, which `Degradation.__post_init__` rejects
    # downstream — so the mutant died on someone else's guard and this test proved nothing about
    # the digest check. Rewriting `why`, `tradeoff` and `lift_condition` keeps the object valid,
    # so without the check `state()` reports attacker-chosen terms as what the operator accepted:
    # silently, with the chain still saying `ratified`.
    body_path = root / "ratifications" / f"{REVIEW_FLOOR.stipulation_id()}.body"
    tampered = json.loads(body_path.read_text(encoding="utf-8"))
    tampered["why"] = "no deficit worth mentioning"
    tampered["tradeoff"] = "none"
    tampered["lift_condition"] = "already lifted"
    body_path.write_text(json.dumps(tampered), encoding="utf-8")

    with pytest.raises(DegradationError, match="changed after consent") as exc:
        state(root)
    assert REVIEW_FLOOR.stipulation_id() in str(exc.value), (
        "the refusal must name the artifact"
    )


def test_a_ratified_row_that_pins_no_artifact_is_refused(tmp_path: Path) -> None:
    """ "The row names no artifact" is not "the artifact is fine".

    An unpinnable body cannot be checked, and an uncheckable degradation must not be reported as
    current state — the same rule as an unscanned class not being a clean one.
    """
    from k0 import degradation as deg

    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    with pytest.raises(DegradationError, match="pins no artifact digest"):
        deg._body_for(root, REVIEW_FLOOR.stipulation_id(), digest=None)


def test_stripping_the_artifact_ref_from_the_chain_does_not_clear_the_deficit(
    tmp_path: Path,
) -> None:
    """THE SECOND DOOR TO THE SAME ROOM, found in review round three.

    The first fix made a missing body fatal WHEN THE ROW PINS A DIGEST, and left "no digest and no
    body" returning None — which `state()` skipped, so the subject vanished and the estate read
    FULL. Shutting one route into a wrong answer while leaving its neighbour open is the half-a-pair
    error, and it recurred here inside the very fix written to close the first half.

    This is the attack in full: edit the ratified row to drop its `stipulation:` ref and delete the
    body. `load_chain` does NOT verify hashes — `verify_chain` is a separate act nobody is obliged
    to perform first — so the edited row is read as authentic, and the degradation had no remaining
    trace to recover. Nothing about the result looked wrong: the chain still said `ratified`.

    The defence is not "detect the edit" (that is verify_chain's job) but "never resolve an
    unreadable deficit to no deficit".
    """
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert "review-floor" in state(root), "fixture precondition: it must be in effect first"

    chain_path = root / RECEIPT_CHAIN_FILENAME
    rows = []
    stripped = 0
    for line in chain_path.read_text(encoding="utf-8").splitlines():
        record = json.loads(line)
        refs = record.get("payload_refs") or []
        kept = [ref for ref in refs if not ref.startswith("stipulation:")]
        stripped += len(refs) - len(kept)
        record["payload_refs"] = kept
        rows.append(json.dumps(record))
    assert stripped, (
        "fixture premise: the chain must have carried a stipulation ref to strip, or this test "
        "would assert the refusal of a condition it never created"
    )
    chain_path.write_text("\n".join(rows) + "\n", encoding="utf-8")
    (root / "ratifications" / f"{REVIEW_FLOOR.stipulation_id()}.body").unlink()

    with pytest.raises(DegradationError, match="pins no artifact digest") as exc:
        state(root)
    assert exc.value.refusal is not None and exc.value.refusal.legal_next.strip()


def test_the_body_is_on_disk_before_the_ratification_that_points_at_it(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """WRITE ORDER, ASSERTED AS ORDER. A ratified row must never exist without its artifact.

    `_accept_body` used to ratify first and write the body second, leaving a window in which the
    chain said `ratified` and no body existed. That was survivable only while a missing body was
    silently tolerated — the very thing now (correctly) fatal. So hardening the READER converted a
    crash in that window from "a degradation quietly disappears" into "the ledger can never be read
    again", with no way out: `lift` calls `state` first, so the estate could not even lift its way
    clear. A fix on one side deepened the defect on the other, and only fixing both is a fix.

    THE FIRST VERSION OF THIS TEST WAS VACUOUS. It drove a ratification failure and asserted the
    ledger survived — which passes under BOTH orderings, because a `ratify` that raises writes no
    body either way. It tested a true statement that has nothing to do with the change. The order
    is the property, so the order is what gets observed: this intercepts `ratify` and records
    whether the artifact was already durable at the moment it was called.
    """
    from k0 import degradation as deg

    observed: dict[str, bool] = {}
    real_ratify = deg.ratify

    def watching_ratify(root: Path, stip, **kwargs):  # type: ignore[no-untyped-def]
        body = root / "ratifications" / f"{stip.stipulation_id}.body"
        observed["body_already_durable"] = body.is_file()
        return real_ratify(root, stip, **kwargs)

    monkeypatch.setattr(deg, "ratify", watching_ratify)

    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    assert observed.get("body_already_durable"), (
        "the ratification was recorded BEFORE its artifact reached disk. A crash in that window "
        "leaves a ratified row whose body never existed — which the reader now refuses forever, "
        "and `lift` cannot clear because it reads `state` first."
    )
    assert "review-floor" in state(root), "the ordering must not change the outcome on success"


def test_a_failed_ratification_leaves_a_readable_ledger(tmp_path: Path) -> None:
    """The other half of the reordering: an orphan body must be inert, not poisonous.

    Writing the artifact first means a failure during ratification can leave a body with no row
    pointing at it. `state()` reads bodies only for rows that exist, so it must be invisible — and
    a retry rewrites identical bytes, since the digest is what names the content.
    """
    root = _root(tmp_path)
    _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)

    not_a_key = tmp_path / "not_a_key"
    not_a_key.write_text("this cannot sign anything", encoding="utf-8")
    with pytest.raises(DegradationError):
        accept(root, REVIEW_FLOOR, key_path=not_a_key, estate_id=ESTATE, kernel_version=KERNEL)

    assert state(root) == {}, (
        "a failed ratification left the ledger unreadable. Nothing was consented to, so nothing "
        "may be reported — and nothing may raise either."
    )
    body_path = root / "ratifications" / f"{REVIEW_FLOOR.stipulation_id()}.body"
    assert body_path.read_bytes() == REVIEW_FLOOR.body(), (
        "the body is written BEFORE the ratification that points at it, so a failed accept "
        "always leaves the whole artifact — never a partial write, and never nothing. Guarding "
        "this assertion on existence would let a write-order regression pass silently."
    )


def test_deleting_a_ratified_body_does_not_clear_the_deficit(tmp_path: Path) -> None:
    """THE CRITICAL: a ratified degradation whose body is deleted must not read as FULL.

    `state()` skips a subject whose body resolves to None, so tolerating a missing file made
    deleting the artifact the most effective way to clear a deficit — the estate reports itself
    healthy precisely BECAUSE its record of being unhealthy is gone. That is the same
    absence-into-zero the digest check closes, one cell over: an earlier revision of `_body_for`
    reasoned correctly about the mismatch case and then tolerated deletion unconditionally, on the
    ground that "an old chain is not a corrupt one". True — but only of a row that pins no digest.
    A row that pins one asserts the artifact EXISTS, and against that assertion absence is deletion.

    This asserts at the `state()` level deliberately. `_body_for` returning a refusal is the
    mechanism; "the estate does not report FULL" is the property, and only the caller shows it.
    """
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert "review-floor" in state(root), (
        "fixture precondition: it must be in effect first"
    )

    (root / "ratifications" / f"{REVIEW_FLOOR.stipulation_id()}.body").unlink()

    with pytest.raises(DegradationError, match="no body is stored") as exc:
        state(root)
    assert REVIEW_FLOOR.stipulation_id() in str(exc.value), (
        "the refusal must name the artifact"
    )


def test_a_decodable_body_that_is_not_a_degradation_body_is_refused(tmp_path: Path) -> None:
    """Shape is part of the artifact (CodeRabbit threads on :407 and :506, held open for this).

    The digest check proves the bytes are the consented ones; it says nothing about whether
    `state()` can READ them. A body of `[]` decodes fine and then dies as a TypeError one caller
    later; a dict missing `lift_condition` dies as a KeyError; an unknown `level` dies inside
    Lifecycle(...); `full` without `lifted` dies in Degradation.__post_init__ — each a bare crash
    where every sibling route produces a Refusal with a legal next move. The bytes being consented
    to does not make them a degradation body.
    """
    root = _root(tmp_path)
    body_dir = root / "ratifications"
    body_dir.mkdir(parents=True, exist_ok=True)
    target = body_dir / "degradation.shape.body"

    good = {
        "subject": "s",
        "level": "degraded",
        "why": "w",
        "tradeoff": "t",
        "lift_condition": "l",
    }
    cases = {
        "not an object": b"[]",
        "a scalar": b"1",
        "missing keys": json.dumps({"subject": "s"}).encode(),
        "unknown level": json.dumps(good | {"level": "mostly"}).encode(),
        "full without lifted": json.dumps(good | {"level": "full"}).encode(),
    }
    for name, blob in cases.items():
        target.write_bytes(blob)
        with pytest.raises(DegradationError, match="not a degradation body") as exc:
            deg._body_for(root, "degradation.shape", digest=hashlib.sha256(blob).hexdigest())
        assert exc.value.refusal is not None and exc.value.refusal.legal_next.strip(), (
            f"{name}: the refusal must carry a legal next move"
        )

    lifted = json.dumps(good | {"level": "full", "lifted": True}).encode()
    target.write_bytes(lifted)
    assert deg._body_for(root, "degradation.shape", digest=hashlib.sha256(lifted).hexdigest())[
        "lifted"
    ], "a lifted FULL body is the one legal FULL — lift() writes it"


def _delete(path: Path) -> tuple[str, str]:
    """Return (digest to pin, expected refusal). The pinned digest is never reached."""
    path.unlink()
    return "0" * 64, "no body is stored"


def _make_unreadable(path: Path) -> tuple[str, str]:
    path.chmod(0o000)
    return "0" * 64, "could not be read"


def _replace_with_undecodable_bytes(path: Path) -> tuple[str, str]:
    """Bytes that hash fine and parse badly.

    The digest check runs first, so reaching the parse branch requires the pinned digest to be the
    hash OF THE CORRUPT BYTES — i.e. the operator consented to something unusable. Passing the real
    pinned digest here would be caught as a mismatch, and a test claiming to cover the parse branch
    would never have entered it.
    """
    raw = b"\xff\xfe not json at all"
    path.write_bytes(raw)
    return hashlib.sha256(raw).hexdigest(), "not decodable JSON"


@pytest.mark.parametrize(
    "corrupt",
    [_delete, _make_unreadable, _replace_with_undecodable_bytes],
    ids=["deleted", "unreadable", "undecodable"],
)
def test_every_unreadable_body_refuses_and_carries_a_legal_next_move(
    tmp_path: Path,
    corrupt: Callable[[Path], tuple[str, str]],
) -> None:
    """A CLAIMED-BUT-UNREADABLE artifact refuses on every route, never resolving to "no deficit".

    The parametrisation is the point. An earlier version covered deletion alone, and "one vector
    refuses" says nothing about the others — the same existential-for-universal error that let one
    verified regex stand in for all six. Each vector here fails at a different statement.

    Each refusal must also carry a legal next move (INV-3): "your ledger is corrupt" with no route
    out is a dead end, and the operator meets these at the moment they can least afford to guess.
    """
    root = _root(tmp_path)
    key = _keypair(tmp_path)
    declare(root, REVIEW_FLOOR, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, REVIEW_FLOOR, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    body_path = root / "ratifications" / f"{REVIEW_FLOOR.stipulation_id()}.body"
    digest, expected = corrupt(body_path)

    try:
        with pytest.raises(DegradationError, match=expected) as exc:
            deg._body_for(root, REVIEW_FLOOR.stipulation_id(), digest=digest)
        refusal = exc.value.refusal
        assert refusal is not None, "a refusal must be data, not only a message (INV-3)"
        assert refusal.legal_next.strip(), "an empty next move teaches nothing"
        assert refusal.gate == "degradation.body-integrity", (
            "a refusal must name its gate"
        )
    finally:
        # Restore read permission or tmp_path teardown fails on the 0o000 case.
        if body_path.exists():
            body_path.chmod(0o600)
