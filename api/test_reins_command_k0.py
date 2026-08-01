"""The governed COMMAND surface under the K0 kernel. (R2.1)

The executor predates K0. It was built with two-valued predicates and bare status strings, which is
one outcome short of the ratified fail-closed law and carries none of the refusal-as-data that INV-3
requires. These tests hold the LAW, not the incident that revealed it.

MEASURED BEFORE THE FIX, against the pre-K0 router:

    POST /command/dispatch  authority_packet="not-a-dict"
      -> HTTP 500, body 'Internal Server Error' (non-JSON)
      -> route_command raised AttributeError and returned no Response at all

It denied in effect — nothing was written — but it denied INCOHERENTLY: no status, no reason, no
legal next move. A caller learns only that something broke. That is the dead end INV-3 forbids, and
the reason "BLOCKED always escapes" is a law about the SHAPE of a denial, not merely its existence.
"""

import pytest
from fastapi.testclient import TestClient

from k0.fail_closed import Evaluation, decide
from k0.refusal import DeadEndRefusalError, Refusal
from reins_command import Envelope, Response, build_command_app, route_command

ENV = Envelope(
    verb="dispatch",
    target="task-1",
    authority_packet={"authority_case": "a", "parent_spec": "p", "message_id": "m"},
    preflight_receipt={},
    idempotency_key="k1",
)


def _route(*, verify=lambda p, t: True, preflight=lambda e: True, transport=None, emitted=None):
    return route_command(
        ENV,
        verify_authority=verify,
        preflight=preflight,
        transport=transport or (lambda e: Response(status="ok", http=200, receipt_id="r1")),
        already_emitted=emitted if emitted is not None else {},
    )


# --- the third outcome ------------------------------------------------------------------------


def _raises(*_args):
    raise AttributeError("'str' object has no attribute 'get'")


def test_a_predicate_that_raises_is_unevaluable_and_denies():
    """The measured defect. A predicate that cannot run must not escape as a 500."""
    resp = _route(verify=_raises)
    assert resp.status == "authority-unevaluable"
    assert resp.http == 403
    assert "could not be evaluated" in resp.reason
    assert "AttributeError" in resp.reason


def test_a_predicate_that_returns_none_stated_no_verdict_and_denies():
    """`None` is not falsy-therefore-rejected: it is an ABSENT verdict, which is a different fact.
    Collapsing it into VIOLATED would report a decision that was never made."""
    resp = _route(verify=lambda p, t: None)
    assert resp.status == "authority-unevaluable"
    assert "stated no verdict" in resp.reason


def test_a_predicate_may_return_the_evaluation_itself():
    resp = _route(verify=lambda p, t: Evaluation.UNEVALUABLE)
    assert resp.status == "authority-unevaluable"
    assert resp.http == 403


def test_unevaluable_is_distinguishable_from_rejected_in_the_body():
    """Both deny with 403 — deliberately, so no existing client can newly PASS on an unknown code.
    The distinction an auditor needs lives in `status`, where adding it breaks nothing."""
    unevaluable = _route(verify=_raises)
    rejected = _route(verify=lambda p, t: False)
    assert unevaluable.http == rejected.http == 403
    assert unevaluable.status != rejected.status


def test_preflight_has_the_same_three_outcomes():
    assert _route(preflight=_raises).status == "preflight-unevaluable"
    assert _route(preflight=lambda e: False).status == "preflight-failed"
    assert _route(preflight=_raises).http == 409


# --- refusal-as-data: no dead ends ------------------------------------------------------------


@pytest.mark.parametrize(
    "kwargs",
    [
        {"verify": _raises},
        {"verify": lambda p, t: False},
        {"preflight": _raises},
        {"preflight": lambda e: False},
        {"transport": lambda e: None},
        {"transport": _raises},
    ],
    ids=["auth-unevaluable", "auth-rejected", "pre-unevaluable", "pre-failed", "no-receipt", "transport-raised"],
)
def test_every_refusing_arm_states_a_legal_next_move(kwargs):
    """INV-3: BLOCKED always escapes. Not one arm may leave the caller without a move."""
    resp = _route(**kwargs)
    assert resp.http >= 400
    assert resp.legal_next and resp.legal_next.strip()
    assert resp.reason and resp.reason.strip()


def test_a_refusal_never_carries_the_value_it_refused():
    """k0.Refusal states the rule: a refusal never carries the value it refused, because that value
    is frequently the thing under restriction. `reason` is serialized straight into the HTTP body, so
    interpolating an exception's MESSAGE publishes whatever the predicate happened to be holding —
    packet contents, filesystem paths, upstream error bodies. The TYPE is kept for auditability; the
    message goes to the log."""

    def leaks(*_args):
        raise ValueError("authority_packet={'secret_token': 'hunter2'} at /srv/hapax/keys/id_ed25519")

    for resp in (_route(verify=leaks), _route(preflight=leaks), _route(transport=leaks)):
        assert "hunter2" not in (resp.reason or "")
        assert "id_ed25519" not in (resp.reason or "")
        assert "ValueError" in (resp.reason or "")


