"""Quota phase history is a bounded observation window, never an admission writer."""

from __future__ import annotations

import builtins
import io
import json
import os
import subprocess
import threading
import time
from contextlib import contextmanager
from datetime import UTC, datetime, timedelta

import pytest
import reins_quota_history as history_module
from fastapi.testclient import TestClient
from reins_quota_history import QuotaPhaseHistory
from reins_read import build_app, instance_config


def stamp(seconds=0):
    return (datetime(2026, 1, 1, tzinfo=UTC) + timedelta(seconds=seconds)).isoformat()


def ledger(path, seconds=0, state=None, quota="fresh", expiry=600):
    path.write_text(
        json.dumps(
            {
                "captured_at": stamp(seconds),
                "quota_snapshots": [
                    {
                        "route_id": "review.example",
                        "captured_at": stamp(seconds),
                        "state": state,
                        "subscription_quota_state": quota,
                        "fresh_until": stamp(expiry) if expiry is not None else None,
                        "operator_visible_reason": "DO-NOT-PROJECT-REASON",
                        "evidence_refs": ["DO-NOT-PROJECT-EVIDENCE"],
                    }
                ],
                "spend_receipts": ["DO-NOT-PROJECT-RECEIPT"],
            }
        )
    )


@contextmanager
def no_effects(monkeypatch):
    """Trip on attempted writes (including transient ones) and timer/receipt commands."""

    def forbidden(*args, **kwargs):
        raise AssertionError("projection attempted a persistent or external effect")

    def guarded_open(opener):
        def read_only(file, mode="r", *args, **kwargs):
            assert not any(c in mode for c in "wax+"), "projection opened a writer"
            return opener(file, mode, *args, **kwargs)

        return read_only

    original_os_open = os.open

    def read_only_fd(path, flags, *args, **kwargs):
        assert not flags & (
            os.O_WRONLY | os.O_RDWR | os.O_CREAT | os.O_TRUNC | os.O_APPEND
        )
        return original_os_open(path, flags, *args, **kwargs)

    with monkeypatch.context() as guard:
        guard.setattr(builtins, "open", guarded_open(builtins.open))
        guard.setattr(io, "open", guarded_open(io.open))
        guard.setattr(os, "open", read_only_fd)
        for name in (
            "mkdir",
            "remove",
            "unlink",
            "rename",
            "replace",
            "chmod",
            "utime",
            "system",
        ):
            guard.setattr(os, name, forbidden)
        guard.setattr(subprocess, "Popen", forbidden)
        yield


def tree(root):
    return {
        str(p.relative_to(root)): (
            p.read_bytes(),
            p.stat().st_mtime_ns,
            p.stat().st_mode,
        )
        for p in root.rglob("*")
        if p.is_file()
    }


def test_no_write_oracle_through_read_app(tmp_path, monkeypatch):
    source = tmp_path / "quota.json"
    ledger(source)
    (tmp_path / "receipts").mkdir()
    (tmp_path / "refresh.timer").write_text("enabled=true")
    before = tree(tmp_path)
    app = build_app("", [], {"quota_spend_ledger_live": str(source)})
    with no_effects(monkeypatch), TestClient(app) as client:
        response = client.get("/read/quota-history")
        assert response.status_code == 200
        data = response.json()
        assert data["state"] == "observed"
        assert data["rows"][0]["route_id"] == "review.example"
        assert "DO-NOT-PROJECT" not in response.text
    assert tree(tmp_path) == before


def test_missing_ledger_is_typed_unavailable(tmp_path):
    source = tmp_path / "nonexistent.json"
    with TestClient(
        build_app("", [], {"quota_spend_ledger_live": str(source)})
    ) as client:
        response = client.get("/read/quota-history")
        assert response.status_code == 200
        data = response.json()
        assert data["state"] == "ledger-unavailable"
        assert data["dark"] is True
        assert data["reason"] == "ledger-missing"
        assert data["rows"] == []
    assert not source.exists()


