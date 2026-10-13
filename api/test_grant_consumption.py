"""Grant history is a metadata projection, never a grant decision or writer."""

import json
import os
import subprocess
import sys
from pathlib import Path

import pytest
from fastapi.testclient import TestClient
from reins_read import build_app, instance_config


def test_grant_consumption_is_read_only(tmp_path, monkeypatch):
    grants = tmp_path / "grants"
    grants.mkdir()
    ledger = tmp_path / "honors.jsonl"
    grant = {
        "kind": "escape",
        "grant_id": "a" * 32,
        "grantor": "operator",
        "role": "lane-mint",
        "session": "session-mint",
        "issued_at": 1791716400.0,
        "expires_at": 1791720000.0,
        "lease_state": "expired",
        "reason": "PRIVATE_REASON",
        "signature": "PRIVATE_SIGNATURE",
        "token": "PRIVATE_TOKEN",
        "paths": ["PRIVATE_PATH"],
    }
    (grants / "first.grant").write_text(json.dumps(grant))
    legacy = {
        k: v for k, v in grant.items() if k not in {"role", "session", "lease_state"}
    }
    legacy["grant_id"] = "b" * 32
    (grants / "legacy.grant").write_text(json.dumps(legacy))
    key = tmp_path / "grant-key"
    key.write_text("PRIVATE_KEY")
    events = [
        {
            "kind": "escape_grant_honored",
            "grant_id": "a" * 32,
            "role": "lane-use",
            "session": "session-use",
            "ts": f"2026-10-11T10:47:0{i}Z",
            "grant_file": "PRIVATE_PATH",
            "token": "PRIVATE_TOKEN",
        }
        for i in range(6)
    ]
    # A role is not a session id. Preserve an absent session as unknown.
    del events[0]["session"]
    events[0]["ts"] = "2026-10-11T12:47:00+02:00"
    events.append({"kind": "unrelated", "token": "PRIVATE_TOKEN"})
    ledger.write_text("\n".join(json.dumps(e) for e in events) + "\n")
    config = tmp_path / "config.toml"
    config.write_text(f'grant_dir = "{grants}"\ngrant_honor_ledger = "{ledger}"\n')
    monkeypatch.setenv("REINS_CONFIG", str(config))
    monkeypatch.delenv("REINS_GRANT_DIR", raising=False)
    monkeypatch.delenv("REINS_GRANT_HONOR_LEDGER", raising=False)
    before = {
        p: (p.read_bytes(), p.stat().st_mtime_ns)
        for p in tmp_path.rglob("*")
        if p.is_file()
    }
    # The child installs a process-wide audit oracle after imports/client startup.
    # It catches attempted writes (even same-byte writes), key reads, subprocesses,
    # and imports of the grant decision surface, before any effect can occur.
    script = r"""
import json, os, sys
from pathlib import Path
from fastapi.testclient import TestClient
from reins_read import build_app, instance_config
cfg = instance_config()
with TestClient(build_app("", [], cfg)) as client:
    active = True
    violations = []
    def audit(event, args):
        if not active:
            return
        forbidden = event in {"os.remove", "os.rename", "os.mkdir", "os.rmdir",
                              "os.chmod", "os.utime", "os.link", "os.symlink",
                              "subprocess.Popen", "os.system", "socket.connect"}
        if event == "open":
            name, mode, flags = args
            forbidden |= bool(flags & (os.O_WRONLY | os.O_RDWR | os.O_CREAT | os.O_TRUNC | os.O_APPEND))
            forbidden |= isinstance(name, (str, bytes)) and os.fsdecode(name).endswith("grant-key")
        if event == "import":
            forbidden |= "coord_capabilities" in args[0] or "coord_grant" in args[0]
        if forbidden:
            violations.append(event)
            raise AssertionError("read projection attempted forbidden effect: " + event)
    sys.addaudithook(audit)
    try:
        response = client.get("/read/grant-consumption")
    finally:
        active = False
    assert not violations, violations
    assert response.status_code == 200, response.text
    print(json.dumps(response.json()))
"""
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1")
    result = subprocess.run(
        [sys.executable, "-c", script],
        cwd=Path(__file__).parent,
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    payload = json.loads(result.stdout)
    assert payload["dark"] is False
    assert payload["authority"] == "metadata-only"
    rows = {r["grant_id"]: r for r in payload["grants"]}
    row = rows["aaaaaaaa"]
    assert row["minting_lane"] == "lane-mint"
    assert row["minting_session"] == "session-mint"
    assert row["binding"] == {"role": "lane-mint", "session": "session-mint"}
    assert row["lease"]["state"] == "expired"
    assert row["lease"]["issued_at"] == "2026-10-11T11:00:00Z"
    assert row["lease"]["expires_at"] == "2026-10-11T12:00:00Z"
    assert row["honor_count"] == 6 == len(row["honors"])
    assert [(h["session"], h["ts"]) for h in row["honors"]] == [
        (None if i == 0 else "session-use", f"2026-10-11T10:47:0{i}Z") for i in range(6)
    ]
    assert all(h["role"] == "lane-use" for h in row["honors"])
    assert rows["bbbbbbbb"]["minting_lane"] is None
    assert rows["bbbbbbbb"]["minting_session"] is None
    assert rows["bbbbbbbb"]["binding"] == {}
    assert rows["bbbbbbbb"]["honor_count"] == 0
    assert row["air"]["honors"] == "deny"
    assert "PRIVATE_" not in json.dumps(payload)
    assert {
        p: (p.read_bytes(), p.stat().st_mtime_ns)
        for p in tmp_path.rglob("*")
        if p.is_file()
    } == before
    assert instance_config()["grant_dir"] == str(grants)


@pytest.mark.parametrize(
    "fault", ["unconfigured", "grant-dir", "honor-ledger", "malformed"]
)
def test_missing_grant_source_is_typed_unavailable(tmp_path, fault):
    grants = tmp_path / "grants"
    grants.mkdir()
    ledger = tmp_path / "honors.jsonl"
    ledger.write_text("")
    cfg = {"grant_dir": str(grants), "grant_honor_ledger": str(ledger)}
    if fault == "unconfigured":
        cfg = {}
    elif fault == "grant-dir":
        cfg["grant_dir"] = str(tmp_path / "missing")
    elif fault == "honor-ledger":
        cfg["grant_honor_ledger"] = str(tmp_path / "missing.jsonl")
    else:
        (grants / "bad.grant").write_text('{"token": "PRIVATE_TOKEN"')
    with TestClient(build_app("", [], cfg)) as client:
        response = client.get("/read/grant-consumption")
    assert response.status_code == 200
    payload = response.json()
    assert payload["dark"] is True
    assert payload["error"] == "grant-ledger-unavailable"
    assert payload["grants"] == []
    assert "PRIVATE_" not in json.dumps(payload)
