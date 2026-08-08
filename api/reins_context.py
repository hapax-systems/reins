"""Strict operator-private carrier for canonical Hapax context readouts.

Reins admits bounded producer bytes, validates a canonical projection or
compatibility carrier, and exposes those exact bytes under one fixed DARK/HOLD
envelope. It does not compile, project, correlate, enrich, or authorize.
"""

from __future__ import annotations

import json
import re
from collections.abc import Iterable
from typing import Any, Callable, Final, Literal

CONTEXT_READ_SCHEMA: Final = "hapax.reins-context-read.v1"
CONTEXT_READ_AUDIENCE: Final = "operator_private"
PROJECTION_SCHEMA: Final = "hapax.projection-envelope.v1"
COMPATIBILITY_SCHEMA: Final = "hapax.context-bundle-v1-compatibility.v1"
MAX_CONTEXT_READ_BYTES: Final = 16 << 20
MAX_REASON_CODES: Final = 64
MAX_REASON_CODE_BYTES: Final = 128

_REASON_CODE_PATTERN = re.compile(r"^[a-z0-9][a-z0-9_.:-]*$")
_JSON_EDGE_WHITESPACE: Final = b" \t\r\n"
_VALIDATED_CARRIER_SEAL: Final = object()
_CURRENT_PROJECTION_FIELDS: Final = frozenset(
    {
        "actions",
        "audience",
        "audience_policy_digest",
        "blind_spots",
        "decoder_ref",
        "demand_shape",
        "depth",
        "derivations",
        "device_class",
        "events",
        "facts",
        "focus_ref",
        "generated_at",
        "impingements",
        "implications",
        "legal_next",
        "lifecycle_possibility",
        "lineage_refs",
        "loss",
        "mapping_manifest",
        "may_authorize",
        "meaning",
        "no_effect",
        "observations",
        "orientation",
        "orienting_signals",
        "portal_offers",
        "position",
        "producer_ref",
        "producer_verification_required",
        "prohibited_next",
        "projection_hash",
        "projection_ref",
        "purpose",
        "redacted_objects",
        "register",
        "relations",
        "resolution_coordinates",
        "schema",
        "scopes",
        "signal_constellations",
        "signal_estimates",
        "signal_learning_receipts",
        "signal_lenses",
        "source_admissions",
        "stale_after",
        "state",
        "supersedes_refs",
        "temporal_coordinates",
        "verification_scope",
    }
)


class ContextReadError(ValueError):
    """A bounded context admission failure safe to expose as a reason token."""

    def __init__(self, reason_code: str) -> None:
        self.reason_code = reason_code
        super().__init__(reason_code)


class _ValidatedCarrier:
    """Opaque token proving package, audience, and JSON admission already passed."""

    __slots__ = ("kind", "payload")

    def __init__(
        self,
        kind: Literal["projection", "compatibility"],
        payload: bytes,
        seal: object,
    ) -> None:
        if (
            seal is not _VALIDATED_CARRIER_SEAL
            or kind not in {"projection", "compatibility"}
            or not isinstance(payload, bytes)
        ):
            raise ContextReadError("context_read_error")
        self.kind = kind
        self.payload = payload


def _reason_codes(values: Iterable[str], *, allow_empty: bool = False) -> list[str]:
    try:
        codes = sorted(set(values))
    except (TypeError, ValueError) as exc:
        raise ContextReadError("context_read_error") from exc
    if (not allow_empty and not codes) or len(codes) > MAX_REASON_CODES:
        raise ContextReadError("context_read_error")
    for code in codes:
        try:
            encoded = code.encode("ascii", errors="strict")
        except (AttributeError, UnicodeError) as exc:
            raise ContextReadError("context_read_error") from exc
        if (
            len(encoded) > MAX_REASON_CODE_BYTES
            or _REASON_CODE_PATTERN.fullmatch(code) is None
        ):
            raise ContextReadError("context_read_error")
    return codes


def _ascii_json_bytes(value: object) -> bytes:
    try:
        return json.dumps(
            value,
            allow_nan=False,
            ensure_ascii=True,
            separators=(",", ":"),
        ).encode("ascii")
    except (TypeError, ValueError, UnicodeError) as exc:
        raise ContextReadError("context_read_error") from exc


def _render_readout_bytes(
    *,
    state: Literal["dark", "hold"],
    reason_codes: Iterable[str],
    carrier: _ValidatedCarrier | None,
) -> bytes:
    """Assemble once, budget once, and return those same immutable wire bytes."""

    if state not in {"dark", "hold"}:
        raise ContextReadError("context_read_error")
    if state == "dark" and carrier is not None:
        raise ContextReadError("context_read_error")
    if state == "hold":
        if type(carrier) is not _ValidatedCarrier:
            raise ContextReadError("context_read_error")
        active = carrier.payload
        if not active.startswith(b"{") or not active.endswith(b"}"):
            raise ContextReadError("context_read_error")

    projection = (
        carrier.payload
        if carrier is not None and carrier.kind == "projection"
        else b"null"
    )
    compatibility = (
        carrier.payload
        if carrier is not None and carrier.kind == "compatibility"
        else b"null"
    )
    body = b"".join(
        (
            b'{"schema":',
            _ascii_json_bytes(CONTEXT_READ_SCHEMA),
            b',"state":',
            _ascii_json_bytes(state),
            b',"audience":',
            _ascii_json_bytes(CONTEXT_READ_AUDIENCE),
            b',"reason_codes":',
            _ascii_json_bytes(_reason_codes(reason_codes)),
            b',"projection":',
            projection,
            b',"compatibility":',
            compatibility,
            b"}",
        )
    )
    if len(body) > MAX_CONTEXT_READ_BYTES:
        raise ContextReadError("producer_payload_too_large")
    return body


