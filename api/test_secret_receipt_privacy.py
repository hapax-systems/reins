"""Secret receipts witness operations without exposing a value fingerprint."""

import hashlib
import json

import pytest
from fastapi.testclient import TestClient

import reins_ledger
import reins_serve
from k0.key_capture import FileStore


@pytest.fixture
def scenario(tmp_path, monkeypatch):
    monkeypatch.setattr(reins_ledger, "iso_utc_now", lambda: "2026-01-01T00:00:00Z")

    def create(name):
        root = tmp_path / name
        root.mkdir()
        ledger = root / "commands.jsonl"
        key = root / "ledger-key"
        key.write_bytes(b"synthetic-ledger-signing-key-00000")
        monkeypatch.setenv("REINS_COMMAND_LEDGER", str(ledger))
        monkeypatch.setenv("REINS_COMMAND_LEDGER_KEY", str(key))
        monkeypatch.setenv("REINS_SECRET_STORE", str(root / "store"))
        return TestClient(reins_serve.build_serve_app("", [])), ledger

    return create


def request(op, value=None, *, identity=None):
    packet = {"kind": "secret", "op": op}
    if value is not None:
        packet["value"] = value
    return {
        "target": "synthetic-entry",
        "authority_packet": packet,
        "preflight_receipt": {},
        "idempotency_key": identity or "synthetic-" + op,
    }


def send(client, op, value=None, **kwargs):
    response = client.post("/command/secret", json=request(op, value, **kwargs))
    assert response.status_code == 200
    return response.json()


@pytest.mark.parametrize("operation", ["put", "get", "delete"])
def test_receipt_and_persisted_effect_have_no_value_digest(scenario, operation):
    client, ledger = scenario(operation)
    value = "synthetic-private-entry"
    result = send(client, "put", value)
    if operation != "put":
        result = send(client, operation)
    assert result["fold_delta"] is None
    rows = [json.loads(line) for line in ledger.read_text().splitlines()]
    effects = [row["effect"] for row in rows if row["kind"] == "verdict"]
    assert effects and all(effect["fold_delta"] is None for effect in effects)
    digest = hashlib.sha256(value.encode()).hexdigest()
    assert digest not in json.dumps(result) + ledger.read_text()


def test_known_guess_has_no_value_or_length_confirmation_signal(scenario):
    observations = []
    for index, value in enumerate(("synthetic-short", "synthetic-different-and-longer")):
        client, ledger = scenario(str(index))
        put = send(client, "put", value)
        send(client, "get")
        deleted = send(client, "delete")
        observations.append((put, deleted, ledger.read_bytes()))
    # Same public command identity, clock and signing key; changing only the
    # private bytes cannot change receipts or even the signed ledger bytes.
    assert observations[0] == observations[1]


def test_reload_replay_and_audit_work_with_signed_null_effect(scenario, monkeypatch):
    client, ledger = scenario("replay")
    first = send(client, "put", "synthetic-original")
    before = ledger.read_bytes()
    client.close()
    reloaded = TestClient(reins_serve.build_serve_app("", []))

    def refuse_write(*args, **kwargs):
        pytest.fail("replay invoked the secret transport again")

    monkeypatch.setattr(FileStore, "put", refuse_write)
    replay = send(reloaded, "put", "synthetic-replacement")
    assert replay["status"] == "idempotent-replay"
    assert replay["event_id"] == first["event_id"] and replay["duplicate"] is True
    assert ledger.read_bytes() == before
    assert first["fold_delta"] is None
    projection = reins_ledger.read_commands(str(ledger))
    assert projection["integrity"] == "verified"
    assert len(projection["commands"]) == 1
    command = projection["commands"][0]
    assert (command["status"], command["applied"], command["witness"]) == (
        "ok", "yes", "pending"
    )
    assert command["event_id"] == first["event_id"] and "fold_delta" not in command
    chain = reins_ledger.CommandLedger(str(ledger))
    assert chain.verify_chain() == (True, 0, "ok")
