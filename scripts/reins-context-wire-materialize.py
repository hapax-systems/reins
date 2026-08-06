#!/usr/bin/env python3
"""Materialize exact FastAPI context carrier bytes for the opt-in Go proof."""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
import sys
from contextlib import nullcontext
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from unittest.mock import patch

REPO_ROOT = Path(__file__).resolve().parents[1]
API_ROOT = REPO_ROOT / "api"
if str(API_ROOT) not in sys.path:
    sys.path.insert(0, str(API_ROOT))

import reins_context  # noqa: E402
from fastapi.testclient import TestClient  # noqa: E402
from reins_read import build_app  # noqa: E402

SCHEMA = "hapax.reins-context-wire-conformance.v2"
# The pinned current fixture sha is updated atomically with the fixture file
# itself (amendment reins-strict-current-carrier-atomic-scope-20260712: one
# atomic transition, no predecessor fallback). 594b6b96… is the Jul-13
# generation, retained below only as a rejected predecessor value.
FIXTURE_SHA256 = "34e1c2874f113c4bda72ff69a57a41409b6aa3ed805630d3c333092cd4d851f4"
PREDECESSOR_FIXTURE_SHA256 = (
    "c16fce720b4bfb80233b0a3b94a9d5903796c646261651788a9084bfc0e97704"
)
REJECTED_FIXTURE_SHA256 = {
    PREDECESSOR_FIXTURE_SHA256,
    "594b6b96656cea2a46e4d50c2201152523bcf4530f0afcb0360425e76c17fae9",
}
PREDECESSOR_WHEEL_SHA256 = (
    "2c2202acad9050977d9f773c952c2d92c44ac0c0fb626a5395b041eb128cfe08"
)
# Exact-current artifact identity (Gate 0A lockstep proposal
# kimi/gate0a-exact-current-lockstep): the wheel is reproducible — build
# packages/hapax-context-canon at council repo SHA
# cc2bf944ad830dbb7c1bf2ed7b80eee04a94cef8 with SOURCE_DATE_EPOCH=1754500000
# (`uv build --wheel`) and this digest must result, on any host. Any other
# digest refuses. Package *publication* remains a separately held act; this
# pins the artifact's content identity, not a registry.
EXACT_CURRENT_WHEEL_SHA256 = (
    "882c5fc1a154d6340b5ce9ec9cf9698637e2079d6e8c14032f1fbd7907746277"
)
# The producer fixtures the regenerator consumes are pinned by content, not by
# path — a stale-but-schema-valid producer image must refuse at regeneration,
# not at the boundary. These are the council fixtures at cc2bf944.
PRODUCER_FIXTURE_SHA256 = {
    "gate0-projections.json": (
        "fb0b41086158cbc3bc3c21dc4eb91ff38d8322a4f0776c801e8086213a0a43ca"
    ),
    "gate0-purpose-projections.json": (
        "0bbe63eff12d8e01e8127cc93dc44821bd0d695c13aed388b7594565d84d4106"
    ),
    "gate0-compatibility.json": (
        "128d7397c2512edae7703f3392a9a6f2a9bdbc43406fbdc9b110251921c69bf6"
    ),
}
EXACT_CURRENT_PACKAGE_STATE = "exact_current"
PACKAGE_STATE = "hold_missing_exact_current_artifact"
OUTER_KEYS = {
    "schema",
    "state",
    "audience",
    "reason_codes",
    "projection",
    "compatibility",
}


@dataclass(frozen=True)
class Case:
    name: str
    producer: bytes | None
    state: str
    reasons: tuple[str, ...]
    active_field: str
    query: str = ""
    validation_mode: str = "test_only_current_fixture_contract"
    patch_models: bool = True


class _PermissiveBoundaryModel:
    """Test-only validator used solely to exercise the outer byte boundary."""

    def __init__(self, payload: dict[str, Any]) -> None:
        self.payload = payload

    @classmethod
    def model_validate(cls, payload: dict[str, Any]) -> _PermissiveBoundaryModel:
        return cls(payload)

    def model_dump(self, *, mode: str, by_alias: bool) -> dict[str, Any]:
        if mode != "json" or by_alias is not True:
            raise AssertionError("unexpected boundary-model dump mode")
        return self.payload


