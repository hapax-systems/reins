"""Tests for /read/obligations ligatory and capability-fulfillment synthesis."""

from __future__ import annotations

import pytest

from reins_obligations import REDACTION_TOKEN, _classified, read_obligations_summary
from reins_read import classify_air


def test_deny_all_classifier_and_secret_scrubbing() -> None:
    """Residual risk #3, R2-F01 & R3-F01: both custom/selective AND default classify_air redact denied opaque fields, and secret/canary markers never survive."""
    # Direct R3-F01 check on _classified with default classify_air
    direct = _classified(
        {"kind": "x", "debtor": "v7q2z9-private-fixture"},
        ["kind"],
        classify_air,
    )
    assert direct["kind"] == "x"
    assert direct["air"]["kind"] == "ok"
    assert direct["debtor"] == REDACTION_TOKEN
    assert direct["air"]["debtor"] == "deny"

    res = read_obligations_summary(
        allowlist=["state"],
        classify=lambda fields, _allow: {
            k: ("ok" if k == "state" else "deny") for k in fields
        },
        lanebus={
            "inboxes": [
                {
                    "lane": "lane-secret_marker_123",
                    "unacked_count": 1,
                    "state": "unacked",
                }
            ]
        },
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={
            "routes": [
                {
                    "route_id": "r.local",
                    "capability_id": "cap",
                    "mode": "local",
                    "model_id": "vendor-a/m1",
                    "route_state": "ok",
                }
            ]
        },
        context_bundle={
            "generated_at": "2026-10-20T12:10:00Z",
            "facts": [
                {
                    "id": "locus:h1:8000",
                    "kind": "locus",
                    "state": "live",
                    "observed_at": "2026-10-20T12:09:50Z",
                    "payload": {
                        "host": "h1",
                        "port": "8000",
                        "loaded_models": ["vendor-a/m1"],
                        "resident_models": ["vendor-a/m1"],
                        "admitted_by": ["r.local"],
                    },
                }
            ],
        },
        now="2026-10-20T12:10:05Z",
    )
    row = res["lanebus_owes"][0]
    assert row["debtor"] == REDACTION_TOKEN
    assert row["subject"] == REDACTION_TOKEN
    assert row["state"] == "unacked"
    locus_row = res["local_substrate_loci"][0]
    assert locus_row["host"] == REDACTION_TOKEN
    assert locus_row["loaded_models"] == [REDACTION_TOKEN]
    cap_row = res["capability_fulfillment"][0]
    assert cap_row["occupied_loci"] == [REDACTION_TOKEN]
    assert cap_row["model_id"] == REDACTION_TOKEN
    assert "secret_marker" not in str(res)

    # Also verify default classify_air in read_obligations_summary redacts arbitrary non-allowlisted opaque strings
    default_air_res = read_obligations_summary(
        allowlist=["kind", "state", "action_count"],
        classify=classify_air,
        lanebus={
            "inboxes": [
                {
                    "lane": "v7q2z9-private-fixture",
                    "unacked_count": 2,
                    "state": "unacked",
                    "attention": "action",
                }
            ]
        },
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={"routes": []},
        context_bundle={"facts": []},
    )
    assert default_air_res["lanebus_owes"][0]["debtor"] == REDACTION_TOKEN
    assert default_air_res["lanebus_owes"][0]["subject"] == REDACTION_TOKEN
    assert default_air_res["summary"]["action_count"] == 1


def _locus_summary(freshness="fresh", loaded=True):
    ts = "2026-10-20T12:10:00Z"
    return read_obligations_summary(
        allowlist=["state", "loaded_models", "local_loci_occupied_in_memory"],
        classify=classify_air,
        lanebus={},
        grants={},
        claim_custody={},
        seat_succession={},
        capabilities={"routes": [{"route_id": "r.local", "model_id": "m1"}]},
        context_bundle={
            "generated_at": ts,
            "facts": [
                {
                    "id": "locus:fixture:8000",
                    "kind": "locus",
                    "state": "live",
                    "freshness_state": freshness,
                    "observed_at": ts,
                    "payload": {
                        "loaded_models": ["m1"] if loaded else [],
                        "resident_models": ["m1"],
                    },
                }
            ],
        },
        now=ts,
    )


@pytest.mark.parametrize(
    "loaded,field", [(True, "occupied_loci"), (False, "resident_loci")]
)
def test_default_air_denies_locus_identifiers(loaded, field):
    result = _locus_summary(loaded=loaded)
    row = result["capability_fulfillment"][0]
    assert row[field] == [REDACTION_TOKEN]
    assert row["air"][field] == "deny"