def test_phase_flaps_null_state_and_expiry_renewal(tmp_path):
    source = tmp_path / "quota.json"
    history = QuotaPhaseHistory(str(source))
    series = [
        (0, "fresh", "fresh", 600),
        (60, None, "unknown", None),
        (120, None, "fresh", 800),
        (180, None, "fresh", 900),
        (240, None, "fresh", 900),
    ]
    for seconds, state, quota, expiry in series:
        ledger(source, seconds, state, quota, expiry)
        history.sample()
    data = history.snapshot()
    rows = data["rows"]
    assert (
        len(rows) == 4
    )  # identical phase with a later capture is not another transition
    assert [r["subscription_quota_state"] for r in rows] == [
        "fresh",
        "unknown",
        "fresh",
        "fresh",
    ]
    assert rows[1]["prior_state"] == "fresh" and rows[1]["state"] is None
    assert rows[2]["prior_subscription_quota_state"] == "unknown"
    assert rows[2]["state"] is None and rows[2]["fresh_until_delta_s"] is None
    assert rows[3]["fresh_until_delta_s"] == 100
    assert rows[3]["captured_at"] == stamp(180).replace("+00:00", "Z")
    assert data["source_captured_at"] == stamp(240).replace("+00:00", "Z")
    assert data["capture_coverage"] == "sampled-captures-only"
    assert data["history_complete"] is False
    assert set(rows[0]["air"].values()) == {"deny"}
    assert history.snapshot(["captured_at"])["rows"][0]["air"]["captured_at"] == "ok"
    rows[0]["state"] = "tampered"
    assert history.snapshot()["rows"][0]["state"] == "fresh"


def test_bounded_store_under_transition_flood_and_process_restart(tmp_path):
    source = tmp_path / "quota.json"
    history = QuotaPhaseHistory(str(source), capacity=7)
    for n in range(200):
        ledger(source, n, quota="fresh" if n % 2 == 0 else "unknown")
        history.sample()
    data = history.snapshot()
    assert len(data["rows"]) == 7
    assert data["evicted_rows"] == 193
    assert [r["captured_at"] for r in data["rows"]] == [
        stamp(n).replace("+00:00", "Z") for n in range(193, 200)
    ]
    assert data["retention"] == "process-local-bounded-window"
    restarted = QuotaPhaseHistory(str(source), capacity=7)
    assert restarted.snapshot()["rows"] == []
    restarted.sample()
    assert len(restarted.snapshot()["rows"]) == 1
    assert restarted.snapshot()["rows"][0]["kind"] == "initial"


@pytest.mark.parametrize(
    "bad",
    [
        "{",
        "[]",
        "{}",
        '{"quota_snapshots": {}}',
        '{"captured_at": "2026-01-01", "quota_snapshots": []}',
        b"\xff",
    ],
)
def test_malformed_is_unavailable_retains_history_and_recovery_is_not_contiguous(
    tmp_path, bad
):
    source = tmp_path / "quota.json"
    ledger(source)
    history = QuotaPhaseHistory(str(source))
    history.sample()
    before = history.snapshot()["rows"]
    source.write_bytes(bad.encode() if isinstance(bad, str) else bad)
    history.sample()
    data = history.snapshot()
    assert (
        data["state"] == "ledger-unavailable" and data["reason"] == "ledger-malformed"
    )
    assert data["rows"] == before and data["observation_gap"] is True
    ledger(source)  # old bytes restored, no new capture yet
    history.sample()
    assert history.snapshot()["observation_gap"] is True
    ledger(source, 60, quota="unknown")
    history.sample()
    data = history.snapshot()
    assert data["state"] == "observed" and data["observation_gap"] is False
    assert data["rows"][-1]["after_observation_gap"] is True


def test_missing_then_created_recovers_without_creating_a_ledger(tmp_path):
    source = tmp_path / "quota.json"
    history = QuotaPhaseHistory(str(source))
    history.sample()
    assert history.snapshot()["state"] == "ledger-unavailable"
    assert not source.exists()
    ledger(source)
    history.sample()
    assert history.snapshot()["state"] == "observed"


def test_large_evidence_payload_is_supported_but_not_retained(tmp_path):
    source = tmp_path / "quota.json"
    ledger(source)
    doc = json.loads(source.read_text())
    doc["quota_snapshots"][0]["operator_visible_reason"] = "PRIVATE-EVIDENCE" * 280_000
    source.write_text(json.dumps(doc))
    history = QuotaPhaseHistory(str(source))
    history.sample()
    data = history.snapshot()
    assert data["state"] == "observed"
    assert len(data["rows"]) == 1
    assert len(json.dumps(data)) < 4096
    assert "PRIVATE-EVIDENCE" not in json.dumps(data)


