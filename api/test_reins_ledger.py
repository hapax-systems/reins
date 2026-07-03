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


def test_durable_success_dedup_across_reload(tmp_path):
    # idempotency dedups on terminal SUCCESS and survives a restart: a demand + an ok verdict, then a
    # FRESH ledger over the same file rebuilds the succeeded-set, so a replayed key is a duplicate that
    # replays the original success — NOT a re-run.
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t")
    first = led.record_demand("dispatch", "lane-a", "idem-42")
    assert first["duplicate"] is False
    led.record_verdict(first["event_id"], "ok", 200)

    reloaded = CommandLedger(p, clock=lambda: "t")  # simulate a process restart
    replay = reloaded.record_demand("dispatch", "lane-a", "idem-42")
    assert replay["duplicate"] is True and replay["event_id"] == first["event_id"]
    assert replay["prior_http"] == 200  # replays the original success outcome
    # no SECOND demand row appended for the succeeded key (dedup, not re-demand).
    assert len([r for r in reloaded.rows() if r["kind"] == "demand"]) == 1


def test_valid_json_nonobject_line_is_skipped_not_crashed(tmp_path):
    # a valid-JSON-but-non-object line (null/true/42/"s"/[...]) must be skipped on reload AND read,
    # never an AttributeError on row.get — a single such line otherwise bricked the whole composed app.
    p = str(tmp_path / "commands.jsonl")
    with open(p, "w") as f:
        for junk in ("null", "true", "42", '"a string"', "[1,2,3]"):
            f.write(junk + "\n")
        f.write('{"kind":"demand","event_id":"e1","receipt_id":"r1"}\n')
    led = CommandLedger(p, clock=lambda: "t")  # must NOT raise
    rows = led.rows()  # must NOT raise
    assert all(isinstance(r, dict) for r in rows)
    assert len(rows) == 1 and rows[0]["event_id"] == "e1"
    # read_commands over the same file must not 500
    proj = read_commands(p, allowlist=["verb", "status"])
    assert proj["dark"] is False


def test_retry_before_success_is_not_a_duplicate(tmp_path):
    # a demand whose verdict never reached success is RETRYABLE across a reload — not a duplicate.
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t")
    led.record_demand("dispatch", "lane-a", "idem-99")
    led.record_verdict(led.record_demand("dispatch", "lane-a", "idem-99")["event_id"], "not-wired", 501)
    reloaded = CommandLedger(p, clock=lambda: "t")
    retry = reloaded.record_demand("dispatch", "lane-a", "idem-99")
    assert retry["duplicate"] is False  # never succeeded -> retryable, no fabricated duplicate


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


# --- tamper-evidence: the signed hash-chain (avsdlc-receipt-integrity / closes G8) ---

_KEY = b"deterministic-test-key-for-hmac!"


