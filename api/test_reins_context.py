"""Canonical context carrier admission and exact-wire tests."""

from __future__ import annotations

import copy
import hashlib
import inspect
import json
import re
from pathlib import Path

import pytest
import reins_context as rc

FIXTURE_PATH = Path(__file__).parent / "fixtures" / "context-canon-gate0-carriers.json"
FIXTURE_SHA256 = "594b6b96656cea2a46e4d50c2201152523bcf4530f0afcb0360425e76c17fae9"
PREDECESSOR_FIXTURE_SHA256 = (
    "c16fce720b4bfb80233b0a3b94a9d5903796c646261651788a9084bfc0e97704"
)
OUTER_KEYS = {
    "schema",
    "state",
    "audience",
    "reason_codes",
    "projection",
    "compatibility",
}
REASON_PATTERN = re.compile(r"^[a-z0-9][a-z0-9_.:-]*$")


def _fixtures() -> dict:
    raw = FIXTURE_PATH.read_bytes()
    assert hashlib.sha256(raw).hexdigest() == FIXTURE_SHA256
    return json.loads(raw)


def _bytes(value: object) -> bytes:
    return json.dumps(
        value,
        allow_nan=False,
        ensure_ascii=True,
        separators=(",", ":"),
        sort_keys=True,
    ).encode("ascii")


class _PinnedProjectionModel:
    def __init__(self, payload: dict) -> None:
        self.payload = payload

    @classmethod
    def model_validate(cls, payload: dict):
        fixtures = _fixtures()
        candidates = tuple(
            fixtures[name]
            for name in (
                "operator_projection",
                "yard_projection",
                "lifecycle_projection",
                "operation_projection",
            )
        )
        if _bytes(payload) not in {_bytes(item) for item in candidates}:
            raise ValueError("not the pinned current projection fixture")
        return cls(payload)

    def model_dump(self, *, mode: str, by_alias: bool) -> dict:
        assert mode == "json" and by_alias is True
        return self.payload


class _PinnedCompatibilityModel(_PinnedProjectionModel):
    @classmethod
    def model_validate(cls, payload: dict):
        if _bytes(payload) != _bytes(_fixtures()["compatibility"]):
            raise ValueError("not the pinned compatibility fixture")
        return cls(payload)


@pytest.fixture(autouse=True)
def _use_pinned_current_contract(monkeypatch):
    """Exercise Reins source without pretending an unpublished wheel exists."""

    monkeypatch.setattr(
        rc,
        "_canonical_models",
        lambda: (_PinnedProjectionModel, _PinnedCompatibilityModel, _bytes),
    )


def _reverse_objects(value: object) -> object:
    if isinstance(value, dict):
        return {
            key: _reverse_objects(item) for key, item in reversed(tuple(value.items()))
        }
    if isinstance(value, list):
        return [_reverse_objects(item) for item in value]
    return value


def _assert_outer(body: bytes) -> dict:
    assert isinstance(body, bytes)
    assert len(body) <= rc.MAX_CONTEXT_READ_BYTES
    value = json.loads(body)
    assert set(value) == OUTER_KEYS
    assert value["schema"] == rc.CONTEXT_READ_SCHEMA
    assert value["state"] in {"dark", "hold"}
    assert value["audience"] == rc.CONTEXT_READ_AUDIENCE
    assert value["reason_codes"] == sorted(set(value["reason_codes"]))
    assert 1 <= len(value["reason_codes"]) <= rc.MAX_REASON_CODES
    for reason in value["reason_codes"]:
        assert reason.isascii()
        assert len(reason.encode("ascii")) <= rc.MAX_REASON_CODE_BYTES
        assert REASON_PATTERN.fullmatch(reason)
    if value["state"] == "dark":
        assert value["projection"] is None
        assert value["compatibility"] is None
    else:
        assert (value["projection"] is None) != (value["compatibility"] is None)
    return value


def _assert_dark(body: bytes, reason: str) -> dict:
    value = _assert_outer(body)
    assert value["state"] == "dark"
    assert value["reason_codes"] == [reason]
    return value


def test_fixture_is_sha_pinned_current_contract_not_predecessor() -> None:
    assert FIXTURE_SHA256 != PREDECESSOR_FIXTURE_SHA256
    fixtures = _fixtures()
    for name in (
        "operator_projection",
        "yard_projection",
        "lifecycle_projection",
        "operation_projection",
    ):
        projection = fixtures[name]
        assert len(projection) == 50
        assert len(projection["events"]) > 0
        assert all(action["operation"] for action in projection["actions"])
        descriptor = projection["demand_shape"]["descriptor"]
        assert descriptor["canon"]["canonical_json"]
        assert descriptor["position_basis"]["canonical_json"]