@pytest.mark.parametrize(
    "change",
    [
        lambda d: d["quota_snapshots"].append(d["quota_snapshots"][0]),
        lambda d: d["quota_snapshots"].append(None),
        lambda d: d["quota_snapshots"][0].update(route_id="path/secret"),
        lambda d: d["quota_snapshots"][0].update(state="free text must not escape"),
        lambda d: d["quota_snapshots"][0].update(subscription_quota_state=None),
        lambda d: d["quota_snapshots"][0].update(fresh_until="not a timestamp"),
        lambda d: d["quota_snapshots"][0].update(captured_at=stamp(100)),
        lambda d: d.update(
            quota_snapshots=[
                {**d["quota_snapshots"][0], "route_id": f"review.route-{n}"}
                for n in range(513)
            ]
        ),
    ],
)
def test_bad_route_capture_is_rejected_atomically(tmp_path, change):
    source = tmp_path / "quota.json"
    ledger(source)
    doc = json.loads(source.read_text())
    change(doc)
    source.write_text(json.dumps(doc))
    history = QuotaPhaseHistory(str(source))
    history.sample()
    data = history.snapshot()
    assert data["state"] == "ledger-unavailable"
    assert data["reason"] == "ledger-malformed"
    assert data["rows"] == [] and data["route_count"] == 0


@pytest.mark.parametrize("shape", ["large", "fifo", "directory"])
def test_bounded_input_and_non_regular_files(tmp_path, shape):
    source = tmp_path / "quota.json"
    if shape == "large":
        source.write_text(
            json.dumps(
                {
                    "captured_at": stamp(),
                    "quota_snapshots": [],
                    "padding": "x" * (8 * 1024 * 1024),
                }
            )
        )
    elif shape == "fifo":
        os.mkfifo(source)
    else:
        source.mkdir()
    history = QuotaPhaseHistory(str(source))
    history.sample()  # FIFO must not block waiting for a writer
    assert history.snapshot()["state"] == "ledger-unavailable"


def test_read_error_is_unavailable_not_an_empty_success(tmp_path, monkeypatch):
    source = tmp_path / "quota.json"
    ledger(source)
    history = QuotaPhaseHistory(str(source))
    history.sample()

    def denied(*args, **kwargs):
        raise PermissionError("PRIVATE-PATH")

    monkeypatch.setattr(history_module, "_read_capture", denied)
    history.sample()
    data = history.snapshot()
    assert (
        data["state"] == "ledger-unavailable" and data["reason"] == "ledger-unreadable"
    )
    assert len(data["rows"]) == 1
    assert "PRIVATE-PATH" not in json.dumps(data)


def test_duplicate_regressed_conflicting_and_removed_captures(tmp_path):
    source = tmp_path / "quota.json"
    ledger(source, 60)
    history = QuotaPhaseHistory(str(source))
    history.sample()
    history.sample()
    assert len(history.snapshot()["rows"]) == 1
    for seconds in (0, 60):
        ledger(source, seconds, quota="unknown")
        history.sample()
        assert history.snapshot()["state"] == "ledger-unavailable"
        assert len(history.snapshot()["rows"]) == 1
    source.write_text(json.dumps({"captured_at": stamp(120), "quota_snapshots": []}))
    history.sample()
    data = history.snapshot()
    assert data["state"] == "observed" and data["route_count"] == 0
    assert data["rows"][-1]["kind"] == "removed"
    assert data["rows"][-1]["prior_subscription_quota_state"] == "fresh"
    assert data["rows"][-1]["subscription_quota_state"] is None


@pytest.mark.parametrize(
    "config",
    [
        {"quota_history_capacity": 0},
        {"quota_history_capacity": 4097},
        {"quota_history_capacity": True},
        {"quota_history_interval_s": 0},
        {"quota_history_interval_s": float("inf")},
        {"quota_history_interval_s": "60"},
        {"quota_spend_ledger_live": ["not-a-path"]},
    ],
)
def test_bad_binding_is_typed_and_does_not_read(tmp_path, monkeypatch, config):
    def unexpected(*args):
        pytest.fail("invalid configuration attempted to sample")

    monkeypatch.setattr(history_module, "_read_capture", unexpected)
    history = QuotaPhaseHistory.from_config(
        {"quota_spend_ledger_live": str(tmp_path / "ledger"), **config}
    )
    history.sample()
    assert history.snapshot()["state"] == "unconfigured"
    assert history.snapshot()["reason"] == "invalid-history-config"


def test_unbound_app_never_reads_a_default_estate_path(monkeypatch):
    def unexpected(*args):
        pytest.fail("unbound app attempted to sample")

    monkeypatch.setattr(history_module, "_read_capture", unexpected)
    with TestClient(build_app("", [])) as client:
        data = client.get("/read/quota-history").json()
    assert data["state"] == "unconfigured" and data["rows"] == []