@pytest.mark.parametrize("value", [7, 1.5, True])
def test_denied_scalars_and_nested_values(value):
    fields = {"action_count": value, "nested": {"value": value}}
    result = _classified(fields, ["state"], classify_air)
    assert result == {
        "action_count": None,
        "nested": {"value": None},
        "air": {"action_count": "deny", "nested": "deny"},
    }


@pytest.mark.parametrize(
    "freshness", ["fresh", "dark", "unknown", "stale", "aging", "absent"]
)
def test_only_fresh_loci_are_occupied(freshness):
    result = _locus_summary(freshness)
    live = freshness == "fresh"
    assert result["summary"]["local_loci_occupied_in_memory"] == int(live)
    row = result["local_substrate_loci"][0]
    assert row["loaded_models"] == (["m1"] if live else [])
    assert (row["state"] == "live") == live


# Explicit permissions for synthetic outcome fixtures; the O1 denial tests above
# keep their narrow allowlists. No production policy is widened by these tests.
_SYNTHESIS_FIELDS = [
    "kind",
    "relation",
    "creditor",
    "debtor",
    "state",
    "attention",
    "count",
    "age_s",
    "id",
    "route_id",
    "model_id",
    "route_state",
    "fulfillment_state",
    "occupied_loci",
    "resident_loci",
    "topology",
    "loaded_models",
    "resident_models_count",
    "total_owes",
    "lanebus_owes_count",
    "grant_owes_count",
    "custody_owes_count",
    "locus_anomaly_count",
    "dark_sources_count",
    "action_count",
    "warning_count",
    "routes_total",
    "routes_occupied_in_memory",
    "local_loci_occupied_in_memory",
    "local_loci_total",
]
_NOW = "2026-10-20T12:10:00Z"


def _synthesis(*, allowlist=(), **inputs):
    facets = dict(
        lanebus={"inboxes": []},
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={"routes": []},
        context_bundle={"facts": []},
        now=_NOW,
    )
    facets.update(inputs)
    return read_obligations_summary(
        allowlist=[*allowlist, *_SYNTHESIS_FIELDS], classify=classify_air, **facets
    )


def _fact(
    kind="locus",
    state="live",
    *,
    observed=_NOW,
    freshness="fresh",
    reasons=(),
    **payload,
):
    value = {
        "host": "fixture",
        "port": "8000",
        "loaded_models": ["m"],
        "resident_models": ["m"],
        "admitted_by": ["r.local"],
    }
    value.update(payload)
    return {
        "id": "locus:fixture:8000",
        "kind": kind,
        "state": {"value_state": state, "reason_codes": list(reasons)},
        "observed_at": observed,
        "freshness_state": freshness,
        "payload": value,
    }


def _with_facts(facts, *, generated=_NOW, routes=None):
    return _synthesis(
        context_bundle={"generated_at": generated, "facts": facts},
        capabilities={
            "routes": routes
            if routes is not None
            else [
                {
                    "route_id": "r.local",
                    "model_id": "m",
                    "mode": "local",
                    "route_state": "ok",
                }
            ]
        },
    )


@pytest.mark.parametrize(
    "state,honors,retained,expected",
    [
        ("reconciled", 1, True, None),
        ("honored", 1, True, None),
        ("ok", 0, False, None),
        ("", 0, False, None),
        ("unmatched_receipt", 1, True, "unmatched_receipt"),
        ("ok", "invalid", True, "unparseable_honor_count"),
        ("honored", True, True, "unparseable_honor_count"),
        ("reconciled", 0, True, "retained_unhonored"),
        ("", 0, True, "retained_unhonored"),
    ],
)
def test_grant_reconciliation_outcomes(state, honors, retained, expected):
    result = _synthesis(
        grants={
            "rows": [
                {
                    "state": state,
                    "honor_count": honors,
                    "grant_record_present": retained,
                    "grant_id": "g",
                    "minting_lane": "lane",
                }
            ]
        }
    )
    rows = result["grant_owes"]
    assert result["summary"]["grant_owes_count"] == (0 if expected is None else 1)
    assert [
        (r["kind"], r["state"], r["debtor"], r["creditor"], r["relation"]) for r in rows
    ] == (
        []
        if expected is None
        else [("grant." + expected, expected, "lane", "admission_authority", "owes")]
    )
    if rows:
        assert rows[0]["attention"] == (
            "info" if expected == "retained_unhonored" else "warning"
        )


@pytest.mark.parametrize(
    "state", ["stale", "orphaned", "blocked", "conflict", "settled", "active"]
)
def test_custody_outcomes(state):
    result = _synthesis(
        claim_custody={
            "rows": [{"custody_state": state, "holder": "lane", "task_id": "t"}]
        }
    )
    owing = state in {"stale", "orphaned", "blocked", "conflict"}
    assert [
        (r["kind"], r["state"], r["relation"], r["creditor"], r["debtor"])
        for r in result["custody_owes"]
    ] == (
        [("claim_custody." + state, state, "owes", "task_steward", "lane")]
        if owing
        else []
    )
    assert result["summary"]["custody_owes_count"] == int(owing)