_CURRENT_PROJECTIONS: set[bytes] = set()
_CURRENT_COMPATIBILITY = b""


class _PinnedCurrentProjectionModel(_PermissiveBoundaryModel):
    @classmethod
    def model_validate(cls, payload: dict[str, Any]) -> _PinnedCurrentProjectionModel:
        if _compact(payload) not in _CURRENT_PROJECTIONS:
            raise ValueError("projection is not the pinned current fixture")
        return cls(payload)


class _PinnedCurrentCompatibilityModel(_PermissiveBoundaryModel):
    @classmethod
    def model_validate(
        cls, payload: dict[str, Any]
    ) -> _PinnedCurrentCompatibilityModel:
        if _compact(payload) != _CURRENT_COMPATIBILITY:
            raise ValueError("compatibility carrier is not the pinned fixture")
        return cls(payload)


def _sha256(payload: bytes) -> str:
    return hashlib.sha256(payload).hexdigest()


def _compact(value: object) -> bytes:
    return json.dumps(
        value,
        allow_nan=False,
        ensure_ascii=True,
        separators=(",", ":"),
        sort_keys=True,
    ).encode("ascii")


def _reverse_objects(value: object) -> object:
    if isinstance(value, dict):
        return {
            key: _reverse_objects(item) for key, item in reversed(tuple(value.items()))
        }
    if isinstance(value, list):
        return [_reverse_objects(item) for item in value]
    return value


def _read_pinned(path: Path, expected: str, label: str) -> bytes:
    payload = path.read_bytes()
    observed = _sha256(payload)
    if observed != expected:
        raise SystemExit(f"{label} SHA-256 mismatch: {observed} != {expected}")
    return payload