def test_config_lifespan_cadence_without_gets_and_shutdown(tmp_path, monkeypatch):
    source = tmp_path / "quota.json"
    ledger(source)
    config = tmp_path / "instance.toml"
    config.write_text(
        f'quota_spend_ledger_live = "{source.as_posix()}"\nquota_history_capacity = 3\nquota_history_interval_s = 0.02\n'
    )
    monkeypatch.setenv("REINS_CONFIG", str(config))
    monkeypatch.delenv("REINS_QUOTA_SPEND_LEDGER_LIVE", raising=False)
    original = history_module._read_capture
    sampled = threading.Event()
    calls = []

    def observe(path):
        capture = original(path)
        calls.append(capture[0])
        if capture[0] == stamp(60).replace("+00:00", "Z"):
            sampled.set()
        return capture

    monkeypatch.setattr(history_module, "_read_capture", observe)
    cfg = instance_config()
    with TestClient(build_app("", [], cfg)) as client:
        ledger(source, 60, quota="unknown")
        assert sampled.wait(3), "sampler required a GET to observe the transition"
        data = client.get("/read/quota-history").json()
        assert data["capacity"] == 3 and len(data["rows"]) == 2
        assert data["rows"][-1]["subscription_quota_state"] == "unknown"
    n = len(calls)
    time.sleep(0.08)
    assert len(calls) == n
    monkeypatch.setenv("REINS_QUOTA_SPEND_LEDGER_LIVE", str(tmp_path / "override"))
    assert instance_config()["quota_spend_ledger_live"] == str(tmp_path / "override")


def test_outage_recovery_with_unchanged_route_retains_durable_gap_evidence(tmp_path):
    """RD1: an outage at t1 that recovers at t2 with the route unchanged must leave durable
    gap evidence in the snapshot even though no new transition row is appended."""
    source = tmp_path / "quota.json"
    ledger(source, 0, state="admitted", quota="fresh", expiry=600)
    history = QuotaPhaseHistory(str(source))
    history.sample()
    snap0 = history.snapshot()
    assert snap0["state"] == "observed"
    assert snap0["observation_gap"] is False
    assert snap0["observation_gap_count"] == 0
    assert snap0["last_gap_started_at"] is None
    assert snap0["last_gap_ended_at"] is None
    assert len(snap0["rows"]) == 1

    # t1: ledger disappears -> outage gap opens
    source.unlink()
    history.sample()
    snap1 = history.snapshot()
    assert snap1["state"] == "ledger-unavailable"
    assert snap1["observation_gap"] is True
    assert snap1["observation_gap_count"] == 1
    assert snap1["last_gap_started_at"] is not None
    assert snap1["last_gap_ended_at"] is None

    # t2: valid advanced capture arrives with the route unchanged
    ledger(source, 60, state="admitted", quota="fresh", expiry=600)
    history.sample()
    snap2 = history.snapshot()
    assert snap2["state"] == "observed"
    assert snap2["observation_gap"] is False
    assert len(snap2["rows"]) == 1  # no new transition row
    assert snap2["observation_gap_count"] == 1
    assert snap2["last_gap_started_at"] == snap1["last_gap_started_at"]
    assert snap2["last_gap_ended_at"] is not None


def test_read_capability_summary_passes_configured_quota_ledger_path(tmp_path, monkeypatch):
    """RD2: when quota_spend_ledger_live is configured, read_capability_summary must pass
    quota_ledger_path to load_dispatch_policy_sources so the displayed path and status match."""
    from pathlib import Path
    import hapax.spine.dispatcher_policy as dp
    from reins_read import read_capability_summary

    configured_ledger = tmp_path / "configured-quota-ledger.json"
    captured_kwargs: dict[str, object] = {}
    orig_loader = dp.load_dispatch_policy_sources

    def spy_loader(**kwargs):
        captured_kwargs.update(kwargs)
        return orig_loader(**kwargs)

    monkeypatch.setattr(dp, "load_dispatch_policy_sources", spy_loader)
    summary = read_capability_summary(
        str(tmp_path),
        [],
        {"quota_spend_ledger_live": str(configured_ledger)},
    )
    assert captured_kwargs.get("quota_ledger_path") == Path(configured_ledger)
    quota_source = next(s for s in summary["sources"] if s["id"] == "quota_ledger")
    assert quota_source["path"] == str(configured_ledger)
    assert quota_source["status"] == "missing"