@pytest.mark.parametrize(
    "pane,claim,owes",
    [
        ("absent", True, True),
        ("present", True, False),
        ("absent", False, False),
        ("absent", "true", False),
    ],
)
def test_claimed_without_pane_outcome(pane, claim, owes):
    result = _synthesis(
        seat_succession={
            "rows": [{"role": "lane", "pane_state": pane, "claims": {"present": claim}}]
        }
    )
    assert [
        (r["kind"], r["state"], r["attention"], r["creditor"])
        for r in result["custody_owes"]
    ] == (
        [("seat_succession.claimed_without_pane", "absent", "action", "seat_steward")]
        if owes
        else []
    )
    assert result["summary"]["action_count"] == int(owes)
    assert result["summary"]["custody_owes_count"] == int(owes)


@pytest.mark.parametrize(
    "source",
    [
        "lanebus",
        "grants",
        "claim_custody",
        "seat_succession",
        "capabilities",
        "context_bundle",
    ],
)
@pytest.mark.parametrize(
    "dark",
    [
        {"dark": True},
        {"bundle_state": "dark"},
        {"evaluation": {"bundle_state": "dark"}},
        {},
    ],
)
def test_partial_source_dark_aggregation(source, dark):
    result = _synthesis(**{source: dark})
    assert result["dark"] is False
    assert result["partial_dark"] is True
    assert result["dark_sources_count"] == result["summary"]["dark_sources_count"] == 1
    assert result["source_status"] == {
        name: "dark" if name == source else "live"
        for name in (
            "lanebus",
            "grants",
            "claim_custody",
            "seat_succession",
            "capabilities",
            "context_bundle",
        )
    }
    assert [
        (r["kind"], r["id"], r["attention"]) for r in result["locus_anomalies"]
    ] == [("source.dark", "source:" + source, "warning")]
    assert result["summary"]["warning_count"] == 1


def test_dark_capabilities_cannot_supply_routes():
    result = _synthesis(
        capabilities={
            "dark": True,
            "routes": [{"route_id": "r.local", "model_id": "m", "route_state": "ok"}],
        },
        context_bundle={"generated_at": _NOW, "facts": [_fact()]},
    )
    assert result["capability_fulfillment"] == []
    assert result["summary"]["routes_total"] == 0
    assert result["summary"]["routes_occupied_in_memory"] == 0


@pytest.mark.parametrize(
    "bundle,observed,state",
    [
        (_NOW, _NOW, "live"),
        (_NOW, None, "absent"),
        (_NOW, "invalid", "absent"),
        ("invalid", _NOW, "absent"),
        (_NOW, "2026-10-20T11:54:59Z", "stale"),
        (_NOW, "2026-10-20T12:11:01Z", "stale"),
        ("2026-10-20T11:54:59Z", _NOW, "stale"),
        ("2026-10-20T12:11:01Z", _NOW, "stale"),
        (_NOW, "2026-10-20T12:04:59Z", "stale"),
        ("2026-10-20T12:08:59Z", _NOW, "stale"),
        (_NOW, "2026-10-20T12:05:00Z", "live"),
        (_NOW, "2026-10-20T12:11:00Z", "live"),
        ("2026-10-20T11:55:00Z", "2026-10-20T11:54:59Z", "stale"),
        ("2026-10-20T12:11:00Z", "2026-10-20T12:11:01Z", "stale"),
    ],
)
def test_timestamp_validation_outcomes(bundle, observed, state):
    result = _with_facts([_fact(observed=observed)], generated=bundle)
    assert result["local_substrate_loci"][0]["state"] == state
    assert result["local_substrate_loci"][0]["loaded_models"] == (
        ["m"] if state == "live" else []
    )
    assert result["summary"]["local_loci_occupied_in_memory"] == int(state == "live")
    assert result["summary"]["routes_occupied_in_memory"] == int(state == "live")
    assert (
        result["capability_fulfillment"][0]["fulfillment_state"]
        == {
            "live": "occupied_in_memory",
            "stale": "stale_locus",
            "absent": "unfulfilled_local_locus",
        }[state]
    )


@pytest.mark.parametrize("location", ["provenance", "confidence_basis"])
def test_nested_observation_timestamp(location):
    fact = _fact(observed=None)
    fact[location] = {"observed_at": _NOW}
    result = _with_facts([fact])
    assert result["local_substrate_loci"][0]["state"] == "live"
    assert result["summary"]["routes_occupied_in_memory"] == 1


