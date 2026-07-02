"""U3 command-ledger tests — the witnessed frontdoor (CP-A)."""

import json
import os

from reins_ledger import (
    CommandLedger,
    CommandRefs,
    canonical_event_id,
    read_commands,
)


def test_canonical_event_id_no_delimiter_collision():
    # canonical JSON keys the id — two field sets that a delimiter-concat would collide must differ.
    a = canonical_event_id("dispatch", "task:a", "k")
    b = canonical_event_id("dispatch", "task", "a:k")  # a naive "verb|target|key" join would tie a & b
    assert a != b
    # stable + order-independent (canonical json sorts keys)
    assert canonical_event_id("dispatch", "t", "k") == canonical_event_id("dispatch", "t", "k")


def test_demand_then_verdict_rows(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "2026-07-02T03:00:00Z")
    d = led.record_demand("claim", "cc-task-x", "idem-1", CommandRefs(task_id="cc-task-x"))
    assert d["duplicate"] is False and d["kind"] == "demand"
    led.record_verdict(d["event_id"], "ok", 200)
    rows = led.rows()
    assert [r["kind"] for r in rows] == ["demand", "verdict"]
    assert rows[0]["refs"]["task_id"] == "cc-task-x"
    assert rows[1]["witness"] == "pending"  # spine echo not yet wired (U7/SA-3)


def test_durable_idempotency_across_reload(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t")
    first = led.record_demand("dispatch", "lane-a", "idem-42")
    assert first["duplicate"] is False

    # a FRESH ledger over the same file (simulating a process restart) must rebuild the seen-set.
    reloaded = CommandLedger(p, clock=lambda: "t")
    replay = reloaded.record_demand("dispatch", "lane-a", "idem-42")
    assert replay["duplicate"] is True
    assert replay["event_id"] == first["event_id"]
    # exactly ONE demand row on disk despite the replay.
    demand_rows = [r for r in reloaded.rows() if r["kind"] == "demand"]
    assert len(demand_rows) == 1


def test_corrupt_line_is_skipped_not_crashed(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    with open(p, "w") as f:
        f.write("not json\n")
        f.write(json.dumps({"kind": "demand", "event_id": "e1", "receipt_id": "r1"}) + "\n")
    led = CommandLedger(p, clock=lambda: "t")  # must not raise on the corrupt line
    # the valid demand still registers for idempotency
    again = led.record_demand("v", "t", "k")  # different id, appends fine
    assert again["duplicate"] is False


def test_read_commands_projection_honest_witness_and_absent_enforcement(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "2026-07-02T03:00:00Z")
    d = led.record_demand("approve", "cc-task-y", "idem-9", CommandRefs(task_id="cc-task-y"))
    led.record_verdict(d["event_id"], "ok", 200)

    proj = read_commands(p, allowlist=["verb", "status"])
    assert proj["dark"] is False
    # enforcement is ABSENT, never dark (the gate does not exist until U13).
    assert proj["enforcement"] == "absent"
    assert len(proj["commands"]) == 1
    cmd = proj["commands"][0]
    assert cmd["verb"] == "approve" and cmd["status"] == "ok"
    assert cmd["witness"] == "pending"  # honest: spine echo not wired
    assert cmd["task_id"] == "cc-task-y"
    # AIR default-deny: allowlisted fields ok, others deny.
    assert cmd["air"]["verb"] == "ok" and cmd["air"]["status"] == "ok"
    assert cmd["air"]["target"] == "deny" and cmd["air"]["command_id"] == "deny"


def test_read_commands_pending_when_no_verdict(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t")
    led.record_demand("dispatch", "lane-z", "idem-1")  # demand only, no verdict
    proj = read_commands(p)
    assert proj["commands"][0]["status"] == "pending"  # honest, not fabricated ok


def test_missing_ledger_is_empty_not_dark(tmp_path):
    proj = read_commands(str(tmp_path / "nope.jsonl"))
    assert proj["dark"] is False and proj["commands"] == [] and proj["enforcement"] == "absent"
    assert not os.path.exists(str(tmp_path / "nope.jsonl"))


def test_command_target_denied_on_air_under_production_allowlist(tmp_path):
    # H1 regression: a command target is path-class (design pack §9) and MUST deny on air even under
    # the PRODUCTION allowlist — the generic facet allowlist classifies `target` (EDGES) as ok, which
    # would leak a path on the derived channel. The earlier toy-allowlist test masked this.
    import facet_registry

    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t")
    d = led.record_demand("claim", "/home/x/projects/secret-worktree", "idem-air",
                          CommandRefs(task_id="cc-task-x", route_id="r1"))
    led.record_verdict(d["event_id"], "ok", 200)

    proj = read_commands(p, allowlist=facet_registry.air_allowlist())
    cmd = proj["commands"][0]
    assert cmd["air"]["target"] == "deny", "command target must NEVER air (path-class §9)"