def _write_private(path: Path, payload: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(payload)
    except BaseException:
        path.unlink(missing_ok=True)
        raise


def _boundary_producers() -> tuple[bytes, bytes]:
    payload = {
        "schema": reins_context.PROJECTION_SCHEMA,
        "audience": reins_context.CONTEXT_READ_AUDIENCE,
        "padding": "",
    }
    models = (_PermissiveBoundaryModel, _PermissiveBoundaryModel, _compact)
    with (
        patch.object(reins_context, "_canonical_models", return_value=models),
        patch.object(reins_context, "_require_current_projection_shape"),
    ):
        base_body = reins_context.parse_context_payload_bytes(_compact(payload))
    padding_bytes = reins_context.MAX_CONTEXT_READ_BYTES - len(base_body)
    if padding_bytes <= 0:
        raise AssertionError("test-only boundary payload has no available padding")
    payload["padding"] = "x" * padding_bytes
    at_limit = _compact(payload)
    payload["padding"] += "x"
    over_limit = _compact(payload)
    return at_limit, over_limit


def _exact_cases(fixtures: dict[str, Any]) -> list[Case]:
    operator = fixtures["operator_projection"]
    reordered_operator = _reverse_objects(operator)
    if list(reordered_operator) != list(reversed(tuple(operator))):
        raise AssertionError("operator key-order canary was not reversed")
    pretty_operator = json.dumps(
        reordered_operator,
        allow_nan=False,
        ensure_ascii=True,
        indent=2,
    ).encode("ascii")
    pretty_operator = pretty_operator.replace(
        b'"audience": "operator_private"',
        b'"audience": "oper\\u0061tor_private"',
        1,
    )
    bool_integer = _compact(operator).replace(
        b'"may_authorize":false', b'"may_authorize":0'
    )
    at_limit, over_limit = _boundary_producers()
    return [
        Case(
            "operator_escaped_pretty_hold",
            pretty_operator,
            "hold",
            (
                "canonical_projection_verification_unavailable",
                "producer_receipt_missing",
            ),
            "projection",
        ),
        Case(
            "lifecycle_possibility_hold",
            _compact(fixtures["lifecycle_projection"]),
            "hold",
            (
                "canonical_projection_verification_unavailable",
                "producer_receipt_missing",
            ),
            "projection",
        ),
        Case(
            "operation_hold",
            _compact(fixtures["operation_projection"]),
            "hold",
            (
                "canonical_projection_verification_unavailable",
                "producer_receipt_missing",
            ),
            "projection",
        ),
        Case(
            "compatibility_hold",
            _compact(fixtures["compatibility"]),
            "hold",
            ("compatibility_only",),
            "compatibility",
        ),
        Case(
            "yard_dark",
            _compact(fixtures["yard_projection"]),
            "dark",
            ("audience_unsupported",),
            "",
        ),
        Case(
            "query_dark",
            _compact(operator),
            "dark",
            ("audience_unsupported",),
            "",
            query="audience=operator_private",
        ),
        Case("absent_dark", None, "dark", ("producer_absent",), ""),
        Case("malformed_dark", b"{not-json", "dark", ("producer_malformed",), ""),
        Case(
            "duplicate_key_dark",
            b'{"schema":"one","\\u0073chema":"two"}',
            "dark",
            ("producer_malformed",),
            "",
        ),
        Case("invalid_utf8_dark", b"\xff", "dark", ("producer_malformed",), ""),
        Case(
            "edge_whitespace_dark",
            b" " + _compact(operator),
            "dark",
            ("producer_malformed",),
            "",
        ),
        Case(
            "unsupported_schema_dark",
            b'{"schema":"unsupported"}',
            "dark",
            ("producer_schema_unsupported",),
            "",
        ),
        Case(
            "bool_integer_dark",
            bool_integer,
            "dark",
            ("producer_malformed",),
            "",
        ),
        Case(
            "oversized_producer_dark",
            b" " * (reins_context.MAX_CONTEXT_READ_BYTES + 1),
            "dark",
            ("producer_payload_too_large",),
            "",
        ),
        Case(
            "outer_exact_limit_hold",
            at_limit,
            "hold",
            (
                "canonical_projection_verification_unavailable",
                "producer_receipt_missing",
            ),
            "projection",
            validation_mode="test_only_boundary_stub",
            patch_models=True,
        ),
        Case(
            "outer_over_limit_dark",
            over_limit,
            "dark",
            ("producer_payload_too_large",),
            "",
            validation_mode="test_only_boundary_stub",
            patch_models=True,
        ),
    ]


def _validate_response(case: Case, response: Any) -> dict[str, Any]:
    if response.status_code != 200:
        raise AssertionError(f"{case.name}: HTTP {response.status_code}")
    body = response.content
    if len(body) > reins_context.MAX_CONTEXT_READ_BYTES:
        raise AssertionError(f"{case.name}: Python emitted an oversized body")
    if response.headers.get("content-type") != "application/json":
        raise AssertionError(f"{case.name}: wrong content type")
    if response.headers.get("content-length") != str(len(body)):
        raise AssertionError(f"{case.name}: wrong content length")
    if response.headers.get("cache-control") != "no-store":
        raise AssertionError(f"{case.name}: missing no-store")
    if response.headers.get("x-content-type-options") != "nosniff":
        raise AssertionError(f"{case.name}: missing nosniff")
    if "content-encoding" in response.headers:
        raise AssertionError(f"{case.name}: unexpected content encoding")

    decoded = json.loads(body)
    if set(decoded) != OUTER_KEYS or len(decoded) != len(OUTER_KEYS):
        raise AssertionError(f"{case.name}: outer key matrix changed")
    expected = {
        "schema": reins_context.CONTEXT_READ_SCHEMA,
        "state": case.state,
        "audience": reins_context.CONTEXT_READ_AUDIENCE,
        "reason_codes": list(case.reasons),
    }
    for key, value in expected.items():
        if decoded.get(key) != value:
            raise AssertionError(f"{case.name}: {key} mismatch")
    if case.active_field == "projection":
        if decoded["compatibility"] is not None or case.producer is None:
            raise AssertionError(f"{case.name}: projection matrix mismatch")
        if b'"projection":' + case.producer + b',"compatibility":null}' not in body:
            raise AssertionError(
                f"{case.name}: projection bytes were not retained exactly"
            )
    elif case.active_field == "compatibility":
        if decoded["projection"] is not None or case.producer is None:
            raise AssertionError(f"{case.name}: compatibility matrix mismatch")
        if not body.endswith(b'"compatibility":' + case.producer + b"}"):
            raise AssertionError(
                f"{case.name}: compatibility bytes were not retained exactly"
            )
    elif decoded["projection"] is not None or decoded["compatibility"] is not None:
        raise AssertionError(f"{case.name}: DARK retained a producer payload")
    return {"body": body, "decoded": decoded}


def _materialize_case(output: Path, case: Case) -> dict[str, Any]:
    cases_root = output / "cases"
    producer_path: Path | None = None
    if case.producer is not None:
        producer_path = cases_root / f"{case.name}.producer.json"
        _write_private(producer_path, case.producer)
        os.environ["REINS_CONTEXT_BUNDLE"] = str(producer_path)
    else:
        os.environ["REINS_CONTEXT_BUNDLE"] = str(cases_root / f"{case.name}.absent")

    boundary_stub = case.validation_mode == "test_only_boundary_stub"
    model_type = (
        _PermissiveBoundaryModel if boundary_stub else _PinnedCurrentProjectionModel
    )
    compatibility_type = (
        _PermissiveBoundaryModel if boundary_stub else _PinnedCurrentCompatibilityModel
    )
    models = (model_type, compatibility_type, _compact)
    model_patch = (
        patch.object(reins_context, "_canonical_models", return_value=models)
        if case.patch_models
        else nullcontext()
    )
    shape_patch = (
        patch.object(reins_context, "_require_current_projection_shape")
        if boundary_stub
        else nullcontext()
    )
    target = "/read/context" + (f"?{case.query}" if case.query else "")
    with model_patch, shape_patch:
        with TestClient(build_app("", [])) as client:
            response = client.get(target)
    validated = _validate_response(case, response)
    body = validated["body"]
    body_path = cases_root / f"{case.name}.body.json"
    _write_private(body_path, body)

    entry: dict[str, Any] = {
        "name": case.name,
        "body_path": body_path.relative_to(output).as_posix(),
        "body_sha256": _sha256(body),
        "body_bytes": len(body),
        "producer_path": None,
        "producer_sha256": None,
        "producer_bytes": 0,
        "expected_state": case.state,
        "expected_reason_codes": list(case.reasons),
        "active_field": case.active_field,
        "query": case.query,
        "validation_mode": case.validation_mode,
    }
    if producer_path is not None and case.producer is not None:
        entry.update(
            producer_path=producer_path.relative_to(output).as_posix(),
            producer_sha256=_sha256(case.producer),
            producer_bytes=len(case.producer),
        )
    return entry


def _write_manifest(path: Path, manifest: dict[str, Any], *, replace: bool) -> None:
    payload = json.dumps(manifest, indent=2, sort_keys=True).encode("utf-8") + b"\n"
    if not replace:
        _write_private(path, payload)
        return
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.unlink(missing_ok=True)
    _write_private(temporary, payload)
    os.replace(temporary, path)
    os.chmod(path, 0o600)


def _source_fixture_mode(args: argparse.Namespace, fixtures: dict[str, Any]) -> None:
    global _CURRENT_COMPATIBILITY
    _CURRENT_PROJECTIONS.clear()
    _CURRENT_PROJECTIONS.update(
        _compact(fixtures[name])
        for name in (
            "operator_projection",
            "yard_projection",
            "lifecycle_projection",
            "operation_projection",
        )
    )
    _CURRENT_COMPATIBILITY = _compact(fixtures["compatibility"])
    if args.output.exists() and any(args.output.iterdir()):
        raise SystemExit(f"output directory is not empty: {args.output}")
    args.output.mkdir(parents=True, exist_ok=True, mode=0o700)
    cases = [_materialize_case(args.output, case) for case in _exact_cases(fixtures)]
    manifest = {
        "schema": SCHEMA,
        "fixture_sha256": FIXTURE_SHA256,
        "wheel_sha256": None,
        "package_state": PACKAGE_STATE,
        "max_context_read_bytes": reins_context.MAX_CONTEXT_READ_BYTES,
        "cases": cases,
    }
    _write_manifest(args.output / "manifest.json", manifest, replace=False)


def _exact_wheel_mode(args: argparse.Namespace) -> None:
    if args.wheel is None:
        raise SystemExit("--wheel is required in exact-wheel mode")
    wheel = args.wheel.resolve()
    observed = _sha256(wheel.read_bytes())
    if observed == PREDECESSOR_WHEEL_SHA256:
        raise SystemExit(
            "predecessor context-canon wheel is rejected by the strict current boundary"
        )
    if observed != EXACT_CURRENT_WHEEL_SHA256:
        raise SystemExit(
            "context-canon wheel SHA-256 mismatch: "
            f"{observed} != {EXACT_CURRENT_WHEEL_SHA256} "
            "(rebuild at the pinned council SHA with the pinned SOURCE_DATE_EPOCH)"
        )
    try:
        import hapax.context_canon as _canon_package
    except Exception as exc:
        raise SystemExit(
            "exact-wheel mode requires the pinned current hapax.context_canon "
            "wheel importable in the ordinary environment"
        ) from exc
    module_path = Path(_canon_package.__file__).resolve()
    for forbidden_root in (REPO_ROOT.resolve(), Path.cwd().resolve()):
        if module_path.is_relative_to(forbidden_root):
            raise SystemExit(
                "hapax.context_canon must import from an installed environment, "
                f"not the working tree (resolved to {module_path})"
            )
    fixtures = json.loads(args.fixture.read_bytes())
    if args.output.exists() and any(args.output.iterdir()):
        raise SystemExit(f"output directory is not empty: {args.output}")
    if args.output.exists() and not args.output.is_dir():
        raise SystemExit(
            f"--output must be a directory in exact-wheel mode: {args.output}"
        )
    args.output.mkdir(parents=True, exist_ok=True, mode=0o700)
    if (args.output.stat().st_mode & 0o777) != 0o700:
        raise SystemExit(f"output directory mode must be exactly 0700: {args.output}")
    # Real wheel models, not the pinned test doubles: with patch_models=False
    # the consumer's _canonical_models() resolves to the installed wheel, so
    # the battery below validates the current carriers through the actual
    # current contract (canonical round-trip included).
    cases = [
        Case(
            case.name,
            case.producer,
            case.state,
            case.reasons,
            case.active_field,
            query=case.query,
            validation_mode=(
                "test_only_boundary_stub"
                if case.validation_mode == "test_only_boundary_stub"
                else "exact_current_wheel_contract"
            ),
            patch_models=case.validation_mode == "test_only_boundary_stub",
        )
        for case in _exact_cases(fixtures)
    ]
    manifest = {
        "schema": SCHEMA,
        "fixture_sha256": FIXTURE_SHA256,
        "wheel_sha256": observed,
        "package_state": EXACT_CURRENT_PACKAGE_STATE,
        "omissions": {
            "no_wheel_dark": "not_applicable_package_present",
        },
        "max_context_read_bytes": reins_context.MAX_CONTEXT_READ_BYTES,
        "cases": [_materialize_case(args.output, case) for case in cases],
    }
    _write_manifest(args.output / "manifest.json", manifest, replace=False)


def _regenerate_fixture_mode(args: argparse.Namespace) -> None:
    """Regenerate the consumer carrier fixture from the producer's current
    canonical fixtures, validated through the installed current wheel.

    This is the regenerator the atomic transition pins: every carrier is
    model-validated and canonical-round-tripped under the real contract, so a
    stale or drifted producer image refuses here instead of at the boundary.
    """
    try:
        from hapax.context_canon import (
            ContextBundleCompatibilityProjection,
            ProjectionEnvelope,
            canonical_json_bytes,
        )
    except Exception as exc:
        raise SystemExit(
            "regenerate-fixture requires the pinned current hapax.context_canon "
            "wheel importable in the ordinary environment"
        ) from exc
    producer = args.producer_fixtures.resolve()
    producer_bytes = {}
    for fixture_name, expected in PRODUCER_FIXTURE_SHA256.items():
        raw = (producer / fixture_name).read_bytes()
        observed = _sha256(raw)
        if observed != expected:
            raise SystemExit(
                f"producer fixture {fixture_name} SHA-256 mismatch: "
                f"{observed} != {expected} — a stale or drifted producer "
                "image refuses at regeneration, not at the boundary"
            )
        producer_bytes[fixture_name] = raw
    projections = json.loads(producer_bytes["gate0-projections.json"])
    purposes = json.loads(producer_bytes["gate0-purpose-projections.json"])
    compatibility = json.loads(producer_bytes["gate0-compatibility.json"])
    carriers = {
        "operator_projection": projections["operator"],
        "yard_projection": projections["yard"],
        "lifecycle_projection": purposes["lifecycle_possibility"],
        "operation_projection": purposes["operation"],
        "compatibility": compatibility,
    }
    for name, carrier in carriers.items():
        model_type = (
            ContextBundleCompatibilityProjection
            if name == "compatibility"
            else ProjectionEnvelope
        )
        try:
            model = model_type.model_validate(carrier)
        except Exception as exc:
            raise SystemExit(f"{name}: refuses under the current contract: {exc}")
        normalized = model.model_dump(mode="json", by_alias=True)
        if canonical_json_bytes(normalized) != canonical_json_bytes(carrier):
            raise SystemExit(
                f"{name}: canonical round-trip mismatch under the current contract"
            )
    payload = _compact(carriers) + b"\n"
    observed_payload = _sha256(payload)
    if observed_payload in REJECTED_FIXTURE_SHA256:
        raise SystemExit(
            "the regenerator produced a rejected predecessor generation — refusing"
        )
    if observed_payload != FIXTURE_SHA256:
        raise SystemExit(
            "the regenerator produced bytes that do not match the pinned "
            f"FIXTURE_SHA256: {observed_payload} != {FIXTURE_SHA256} — a new "
            "generation is a deliberate pin act, not a silent regeneration"
        )
    if args.output.exists():
        raise SystemExit(f"refusing to overwrite existing fixture: {args.output}")
    if args.output.is_dir():
        raise SystemExit(
            f"--output must be a file in regenerate-fixture mode: {args.output}"
        )
    _write_private(args.output, payload)
    print(observed_payload)


def _no_wheel_mode(args: argparse.Namespace, fixtures: dict[str, Any]) -> None:
    manifest_path = args.output / "manifest.json"
    manifest = json.loads(manifest_path.read_bytes())
    if manifest.get("schema") != SCHEMA:
        raise SystemExit("cannot append to an unknown conformance manifest")
    try:
        package_spec = importlib.util.find_spec("hapax.context_canon")
    except ModuleNotFoundError:
        package_spec = None
    if package_spec is not None:
        raise SystemExit(
            "no-wheel mode found hapax.context_canon in the ordinary environment"
        )
    case = Case(
        "no_wheel_dark",
        _compact(fixtures["operator_projection"]),
        "dark",
        ("canonical_verifier_unavailable",),
        "",
        validation_mode="ordinary_environment_no_wheel",
        patch_models=False,
    )
    manifest["cases"].append(_materialize_case(args.output, case))
    _write_manifest(manifest_path, manifest, replace=True)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--mode",
        choices=("source-fixture", "exact-wheel", "no-wheel", "regenerate-fixture"),
        required=True,
    )
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--fixture", type=Path)
    parser.add_argument("--wheel", type=Path)
    parser.add_argument("--producer-fixtures", type=Path)
    args = parser.parse_args()

    if args.mode == "regenerate-fixture":
        if args.producer_fixtures is None:
            raise SystemExit(
                "--producer-fixtures is required in regenerate-fixture mode"
            )
        _regenerate_fixture_mode(args)
        return 0

    if args.fixture is None:
        raise SystemExit("--fixture is required in this mode")
    fixture_path = args.fixture.resolve()
    fixture_bytes = fixture_path.read_bytes()
    observed_fixture = _sha256(fixture_bytes)
    if observed_fixture in REJECTED_FIXTURE_SHA256:
        raise SystemExit(
            "predecessor context fixture is rejected by the strict current boundary"
        )
    if observed_fixture != FIXTURE_SHA256:
        raise SystemExit(
            f"fixture SHA-256 mismatch: {observed_fixture} != {FIXTURE_SHA256}"
        )
    fixtures = json.loads(fixture_bytes)
    if args.mode == "source-fixture":
        _source_fixture_mode(args, fixtures)
    elif args.mode == "exact-wheel":
        _exact_wheel_mode(args)
    else:
        _no_wheel_mode(args, fixtures)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