@pytest.mark.parametrize(
    "fact,expected",
    [
        (_fact("host", "dark"), ("host.dark", "dark", "warning")),
        (_fact("host", reasons=["host_unreachable"]), ("host.dark", "dark", "warning")),
        (_fact("host", "partial"), ("host.partial_observation", "partial", "info")),
        (
            _fact("host", reasons=["partial_memory_observation"]),
            ("host.partial_observation", "partial", "info"),
        ),
        (
            _fact(
                "resource_vram", reasons=["per_card_free_util_absent_backcompat_gpus"]
            ),
            ("host.partial_observation", "partial", "info"),
        ),
        (_fact("host", "absent"), ("host.absent", "absent", "warning")),
        (_fact("host"), None),
        (_fact("declared_endpoint", "lost"), ("endpoint.lost", "lost", "action")),
        (
            _fact("declared_endpoint", "absent", reasons=["LOST"]),
            ("endpoint.lost", "lost", "action"),
        ),
        (
            _fact(
                "declared_endpoint",
                "live",
                observed=None,
                reasons=["LOST"],
                alive=True,
            ),
            ("endpoint.lost", "lost", "action"),
        ),
        (
            _fact("declared_endpoint", observed=None, alive=True),
            ("endpoint.unverified_freshness", "absent", "warning"),
        ),
        (
            _fact("declared_endpoint", observed=None, alive=False),
            ("endpoint.lost", "lost", "action"),
        ),
        (_fact("declared_endpoint", "absent", reasons=["not_declared"]), None),
        (_fact("declared_endpoint"), None),
        (_fact(freshness="stale"), ("locus.stale", "stale", "warning")),
        (
            _fact(reasons=["OLLAMA_PS_PROBE_FAILED"]),
            ("locus.probe_failed", "live", "warning"),
        ),
        (_fact(admitted_by=[]), ("locus.unexplained_occupancy", "live", "warning")),
        (_fact(), None),
    ],
)
def test_anomaly_outcomes(fact, expected):
    result = _with_facts([fact])
    assert [
        (r["kind"], r["state"], r["attention"]) for r in result["locus_anomalies"]
    ] == ([expected] if expected else [])
    assert result["summary"]["locus_anomaly_count"] == int(expected is not None)
    for severity in ["action", "warning"]:
        assert result["summary"][severity + "_count"] == int(
            expected is not None and expected[2] == severity
        )


@pytest.mark.parametrize(
    "existing,freshness,ids",
    [
        (False, "fresh", ["host:fixture:11434/disk-resident"]),
        (True, "fresh", ["locus:fixture:8000"]),
        (False, "stale", []),
        (False, "dark", []),
    ],
)
def test_disk_resident_reconciliation(existing, freshness, ids):
    facts = [
        _fact("host", freshness=freshness, ollama_disk_resident_models=["m:latest"])
    ]
    if existing:
        facts.append(_fact(loaded_models=[], resident_models=["m"]))
    result = _with_facts(facts)
    route = result["capability_fulfillment"][0]
    assert route["resident_loci"] == ids
    assert route["occupied_loci"] == []
    assert route["fulfillment_state"] == (
        "resident_on_disk_only" if ids else "unfulfilled_local_locus"
    )
    assert result["summary"]["routes_occupied_in_memory"] == 0
    assert result["summary"]["local_loci_occupied_in_memory"] == 0


@pytest.mark.parametrize(
    "route_state,locus,mode,expected",
    [
        ("ok", "live", "local", "occupied_in_memory"),
        *[
            (s, "live", "local", "route_blocked_locus_occupied")
            for s in ("blocked", "error", "dark", "refused")
        ],
        *[(s, "none", "local", s) for s in ("blocked", "error", "dark", "refused")],
        ("ok", "stale", "local", "stale_locus"),
        ("ok", "resident", "local", "resident_on_disk_only"),
        ("ok", "none", "local", "unfulfilled_local_locus"),
        *[
            (s, "none", "remote", "admitted_remote_or_cloud")
            for s in ("ok", "active", "observed", "live", "ready")
        ],
        ("queued", "none", "remote", "queued"),
        ("", "none", "remote", "unfulfilled"),
    ],
)
def test_capability_fulfillment_route_states(route_state, locus, mode, expected):
    facts = (
        []
        if locus == "none"
        else [
            _fact(
                freshness="stale" if locus == "stale" else "fresh",
                loaded_models=[] if locus == "resident" else ["m"],
            )
        ]
    )
    result = _with_facts(
        facts,
        routes=[
            {
                "route_id": "r.local" if mode == "local" else "r.remote",
                "mode": mode,
                "model_id": "m",
                "route_state": route_state,
            }
        ],
    )
    row = result["capability_fulfillment"][0]
    assert row["route_state"] == route_state
    assert row["fulfillment_state"] == expected
    assert row["occupied_loci"] == (["locus:fixture:8000"] if locus == "live" else [])
    assert row["resident_loci"] == (
        ["locus:fixture:8000"] if locus == "resident" else []
    )
    assert result["summary"]["routes_total"] == 1
    assert result["summary"]["routes_occupied_in_memory"] == int(
        expected == "occupied_in_memory"
    )
    assert result["summary"]["local_loci_occupied_in_memory"] == int(locus == "live")