def dark_readout_bytes(*reason_codes: str) -> bytes:
    """Return an exact DARK carrier with no producer payload."""

    return _render_readout_bytes(
        state="dark",
        reason_codes=reason_codes or ("context_read_error",),
        carrier=None,
    )


def _unique_json_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    value: dict[str, object] = {}
    for key, item in pairs:
        if key in value:
            raise ContextReadError("producer_malformed")
        value[key] = item
    return value


def _reject_json_constant(_value: str) -> object:
    raise ContextReadError("producer_malformed")


def _decode_payload(payload: bytes) -> dict[str, Any]:
    if not isinstance(payload, bytes):
        raise ContextReadError("producer_malformed")
    if len(payload) > MAX_CONTEXT_READ_BYTES:
        raise ContextReadError("producer_payload_too_large")
    if payload != payload.strip(_JSON_EDGE_WHITESPACE):
        # Go's json.RawMessage intentionally excludes whitespace outside a value.
        raise ContextReadError("producer_malformed")
    try:
        text = payload.decode("utf-8", errors="strict")
        decoded = json.loads(
            text,
            object_pairs_hook=_unique_json_object,
            parse_constant=_reject_json_constant,
        )
    except ContextReadError:
        raise
    except (UnicodeError, json.JSONDecodeError, RecursionError, ValueError) as exc:
        raise ContextReadError("producer_malformed") from exc
    if not isinstance(decoded, dict):
        raise ContextReadError("producer_malformed")
    return decoded


def _canonical_models() -> tuple[type[Any], type[Any], Callable[[object], bytes]]:
    try:
        from hapax.context_canon import (
            ContextBundleCompatibilityProjection,
            ProjectionEnvelope,
            canonical_json_bytes,
        )
    except Exception as exc:
        raise ContextReadError("canonical_verifier_unavailable") from exc
    return (
        ProjectionEnvelope,
        ContextBundleCompatibilityProjection,
        canonical_json_bytes,
    )


def _validate_opaque(
    model_type: type[Any],
    payload: dict[str, Any],
    canonical_json_bytes: Callable[[object], bytes],
) -> None:
    try:
        model = model_type.model_validate(payload)
        normalized = model.model_dump(mode="json", by_alias=True)
        if canonical_json_bytes(normalized) != canonical_json_bytes(payload):
            raise ContextReadError("producer_malformed")
    except ContextReadError:
        raise
    except Exception as exc:
        raise ContextReadError("producer_malformed") from exc


def _validated_payload(payload: bytes) -> _ValidatedCarrier:
    decoded = _decode_payload(payload)
    schema = decoded.get("schema")
    if schema not in {PROJECTION_SCHEMA, COMPATIBILITY_SCHEMA}:
        raise ContextReadError("producer_schema_unsupported")

    if schema == PROJECTION_SCHEMA:
        _require_current_projection_shape(decoded)
        projection_type, _, canonical_json_bytes = _canonical_models()
        _validate_opaque(projection_type, decoded, canonical_json_bytes)
        kind: Literal["projection", "compatibility"] = "projection"
    else:
        _, compatibility_type, canonical_json_bytes = _canonical_models()
        _validate_opaque(compatibility_type, decoded, canonical_json_bytes)
        kind = "compatibility"
    if decoded.get("audience") != CONTEXT_READ_AUDIENCE:
        raise ContextReadError("audience_unsupported")
    return _ValidatedCarrier(kind, payload, _VALIDATED_CARRIER_SEAL)


def _require_current_projection_shape(payload: dict[str, Any]) -> None:
    if frozenset(payload) != _CURRENT_PROJECTION_FIELDS:
        raise ContextReadError("producer_malformed")
    events = payload.get("events")
    actions = payload.get("actions")
    demand = payload.get("demand_shape")
    descriptor = demand.get("descriptor") if isinstance(demand, dict) else None
    if (
        not isinstance(events, list)
        or not events
        or not isinstance(actions, list)
        or any(
            not isinstance(action, dict)
            or not isinstance(action.get("operation"), str)
            or not action["operation"]
            for action in actions
        )
        or not isinstance(descriptor, dict)
        or not isinstance(descriptor.get("canon"), dict)
        or not isinstance(descriptor.get("position_basis"), dict)
    ):
        raise ContextReadError("producer_malformed")


def _parse_context_payload_bytes(payload: bytes) -> bytes:
    carrier = _validated_payload(payload)
    if carrier.kind == "projection":
        return _render_readout_bytes(
            state="hold",
            reason_codes=(
                "canonical_projection_verification_unavailable",
                "producer_receipt_missing",
            ),
            carrier=carrier,
        )
    return _render_readout_bytes(
        state="hold",
        reason_codes=("compatibility_only",),
        carrier=carrier,
    )


def parse_context_payload_bytes(payload: bytes) -> bytes:
    """Admit producer bytes into an exact DARK/HOLD wire carrier.

    This function never raises producer details through the public boundary and
    has no PRESENT path. Full projection verification still requires the source
    frame and a governed producer receipt, neither of which Reins possesses.
    """

    try:
        return _parse_context_payload_bytes(payload)
    except ContextReadError as exc:
        return dark_readout_bytes(exc.reason_code)
    except Exception:
        return dark_readout_bytes("context_read_error")


__all__ = [
    "COMPATIBILITY_SCHEMA",
    "CONTEXT_READ_AUDIENCE",
    "CONTEXT_READ_SCHEMA",
    "ContextReadError",
    "MAX_CONTEXT_READ_BYTES",
    "MAX_REASON_CODE_BYTES",
    "MAX_REASON_CODES",
    "PROJECTION_SCHEMA",
    "dark_readout_bytes",
    "parse_context_payload_bytes",
]