def test_chain_verifies_and_rows_are_linked_and_signed(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t", key=_KEY)
    d = led.record_demand("claim", "cc-task-x", "idem-1")
    led.record_verdict(d["event_id"], "ok", 200)
    rows = led.rows()
    assert all({"prev", "hash", "sig"} <= set(r) for r in rows)  # every row is chained + signed
    assert rows[0]["prev"] == ""  # genesis
    assert rows[1]["prev"] == rows[0]["hash"]  # linked to the prior row
    ok, idx, reason = led.verify_chain()
    assert ok and idx == 0 and reason == "ok"  # idx = first-signed index (genesis at 0)


def test_chain_detects_content_tampering(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t", key=_KEY)
    d = led.record_demand("claim", "cc-task-x", "idem-1")
    led.record_verdict(d["event_id"], "ok", 200)
    rows = [json.loads(line) for line in open(p) if line.strip()]
    rows[0]["target"] = "cc-task-EVIL"  # alter content, keep the now-stale hash/sig
    with open(p, "w") as f:
        for r in rows:
            f.write(json.dumps(r, sort_keys=True, separators=(",", ":")) + "\n")
    ok, idx, reason = CommandLedger(p, clock=lambda: "t", key=_KEY).verify_chain()
    assert not ok and idx == 0 and reason == "hash-mismatch"


def test_chain_detects_deletion(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t", key=_KEY)
    d1 = led.record_demand("claim", "a", "i1")
    led.record_verdict(d1["event_id"], "ok", 200)
    led.record_demand("claim", "b", "i2")
    rows = [json.loads(line) for line in open(p) if line.strip()]
    del rows[1]  # excise the middle row -> the next row's prev-link no longer matches
    with open(p, "w") as f:
        for r in rows:
            f.write(json.dumps(r, sort_keys=True, separators=(",", ":")) + "\n")
    ok, _idx, reason = CommandLedger(p, clock=lambda: "t", key=_KEY).verify_chain()
    assert not ok and reason == "broken-link"


def test_chain_detects_forgery_without_key(tmp_path):
    import hashlib

    p = str(tmp_path / "commands.jsonl")
    CommandLedger(p, clock=lambda: "t", key=_KEY).record_demand("claim", "a", "i1")
    rows = [json.loads(line) for line in open(p) if line.strip()]
    forged = {"kind": "demand", "event_id": "x", "target": "evil", "prev": rows[-1]["hash"]}
    forged["hash"] = hashlib.sha256(
        json.dumps(forged, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    forged["sig"] = "0" * 64  # a forger with write access but not the key cannot compute a valid HMAC
    with open(p, "a") as f:
        f.write(json.dumps(forged, sort_keys=True, separators=(",", ":")) + "\n")
    ok, _idx, reason = CommandLedger(p, clock=lambda: "t", key=_KEY).verify_chain()
    assert not ok and reason == "bad-signature"


def test_prechain_prefix_legal_but_gap_after_start_is_tampering(tmp_path):
    # backward-compat: rows written before signing (no hash) are a legal LEADING prefix.
    p = str(tmp_path / "commands.jsonl")
    with open(p, "w") as f:
        f.write(json.dumps({"kind": "demand", "event_id": "old"}, sort_keys=True, separators=(",", ":")) + "\n")
    led = CommandLedger(p, clock=lambda: "t", key=_KEY)  # head stays "" over the pre-chain prefix
    led.record_demand("claim", "a", "i1")  # first hashed row: genesis prev == ""
    ok, _idx, reason = led.verify_chain()
    assert ok and reason == "ok"
    with open(p, "a") as f:  # an UNhashed row injected AFTER the chain began is tampering
        f.write(json.dumps({"kind": "demand", "event_id": "sneak"}, sort_keys=True, separators=(",", ":")) + "\n")
    ok2, _i2, reason2 = CommandLedger(p, clock=lambda: "t", key=_KEY).verify_chain()
    assert not ok2 and reason2 == "unhashed-row-in-chain"


def test_read_commands_surfaces_integrity(tmp_path):
    # no injected key -> the ledger creates the sibling 0600 key; read_commands re-opens with the SAME
    # key, so a clean chain reports verified (honest integrity, never faked green).
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t")
    d = led.record_demand("claim", "cc-task-x", "idem-1")
    led.record_verdict(d["event_id"], "ok", 200)
    assert read_commands(p, allowlist=[])["integrity"] == "verified"


def test_verdict_records_applied_effect_in_the_signed_record(tmp_path):
    # the APPLY half: a REAL-write verdict (effect.applied True) carries the transport's effect, covered
    # by the hash/sig; /read/commands surfaces only a safe `applied` indicator (never the effect CONTENT).
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t", key=_KEY)
    d = led.record_demand("dispatch", "lane-a", "idem-1")
    led.record_verdict(
        d["event_id"], "ok", 200,
        effect={"receipt_id": "r-123", "event_seq": 7, "fold_delta": "dispatched", "spooled": False, "applied": True},
    )
    verdict = [r for r in led.rows() if r["kind"] == "verdict"][0]
    assert verdict["effect"]["receipt_id"] == "r-123"  # the applied effect IS in the record
    ok, _i, _r = led.verify_chain()
    assert ok  # the effect field is inside the hashed+signed body
    cmd = read_commands(p, allowlist=[])["commands"][0]
    assert cmd["applied"] == "yes"
    assert "fold_delta" not in cmd and "receipt_id" not in cmd  # content stays in the ledger, not the datom


def test_preview_receipt_never_reads_applied(tmp_path):
    # a preview returns a receipt but writes NOTHING (effect.applied False) — it must NEVER read applied.
    # The old receipt-presence inference was a false-green; applied is the writer's assertion, not implied.
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t", key=_KEY)
    d = led.record_demand("resume", "lane-a", "idem-2")
    led.record_verdict(
        d["event_id"], "ok", 200,
        effect={"receipt_id": "preview-idem-2", "fold_delta": "would emit ...", "applied": False},
    )
    assert read_commands(p, allowlist=[])["commands"][0]["applied"] == ""  # preview never false-greens


def test_rejected_verdict_applies_nothing(tmp_path):
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t", key=_KEY)
    d = led.record_demand("stage", "cc-task-x", "idem-2")
    led.record_verdict(d["event_id"], "authority-rejected", 403)  # nothing applied
    verdict = [r for r in led.rows() if r["kind"] == "verdict"][0]
    assert verdict["effect"] == {}  # honest empty effect, never fabricated
    assert read_commands(p, allowlist=[])["commands"][0]["applied"] == ""


# --- bypass regressions (the adversarial review found these shipped green in slice 1) ---


def test_strip_all_hashes_reads_unsigned_not_verified(tmp_path):
    # B1: an attacker who strips every `hash` field forges arbitrary history. It must NOT read verified —
    # a chain-less ledger is "unsigned" (untrusted), never green.
    p = str(tmp_path / "commands.jsonl")
    forged = [
        {"kind": "demand", "event_id": "x", "verb": "dispatch", "target": "/etc/shadow"},
        {"kind": "verdict", "event_id": "x", "status": "ok", "http": 200},
    ]
    with open(p, "w") as f:
        for r in forged:
            f.write(json.dumps(r, sort_keys=True, separators=(",", ":")) + "\n")
    ok, _i, reason = CommandLedger(p, clock=lambda: "t", key=_KEY).verify_chain()
    assert not ok and reason == "unsigned"
    assert read_commands(p, allowlist=[])["integrity"] == "unsigned"  # never "verified"


def test_empty_ledger_reads_empty_not_verified(tmp_path):
    # B5: an absent/emptied ledger must not read "verified" ("never green by default").
    p = str(tmp_path / "commands.jsonl")
    open(p, "w").close()
    ok, _i, reason = CommandLedger(p, clock=lambda: "t", key=_KEY).verify_chain()
    assert not ok and reason == "empty"
    assert read_commands(p, allowlist=[])["integrity"] == "empty"


def test_short_or_empty_key_is_refused(tmp_path):
    # B2: a 0-byte / short key must HARD-error, never a silent b"" (a forgeable no-op HMAC).
    import pytest

    from reins_ledger import load_ledger_key

    kp = str(tmp_path / "ledger-key")
    open(kp, "w").close()  # 0-byte key
    with pytest.raises(RuntimeError):
        load_ledger_key(kp)
    with open(kp, "wb") as f:
        f.write(b"short")  # < 32 bytes
    with pytest.raises(RuntimeError):
        load_ledger_key(kp)


def test_tail_truncation_is_a_documented_residual(tmp_path):
    # B3: verify_chain alone does NOT detect tail truncation (a valid prefix still verifies). This PINS
    # that honest limitation — the mitigation is chain_head() + row-count published OUT-OF-BAND.
    p = str(tmp_path / "commands.jsonl")
    led = CommandLedger(p, clock=lambda: "t", key=_KEY)
    d1 = led.record_demand("claim", "a", "i1")
    led.record_verdict(d1["event_id"], "ok", 200)
    led.record_demand("claim", "b", "i2")  # the row an attacker will hide
    full_head, full_count = led.chain_head(), led.chain_length()  # the out-of-band anchor pair
    rows = [json.loads(line) for line in open(p) if line.strip()]
    with open(p, "w") as f:  # drop the last row (tail truncation)
        for r in rows[:-1]:
            f.write(json.dumps(r, sort_keys=True, separators=(",", ":")) + "\n")
    truncated = CommandLedger(p, clock=lambda: "t", key=_KEY)
    ok, _i, reason = truncated.verify_chain()
    assert ok and reason == "ok"  # RESIDUAL: the chain alone does NOT catch truncation
    # ...but the OUT-OF-BAND anchor (pre-truncation head + count) catches it:
    aok, areason = truncated.verify_against_anchor(full_head, full_count)
    assert not aok and areason in ("anchor-head-mismatch", "anchor-count-mismatch")
    # a ledger verifies clean against its own current anchor:
    assert truncated.verify_against_anchor(truncated.chain_head(), truncated.chain_length()) == (True, "ok")