@pytest.mark.parametrize(
    "admit,occupied", [("r.local.extra", False), ("r.local (active)", True)]
)
def test_route_identity_without_a_model_target(admit, occupied):
    result = _with_facts(
        [_fact(admitted_by=[admit])],
        routes=[{"route_id": "r.local", "mode": "local", "model_id": ""}],
    )
    row = result["capability_fulfillment"][0]
    assert row["fulfillment_state"] == (
        "occupied_in_memory" if occupied else "unfulfilled_local_locus"
    )
    assert row["occupied_loci"] == (["locus:fixture:8000"] if occupied else [])


def test_read_obligations_summary_synthesizes_owes_and_fulfillment() -> None:
    allowlist = [
        "kind",
        "relation",
        "state",
        "attention",
        "route_id",
        "capability_id",
        "mode",
        "model_id",
        "route_state",
    ]
    lanebus = {
        "inboxes": [
            {
                "lane": "lane-alpha",
                "unacked_count": 3,
                "oldest_unacked_age_s": 120.5,
                "state": "unacked",
                "attention": "warning",
            },
            {
                "lane": "lane-beta",
                "unacked_count": 0,
                "state": "clear",
                "attention": "routine",
            },
        ]
    }
    grants = {
        "rows": [
            {
                "grant_id": "g-1",
                "lane": "lane-alpha",
                "route_id": "r.local",
                "state": "unmatched_receipt",
                "attention": "warning",
            },
            {
                "grant_id": "g-2",
                "lane": "lane-beta",
                "route_id": "r.cloud",
                "state": "reconciled",
                "attention": "routine",
            },
        ]
    }
    claim_custody = {
        "rows": [
            {"task_id": "task-1", "holder": "lane-alpha", "custody_state": "stale"},
            {"task_id": "task-2", "holder": "lane-beta", "custody_state": "settled"},
        ]
    }
    seat_succession = {
        "rows": [
            {
                "role": "lane-gamma",
                "session": "sess-gamma",
                "pane_state": "absent",
                "claims": {"present": True},
            }
        ]
    }
    capabilities = {
        "routes": [
            {
                "route_id": "r.local",
                "capability_id": "cap.code",
                "mode": "local",
                "model_id": "qwen3.8-flash-next",
                "route_state": "ok",
            },
            {
                "route_id": "r.disk",
                "capability_id": "cap.embed",
                "mode": "local",
                "model_id": "bge-m3",
                "route_state": "ok",
            },
            {
                "route_id": "r.cloud",
                "capability_id": "cap.review",
                "mode": "metered",
                "model_id": "claude-opus-4-7",
                "route_state": "ok",
            },
        ]
    }
    context_bundle = {
        "generated_at": "2026-10-20T12:10:00Z",
        "facts": [
            {
                "id": "host:node-dark",
                "kind": "host",
                "state": "dark",
                "observed_at": "2026-10-20T12:09:50Z",
                "payload": {"host": "node-dark"},
            },
            {
                "id": "locus:node-a:8000",
                "kind": "locus",
                "state": "live",
                "observed_at": "2026-10-20T12:09:50Z",
                "payload": {
                    "loaded_models": ["qwen3.8-flash-next"],
                    "resident_models": ["qwen3.8-flash-next"],
                    "admitted_by": ["r.local (active)"],
                    "topology": "tp=2,nnodes=2",
                },
            },
            {
                "id": "locus:node-b:11434",
                "kind": "locus",
                "state": "live",
                "observed_at": "2026-10-20T12:09:50Z",
                "payload": {
                    "loaded_models": ["stray-unexplained-model"],
                    "resident_models": ["bge-m3:latest", "stray-unexplained-model"],
                    "admitted_by": [],
                },
            },
        ],
    }
    res = _synthesis(
        allowlist=allowlist,
        lanebus=lanebus,
        grants=grants,
        claim_custody=claim_custody,
        seat_succession=seat_succession,
        capabilities=capabilities,
        context_bundle=context_bundle,
        now="2026-10-20T12:10:05Z",
    )
    assert res["dark"] is False
    assert res["summary"]["total_owes"] == 4
    assert res["summary"]["lanebus_owes_count"] == 1
    assert res["summary"]["grant_owes_count"] == 1
    assert res["summary"]["custody_owes_count"] == 2
    assert res["summary"]["locus_anomaly_count"] == 2
    assert res["summary"]["routes_total"] == 3
    assert res["summary"]["routes_occupied_in_memory"] == 1
    by_route = {r["route_id"]: r for r in res["capability_fulfillment"]}
    assert by_route["r.local"]["fulfillment_state"] == "occupied_in_memory"
    assert by_route["r.local"]["occupied_loci"] == ["locus:node-a:8000"]
    assert by_route["r.local"]["topology"] == "tp=2,nnodes=2"
    assert by_route["r.disk"]["fulfillment_state"] == "resident_on_disk_only"
    assert by_route["r.disk"]["resident_loci"] == ["locus:node-b:11434"]
    assert by_route["r.cloud"]["fulfillment_state"] == "admitted_remote_or_cloud"