# --- a predicate that refuses in the kernel's own idiom -----------------------------------------


def _decides(*_args):
    """A predicate written the way K0 writes them: it calls decide(), which raises RefusalError."""
    decide(
        "ratifier-verify",
        Evaluation.UNEVALUABLE,
        legal_next="register the sovereign's public key in the allowed-signers file",
        unevaluable_why="ssh-keygen is absent; the ratification is unverified",
        teaches="doctrine/ratifier-key",
    )


def test_a_predicates_own_refusal_is_forwarded_not_restated():
    """RefusalError exists so callers receive DATA, never a parsed string. Catching it as a generic
    failure would stringify the Refusal and discard the gate, legal_next and teaches the predicate
    had already stated correctly."""
    resp = _route(verify=_decides)
    assert resp.http == 403
    assert resp.status == "authority-refused"
    assert resp.reason == "ssh-keygen is absent; the ratification is unverified"
    assert resp.legal_next == "register the sovereign's public key in the allowed-signers file"
    assert resp.teaches == "doctrine/ratifier-key"


def test_a_forwarded_refusal_is_not_reclassified():
    """decide() collapses VIOLATED and UNEVALUABLE into one exception, so a propagated refusal
    cannot be sorted back into either. It is reported as the predicate's own refusal rather than
    guessed at — claiming 'we checked and it failed' about a check that may never have run is the
    exact collapse the two separate *_why arguments exist to prevent."""
    assert _route(verify=_decides).status == "authority-refused"
    assert _route(preflight=_decides).status == "preflight-refused"
    assert _route(preflight=_decides).http == 409


def test_a_forwarded_refusal_still_states_a_legal_next_move():
    for kwargs in ({"verify": _decides}, {"preflight": _decides}):
        assert _route(**kwargs).legal_next.strip()


def test_the_dead_end_ban_is_structural_not_conventional():
    """Refusals are built THROUGH k0.Refusal, whose constructor refuses an empty legal_next. That is
    why the guarantee above holds for arms nobody remembered to test."""
    with pytest.raises(DeadEndRefusalError):
        Refusal(gate="g", why="w", legal_next="   ")


# --- the transport boundary --------------------------------------------------------------------


def test_a_raising_transport_reports_that_nothing_was_written():
    """The caller's real question after a failed write is 'did it land?'. A 500 answers 'unknown',
    which is the one answer that cannot be acted on."""
    resp = _route(transport=_raises)
    assert resp.status == "transport-failed"
    assert resp.http == 502
    assert "nothing was written" in resp.legal_next
    assert resp.applied is False


def test_a_transport_failure_never_reports_applied():
    for t in (lambda e: None, _raises):
        assert _route(transport=t).applied is False


# --- what must NOT have changed ----------------------------------------------------------------


def test_bool_predicates_still_mean_what_they_meant():
    assert _route().status == "ok"
    assert _route(verify=lambda p, t: False).status == "authority-rejected"
    assert _route(preflight=lambda e: False).status == "preflight-failed"


def test_the_admit_path_still_returns_the_transports_own_receipt():
    receipt = Response(status="ok", http=200, receipt_id="r9", applied=True)
    assert _route(transport=lambda e: receipt) is receipt


def test_idempotent_replay_is_untouched():
    resp = _route(emitted={"k1": "prior"})
    assert resp.status == "idempotent-replay"
    assert resp.duplicate is True
    assert resp.receipt_id == "prior"


# --- the HTTP surface --------------------------------------------------------------------------


def _app():
    return build_command_app(
        verb="dispatch",
        verify_authority=_raises,
        preflight=lambda e: True,
        transport=lambda e: Response(status="ok", http=200, receipt_id="r"),
    )


def _body():
    return {"target": "t", "authority_packet": "not-a-dict", "preflight_receipt": {}, "idempotency_key": "k"}


def test_the_http_surface_answers_a_governed_refusal_not_a_500():
    resp = TestClient(_app(), raise_server_exceptions=False).post("/command/dispatch", json=_body())
    assert resp.status_code == 403
    body = resp.json()
    assert body["status"] == "authority-unevaluable"
    assert body["legal_next"]


def test_an_unwired_verb_is_a_refusal_that_still_teaches():
    """501 was the last dead end: 'not wired' with nowhere to go."""
    resp = TestClient(_app()).post("/command/arm", json=_body())
    assert resp.status_code == 501
    body = resp.json()
    assert body["status"] == "not-implemented"
    assert "meta.verbs" in body["legal_next"]


def test_applied_is_visible_over_http():
    """The anti-false-green flag existed on the dataclass but was not serialized, so no HTTP client
    could tell a real write from a preview — which is precisely what it was added to prevent."""
    resp = TestClient(_app(), raise_server_exceptions=False).post("/command/dispatch", json=_body())
    assert "applied" in resp.json()