@pytest.mark.parametrize(
    "fixture_name",
    ("operator_projection", "lifecycle_projection", "operation_projection"),
)
def test_operator_private_current_projections_are_exact_opaque_hold(
    fixture_name: str,
) -> None:
    payload = _fixtures()[fixture_name]
    raw = _bytes(payload)
    body = rc.parse_context_payload_bytes(raw)
    value = _assert_outer(body)
    assert value == {
        "schema": "hapax.reins-context-read.v1",
        "state": "hold",
        "audience": "operator_private",
        "reason_codes": [
            "canonical_projection_verification_unavailable",
            "producer_receipt_missing",
        ],
        "projection": payload,
        "compatibility": None,
    }
    assert b'"projection":' + raw + b',"compatibility":null}' in body
    assert not ({"facts", "counts", "correlation", "affordances"} & set(value))


def test_compatibility_is_exact_opaque_hold() -> None:
    payload = _fixtures()["compatibility"]
    raw = _bytes(payload)
    body = rc.parse_context_payload_bytes(raw)
    value = _assert_outer(body)
    assert value == {
        "schema": "hapax.reins-context-read.v1",
        "state": "hold",
        "audience": "operator_private",
        "reason_codes": ["compatibility_only"],
        "projection": None,
        "compatibility": payload,
    }
    assert body.endswith(b'"compatibility":' + raw + b"}")


def test_internal_whitespace_key_order_and_unicode_escape_survive_exactly() -> None:
    payload = _fixtures()["operator_projection"]
    reordered = _reverse_objects(payload)
    assert list(reordered) == list(reversed(tuple(payload)))
    raw = json.dumps(reordered, allow_nan=False, ensure_ascii=True, indent=2).encode(
        "ascii"
    )
    raw = raw.replace(
        b'"audience": "operator_private"',
        b'"audience": "oper\\u0061tor_private"',
        1,
    )
    assert b"oper\\u0061tor_private" in raw

    body = rc.parse_context_payload_bytes(raw)
    value = _assert_outer(body)
    assert value["state"] == "hold"
    assert value["projection"] == payload
    assert b'"projection":' + raw + b',"compatibility":null}' in body
    assert b"oper\\u0061tor_private" in body


def test_non_operator_projection_is_dark() -> None:
    body = rc.parse_context_payload_bytes(_bytes(_fixtures()["yard_projection"]))
    _assert_dark(body, "audience_unsupported")


@pytest.mark.parametrize(
    "payload",
    [
        b"",
        b"{not-json",
        b'{"schema":"one","schema":"two"}',
        b'{"schema":"one","\\u0073chema":"two"}',
        b'{"outer":{"x":1,"x":2}}',
        b"{}{}",
        b"[]",
        b'"text"',
        b'{"schema":NaN}',
        b"\xff",
        b" {}",
        b"{}\n",
        b'{"n":' + b"9" * 5000 + b"}",
    ],
)
def test_malformed_producer_bytes_are_exact_dark(payload: bytes) -> None:
    _assert_dark(rc.parse_context_payload_bytes(payload), "producer_malformed")


def test_oversized_payload_is_rejected_before_json_decode() -> None:
    body = rc.parse_context_payload_bytes(b" " * (rc.MAX_CONTEXT_READ_BYTES + 1))
    _assert_dark(body, "producer_payload_too_large")


@pytest.mark.parametrize(
    "payload",
    [
        {"kind": "context_bundle"},
        {"schema": "hapax.context-frame.v1"},
        {"schema": "unknown"},
    ],
)
def test_raw_bundle_and_unsupported_schemas_are_dark(payload: dict) -> None:
    _assert_dark(
        rc.parse_context_payload_bytes(_bytes(payload)),
        "producer_schema_unsupported",
    )


def test_recognized_but_invalid_carrier_is_malformed_dark() -> None:
    body = rc.parse_context_payload_bytes(_bytes({"schema": rc.PROJECTION_SCHEMA}))
    _assert_dark(body, "producer_malformed")


@pytest.mark.parametrize(
    "mutation",
    ("events", "operation", "canon", "position_basis"),
)
def test_predecessor_or_partial_current_projection_is_rejected(mutation: str) -> None:
    payload = copy.deepcopy(_fixtures()["operator_projection"])
    if mutation == "events":
        del payload["events"]
    elif mutation == "operation":
        del payload["actions"][0]["operation"]
    else:
        del payload["demand_shape"]["descriptor"][mutation]
    _assert_dark(rc.parse_context_payload_bytes(_bytes(payload)), "producer_malformed")


def test_hash_tampering_is_rejected_by_canonical_model() -> None:
    payload = copy.deepcopy(_fixtures()["operator_projection"])
    payload["generated_at"] = "2026-07-10T16:07:00Z"
    _assert_dark(rc.parse_context_payload_bytes(_bytes(payload)), "producer_malformed")


def test_bool_integer_normalization_cannot_admit_noncanonical_wire_type() -> None:
    payload = copy.deepcopy(_fixtures()["operator_projection"])
    assert payload["may_authorize"] is False
    payload["may_authorize"] = 0
    _assert_dark(rc.parse_context_payload_bytes(_bytes(payload)), "producer_malformed")