def test_all_sources_dark_reports_dark_and_source_anomalies() -> None:
    """F01: when all six input facets are dark, obligations must report dark=True and surface source.dark anomalies."""
    dark_facet = {"dark": True}
    res = _synthesis(
        allowlist=["kind", "state", "attention"],
        lanebus=dark_facet,
        grants=dark_facet,
        claim_custody=dark_facet,
        seat_succession=dark_facet,
        capabilities=dark_facet,
        context_bundle=dark_facet,
    )
    assert res["dark"] is True
    assert res["summary"]["dark_sources_count"] == 6
    assert len(res["locus_anomalies"]) == 6
    assert all((a["kind"] == "source.dark" for a in res["locus_anomalies"]))


def test_stale_locus_and_blocked_route_do_not_claim_active_memory_fulfillment() -> None:
    """F01: a stale locus or a blocked route must never report occupied_in_memory."""
    allowlist = ["route_id", "fulfillment_state", "route_state", "kind", "state"]
    capabilities = {
        "routes": [
            {
                "route_id": "r.stale",
                "capability_id": "cap.a",
                "mode": "local",
                "model_id": "m-stale",
                "route_state": "ok",
            },
            {
                "route_id": "r.blocked",
                "capability_id": "cap.b",
                "mode": "local",
                "model_id": "m-live",
                "route_state": "blocked",
            },
        ]
    }
    context_bundle = {
        "generated_at": "2026-10-20T12:10:00Z",
        "facts": [
            {
                "id": "locus:stale:8000",
                "kind": "loaded_model",
                "observed_at": "2026-10-20T11:30:00Z",
                "freshness_state": "stale",
                "state": {"value_state": "stale", "reason_codes": []},
                "value": {
                    "host": "node-a",
                    "endpoint": "8000",
                    "model": "m-stale",
                    "admitting_row": "r.stale",
                },
            },
            {
                "id": "locus:live:8001",
                "kind": "loaded_model",
                "observed_at": "2026-10-20T12:09:50Z",
                "freshness_state": "fresh",
                "state": {"value_state": "lit", "reason_codes": []},
                "value": {
                    "host": "node-a",
                    "endpoint": "8001",
                    "model": "m-live",
                    "admitting_row": "r.blocked",
                },
            },
        ],
    }
    res = _synthesis(
        allowlist=allowlist,
        capabilities=capabilities,
        context_bundle=context_bundle,
        now="2026-10-20T12:10:05Z",
    )
    by_route = {r["route_id"]: r for r in res["capability_fulfillment"]}
    assert by_route["r.stale"]["fulfillment_state"] == "stale_locus"
    assert by_route["r.stale"]["occupied_loci"] == []
    assert by_route["r.blocked"]["fulfillment_state"] == "route_blocked_locus_occupied"
    assert res["summary"]["routes_occupied_in_memory"] == 0


def test_strict_identity_joins_reject_fuzzy_substrings_and_unfulfilled_local_routes() -> (
    None
):
    """F03 + F08: route.a must not match route.a2; qwen3 must not match qwen3-embedding-8b; local unfulfilled route is not cloud."""
    allowlist = ["route_id", "fulfillment_state", "route_state"]
    capabilities = {
        "routes": [
            {
                "route_id": "route.a",
                "mode": "local",
                "model_id": "qwen3",
                "route_state": "ok",
            },
            {
                "route_id": "route.disk",
                "mode": "local",
                "model_id": "ornith-35b",
                "route_state": "ok",
            },
        ]
    }
    context_bundle = {
        "generated_at": "2026-10-20T12:10:00Z",
        "facts": {
            "resource_vram": [
                {
                    "fact_id": "resource_vram:host:node-a",
                    "fact_type": "resource_vram",
                    "subject_ref": "host:node-a",
                    "observed_at": "2026-10-20T12:09:50Z",
                    "freshness_state": "fresh",
                    "state": {"value_state": "lit", "reason_codes": []},
                    "value": {
                        "host": "node-a",
                        "total_mb": 64000,
                        "avail_mb": 32000,
                        "ollama_disk_resident_models": ["ornith-35b:latest"],
                    },
                }
            ],
            "loaded_model": [
                {
                    "fact_id": "loaded_model:host:node-a:8000/qwen3-embedding-8b",
                    "fact_type": "loaded_model",
                    "subject_ref": "host:node-a:8000/qwen3-embedding-8b",
                    "observed_at": "2026-10-20T12:09:50Z",
                    "freshness_state": "fresh",
                    "state": {"value_state": "lit", "reason_codes": []},
                    "value": {
                        "host": "node-a",
                        "endpoint": "8000",
                        "model": "qwen3-embedding-8b",
                        "admitting_row": "route.a2 (active)",
                    },
                }
            ],
        },
    }
    res = _synthesis(
        allowlist=allowlist,
        capabilities=capabilities,
        context_bundle=context_bundle,
        now="2026-10-20T12:10:05Z",
    )
    by_route = {r["route_id"]: r for r in res["capability_fulfillment"]}
    assert by_route["route.a"]["fulfillment_state"] == "unfulfilled_local_locus"
    assert by_route["route.disk"]["fulfillment_state"] == "resident_on_disk_only"


def test_producer_lost_endpoint_and_partial_vram_contracts() -> None:
    """F04: declared_endpoint with value_state='absent' and reason_code='LOST' emits endpoint.lost action; partial VRAM emits host.partial_observation."""
    allowlist = ["kind", "state", "attention"]
    context_bundle = {
        "facts": [
            {
                "fact_id": "declared_endpoint:serving.node-a-5000",
                "fact_type": "declared_endpoint",
                "subject_ref": "serving.node-a-5000",
                "state": {
                    "value_state": "absent",
                    "reason_codes": ["LOST", "declared_endpoint_dead"],
                },
                "value": {"port": "5000", "alive": False},
            },
            {
                "fact_id": "resource_vram:host:node-a/RTX",
                "fact_type": "resource_vram",
                "subject_ref": "host:node-a/RTX",
                "state": {
                    "value_state": "partial",
                    "reason_codes": ["per_card_free_util_absent_backcompat_gpus"],
                },
                "value": {"card": "RTX", "used": "100 MiB"},
            },
        ]
    }
    res = _synthesis(
        allowlist=allowlist, capabilities={"routes": []}, context_bundle=context_bundle
    )
    kinds = {a["kind"]: a for a in res["locus_anomalies"]}
    assert "endpoint.lost" in kinds
    assert kinds["endpoint.lost"]["attention"] == "action"
    assert res["summary"]["action_count"] == 1
    assert "host.partial_observation" in kinds
    assert "host.dark" not in kinds


def test_explicit_zero_unacked_and_malformed_counters_handled_safely() -> None:
    """F10: unacked_count=0 must not fall back to unread_count=3; malformed counters must not raise ValueError."""
    allowlist = ["kind", "state", "count"]
    lanebus = {
        "inboxes": [
            {
                "lane": "lane-zero",
                "unacked_count": 0,
                "unread_count": 3,
                "state": "clear",
            },
            {"lane": "lane-bad", "unacked_count": "bad-counter-value", "state": "ok"},
        ]
    }
    res = _synthesis(
        allowlist=allowlist,
        lanebus=lanebus,
        grants={
            "rows": [
                {
                    "grant_id": "g-bad",
                    "honor_count": "not-an-int",
                    "grant_record_present": True,
                }
            ]
        },
        capabilities={"routes": []},
        context_bundle={
            "facts": [
                {
                    "id": "h1",
                    "kind": "host",
                    "state": "live",
                    "observed_at": "2026-10-20T12:09:50Z",
                }
            ]
        },
        now="2026-10-20T12:10:00Z",
    )
    assert len(res["lanebus_owes"]) == 1
    assert res["lanebus_owes"][0]["kind"] == "lanebus.unparseable"
    assert len(res["grant_owes"]) == 1
    assert res["grant_owes"][0]["kind"] == "grant.unparseable_honor_count"