def test_missing_canonical_package_fails_dark(monkeypatch) -> None:
    def unavailable():
        raise rc.ContextReadError("canonical_verifier_unavailable")

    monkeypatch.setattr(rc, "_canonical_models", unavailable)
    body = rc.parse_context_payload_bytes(_bytes(_fixtures()["operator_projection"]))
    _assert_dark(body, "canonical_verifier_unavailable")


def test_reason_codes_are_sorted_unique_and_bounded() -> None:
    body = rc.dark_readout_bytes("z_reason", "a_reason", "z_reason")
    value = _assert_outer(body)
    assert value["reason_codes"] == ["a_reason", "z_reason"]

    for invalid in ("", "Uppercase", "contains space", "nonascii-\u00e9", "x" * 129):
        with pytest.raises(rc.ContextReadError, match="context_read_error"):
            rc.dark_readout_bytes(invalid)
    with pytest.raises(rc.ContextReadError, match="context_read_error"):
        rc.dark_readout_bytes(*(f"reason_{index}" for index in range(65)))


def test_private_renderer_enforces_exact_dark_hold_matrix() -> None:
    with pytest.raises(rc.ContextReadError, match="context_read_error"):
        rc._render_readout_bytes(
            state="hold",
            reason_codes=("context_read_error",),
            carrier=None,
        )
    with pytest.raises(rc.ContextReadError, match="context_read_error"):
        rc._ValidatedCarrier(
            "projection",
            b'{"x":1,"x":2}',
            object(),
        )
    with pytest.raises(TypeError):
        rc._render_readout_bytes(  # type: ignore[call-arg]
            state="hold",
            reason_codes=("context_read_error",),
            projection_bytes=b'{"x":1,"x":2}',
        )
    assert not hasattr(rc, "hold_projection")
    assert not hasattr(rc, "hold_compatibility")


def test_full_outer_response_exact_limit_and_over_limit(monkeypatch) -> None:
    class PermissiveModel:
        def __init__(self, payload):
            self.payload = payload

        @classmethod
        def model_validate(cls, payload):
            return cls(payload)

        def model_dump(self, *, mode, by_alias):
            assert mode == "json" and by_alias is True
            return self.payload

    monkeypatch.setattr(
        rc,
        "_canonical_models",
        lambda: (PermissiveModel, PermissiveModel, _bytes),
    )
    monkeypatch.setattr(rc, "_require_current_projection_shape", lambda _payload: None)
    payload = {
        "schema": rc.PROJECTION_SCHEMA,
        "audience": rc.CONTEXT_READ_AUDIENCE,
        "padding": "",
    }
    base_raw = _bytes(payload)
    base_body = rc.parse_context_payload_bytes(base_raw)
    assert _assert_outer(base_body)["state"] == "hold"

    payload["padding"] = "x" * (rc.MAX_CONTEXT_READ_BYTES - len(base_body))
    at_limit_raw = _bytes(payload)
    at_limit_body = rc.parse_context_payload_bytes(at_limit_raw)
    assert len(at_limit_body) == rc.MAX_CONTEXT_READ_BYTES
    assert _assert_outer(at_limit_body)["state"] == "hold"
    assert b'"projection":' + at_limit_raw + b',"compatibility":null}' in at_limit_body

    payload["padding"] += "x"
    over_limit_raw = _bytes(payload)
    assert len(over_limit_raw) < rc.MAX_CONTEXT_READ_BYTES
    _assert_dark(
        rc.parse_context_payload_bytes(over_limit_raw),
        "producer_payload_too_large",
    )


def test_public_carrier_surface_is_bytes_only() -> None:
    assert isinstance(rc.dark_readout_bytes("producer_absent"), bytes)
    assert isinstance(
        rc.parse_context_payload_bytes(_bytes(_fixtures()["operator_projection"])),
        bytes,
    )
    assert "dark_readout" not in rc.__all__
    assert "hold_projection" not in rc.__all__
    assert "hold_compatibility" not in rc.__all__


def test_gate0_defines_no_present_or_semantic_projection_path() -> None:
    source = inspect.getsource(rc)
    for forbidden in (
        "reins_context_correlation",
        "project_context_frame",
        "project_context_bundle_v1",
        "fingerprint_correlation",
        "affordance_explanation",
        "public_or_air",
    ):
        assert forbidden not in source
    assert 'state="present"' not in source
    for removed in (
        "AUDIENCES",
        "ContextBundleEnvelope",
        "air_decision",
        "project",
        "project_all",
        "seal_fact",
        "validate_context_bundle",
    ):
        assert not hasattr(rc, removed)


def test_adapter_exposes_no_effectful_path_or_runtime_fixture_dependency() -> None:
    for banned in (
        "send",
        "dispatch",
        "inject",
        "publish",
        "spawn",
        "spend",
        "provider_call",
    ):
        assert not hasattr(rc, banned)
    source = inspect.getsource(rc)
    assert FIXTURE_PATH.name not in source