def test_vendor_namespace_isolation_and_dark_source_freshness_guards() -> None:
    """R2-F04, R2-F06, R2-F07, R2-F09, R3-F02: vendor-a/model-x != vendor-b/model-x, dark context_bundle yields 0 live loci, ancient bundles or missing observed_at never claim occupied_in_memory, timestamp-absent alive endpoint emits endpoint.unverified_freshness, and negative/huge ints are rejected."""
    allowlist = ["route_id", "fulfillment_state", "kind", "state", "attention", "count"]
    capabilities = {
        "routes": [
            {
                "route_id": "r.vendor.b",
                "mode": "local",
                "model_id": "vendor-b/model-x",
                "route_state": "ok",
            }
        ]
    }
    context_bundle = {
        "generated_at": "2026-10-20T12:10:00Z",
        "facts": [
            {
                "fact_id": "loaded_model:host:n1:8000/vendor-a/model-x",
                "fact_type": "loaded_model",
                "subject_ref": "host:n1:8000/vendor-a/model-x",
                "observed_at": "2026-10-20T12:09:50Z",
                "freshness_state": "fresh",
                "state": {"value_state": "lit", "reason_codes": []},
                "value": {
                    "host": "n1",
                    "endpoint": "8000",
                    "model": "vendor-a/model-x",
                    "admitting_row": "r.vendor.a",
                },
            },
            {
                "fact_id": "declared_endpoint:serving.n1-8000",
                "fact_type": "declared_endpoint",
                "subject_ref": "serving.n1-8000",
                "freshness_state": "absent",
                "state": {
                    "value_state": "absent",
                    "reason_codes": ["observation_timestamp_absent"],
                },
                "value": {"host": "n1", "port": "8000", "alive": True},
            },
        ],
    }
    res = _synthesis(
        allowlist=allowlist,
        lanebus={
            "inboxes": [
                {"lane": "neg", "unacked_count": -5, "state": "ok"},
                {"lane": "huge", "unacked_count": "9" * 50, "state": "ok"},
            ]
        },
        capabilities=capabilities,
        context_bundle=context_bundle,
        now="2026-10-20T12:10:05Z",
    )
    by_route = {r["route_id"]: r for r in res["capability_fulfillment"]}
    assert by_route["r.vendor.b"]["fulfillment_state"] == "unfulfilled_local_locus"
    kinds = {a["kind"]: a for a in res["locus_anomalies"]}
    assert "endpoint.unverified_freshness" in kinds
    assert "endpoint.lost" not in kinds
    assert len(res["lanebus_owes"]) == 2
    assert all((x["kind"] == "lanebus.unparseable" for x in res["lanebus_owes"]))
    ancient_res = _synthesis(
        allowlist=allowlist,
        capabilities={
            "routes": [
                {
                    "route_id": "r.vendor.a",
                    "mode": "local",
                    "model_id": "vendor-a/model-x",
                    "route_state": "ok",
                }
            ]
        },
        context_bundle={
            "generated_at": "2000-01-01T00:00:00Z",
            "facts": [
                {
                    "fact_id": "loaded_model:host:n1:8000/vendor-a/model-x",
                    "fact_type": "loaded_model",
                    "subject_ref": "host:n1:8000/vendor-a/model-x",
                    "observed_at": "2000-01-01T00:00:00Z",
                    "freshness_state": "fresh",
                    "state": {"value_state": "lit", "reason_codes": []},
                    "value": {
                        "host": "n1",
                        "endpoint": "8000",
                        "model": "vendor-a/model-x",
                        "admitting_row": "r.vendor.a",
                    },
                }
            ],
        },
        now="2026-10-20T12:10:05Z",
    )
    assert (
        ancient_res["capability_fulfillment"][0]["fulfillment_state"] == "stale_locus"
    )
    assert ancient_res["summary"]["routes_occupied_in_memory"] == 0
    no_ts_res = _synthesis(
        allowlist=allowlist,
        capabilities={
            "routes": [
                {
                    "route_id": "r.vendor.a",
                    "mode": "local",
                    "model_id": "vendor-a/model-x",
                    "route_state": "ok",
                }
            ]
        },
        context_bundle={
            "facts": [
                {
                    "fact_id": "loaded_model:host:n1:8000/vendor-a/model-x",
                    "fact_type": "loaded_model",
                    "subject_ref": "host:n1:8000/vendor-a/model-x",
                    "freshness_state": "fresh",
                    "state": {"value_state": "lit", "reason_codes": []},
                    "value": {
                        "host": "n1",
                        "endpoint": "8000",
                        "model": "vendor-a/model-x",
                        "admitting_row": "r.vendor.a",
                    },
                }
            ]
        },
        now="2026-10-20T12:10:05Z",
    )
    assert (
        no_ts_res["capability_fulfillment"][0]["fulfillment_state"]
        == "unfulfilled_local_locus"
    )
    assert no_ts_res["summary"]["routes_occupied_in_memory"] == 0
    dark_ctx_res = _synthesis(
        allowlist=allowlist,
        capabilities={
            "routes": [
                {
                    "route_id": "r.vendor.a",
                    "mode": "local",
                    "model_id": "vendor-a/model-x",
                    "route_state": "ok",
                }
            ]
        },
        context_bundle={"dark": True, "facts": context_bundle["facts"]},
        now="2026-10-20T12:10:05Z",
    )
    assert dark_ctx_res["summary"]["routes_occupied_in_memory"] == 0
    assert dark_ctx_res["summary"]["local_loci_occupied_in_memory"] == 0
