"""Thin READ service: folds the live coord ledger (via the council spine) into scored,
air-classified events. Engine code; the council root + ledger come from config (instance)."""
from __future__ import annotations
import hashlib
import json
import math
import os
import re
import time
from pathlib import Path
from typing import Any
from urllib.parse import quote
from fastapi import FastAPI

import facet_registry  # the facet-cut SSOT: derives the on-air AIR allowlist + serves /read/facets
import reins_context  # the tri-audience context-fact-bundle projection engine (convergence major-system #1)

_ROUTE_BINDING_TAIL_BYTES = 4 * 1024 * 1024

_KIND_SEVERITY = {  # kind -> base severity in [0,1]; escalations rank above routine
    "review.fail": 0.95, "session.ended": 0.4, "pr.merged": 0.6,
    "task.closed": 0.55, "stage": 0.45, "status": 0.4,
    # live spine kinds (verified against ~/.cache/hapax/coord/ledger.db)
    "coord_dispatch.launch_failed": 0.7, "coord_dispatch.launch_succeeded": 0.5,
    "coord_dispatch.launch_started": 0.4, "sdlc.stage_transition": 0.5,
    "sdlc.authorization_flip": 0.65, "task.claim": 0.45,
}


def _kind(raw: dict) -> str:
    t = str(raw.get("event_type", raw.get("type", raw.get("kind", "event"))))
    return t.split("coord.", 1)[-1]  # coord.pr.merged -> pr.merged


def _age_s(ts_str: str, now: float) -> float:
    """Age in seconds; UNKNOWN (missing/unparseable ts) is +inf, never 0.0 — an unknown
    timestamp must not fabricate max recency/freshness (honest-when-starved: the derived
    recency/freshness collapse to 0.0, the starved direction, instead of minting 1.0)."""
    if not ts_str:
        return float("inf")
    try:
        from datetime import datetime
        dt = datetime.fromisoformat(ts_str.replace("Z", "+00:00"))
        return max(0.0, now - dt.timestamp())
    except Exception:
        return float("inf")


def score_event(ev: dict, age_s: float) -> float:
    sev = _KIND_SEVERITY.get(_kind(ev), 0.3)
    recency = 1.0 / (1.0 + age_s / 60.0)  # decays over minutes
    return round(min(1.0, 0.5 * sev + 0.5 * recency), 3)


def classify_air(fields: dict, allowlist: list[str]) -> dict:
    return {k: ("ok" if k in allowlist else "deny") for k in fields}


def to_event(raw: dict, allowlist: list[str], age_s: float) -> dict:
    kind = _kind(raw)
    payload = raw.get("payload") or {}
    fields = {
        "ts": str(raw.get("timestamp", raw.get("ts", ""))), "kind": kind,
        "subject": str(raw.get("subject", "")), "actor": str(raw.get("actor", "")),
        "summary": str(payload.get("summary", payload.get("reason", ""))),
    }
    return {**fields, "score": score_event(raw, age_s), "air": classify_air(fields, allowlist)}


_TURN_SKELETON_AIR = {"ts", "role", "kind", "prov", "magnitude", "model", "route", "gate"}
_TURN_FIXTURES: dict[str, list[dict[str, Any]]] = {
    # Fixture/replay receipts until the CapabilityIO live turn feed is gated in. These are already
    # typed Turn receipts (not raw JSONL): the endpoint only pages and AIR-classifies them.
    "cc-reins": [
        {
            "ts": "2026-06-26T18:40:01Z",
            "role": "cc-reins",
            "kind": "user",
            "prov": "operator",
            "summary": "fixture operator request body",
            "magnitude": 0.2,
            "model": "—",
            "route": "operator.input",
            "gate": "",
        },
        {
            "ts": "2026-06-26T18:40:05Z",
            "role": "cc-reins",
            "kind": "assistant",
            "prov": "model",
            "summary": "fixture assistant response body",
            "magnitude": 0.3,
            "model": "fugu",
            "route": "codex.exec",
            "gate": "pass",
        },
        {
            "ts": "2026-06-26T18:40:07Z",
            "role": "cc-reins",
            "kind": "tool_call",
            "prov": "structured",
            "summary": "fixture tool invocation body",
            "magnitude": 0.5,
            "model": "fugu",
            "route": "codex.exec",
            "gate": "pass",
        },
    ],
}


def to_turn(raw: dict, allowlist: list[str]) -> dict:
    """Normalize one fixture/replay row into the typed Turn receipt contract.

    The session turn ladder is bimodal: skeleton facets air, body text never does. ``prov`` is a
    provenance channel and can be further clamped by source class; operator/untrusted receipts stay
    shape-only even if an instance allowlist opts the field name in later.
    """
    try:
        magnitude = float(raw.get("magnitude", 0.0) or 0.0)
    except (TypeError, ValueError):
        magnitude = 0.0
    fields = {
        "ts": str(raw.get("ts", "")),
        "role": str(raw.get("role", "")),
        "kind": str(raw.get("kind", "")),
        "prov": str(raw.get("prov", "model") or "model"),
        "summary": str(raw.get("summary", "")),
        "magnitude": max(0.0, min(1.0, magnitude)),
        "model": str(raw.get("model", "")),
        "route": str(raw.get("route", "")),
        "gate": str(raw.get("gate", "")),
    }
    # Keep using the same AIR mechanism, with the turn contract's structural skeleton pinned on-air.
    air = classify_air(fields, sorted(set(allowlist) | _TURN_SKELETON_AIR))
    air["summary"] = "deny"  # body text is never on-air allowlisted
    if fields["prov"] in {"operator", "untrusted"}:
        air["prov"] = "deny"
    return {**fields, "air": air}


def _stage_num(stage: str) -> int:
    m = re.match(r"S(\d+)", stage or "")
    return int(m.group(1)) if m else -1


def _predicted_stage(stage: str, no_go: dict) -> str:
    """Expected next state from the SDLC ladder — held if a release stage isn't release-authorized,
    shipped at terminal, else S(n+1). Derived, never fabricated."""
    n = _stage_num(stage)
    if n < 0:
        return ""
    if isinstance(no_go, dict) and n >= 7 and no_go.get("release_authorized") is False:
        return "hold"
    return "ship" if n >= 11 else f"S{n + 1}"


def _criticality(stage: str, no_go: dict) -> str:
    if not isinstance(no_go, dict):
        return "ok"
    n = _stage_num(stage)
    blk = sum(
        1
        for k in ("implementation_authorized", "source_mutation_authorized", "docs_mutation_authorized")
        if no_go.get(k) is False
    )
    if n >= 7 and no_go.get("release_authorized") is False:
        blk += 1
    return ("ok", "warn", "major", "crit")[min(blk, 3)]


def _freshness(last_ts: str, now: float) -> float:
    return round(math.exp(-_age_s(last_ts, now) / 21600.0), 3)  # tau = 6h


def _task_history(council_root: str) -> dict:
    """One replay pass -> per-task {prior_stage (last transition's from_stage), last_ts, last_actor}."""
    from hapax.spine.coord_event_log import default_event_log

    hist: dict = {}
    for e in default_event_log().replay(fail_open=True).events:
        r = e.to_record()
        tid = r.get("subject")
        if not tid:
            continue
        h = hist.setdefault(tid, {"prior_stage": "", "last_ts": "", "last_actor": ""})
        ts = str(r.get("timestamp", ""))
        if ts >= h["last_ts"]:  # ISO strings sort chronologically
            h["last_ts"], h["last_actor"] = ts, str(r.get("actor", ""))
        if r.get("event_type") == "sdlc.stage_transition":
            fs = (r.get("payload") or {}).get("from_stage")
            if fs:
                h["prior_stage"] = str(fs)
    return hist


def to_task(tid: str, t: dict, allowlist: list[str], hist: dict | None = None, now: float | None = None) -> dict:
    no_go = t.get("no_go") or {}
    stage = str(t.get("stage") or "")
    h = (hist or {}).get(tid, {})
    fields = {
        "task_id": str(t.get("task_id", tid)),
        "stage": stage,
        "authority_case": str(t.get("authority_case") or ""),
        "no_go": ",".join(k for k, v in no_go.items() if v) if isinstance(no_go, dict) else "",
        "prior_stage": str(h.get("prior_stage", "")),       # D6 was (from event log)
        "predicted_stage": _predicted_stage(stage, no_go),  # D7 next (ladder + no_go)
        "owner": str(h.get("last_actor", "")),              # who (last actor)
        "freshness": _freshness(h.get("last_ts", ""), now) if now else 0.0,
        "criticality": _criticality(stage, no_go),          # D4
        "rel_count": 0,  # D2 — no task-edge source yet; structured-silence (honest, not fabricated)
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def to_node(n: dict, allowlist: list[str]) -> dict:
    source_ref_labels = _doc_source_ref_labels(n.get("docs"))
    fields = {
        "id": str(n.get("id", "")),
        "label": str(n.get("label", "")),
        "kind": str(n.get("kind", "")), "layer": str(n.get("layer", "")),
        "status": str(n.get("status", "")), "res": str(n.get("resolution", "")),
        "summary": str(n.get("summary", "")),
        "context": str(n.get("context", "")),
        "docs": _doc_labels(n.get("docs")),
        "hardening_notes": _string_list(n.get("hardening")),
        "aliases": _string_list(n.get("aliases")),
        "tags": _string_list(n.get("tags")),
        "source_refs": _doc_source_ref_summary(source_ref_labels),
        "source_ref_labels": source_ref_labels,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def to_edge(e: dict, allowlist: list[str]) -> dict:
    source_ref_labels = _doc_source_ref_labels(e.get("docs"))
    fields = {
        "id": str(e.get("id", "")),
        "source": str(e.get("source", "")), "target": str(e.get("target", "")),
        "relation": str(e.get("relation", "")), "status": str(e.get("status", "")),
        "layer": str(e.get("layer", "")), "res": str(e.get("resolution", "")),
        "confidence": str(e.get("confidence", "")),
        "summary": str(e.get("summary", "")),
        "docs": _doc_labels(e.get("docs")),
        "source_refs": _doc_source_ref_summary(source_ref_labels),
        "source_ref_labels": source_ref_labels,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _string_list(value: Any) -> str:
    if not isinstance(value, list):
        return ""
    return " · ".join(str(item) for item in value if str(item).strip())


def _doc_labels(value: Any) -> str:
    if not isinstance(value, list):
        return ""
    labels: list[str] = []
    for item in value:
        if isinstance(item, dict):
            label = str(item.get("label") or "").strip()
            if label:
                labels.append(label)
        else:
            label = str(item).strip()
            if label:
                labels.append(label)
    return ", ".join(labels)


def _doc_source_ref_labels(value: Any, limit: int = 8) -> list[str]:
    if not isinstance(value, list):
        return []
    out: list[str] = []
    for item in value:
        refs: list[Any] = []
        if isinstance(item, dict):
            raw_refs = item.get("source_refs") or item.get("evidence_refs") or item.get("refs")
            if isinstance(raw_refs, list):
                refs.extend(raw_refs)
            elif raw_refs:
                refs.append(raw_refs)
            for key in ("source_ref", "ref", "path"):
                if item.get(key):
                    refs.append(item.get(key))
            if not refs:
                refs.append(item.get("label") or "")
        else:
            refs.append(item)
        for ref in refs:
            label = _source_ref_label(ref)
            if label:
                out.append(label)
            if len(out) >= limit:
                return out
    return out


def _doc_source_ref_summary(labels: list[str]) -> str:
    return f"docs:{len(labels)} refs" if labels else ""


def _dyn_source(source_id: str, status: str, count: int, allowlist: list[str], detail: str = "", age_bucket: str = "", path: str = "") -> dict:
    fields = {
        "id": source_id,
        "status": status,
        "count": count,
        "detail": detail,
        "age_bucket": age_bucket,
        "path": path,
        "privacy": "metadata-only",
        "raw_access": False,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _dyn_row(kind: str, item_id: str, status: str, count: int, allowlist: list[str], detail: str = "", source: str = "", severity: str = "info") -> dict:
    fields = {
        "kind": kind,
        "id": item_id,
        "source": source,
        "status": status,
        "severity": severity,
        "count": count,
        "detail": detail,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _age_float(v: Any) -> float:
    try:
        if v is None:
            return 0.0
        return round(float(v), 1)
    except (TypeError, ValueError):
        return 0.0


def _session_state(lane: dict) -> str:
    if lane.get("stalled"):
        return "stalled"
    if not lane.get("alive"):
        return "offline"
    if lane.get("idle"):
        return "idle"
    return "active"


def _session_blocker(lane: dict, state: str, relay_age_s: float) -> str:
    if state == "stalled":
        return "stalled"
    if state == "offline":
        return "offline"
    if not str(lane.get("session") or ""):
        return "no_session"
    if relay_age_s >= 21600:
        return "stale_relay"
    if not str(lane.get("claimed_task") or ""):
        return "no_claim"
    return "none"


def _session_readiness(lane: dict, state: str, blocker: str) -> str:
    if blocker == "stale_relay":
        return "stale"
    if state == "stalled":
        return "stall"
    if state == "offline":
        return "off"
    if str(lane.get("claimed_task") or ""):
        return "claim"
    if state == "idle":
        return "idle"
    if state == "active":
        return "live"
    return "unknown"


def _session_attention(lane: dict, state: str, readiness: str, blocker: str, output_age_s: float, relay_age_s: float) -> float:
    base = {
        "stall": 0.95,
        "claim": 0.88,
        "live": 0.74,
        "stale": 0.62,
        "idle": 0.48,
        "off": 0.08,
        "unknown": 0.20,
    }.get(readiness, 0.20)
    if str(lane.get("claimed_task") or "") and readiness != "claim":
        base += 0.06
    if state in {"active", "stalled"} and 0 < output_age_s < 300:
        base += 0.04
    if blocker in {"no_session", "stale_relay"}:
        base -= 0.10
    if relay_age_s >= 86400:
        base -= 0.08
    return round(max(0.0, min(1.0, base)), 2)


def _orchestration_ledger_dir(cfg: dict | None = None) -> Path:
    configured = (
        os.environ.get("REINS_ORCHESTRATION_LEDGER_DIR")
        or str((cfg or {}).get("orchestration_ledger_dir") or "")
    )
    if configured:
        return Path(os.path.expanduser(configured))
    return Path.home() / ".cache" / "hapax" / "orchestration"


def _iter_jsonl_tail_bytes(path: Path, max_bytes: int = _ROUTE_BINDING_TAIL_BYTES):
    try:
        size = path.stat().st_size
        with path.open("rb") as fh:
            start = max(0, size - max_bytes)
            fh.seek(start)
            if start > 0:
                fh.readline()
            for raw in fh:
                try:
                    yield json.loads(raw.decode("utf-8"))
                except Exception:
                    continue
    except OSError:
        return


def _route_id_from_record(rec: dict) -> str:
    route_id = str(rec.get("route_id") or rec.get("selected_route_id") or rec.get("dimensional_selected_route_id") or "")
    if route_id:
        return route_id
    platform = str(rec.get("platform") or "")
    mode = str(rec.get("mode") or "")
    profile = str(rec.get("profile") or "")
    if platform and mode and profile:
        return f"{platform}.{mode}.{profile}"
    return ""


def _route_binding_ref(rec: dict, source_name: str) -> str:
    return f"{source_name}:{rec.get('decision_id') or rec.get('route_decision_id') or rec.get('created_at') or rec.get('timestamp') or 'record'}"


def _put_latest_binding(bindings: dict[tuple[str, str], dict], key: tuple[str, str], rec: dict) -> None:
    prior = bindings.get(key)
    if prior is not None and str(rec.get("created_at") or "") < str(prior.get("created_at") or ""):
        return
    bindings[key] = rec


def _put_receipt_binding(bindings: dict[tuple[str, str], dict], key: tuple[str, str], rec: dict) -> None:
    prior = bindings.get(key)
    if prior is not None and str(prior.get("source_name") or "") == "methodology-dispatch.jsonl":
        if str(rec.get("created_at") or "") < str(prior.get("created_at") or ""):
            return
    bindings[key] = rec


def _task_prefix_match(a: str, b: str) -> bool:
    if not a or not b or a == b:
        return False
    short, long = (a, b) if len(a) < len(b) else (b, a)
    if len(short) < 24 or not long.startswith(short):
        return False
    return len(long) == len(short) or long[len(short)] in "-_."


def _prefix_binding(role: str, claimed_task: str, bindings: dict[tuple[str, str], dict]) -> dict | None:
    candidates: list[dict] = []
    for (lane, task_id), rec in bindings.items():
        if lane != role or not _task_prefix_match(claimed_task, task_id):
            continue
        candidates.append(rec)
    if not candidates:
        return None
    candidates.sort(
        key=lambda rec: (
            1 if str(rec.get("source_name") or "") == "methodology-dispatch.jsonl" else 0,
            str(rec.get("created_at") or ""),
        ),
        reverse=True,
    )
    return candidates[0]


def _route_decision_state(rec: dict) -> str:
    action = str(rec.get("action") or rec.get("decision") or rec.get("route_policy_action") or "")
    if action == "launch" and rec.get("launch_allowed") is not False:
        return "policy_only"
    if action == "refuse":
        return "policy_refused"
    if action:
        return "policy_held"
    return "policy_unknown"


def _methodology_binding_state(rec: dict) -> str:
    action = str(rec.get("route_policy_action") or "")
    if rec.get("ok") is False:
        if action == "refuse":
            return "policy_refused"
        if action and action != "launch":
            return "policy_held"
        return "launch_failed"
    if action == "launch" and rec.get("launched") is True:
        return "bound"
    if action == "launch" and rec.get("durable_mq_dispatch_bound") is False:
        return "mq_unbound"
    if action == "launch":
        return "eligible_not_launched"
    if action == "refuse":
        return "policy_refused"
    if action:
        return "policy_held"
    return "unbound"


def _route_binding_index(cfg: dict | None = None) -> tuple[dict[tuple[str, str], dict], str, Path]:
    ledger_dir = _orchestration_ledger_dir(cfg)
    route_path = ledger_dir / "route-decisions.jsonl"
    methodology_path = ledger_dir / "methodology-dispatch.jsonl"
    if not route_path.exists() and not methodology_path.exists():
        return {}, "source_missing", ledger_dir
    bindings: dict[tuple[str, str], dict] = {}
    route_decisions: dict[str, dict] = {}
    saw_source = False
    for rec in _iter_jsonl_tail_bytes(route_path):
        if not isinstance(rec, dict):
            continue
        saw_source = True
        lane = str(rec.get("lane") or "")
        task_id = str(rec.get("task_id") or "")
        if not lane or not task_id:
            continue
        route_id = _route_id_from_record(rec)
        created_at = str(rec.get("created_at") or rec.get("timestamp") or "")
        decision_id = str(rec.get("decision_id") or rec.get("route_decision_id") or "")
        binding = {
            "route_id": route_id,
            "mode": str(rec.get("mode") or ""),
            "profile": str(rec.get("profile") or ""),
            "platform": str(rec.get("platform") or ""),
            "created_at": created_at,
            "decision_id": decision_id,
            "source_name": route_path.name,
            "route_binding_state": _route_decision_state(rec),
        }
        _put_latest_binding(bindings, (lane, task_id), binding)
        if decision_id:
            route_decisions[decision_id] = {**binding, "lane": lane, "task_id": task_id}
    for rec in _iter_jsonl_tail_bytes(methodology_path):
        if not isinstance(rec, dict):
            continue
        saw_source = True
        lane = str(rec.get("lane") or "")
        task_id = str(rec.get("task_id") or "")
        if not lane or not task_id:
            continue
        route_id = _route_id_from_record(rec)
        created_at = str(rec.get("timestamp") or rec.get("created_at") or "")
        decision_id = str(rec.get("route_decision_id") or rec.get("decision_id") or "")
        route_decision = route_decisions.get(decision_id)
        if route_decision:
            lane = str(route_decision.get("lane") or lane)
            task_id = str(route_decision.get("task_id") or task_id)
        binding = {
            "route_id": route_id or str((route_decision or {}).get("route_id") or ""),
            "mode": str(rec.get("mode") or (route_decision or {}).get("mode") or ""),
            "profile": str(rec.get("profile") or (route_decision or {}).get("profile") or ""),
            "platform": str(rec.get("platform") or (route_decision or {}).get("platform") or ""),
            "created_at": created_at,
            "decision_id": decision_id,
            "source_name": methodology_path.name,
            "route_binding_state": _methodology_binding_state(rec),
        }
        if route_decision:
            _put_receipt_binding(bindings, (lane, task_id), binding)
        else:
            prior = bindings.get((lane, task_id))
            if prior is None or (
                (str(prior.get("platform") or "") == "" or str(prior.get("platform") or "") == binding["platform"])
                and (str(prior.get("route_id") or "") == "" or str(prior.get("route_id") or "") == binding["route_id"])
            ):
                _put_receipt_binding(bindings, (lane, task_id), binding)
    return bindings, "observed" if saw_source else "source_unreadable", ledger_dir


def _session_route_binding(name: str, lane: dict, bindings: dict[tuple[str, str], dict], source_state: str, source_path: Path) -> dict:
    role = str(lane.get("role") or name)
    claimed_task = str(lane.get("claimed_task") or "")
    base = {
        "route_id": "",
        "mode": "",
        "profile": "",
        "route_binding_state": "no_claim" if not claimed_task else source_state,
        "route_evidence_ref": "",
    }
    if not claimed_task:
        return base
    if source_state != "observed":
        return base
    rec = bindings.get((role, claimed_task))
    if rec is None:
        rec = _prefix_binding(role, claimed_task, bindings)
    if rec is None:
        base["route_binding_state"] = "unbound"
        return base
    if str(rec.get("platform") or "") and str(lane.get("platform") or "") and str(rec.get("platform")) != str(lane.get("platform")):
        base["route_binding_state"] = "platform_mismatch"
        base["route_evidence_ref"] = _route_binding_ref(rec, str(rec.get("source_name") or source_path.name))
        return base
    return {
        "route_id": str(rec.get("route_id") or ""),
        "mode": str(rec.get("mode") or ""),
        "profile": str(rec.get("profile") or ""),
        "route_binding_state": str(rec.get("route_binding_state") or "unbound"),
        "route_evidence_ref": _route_binding_ref(rec, str(rec.get("source_name") or source_path.name)),
    }


def to_session(name: str, lane: dict, allowlist: list[str], route_binding: dict | None = None) -> dict:
    state = _session_state(lane)
    output_age_s = _age_float(lane.get("output_age_s"))
    relay_age_s = _age_float(lane.get("relay_age_s"))
    blocker = _session_blocker(lane, state, relay_age_s)
    readiness = _session_readiness(lane, state, blocker)
    fields = {
        "role": str(lane.get("role") or name),
        "session": str(lane.get("session") or ""),
        "platform": str(lane.get("platform") or ""),
        "state": state,
        "alive": bool(lane.get("alive")),
        "idle": bool(lane.get("idle")),
        "stalled": bool(lane.get("stalled")),
        "claimed_task": str(lane.get("claimed_task") or ""),
        "output_age_s": output_age_s,
        "relay_age_s": relay_age_s,
        "readiness": readiness,
        "blocker": blocker,
        "attention": _session_attention(lane, state, readiness, blocker, output_age_s, relay_age_s),
    }
    if route_binding:
        fields.update(route_binding)
    return {**fields, "air": classify_air(fields, allowlist)}


def _session_sort_key(s: dict) -> tuple[float, str]:
    return (-float(s.get("attention") or 0.0), str(s.get("role") or ""))


def _session_state_path() -> str:
    return os.path.expanduser(os.environ.get("REINS_COORDINATOR_STATE", "/dev/shm/hapax-coordinator/state.json"))


def _raw_sessions() -> list[tuple[str, dict]]:
    with open(_session_state_path(), encoding="utf-8") as fh:
        state = json.load(fh)
    lanes = state.get("lanes") or {}
    if not isinstance(lanes, dict):
        return []
    return sorted(
        [(str(name), lane) for name, lane in lanes.items() if isinstance(lane, dict)],
        key=lambda x: x[0],
    )


def _iso_mtime(path: Path) -> str:
    try:
        return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(path.stat().st_mtime))
    except OSError:
        return ""


def _evidence_ref(kind: str, path: Path, privacy: str, raw_access: bool = False) -> dict:
    try:
        st = path.stat()
        size = int(st.st_size)
    except OSError:
        size = 0
    return {
        "kind": kind,
        "path": str(path),
        "mtime": _iso_mtime(path),
        "size": size,
        "privacy": privacy,
        "raw_access": raw_access,
    }


def _safe_int(v: Any) -> int:
    try:
        if v is None:
            return 0
        return int(v)
    except (TypeError, ValueError):
        return 0


def _read_json(path: Path) -> dict:
    with path.open(encoding="utf-8") as fh:
        data = json.load(fh)
    return data if isinstance(data, dict) else {}


def _file_sha256(path: Path) -> str:
    try:
        h = hashlib.sha256()
        with path.open("rb") as fh:
            for chunk in iter(lambda: fh.read(1024 * 1024), b""):
                h.update(chunk)
        return "sha256:" + h.hexdigest()
    except OSError:
        return ""


def _path_age_bucket(path: Path) -> str:
    try:
        age = max(0.0, time.time() - path.stat().st_mtime)
    except OSError:
        return "missing"
    if age < 300:
        return "<5m"
    if age < 3600:
        return "<1h"
    if age < 21600:
        return "<6h"
    if age < 86400:
        return "<1d"
    return ">1d"


def _intake_source(source_id: str, path: Path, count: int, status: str, allowlist: list[str]) -> dict:
    fields = {
        "id": source_id,
        "path": str(path),
        "exists": path.exists(),
        "mtime": _iso_mtime(path),
        "age_bucket": _path_age_bucket(path),
        "status": status,
        "count": count,
        "privacy": "metadata-only",
        "raw_access": False,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _intake_row(
    source: str,
    kind: str,
    status: str,
    severity: str,
    count: int,
    allowlist: list[str],
    blocker: str = "",
    coverage: str = "",
    task_link_state: str = "",
    evidence_count: int = 0,
    age_bucket: str = "",
) -> dict:
    row_id = re.sub(r"[^A-Za-z0-9_.:-]+", "_", f"{source}:{kind}").strip("_")
    if not row_id:
        row_id = "intake:unknown"
    evidence = f"count:{count}"
    if evidence_count:
        evidence += f" · refs:{evidence_count}"
    source_refs = source
    authority = _intake_authority(source)
    detail = " · ".join(v for v in [
        f"coverage={coverage}" if coverage else "",
        f"task_link={task_link_state}" if task_link_state else "",
        f"blocker={blocker}" if blocker else "",
    ] if v)
    missing = _intake_missing(status, severity, blocker, task_link_state, count)
    action = _intake_action(severity, blocker, count)
    next_evidence = _intake_next_evidence(missing, task_link_state, blocker)
    fields = {
        "id": row_id,
        "source": source,
        "kind": kind,
        "status": status,
        "severity": severity,
        "count": count,
        "blocker": blocker,
        "coverage": coverage,
        "task_link_state": task_link_state,
        "evidence_count": evidence_count,
        "age_bucket": age_bucket,
        "authority": authority,
        "evidence": evidence,
        "missing": missing,
        "action": action,
        "detail": detail,
        "source_refs": source_refs,
        "next_evidence": next_evidence,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _intake_authority(source: str) -> str:
    if source.startswith("p0_incident"):
        return "incident-observation"
    if source == "security_signal_state":
        return "security-observation"
    if source == "planning_feed":
        return "planning-observation"
    if source == "request_state":
        return "workflow-observation"
    return "observation-only"


def _intake_missing(status: str, severity: str, blocker: str, task_link_state: str, count: int) -> str:
    parts: list[str] = []
    s = f"{status} {severity} {blocker}".lower()
    if count > 0 and severity in {"crit", "major"}:
        parts.append("triage receipt")
    if "stale" in s:
        parts.append("fresh source snapshot")
    if blocker:
        parts.append("blocker resolution")
    if task_link_state and task_link_state not in {"task_visible", "task_link_metadata", "task_metadata"}:
        parts.append("task linkage")
    return " · ".join(parts)


def _intake_action(severity: str, blocker: str, count: int) -> str:
    if count <= 0:
        return "observe"
    if severity == "crit":
        return "triage-critical"
    if severity == "major" or blocker:
        return "triage"
    if severity == "warn":
        return "review"
    return "observe"


def _intake_next_evidence(missing: str, task_link_state: str, blocker: str) -> str:
    if blocker:
        return "attach blocker disposition and governed receipt"
    if "task linkage" in missing:
        return "attach task/claim linkage metadata"
    if missing:
        return "refresh source snapshot and attach evidence receipt"
    if task_link_state:
        return "verify linkage remains fresh"
    return "refresh source snapshot before action"


def _gate_source(source_id: str, status: str, count: int, allowlist: list[str], detail: str = "", age_bucket: str = "", path: str = "") -> dict:
    fields = {
        "id": source_id,
        "status": status,
        "count": count,
        "detail": detail,
        "age_bucket": age_bucket,
        "path": path,
        "raw_access": False,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _gate_row(
    gate_id: str,
    domain: str,
    subject: str,
    state: str,
    severity: str,
    authority: str,
    evidence: str,
    missing: str,
    allowlist: list[str],
    source: str = "",
    action: str = "",
) -> dict:
    fields = {
        "gate_id": gate_id,
        "domain": domain,
        "source": source,
        "subject": subject,
        "state": state,
        "severity": severity,
        "authority": authority,
        "evidence": evidence,
        "missing": missing,
        "action": action,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _read_jsonl_tail(path: Path, limit: int = 1000) -> list[dict]:
    if not path.exists():
        return []
    lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    out: list[dict] = []
    for line in lines[-limit:]:
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(row, dict):
            out.append(row)
    return out


def _intake_paths(cfg: dict) -> dict[str, Path]:
    home = Path.home()
    cache = home / ".cache" / "hapax"
    def cfg_path(key: str, env: str, default: Path) -> Path:
        raw = os.environ.get(env) or str(cfg.get(key, ""))
        return Path(os.path.expanduser(raw)) if raw else default
    return {
        "request_state": cfg_path("request_intake_state", "REINS_REQUEST_INTAKE_STATE", cache / "request-intake-state.json"),
        "planning_feed": cfg_path("planning_feed_state", "REINS_PLANNING_FEED_STATE", cache / "planning-feed-state.json"),
        "p0_incident_state": cfg_path("p0_incident_state", "REINS_P0_INCIDENT_STATE", cache / "p0-incident-intake" / "state.json"),
        "p0_incident_ledger": cfg_path("p0_incident_events", "REINS_P0_INCIDENT_EVENTS", cache / "p0-incident-intake" / "events.jsonl"),
        "security_signal_state": cfg_path("security_signal_state", "REINS_SECURITY_SIGNAL_STATE", cache / "security-signal-intake-state.json"),
    }


def read_intake_summary(cfg: dict, allowlist: list[str]) -> dict:
    """Read bounded intake metadata from durable local snapshots.

    This is intentionally aggregate-first. It does not read Obsidian note bodies, desktop
    notification messages, GitHub URLs, or raw request/task IDs into the visible row model.
    """
    sources: list[dict] = []
    rows: list[dict] = []
    totals: dict[str, int] = {}
    paths = _intake_paths(cfg)

    # Request intake state: counts only.
    p = paths["request_state"]
    if p.exists():
        data = _read_json(p)
        attention = _safe_int(data.get("combined_attention_count"))
        malformed = _safe_int(data.get("malformed_count"))
        unread = _safe_int(data.get("unread_count"))
        stale = _safe_int(data.get("stale_count"))
        count = attention + malformed + unread + stale
        sources.append(_intake_source("request_state", p, count, "observed", allowlist))
        totals["request_attention"] = attention
        totals["request_malformed"] = malformed
        rows.append(_intake_row("request_state", "request_attention", "attention", "warn" if attention else "ok", attention, allowlist, coverage="workflow_attention", age_bucket=_path_age_bucket(p)))
        if malformed:
            rows.append(_intake_row("request_state", "malformed_request", "needs repair", "major", malformed, allowlist, blocker="malformed_frontmatter", coverage="malformed", age_bucket=_path_age_bucket(p)))
    else:
        sources.append(_intake_source("request_state", p, 0, "missing", allowlist))

    # Planning feed: aggregate coverage/staleness buckets, not individual request ids.
    p = paths["planning_feed"]
    if p.exists():
        data = _read_json(p)
        total = _safe_int(data.get("total_requests"))
        attention = len(data.get("attention_required") or []) if isinstance(data.get("attention_required"), list) else _safe_int(data.get("attention_required"))
        sources.append(_intake_source("planning_feed", p, total, "observed", allowlist))
        totals["planning_requests"] = total
        totals["planning_attention"] = attention
        coverage = data.get("coverage_summary") or {}
        if isinstance(coverage, dict):
            for key, val in sorted(coverage.items(), key=lambda kv: str(kv[0])):
                n = _safe_int(val)
                sev = "ok"
                if key in {"untracked", "needs_cctv_hardening"} and n:
                    sev = "major"
                elif n:
                    sev = "warn"
                rows.append(_intake_row("planning_feed", "coverage:"+str(key), "bucket", sev, n, allowlist, coverage=str(key), task_link_state=str(key), age_bucket=_path_age_bucket(p)))
        stale = data.get("stale_summary") or {}
        if isinstance(stale, dict):
            for key, val in sorted(stale.items(), key=lambda kv: str(kv[0])):
                n = _safe_int(val)
                if n:
                    rows.append(_intake_row("planning_feed", "stale:"+str(key), "stale", "warn", n, allowlist, blocker=str(key), coverage="staleness", age_bucket=_path_age_bucket(p)))
        if attention:
            rows.append(_intake_row("planning_feed", "attention_required", "attention", "warn", attention, allowlist, coverage="attention_required", age_bucket=_path_age_bucket(p)))
    else:
        sources.append(_intake_source("planning_feed", p, 0, "missing", allowlist))

    # P0 incident coalesced state: aggregate by incident kind.
    p = paths["p0_incident_state"]
    if p.exists():
        data = _read_json(p)
        incidents = data.get("incidents") or {}
        if not isinstance(incidents, dict):
            incidents = {}
        sources.append(_intake_source("p0_incident_state", p, len(incidents), "observed", allowlist))
        totals["p0_incidents"] = len(incidents)
        by_kind: dict[str, int] = {}
        for inc in incidents.values():
            if isinstance(inc, dict):
                k = str(inc.get("kind") or "unknown")
                by_kind[k] = by_kind.get(k, 0) + 1
        for kind, n in sorted(by_kind.items(), key=lambda kv: (-kv[1], kv[0]))[:10]:
            rows.append(_intake_row("p0_incident_state", "incident:"+kind, "active", "crit", n, allowlist, blocker="p0_incident", coverage="coalesced", task_link_state="task_link_metadata", age_bucket=_path_age_bucket(p)))
    else:
        sources.append(_intake_source("p0_incident_state", p, 0, "missing", allowlist))

    # P0 incident event ledger: recent aggregate counts.
    p = paths["p0_incident_ledger"]
    if p.exists():
        ledger = _read_jsonl_tail(p, 1000)
        sources.append(_intake_source("p0_incident_ledger", p, len(ledger), "observed", allowlist))
        totals["p0_ledger_tail"] = len(ledger)
        by_kind: dict[str, int] = {}
        for item in ledger:
            k = str(item.get("kind") or "unknown")
            by_kind[k] = by_kind.get(k, 0) + 1
        for kind, n in sorted(by_kind.items(), key=lambda kv: (-kv[1], kv[0]))[:6]:
            sev = "crit" if "notification" in kind else "ok"
            rows.append(_intake_row("p0_incident_ledger", kind, "recent", sev, n, allowlist, blocker="durable_eventlog", coverage="tail_1000", age_bucket=_path_age_bucket(p)))
    else:
        sources.append(_intake_source("p0_incident_ledger", p, 0, "missing", allowlist))

    # Security signal snapshot: aggregate by status/kind.
    p = paths["security_signal_state"]
    if p.exists():
        data = _read_json(p)
        reqs = data.get("requests") or []
        if not isinstance(reqs, list):
            reqs = []
        total = _safe_int(data.get("total_signals")) or len(reqs)
        sources.append(_intake_source("security_signal_state", p, total, "observed", allowlist))
        totals["security_signals"] = total
        by_kind: dict[str, int] = {}
        by_status: dict[str, int] = {}
        for req in reqs:
            if not isinstance(req, dict):
                continue
            k = str(req.get("kind") or "unknown")
            s = str(req.get("status") or "unknown")
            by_kind[k] = by_kind.get(k, 0) + 1
            by_status[s] = by_status.get(s, 0) + 1
        for kind, n in sorted(by_kind.items(), key=lambda kv: (-kv[1], kv[0]))[:6]:
            rows.append(_intake_row("security_signal_state", "security:"+kind, "snapshot", "crit", n, allowlist, blocker="security_signal", coverage="snapshot", task_link_state="request_metadata", age_bucket=_path_age_bucket(p)))
        for status, n in sorted(by_status.items(), key=lambda kv: (-kv[1], kv[0]))[:4]:
            rows.append(_intake_row("security_signal_state", "security_status:"+status, status, "warn", n, allowlist, coverage="status", age_bucket=_path_age_bucket(p)))
    else:
        sources.append(_intake_source("security_signal_state", p, 0, "missing", allowlist))

    totals["sources"] = len(sources)
    totals["rows"] = len(rows)
    return {"sources": sources, "rows": rows, "totals": totals}


def _capability_source(source_id: str, path: Path, count: int, status: str, allowlist: list[str], detail: str = "", path_label: str = "") -> dict:
    has_path = bool(str(path))
    fields = {
        "id": source_id,
        "path": path_label or (str(path) if has_path else ""),
        "exists": bool(has_path and path.exists()),
        "mtime": _iso_mtime(path) if has_path else "",
        "age_bucket": _path_age_bucket(path) if has_path else "missing",
        "status": status,
        "count": count,
        "detail": detail,
        "privacy": "metadata-only",
        "raw_access": False,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _count_jsonl_lines(path: Path) -> int:
    try:
        with path.open(encoding="utf-8", errors="replace") as fh:
            return sum(1 for line in fh if line.strip())
    except OSError:
        return 0


def _yaml_scalar(value: str) -> str:
    return value.strip().strip("'\"")


def _read_simple_yaml_doc(path: Path) -> dict[str, Any]:
    """Read simple top-level scalars/lists from HKP metadata without adding YAML deps."""
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return {}
    out: dict[str, Any] = {}
    current: str | None = None
    for raw in lines:
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        if not raw.startswith((" ", "\t")) and ":" in raw:
            key, val = raw.split(":", 1)
            key = key.strip()
            val = val.strip()
            if not val:
                out[key] = []
                current = key
            else:
                out[key] = _yaml_scalar(val)
                current = None
            continue
        if current and raw.lstrip().startswith("-"):
            vals = out.setdefault(current, [])
            if isinstance(vals, list):
                vals.append(_yaml_scalar(raw.lstrip()[1:].strip()))
    return out


def _read_hkp_consumer_defaults(path: Path) -> dict[str, str]:
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return {}
    out: dict[str, str] = {}
    current = ""
    for raw in lines:
        stripped = raw.strip()
        if stripped.startswith("- consumer:"):
            current = _yaml_scalar(stripped.split(":", 1)[1])
        elif current and stripped.startswith("default:"):
            out[current] = _yaml_scalar(stripped.split(":", 1)[1])
    return out


def _hkp_cfg_list(cfg: dict, key: str) -> list[str]:
    raw = cfg.get(key) or []
    if isinstance(raw, list):
        return [str(v).strip() for v in raw if str(v).strip()]
    text = str(raw).strip()
    if not text:
        return []
    if "," in text:
        return [v.strip() for v in text.split(",") if v.strip()]
    return [v for v in text.split(os.pathsep) if v]


def _hkp_support_summary(cfg: dict, allowlist: list[str]) -> tuple[list[dict], int, str, str]:
    shadow_root_raw = str(cfg.get("hkp_shadow_root") or "").strip()
    if not shadow_root_raw:
        return [], 0, "", "HKP cache root not configured"
    shadow_root = Path(os.path.expanduser(shadow_root_raw))
    index_root_raw = str(cfg.get("hkp_index_root") or "").strip()
    report_root_raw = str(cfg.get("hkp_report_root") or "").strip()
    index_root = Path(os.path.expanduser(index_root_raw)) if index_root_raw else Path("")
    report_root = Path(os.path.expanduser(report_root_raw)) if report_root_raw else Path("")
    bundles = _hkp_cfg_list(cfg, "hkp_bundles") or ["sdlc"]

    sources: list[dict] = []
    total_refs = 0
    source_refs: list[str] = []
    blockers: list[str] = []
    required_denials = {"dispatcher", "release_gate", "provider_spend_gate", "public_export", "runtime_loader", "close_gate"}
    for bundle in bundles:
        safe_bundle = re.sub(r"[^A-Za-z0-9_.:-]+", "_", bundle).strip("._") or "bundle"
        bundle_dir = shadow_root / safe_bundle
        hkp_dir = bundle_dir / "_hkp"
        manifest_path = hkp_dir / "manifest.yaml"
        snapshot_path = hkp_dir / "snapshot.json"
        policy_path = hkp_dir / "consumer_policy.yaml"
        events_path = hkp_dir / "events.jsonl"
        edges_path = hkp_dir / "edges.jsonl"
        manifest = _read_simple_yaml_doc(manifest_path)
        snapshot = _read_json(snapshot_path) if snapshot_path.exists() else {}
        policy_defaults = _read_hkp_consumer_defaults(policy_path)

        cache_only = str(manifest.get("cache_only", "")).lower() == "true"
        allowed = set(manifest.get("allowed_consumers") or [])
        forbidden = set(manifest.get("forbidden_consumers") or [])
        denied = {name for name, default in policy_defaults.items() if default == "deny"}
        safe_policy = cache_only and {"research_viewer", "local_prompt_context"}.issubset(allowed) and required_denials.issubset(forbidden | denied)
        concept_count = _safe_int(snapshot.get("concept_count"))
        edge_count = _safe_int(snapshot.get("edge_count"))
        event_count = _count_jsonl_lines(events_path)
        edge_ledger_count = _count_jsonl_lines(edges_path)
        index_path = index_root / f"{safe_bundle}.jsonl" if str(index_root) else Path("")
        index_count = _count_jsonl_lines(index_path) if str(index_path) else 0
        ref_count = max(index_count, concept_count + edge_count, event_count + edge_ledger_count)
        total_refs += ref_count
        if ref_count:
            source_refs.append(f"hkp:{safe_bundle}:{ref_count} refs")
        if not safe_policy:
            blockers.append(f"{safe_bundle}: policy/cache-only guard incomplete")

        status = "support-only" if safe_policy and manifest_path.exists() else "read-missing"
        generated = str(snapshot.get("generated_at") or manifest.get("generated_at") or "")
        detail = "cache-only HKP bundle; concepts=%d edges=%d generated=%s" % (concept_count, edge_count, generated or "unknown")
        sources.append(_capability_source(
            f"hkp_bundle:{safe_bundle}",
            manifest_path,
            ref_count,
            status,
            allowlist,
            detail,
            f"hkp-shadow:{safe_bundle}/_hkp/manifest.yaml",
        ))
        policy_detail = "allowed=research_viewer,local_prompt_context; denied=dispatcher,release_gate,provider_spend_gate,public_export"
        sources.append(_capability_source(
            f"hkp_policy:{safe_bundle}",
            policy_path,
            len(policy_defaults),
            "support-only" if safe_policy else "read-missing",
            allowlist,
            policy_detail,
            f"hkp-shadow:{safe_bundle}/_hkp/consumer_policy.yaml",
        ))
        if str(index_path):
            sources.append(_capability_source(
                f"hkp_index:{safe_bundle}",
                index_path,
                index_count,
                "observed" if index_count else "missing",
                allowlist,
                "derived HKP index metadata only; no concept bodies",
                f"hkp-shadow-index:{safe_bundle}.jsonl",
            ))

    if str(report_root):
        try:
            report_dirs = [p for p in report_root.iterdir() if p.is_dir() and (p / "report.json").exists()]
        except OSError:
            report_dirs = []
        newest = max((p / "report.json" for p in report_dirs), key=lambda p: p.stat().st_mtime, default=Path(""))
        sources.append(_capability_source(
            "hkp_reports",
            newest,
            len(report_dirs),
            "observed" if report_dirs else "missing",
            allowlist,
            "sanitized HKP research-viewer reports; support evidence only",
            "hkp-reports:report.json",
        ))

    blocker = "; ".join(blockers) if blockers else "support-only cache/context; not source truth, dispatch authority, calibration evidence, release authority, provider spend, or public export"
    return sources, total_refs, "; ".join(source_refs), blocker


def _capability_row(
    capability_id: str,
    status: str,
    authority: str,
    route_count: int,
    ok_count: int,
    blocked_count: int,
    evidence_count: int,
    blocker: str,
    hkp_posture: str,
    allowlist: list[str],
    source_refs: str = "",
    source_ref_labels: list[str] | None = None,
) -> dict:
    taxonomy = _capability_taxonomy(capability_id)
    fields = {
        "capability_id": capability_id,
        "status": status,
        "authority": authority,
        **taxonomy,
        "route_count": route_count,
        "ok_count": ok_count,
        "blocked_count": blocked_count,
        "evidence_count": evidence_count,
        "blocker": blocker,
        "hkp_posture": hkp_posture,
        "source_refs": source_refs,
        "source_ref_labels": source_ref_labels or [],
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _capability_route(route: Any, freshness: Any | None, allowlist: list[str]) -> dict:
    route_id = str(getattr(route, "route_id", ""))
    errors = list(getattr(freshness, "errors", ()) or []) if freshness is not None else []
    blocked_reasons = list(getattr(route, "blocked_reasons", ()) or [])
    freshness_ok = bool(getattr(freshness, "ok", False)) if freshness is not None else False
    evidence_count = len(getattr(freshness, "evidence_refs", ()) or [])
    quota_source = str(getattr(getattr(route, "telemetry", None), "quota_source", "unknown"))
    if "." in quota_source:
        quota_source = quota_source.rsplit(".", 1)[-1]
    quota_state = "unknown" if quota_source in {"none", "unknown"} else "observed"
    model_id = ""
    desc = getattr(route, "execution_descriptor", None)
    if desc is not None:
        model_id = str(getattr(desc, "model_id", "") or "")
    if not model_id:
        model_id = str(getattr(route, "model_or_engine", "") or "")
    state = str(getattr(route, "route_state", "") or "")
    if blocked_reasons:
        state = "blocked"
    fields = {
        "route_id": route_id,
        "capability_id": "route_envelope",
        "platform": str(getattr(route, "platform", "") or ""),
        "mode": str(getattr(route, "mode", "") or ""),
        "profile": str(getattr(route, "profile", "") or ""),
        "model_id": model_id,
        "effort": _route_axis(route, desc, ["effort", "reasoning_effort"]),
        "context_mode": _route_axis(route, desc, ["context_mode", "context_window", "context_budget"]),
        "fast_mode": _route_axis(route, desc, ["fast_mode", "fast_path", "low_latency"]),
        "quantization": _route_axis(route, desc, ["quantization", "quant", "precision"]),
        "capacity_pool": _route_axis(route, desc, ["capacity_pool", "pool", "capacity"]),
        "demand_vector": _route_axis(route, desc, ["demand_vector", "demand", "task_shape"]),
        "hardening": _route_axis(route, desc, ["hardening", "hardening_intensity", "request_hardening"]),
        "eval_plane": _route_axis(route, desc, ["eval_plane", "eval", "evaluation_plane"]),
        "review_obligation": _route_axis(route, desc, ["review_obligation", "review", "reviewer_obligation"]),
        "learning_eligibility": _route_axis(route, desc, ["learning_eligibility", "learning", "calibration_eligibility"]),
        "benchmark_coverage": _route_axis(route, desc, ["benchmark_coverage", "benchmark", "coverage"]),
        "fixed_overhead": _route_axis(route, desc, ["fixed_overhead", "overhead", "setup_cost"]),
        "route_state": state,
        "authority_ceiling": str(getattr(getattr(route, "authority_ceiling", ""), "value", getattr(route, "authority_ceiling", "")) or ""),
        "freshness_ok": freshness_ok,
        "quota_state": quota_state,
        "receipt_count": evidence_count,
        "blockers": [*blocked_reasons, *errors[:3]],
        "evidence_count": evidence_count,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _field_value(obj: Any, field: str, default: Any = None) -> Any:
    if isinstance(obj, dict):
        return obj.get(field, default)
    return getattr(obj, field, default)


def _text_value(value: Any) -> str:
    if value is None:
        return ""
    if hasattr(value, "value"):
        value = value.value
    if hasattr(value, "isoformat"):
        return str(value.isoformat()).replace("+00:00", "Z")
    return str(value)


def _bool_value(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    return str(value).strip().lower() in {"1", "true", "yes", "available", "observed"}


def _text_list(value: Any) -> list[str]:
    if value is None:
        return []
    if isinstance(value, str):
        return [value]
    if isinstance(value, dict):
        value = value.values()
    try:
        return [_text_value(v) for v in value]
    except TypeError:
        return [_text_value(value)]


def _axis_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, dict):
        parts = [f"{k}={_axis_text(v) or 'missing'}" for k, v in sorted(value.items())]
        return ",".join(parts)
    if isinstance(value, (list, tuple, set)):
        return ",".join(v for v in (_axis_text(v) for v in value) if v)
    return _text_value(value)


def _route_axis(route: Any, desc: Any, fields: list[str], default: str = "missing") -> str:
    for field in fields:
        val = _field_value(route, field, None)
        text = _axis_text(val)
        if text:
            return text
        if desc is not None:
            val = _field_value(desc, field, None)
            text = _axis_text(val)
            if text:
                return text
    return default


_CAPABILITY_TAXONOMY: dict[str, dict[str, str]] = {
    "route_envelope": {
        "capability_class": "core_route",
        "surface_family": "route_governance",
        "spend_model": "metadata_only",
        "egress_class": "none",
        "receipt_requirement": "route envelope + authority receipt",
    },
    "quota_context": {
        "capability_class": "core_route",
        "surface_family": "quota_context",
        "spend_model": "metadata_only",
        "egress_class": "none",
        "receipt_requirement": "quota/context receipt",
    },
    "route_authority_receipts": {
        "capability_class": "core_route",
        "surface_family": "authority_receipt",
        "spend_model": "metadata_only",
        "egress_class": "none",
        "receipt_requirement": "fresh route-authority receipt",
    },
    "hkp_support_context": {
        "capability_class": "context_support",
        "surface_family": "hkp",
        "spend_model": "none",
        "egress_class": "none",
        "receipt_requirement": "cited-source promotion receipt",
    },
    "source_acquisition": {
        "capability_class": "source_acquisition",
        "surface_family": "source_acquisition",
        "spend_model": "mixed_api_connector",
        "egress_class": "source_query",
        "receipt_requirement": "source, egress, quota, and route receipt",
    },
    "verifier_floor_checker": {
        "capability_class": "verifier_floor_checker",
        "surface_family": "verifier_floor_checker",
        "spend_model": "ci_or_local",
        "egress_class": "verifier_signal",
        "receipt_requirement": "floor-check/verifier receipt",
    },
    "publication_egress": {
        "capability_class": "publication_egress",
        "surface_family": "publication_egress",
        "spend_model": "external_account",
        "egress_class": "public",
        "receipt_requirement": "publication + redaction + operator receipt",
    },
    "audio_avsdlc_tool": {
        "capability_class": "audio_avsdlc_tool",
        "surface_family": "audio_avsdlc_tool",
        "spend_model": "api_or_local",
        "egress_class": "audio_media",
        "receipt_requirement": "consent + quota + media-egress receipt",
    },
    "provider_gateway": {
        "capability_class": "provider_gateway",
        "surface_family": "provider_gateway",
        "spend_model": "api_spend",
        "egress_class": "provider_api",
        "receipt_requirement": "explicit provider-spend route receipt",
    },
    "subscription_tool_surface": {
        "capability_class": "subscription_tool_surface",
        "surface_family": "subscription_tool_surface",
        "spend_model": "subscription",
        "egress_class": "tool_runtime",
        "receipt_requirement": "route envelope + tool-surface receipt",
    },
    "infrastructure_control": {
        "capability_class": "infrastructure_control",
        "surface_family": "infrastructure_control",
        "spend_model": "infra_account",
        "egress_class": "infrastructure",
        "receipt_requirement": "target + destructive preflight + rollback + infra receipt",
    },
}

_CAPABILITY_SURFACE_TAXONOMY: dict[str, tuple[str, str, str, str, str]] = {
    "tavily_source_acquisition": ("source_acquisition", "tavily", "api_spend_budgeted", "source_query", "usage + budget + route receipt"),
    "perplexity_source_acquisition": ("source_acquisition", "perplexity", "api_spend", "source_query", "api-credit + route receipt"),
    "context7_docs_currentness": ("source_acquisition", "docs_currentness", "connector_or_mcp", "docs_query", "docs-source receipt"),
    "google_drive_docs_connector": ("source_acquisition", "google_workspace", "connector", "workspace_connector", "connector authority + egress receipt"),
    "github_repo_ci": ("verifier_floor_checker", "github_repo_ci", "connector", "repo_ci", "governed task/review/queue receipt"),
    "codex_worker_reviewer_surface": ("subscription_tool_surface", "worker_reviewer", "subscription", "coding_cli_session", "cc-task + review/close receipt"),
    "claude_worker_reviewer_surface": ("subscription_tool_surface", "worker_reviewer", "subscription", "coding_cli_session", "cc-task + review/close receipt"),
    "antigravity_agy_tool_surface": ("subscription_tool_surface", "audited_worker_ide", "subscription", "ide_runtime", "agy/Antigravity route receipt"),
    "mistral_vibe_worker_surface": ("subscription_tool_surface", "bounded_worker", "subscription", "coding_cli_session", "route envelope + scope receipt"),
    "glmcp_review_quota_admission": ("verifier_floor_checker", "review_quota_admission", "api_or_subscription", "review_quota_signal", "review/quota/admission receipt"),
    "gemini_agy_support_review": ("subscription_tool_surface", "support_review", "subscription", "tool_runtime", "agy support/review receipt"),
    "glm_coding_plan_tool_surface": ("subscription_tool_surface", "manual_coding_plan", "subscription", "tool_runtime", "manual bakeoff/admission receipt"),
    "fugu_raw_codex": ("subscription_tool_surface", "sakana_fugu", "subscription", "coding_cli_session", "governed route + admission + bakeoff receipt"),
    "fugu_ultra_raw_codex": ("subscription_tool_surface", "sakana_fugu_ultra", "subscription", "coding_cli_session", "governed route + admission + clean-host preflight + bakeoff receipt"),
    "huggingface_provider_gateway": ("provider_gateway", "huggingface", "api_spend", "provider_api", "tier/license/egress receipt"),
    "cohere_embed_rerank": ("source_acquisition", "embed_rerank", "api_spend", "retrieval_api", "model/pricing/use-case receipt"),
    "litellm_provider_gateway": ("provider_gateway", "litellm", "gateway", "provider_api", "route envelope; no silent fallback"),
    "openrouter_break_glass": ("provider_gateway", "openrouter", "break_glass_api_spend", "provider_api", "emergency spend receipt"),
    "semgrep_static_analysis": ("verifier_floor_checker", "static_analysis", "ci_or_local", "security_verifier", "static-analysis receipt"),
    "codecov_coverage_signal": ("verifier_floor_checker", "coverage_signal", "connector", "coverage_verifier", "coverage/floor receipt"),
    "codeql_status_floor": ("verifier_floor_checker", "codeql_status", "ci", "security_verifier", "CodeQL status receipt"),
    "deterministic_test_floor": ("verifier_floor_checker", "deterministic_tests", "local_ci", "test_verifier", "test run receipt"),
    "runtime_witness_floor": ("verifier_floor_checker", "runtime_witness", "local_runtime", "runtime_verifier", "runtime witness receipt"),
    "worker_failure_witness": ("verifier_floor_checker", "failure_classification", "local_runtime", "worker_witness", "failure witness receipt"),
    "langfuse_trace_eval": ("verifier_floor_checker", "trace_eval", "api_or_local", "trace_telemetry", "redaction + trace receipt"),
    "elevenlabs_audio_generation": ("audio_avsdlc_tool", "audio_generation", "api_spend", "audio_public_egress", "consent + quota + audio receipt"),
    "picovoice_audio_verifier": ("audio_avsdlc_tool", "audio_verifier", "api_or_local", "audio_runtime", "audio/runtime receipt"),
    "audio_fingerprint_source_id": ("audio_avsdlc_tool", "audio_fingerprint", "api_spend", "media_external_query", "media privacy + egress receipt"),
    "media_catalog_publication_support": ("audio_avsdlc_tool", "media_catalog", "api_or_account", "media_publication", "copyright + publication receipt"),
    "research_publication_deposit": ("publication_egress", "research_deposit", "external_account", "public", "publication + legal-name receipt"),
    "public_social_distribution": ("publication_egress", "social_distribution", "external_account", "public", "publication + operator receipt"),
    "google_workspace_youtube_connector": ("publication_egress", "google_workspace_youtube", "connector", "public_or_workspace", "connector + public-egress receipt"),
    "research_storage_infra": ("infrastructure_control", "backup_storage", "infra_account", "infrastructure", "destructive preflight + infra receipt"),
    "network_admin_tailscale": ("infrastructure_control", "network_admin", "infra_account", "network_control", "target + rollback receipt"),
    "local_inference_eval": ("provider_gateway", "local_inference_eval", "local_compute", "local_runtime", "local route decision receipt"),
}


def _capability_taxonomy(capability_id: str) -> dict[str, str]:
    if capability_id in _CAPABILITY_TAXONOMY:
        return dict(_CAPABILITY_TAXONOMY[capability_id])
    if capability_id in _CAPABILITY_SURFACE_TAXONOMY:
        capability_class, family, spend, egress, receipt = _CAPABILITY_SURFACE_TAXONOMY[capability_id]
        return {
            "capability_class": capability_class,
            "surface_family": family,
            "spend_model": spend,
            "egress_class": egress,
            "receipt_requirement": receipt,
        }
    return {
        "capability_class": "registry_score",
        "surface_family": "platform_capability_registry",
        "spend_model": "route_defined",
        "egress_class": "route_defined",
        "receipt_requirement": "registry evidence + route receipt",
    }


def _capability_surface_pack_paths(cfg: dict | None = None) -> list[Path]:
    cfg = cfg or {}
    env = os.environ.get("REINS_CAPABILITY_SURFACE_PACKS")
    if env:
        vals = [v for v in env.split(os.pathsep) if v]
    else:
        raw = cfg.get("capability_surface_pack_paths") or []
        if isinstance(raw, list):
            vals = [str(v) for v in raw if str(v).strip()]
        elif raw:
            vals = [str(raw)]
        else:
            vals = []
    return [Path(os.path.expanduser(v)) for v in vals]


def _row_source_ref_summary(row: dict, source_id: str) -> str:
    refs = row.get("source_refs") or row.get("evidence_refs") or row.get("sources")
    if isinstance(refs, list):
        return f"{source_id}:{len(refs)} refs" if refs else source_id
    if isinstance(refs, dict):
        return f"{source_id}:{len(refs)} refs" if refs else source_id
    return f"{source_id}:1 refs" if refs else source_id


def _source_ref_label(ref: Any) -> str:
    text = str(ref or "").strip()
    if not text:
        return ""
    frag = ""
    if "#" in text:
        text, frag = text.split("#", 1)
        frag = "#" + frag.strip()
    if "://" in text:
        text = text.rstrip("/").rsplit("/", 1)[-1]
    elif "/" in text or "\\" in text:
        text = Path(text).name
    text = text.strip()
    if not text:
        text = "source"
    label = f"{text}{frag}"
    return label[:160]


def _row_source_ref_labels(row: dict, limit: int = 8) -> list[str]:
    refs = row.get("source_refs") or row.get("evidence_refs") or row.get("sources")
    if isinstance(refs, dict):
        refs = list(refs.keys())
    elif isinstance(refs, str):
        refs = [refs] if refs.strip() else []
    elif refs is None:
        refs = []
    out: list[str] = []
    try:
        iterable = list(refs)
    except TypeError:
        iterable = [refs]
    for ref in iterable:
        label = _source_ref_label(ref)
        if label:
            out.append(label)
        if len(out) >= limit:
            break
    return out


def _capability_pack_row(row: dict, source_id: str, allowlist: list[str], row_kind: str = "surface") -> dict:
    capability_id = str(row.get("capability_id") or row.get("id") or row.get("name") or "")
    out = _capability_row(
        capability_id,
        str(row.get("status") or row.get("state") or "read-missing"),
        str(row.get("routing_meaning") or row.get("evidence_posture") or row.get("authority") or ""),
        _safe_int(row.get("route_count") or row.get("surface_count") or row.get("count")),
        _safe_int(row.get("ok_count") or row.get("ready_count") or row.get("observed_count")),
        _safe_int(row.get("blocked_count") or row.get("gap_count") or row.get("missing_count")),
        _safe_int(row.get("evidence_count")),
        str(row.get("blocker") or row.get("missing") or ""),
        str(row.get("hkp_posture") or "not_applicable"),
        allowlist,
        _row_source_ref_summary(row, source_id),
        _row_source_ref_labels(row),
    )
    if row_kind == "class":
        out["capability_class"] = str(row.get("capability_class") or capability_id)
        out["surface_family"] = str(row.get("surface_family") or capability_id)
    for key in ("capability_class", "surface_family", "spend_model", "egress_class", "receipt_requirement"):
        if row.get(key):
            out[key] = str(row[key])
    out["air"] = classify_air({k: v for k, v in out.items() if k != "air"}, allowlist)
    return out


def _compiled_capability_surface_rows(allowlist: list[str]) -> list[dict]:
    rows = _capability_class_rows(allowlist)
    rows.extend(_capability_surface_rows(allowlist))
    for row in rows:
        row["source_refs"] = "compiled_fallback"
        row["source_ref_labels"] = []
        row["air"] = classify_air({k: v for k, v in row.items() if k != "air"}, allowlist)
    return rows


def _capability_surface_pack_rows(cfg: dict | None, allowlist: list[str]) -> tuple[list[dict], list[dict]]:
    paths = _capability_surface_pack_paths(cfg)
    sources: list[dict] = []
    rows: list[dict] = []
    if not paths:
        sources.append(_capability_source(
            "capability_surface_pack_paths",
            Path(""),
            0,
            "missing",
            allowlist,
            "no REINS_CAPABILITY_SURFACE_PACKS/capability_surface_pack_paths configured",
        ))
        fallback = _compiled_capability_surface_rows(allowlist)
        sources.append(_capability_source(
            "capability_surface_compiled_fallback",
            Path(__file__),
            len(fallback),
            "support-only",
            allowlist,
            "compiled fallback capability taxonomy; discovery evidence only",
        ))
        return sources, fallback

    for path in paths:
        source_id = path.stem or "capability_surface_pack"
        if not path.exists():
            sources.append(_capability_source(source_id, path, 0, "missing", allowlist, "configured capability-surface source absent"))
            continue
        try:
            doc = _read_json(path)
        except Exception as e:
            sources.append(_capability_source(source_id, path, 0, "dark", allowlist, str(e)))
            continue
        pack_id = str(doc.get("pack_id") or doc.get("id") or source_id)
        class_rows = doc.get("capability_classes") or doc.get("classes") or []
        surface_rows = doc.get("surfaces") or []
        extra_rows = doc.get("rows") or []
        if isinstance(class_rows, dict):
            class_rows = list(class_rows.values())
        if isinstance(surface_rows, dict):
            surface_rows = list(surface_rows.values())
        if isinstance(extra_rows, dict):
            extra_rows = list(extra_rows.values())
        pack_rows = [r for r in [*class_rows, *surface_rows, *extra_rows] if isinstance(r, dict)]
        for row in class_rows:
            if isinstance(row, dict):
                rows.append(_capability_pack_row(row, pack_id, allowlist, "class"))
        for row in [*surface_rows, *extra_rows]:
            if isinstance(row, dict):
                rows.append(_capability_pack_row(row, pack_id, allowlist, "surface"))
        status = "observed" if pack_rows else "empty"
        detail = "capability-surface discovery pack; discovery evidence only"
        authority_case = str(doc.get("authority_case") or doc.get("authority") or "")
        if authority_case:
            detail += f"; authority={authority_case}"
        sources.append(_capability_source(pack_id, path, len(pack_rows), status, allowlist, detail))

    if not rows:
        fallback = _compiled_capability_surface_rows(allowlist)
        sources.append(_capability_source(
            "capability_surface_compiled_fallback",
            Path(__file__),
            len(fallback),
            "support-only",
            allowlist,
            "configured packs yielded no rows; compiled fallback capability taxonomy",
        ))
        rows = fallback
    return sources, rows


def _capability_tool_rows(route: Any, allowlist: list[str]) -> list[dict]:
    tools = _field_value(route, "tool_state", []) or []
    if isinstance(tools, dict):
        iterable = tools.values()
    else:
        iterable = tools
    rows: list[dict] = []
    route_id = str(getattr(route, "route_id", "") or "")
    platform = str(getattr(route, "platform", "") or "")
    for tool in iterable:
        tool_id = _text_value(_field_value(tool, "tool_id", ""))
        available = _bool_value(_field_value(tool, "available", False))
        authority_use = ",".join(_text_list(_field_value(tool, "authority_use", [])))
        observed_at = _text_value(_field_value(tool, "observed_at", ""))
        stale_after = _text_value(_field_value(tool, "stale_after", ""))
        evidence_ref = _text_value(_field_value(tool, "evidence_ref", ""))
        status = "observed"
        if not available:
            status = "unavailable"
        elif not observed_at or not evidence_ref:
            status = "read-missing"
        fields = {
            "route_id": route_id,
            "platform": platform,
            "tool_id": tool_id,
            "status": status,
            "available": available,
            "authority_use": authority_use,
            "observed_at": observed_at,
            "stale_after": stale_after,
            "evidence_ref": evidence_ref,
            "privacy": "metadata-only",
            "raw_access": False,
        }
        rows.append({**fields, "air": classify_air(fields, allowlist)})
    return rows


def _capability_class_rows(allowlist: list[str]) -> list[dict]:
    """Handoff-derived capability classes; these are admission scaffolds only."""
    specs = [
        (
            "source_acquisition",
            "admission-incomplete",
            "sub-router",
            5,
            3,
            2,
            5,
            "Tavily usage telemetry schema; Perplexity API-credit receipt; connector egress policy",
        ),
        (
            "verifier_floor_checker",
            "preview-only",
            "verifier",
            5,
            3,
            2,
            5,
            "Semgrep/Codecov/CodeQL/runtime witness receipts not unified",
        ),
        (
            "publication_egress",
            "admission-incomplete",
            "publication gate",
            6,
            0,
            6,
            6,
            "publication authority, legal-name redaction, and public-egress receipt required",
        ),
        (
            "audio_avsdlc_tool",
            "admission-incomplete",
            "audio/public-egress gate",
            7,
            0,
            7,
            7,
            "consent, media privacy, quota, and external-query egress receipts required",
        ),
        (
            "provider_gateway",
            "spend-forbidden",
            "provider-spend gate",
            2,
            0,
            2,
            2,
            "LiteLLM/OpenRouter require explicit route envelope; no silent fallback",
        ),
        (
            "subscription_tool_surface",
            "preview-only",
            "tool-surface evidence",
            8,
            3,
            5,
            8,
            "subscription wrappers are evidence; GLM/Fugu raw candidates require bakeoff/admission and route envelope receipts",
        ),
        (
            "infrastructure_control",
            "read-missing",
            "infra authority gate",
            5,
            0,
            5,
            5,
            "storage, backup, and network/admin controls require target, destructive preflight, rollback, and infra receipt",
        ),
    ]
    return [_capability_row(*spec, "not_applicable", allowlist) for spec in specs]


def _capability_surface_rows(allowlist: list[str]) -> list[dict]:
    """Concrete surfaces discovered in the capability-surface handoff.

    Rows here remain metadata/readiness evidence. A visible credential, wrapper,
    or MCP is not route authority; blockers name the receipt/policy still
    required before routine SDLC routing.
    """
    specs = [
        (
            "tavily_source_acquisition",
            "admission-incomplete",
            "source-acquisition",
            1,
            0,
            1,
            4,
            "local MCP usage telemetry deployment still stale; route on refreshed usage receipt",
        ),
        (
            "perplexity_source_acquisition",
            "admission-incomplete",
            "api-credit receipt",
            1,
            0,
            1,
            2,
            "API spend is separate from subscription; require credit/cost receipt",
        ),
        (
            "context7_docs_currentness",
            "observed",
            "docs support",
            1,
            1,
            0,
            2,
            "current library/framework/API docs support; not repo mutation authority",
        ),
        (
            "google_drive_docs_connector",
            "preview-only",
            "connector route",
            1,
            1,
            0,
            2,
            "tool-specific authority, redaction, and public-egress/send gates required",
        ),
        (
            "github_repo_ci",
            "observed",
            "repo/PR/CI route",
            1,
            1,
            0,
            3,
            "repo authority still flows through governed task, review, queue, and receipt gates",
        ),
        (
            "codex_worker_reviewer_surface",
            "observed",
            "governed worker/reviewer",
            3,
            3,
            0,
            4,
            "primary worker/reviewer family; Spark remains support-only unless explicitly routed",
        ),
        (
            "claude_worker_reviewer_surface",
            "observed",
            "governed worker/reviewer",
            3,
            3,
            0,
            4,
            "primary frontier worker/reviewer family; provider quota can still block routing",
        ),
        (
            "antigravity_agy_tool_surface",
            "preview-only",
            "audited worker IDE",
            2,
            1,
            1,
            3,
            "route through agy/Antigravity wrappers; computer-use needs a live official-source receipt",
        ),
        (
            "mistral_vibe_worker_surface",
            "admission-incomplete",
            "bounded implementation worker",
            1,
            0,
            1,
            2,
            "bounded JR+/mechanical implementation lane; broader use needs route-envelope receipts",
        ),
        (
            "glmcp_review_quota_admission",
            "observed",
            "review/quota/admission spine",
            1,
            1,
            0,
            4,
            "review/quota/admission spine exists; coding workhorse remains bakeoff/manual-only",
        ),
        (
            "gemini_agy_support_review",
            "preview-only",
            "subscription support/review",
            2,
            1,
            1,
            3,
            "support/review experiments route through agy or Antigravity, not direct Gemini CLI authority",
        ),
        (
            "glm_coding_plan_tool_surface",
            "manual-bakeoff",
            "subscription coding plan",
            1,
            0,
            1,
            4,
            "GLM Coding Plan is narrow supported-tool candidate only; S15 proves invocation, not quality promotion; workhorse remains manual-only pending bakeoff",
        ),
        (
            "fugu_raw_codex",
            "raw-manual",
            "not dispatchable",
            0,
            0,
            1,
            4,
            "raw Sakana/Fugu access is actor evidence only; no governed Fugu route, admission receipt, bakeoff, or dispatch integration exists",
        ),
        (
            "fugu_ultra_raw_codex",
            "raw-manual",
            "not dispatchable",
            0,
            0,
            1,
            4,
            "raw Fugu-Ultra smoke evidence is not governed route authority; current lanes require execution-host preflight and wrapper promotion before use",
        ),
        (
            "huggingface_provider_gateway",
            "preview-only",
            "provider-spend gate",
            1,
            0,
            1,
            2,
            "verify account tier, dataset/model license, egress posture, and PAYG receipt",
        ),
        (
            "cohere_embed_rerank",
            "preview-only",
            "source/verifier support",
            1,
            0,
            1,
            3,
            "verify active account, current models/pricing, and receipt-bounded use case",
        ),
        (
            "litellm_provider_gateway",
            "spend-forbidden",
            "provider gateway",
            1,
            0,
            1,
            2,
            "must stay behind route envelope; no silent provider fallback",
        ),
        (
            "openrouter_break_glass",
            "spend-forbidden",
            "break-glass spend",
            1,
            0,
            1,
            1,
            "explicit emergency/provider-spend receipt required; never routine throughput",
        ),
        (
            "semgrep_static_analysis",
            "preview-only",
            "verifier",
            1,
            1,
            0,
            2,
            "security/static-analysis floor checker, not worker lane",
        ),
        (
            "codecov_coverage_signal",
            "preview-only",
            "verifier",
            1,
            1,
            0,
            2,
            "coverage telemetry only; unify floor-check receipt before routing",
        ),
        (
            "codeql_status_floor",
            "read-missing",
            "security verifier",
            1,
            0,
            1,
            1,
            "CodeQL status is a floor-check signal; route-facing receipt not yet represented",
        ),
        (
            "deterministic_test_floor",
            "read-missing",
            "test verifier",
            1,
            0,
            1,
            1,
            "deterministic test runs need a standard receipt row before routing learns from them",
        ),
        (
            "runtime_witness_floor",
            "read-missing",
            "runtime verifier",
            1,
            0,
            1,
            1,
            "runtime witness receipts are not unified into capability readiness",
        ),
        (
            "worker_failure_witness",
            "preview-only",
            "failure classifier",
            1,
            1,
            0,
            3,
            "failure classification exists; remaining work is coverage and route integration",
        ),
        (
            "langfuse_trace_eval",
            "preview-only",
            "trace/eval telemetry",
            1,
            0,
            1,
            2,
            "redaction and receipt schema required before routing can learn from traces",
        ),
        (
            "elevenlabs_audio_generation",
            "admission-incomplete",
            "audio/public-egress gate",
            1,
            0,
            1,
            3,
            "audio/public-egress, quota, consent, and redaction receipts required",
        ),
        (
            "picovoice_audio_verifier",
            "admission-incomplete",
            "audio/runtime verifier",
            1,
            0,
            1,
            2,
            "audio/runtime authority and local telemetry gate required",
        ),
        (
            "audio_fingerprint_source_id",
            "admission-incomplete",
            "media privacy gate",
            2,
            0,
            2,
            2,
            "AcoustID/ACRCloud need media privacy and external-query egress receipts",
        ),
        (
            "media_catalog_publication_support",
            "admission-incomplete",
            "media/copyright gate",
            3,
            0,
            3,
            3,
            "Reverb/SoundCloud/Epidemic-style media support needs copyright, source, and publication receipts",
        ),
        (
            "research_publication_deposit",
            "admission-incomplete",
            "publication gate",
            4,
            0,
            4,
            4,
            "OSF/Zenodo/ORCID/PhilArchive require publication authority and legal-name/redaction guard",
        ),
        (
            "public_social_distribution",
            "admission-incomplete",
            "public-egress gate",
            5,
            0,
            5,
            5,
            "Dev.to/Hashnode/HN/Mastodon/Bluesky/omg.lol require publication receipt",
        ),
        (
            "google_workspace_youtube_connector",
            "preview-only",
            "connector/public-egress gate",
            3,
            1,
            2,
            3,
            "Google mail/calendar/docs/YouTube/livestream surfaces need connector-specific authority and send/public-egress gates",
        ),
        (
            "research_storage_infra",
            "read-missing",
            "infrastructure authority",
            4,
            0,
            4,
            4,
            "Backblaze/MinIO/Synology/restic require target, destructive preflight, rollback, and infra receipt",
        ),
        (
            "network_admin_tailscale",
            "read-missing",
            "network/admin control",
            1,
            0,
            1,
            1,
            "Tailscale/network control requires explicit target, authority task, and rollback receipt",
        ),
        (
            "local_inference_eval",
            "preview-only",
            "local-tool decision",
            3,
            0,
            3,
            3,
            "tabbyapi/ollama/logos-api need explicit local route row; process names are not authority",
        ),
    ]
    return [_capability_row(*spec, "not_applicable", allowlist) for spec in specs]


def _receipt_count(path: Path) -> int:
    if not path.exists():
        return 0
    return sum(1 for p in path.glob("*.json") if p.is_file())


def read_capability_summary(council_root: str, allowlist: list[str], cfg: dict | None = None) -> dict:
    """Read capability-routing metadata from the council registry and local receipts.

    This does not choose a route, write route decisions, mint receipts, launch providers,
    or read raw transcripts. Platforms are evidence rows; capabilities are first-class.
    """
    cfg = cfg or {}
    from hapax.spine.dispatcher_policy import (
        ROUTE_AUTHORITY_RECEIPT_DIRNAME,
        load_dispatch_policy_sources,
    )
    from hapax.spine.platform_capability_receipts import (
        DEFAULT_PLATFORM_CAPABILITY_RECEIPT_DIR,
    )
    from hapax.spine.platform_capability_registry import (
        check_registry_freshness,
    )
    from hapax.spine.quota_spend_ledger import (
        DEFAULT_QUOTA_SPEND_LEDGER_LIVE,
    )

    # reins injects council's config dir as the registry path — the wheel's default resolves via
    # HAPAX_SPINE_CONFIG_DIR (a sentinel when unset), so pass it explicitly (dark-regression guard).
    registry_path = Path(os.path.expanduser(council_root)) / "config" / "platform-capability-registry.json"

    receipt_dir = Path(os.path.expanduser(str(cfg.get("capability_receipt_dir") or DEFAULT_PLATFORM_CAPABILITY_RECEIPT_DIR)))
    quota_path = Path(os.path.expanduser(str(cfg.get("quota_spend_ledger_live") or DEFAULT_QUOTA_SPEND_LEDGER_LIVE)))
    sources_obj = load_dispatch_policy_sources(receipt_dir=receipt_dir, registry_path=registry_path)
    registry = sources_obj.registry
    routes = list(getattr(registry, "routes", ()) or []) if registry is not None else []
    freshness = check_registry_freshness(registry) if registry is not None else None
    freshness_by_route = {r.route_id: r for r in (getattr(freshness, "routes", ()) or [])}
    route_auth_dir = receipt_dir / ROUTE_AUTHORITY_RECEIPT_DIRNAME

    pack_sources, pack_rows = _capability_surface_pack_rows(cfg, allowlist)
    hkp_sources, hkp_evidence_count, hkp_source_refs, hkp_blocker = _hkp_support_summary(cfg, allowlist)
    sources = [
        _capability_source("platform_registry", registry_path, len(routes), "observed" if registry is not None else "error", allowlist, sources_obj.registry_error or "typed platform capability registry"),
        _capability_source("platform_receipts", receipt_dir, _receipt_count(receipt_dir), "observed" if receipt_dir.exists() else "missing", allowlist, "local platform capability receipts"),
        _capability_source("route_authority_receipts", route_auth_dir, len(sources_obj.route_authority_receipts), "observed" if route_auth_dir.exists() else "missing", allowlist, "fresh route authority receipts"),
        _capability_source("quota_ledger", quota_path, 1 if sources_obj.quota_ledger is not None else 0, "observed" if sources_obj.quota_ledger is not None else "missing", allowlist, sources_obj.quota_error or sources_obj.quota_live_error or str(sources_obj.quota_ledger_source or "")),
    ] + pack_sources + hkp_sources

    route_rows = [_capability_route(route, freshness_by_route.get(route.route_id), allowlist) for route in routes]
    tool_rows = [tool for route in routes for tool in _capability_tool_rows(route, allowlist)]
    blocked_routes = sum(1 for r in route_rows if r["route_state"] == "blocked" or r["blockers"])
    stale_routes = sum(1 for r in route_rows if not r["freshness_ok"])
    evidence_total = sum(_safe_int(r.get("evidence_count")) for r in route_rows)
    rows: list[dict] = []
    route_count = len(routes)
    rows.append(_capability_row(
        "route_envelope",
        "observed" if registry is not None and route_count else "read-missing",
        "metadata-only",
        route_count,
        route_count - blocked_routes,
        blocked_routes,
        route_count,
        sources_obj.registry_error or "",
        "not_applicable",
        allowlist,
    ))

    quota_ok = sources_obj.quota_ledger is not None and not sources_obj.quota_error
    quota_route_ok = sum(1 for route in routes if str(getattr(getattr(route, "telemetry", None), "quota_source", "")).rsplit(".", 1)[-1] not in {"none", "unknown", ""})
    rows.append(_capability_row(
        "quota_context",
        "observed" if quota_ok else "read-missing",
        "metadata-only",
        route_count,
        quota_route_ok,
        max(0, route_count - quota_route_ok),
        1 if quota_ok else 0,
        sources_obj.quota_error or sources_obj.quota_live_error or "",
        "not_applicable",
        allowlist,
    ))

    route_auth_count = len(sources_obj.route_authority_receipts)
    rows.append(_capability_row(
        "route_authority_receipts",
        "observed" if route_auth_count else "read-missing",
        "governed route",
        route_count,
        route_auth_count,
        max(0, route_count - route_auth_count),
        route_auth_count,
        "authority receipts are evidence, not dispatch grants" if route_auth_count else "route authority receipts absent",
        "not_applicable",
        allowlist,
    ))

    score_dims: dict[str, dict[str, int]] = {}
    for route in routes:
        scores = getattr(route, "capability_scores", None)
        payload = scores.model_dump() if hasattr(scores, "model_dump") else {}
        route_errors = freshness_by_route.get(route.route_id)
        errors = " ".join(getattr(route_errors, "errors", ()) or [])
        for dim, score in payload.items():
            bucket = score_dims.setdefault(dim, {"routes": 0, "ok": 0, "blocked": 0, "evidence": 0, "stale": 0})
            bucket["routes"] += 1
            evidence_refs = score.get("evidence_refs") or []
            if evidence_refs:
                bucket["evidence"] += len(evidence_refs)
            if score.get("observed_at") and evidence_refs:
                bucket["ok"] += 1
            else:
                bucket["blocked"] += 1
            if dim in errors:
                bucket["stale"] += 1
    for dim, bucket in sorted(score_dims.items()):
        status = "observed"
        blocker = ""
        if bucket["evidence"] == 0:
            status, blocker = "read-missing", "evidence refs absent"
        elif bucket["stale"] > 0:
            status, blocker = "stale", f"{bucket['stale']} routes failed freshness"
        elif bucket["blocked"] > 0:
            status, blocker = "partial", f"{bucket['blocked']} routes missing score evidence"
        rows.append(_capability_row(
            dim,
            status,
            "registry evidence",
            bucket["routes"],
            bucket["ok"],
            bucket["blocked"] + bucket["stale"],
            bucket["evidence"],
            blocker,
            "not_applicable",
            allowlist,
        ))

    rows.append(_capability_row(
        "hkp_support_context",
        "support-only" if hkp_evidence_count else "read-missing",
        "authority-capped",
        0,
        0,
        0,
        hkp_evidence_count,
        hkp_blocker,
        "support_only",
        allowlist,
        hkp_source_refs,
    ))
    rows.extend(pack_rows)

    totals = {
        "capabilities": len(rows),
        "routes": route_count,
        "blocked": blocked_routes,
        "stale": stale_routes,
        "receipts": _receipt_count(receipt_dir) + route_auth_count,
        "evidence": evidence_total + hkp_evidence_count,
        "sources": len(sources),
        "tools": len(tool_rows),
    }
    return {"sources": sources, "rows": rows, "routes": route_rows, "tools": tool_rows, "totals": totals}


def _gate_stage_counts(items: list[dict]) -> str:
    counts: dict[str, int] = {}
    for t in items:
        stage = str(t.get("stage") or "unknown")
        stage = stage.split("_", 1)[0] if stage else "unknown"
        counts[stage] = counts.get(stage, 0) + 1
    return " ".join(f"{k}:{counts[k]}" for k in sorted(counts))


def _gate_severity(gate_id: str, count: int) -> str:
    if count <= 0:
        return "ok"
    if gate_id in {"release_authorized", "implementation_authorized", "source_mutation_authorized"}:
        return "crit"
    if gate_id in {"docs_mutation_authorized", "axiom_mutation_authorized"}:
        return "major"
    return "warn"


def _lane_gate_severity(blocker: str, count: int) -> str:
    if count <= 0 or blocker in {"", "none"}:
        return "ok"
    if blocker in {"stalled", "offline", "no_session"}:
        return "crit"
    if blocker in {"stale_relay", "no_claim"}:
        return "warn"
    return "major"


def read_gate_summary(council_root: str, allowlist: list[str], cfg: dict | None = None) -> dict:
    """Source-backed readiness/gate projection.

    This preserves raw false no_go names and lane blockers as gate rows. It is a
    read contract only: rows can explain legal next moves, never grant authority.
    """
    cfg = cfg or {}
    sources: list[dict] = []
    rows: list[dict] = []
    totals = {"sources": 0, "rows": 0, "blocked": 0, "tasks": 0, "lanes": 0, "commands": 0}

    tasks: dict[str, dict] = {}
    try:
        tasks = (_projection(council_root).get("tasks") or {})
        if not isinstance(tasks, dict):
            tasks = {}
        sources.append(_gate_source(
            "task_projection",
            "observed",
            len(tasks),
            allowlist,
            detail="coord projection task stage/no_go snapshot",
            age_bucket="live",
        ))
    except Exception as e:
        sources.append(_gate_source("task_projection", "dark", 0, allowlist, detail=str(e), age_bucket="dark"))

    sessions: list[dict] = []
    session_path = Path(_session_state_path())
    try:
        bindings, binding_source, binding_path = _route_binding_index(cfg)
        sessions = [
            to_session(name, lane, allowlist, _session_route_binding(name, lane, bindings, binding_source, binding_path))
            for name, lane in _raw_sessions()
        ]
        sources.append(_gate_source(
            "session_state",
            "observed",
            len(sessions),
            allowlist,
            detail="coordinator lane readiness/blockers",
            age_bucket=_path_age_bucket(session_path),
            path=str(session_path),
        ))
        route_covered = sum(1 for s in sessions if str(s.get("route_binding_state") or s.get("route_id") or "").strip())
        sources.append(_gate_source(
            "route_binding",
            "observed" if binding_source == "observed" else "missing",
            route_covered,
            allowlist,
            detail=f"route binding lane coverage; ledger_records={len(bindings)}",
            age_bucket=_path_age_bucket(binding_path),
            path=str(binding_path),
        ))
    except Exception as e:
        sources.append(_gate_source(
            "session_state",
            "dark",
            0,
            allowlist,
            detail=str(e),
            age_bucket=_path_age_bucket(session_path),
            path=str(session_path),
        ))

    events: list[dict] = []
    try:
        events = _raw_tail(council_root, 200)
        sources.append(_gate_source("event_log", "observed", len(events), allowlist, detail="recent coord events", age_bucket="live"))
    except Exception as e:
        sources.append(_gate_source("event_log", "dark", 0, allowlist, detail=str(e), age_bucket="dark"))

    false_by_gate: dict[str, list[dict]] = {}
    missing_authority: list[dict] = []
    for tid, task in tasks.items():
        if not isinstance(task, dict):
            continue
        if not str(task.get("authority_case") or "").strip():
            missing_authority.append(task)
        no_go = task.get("no_go") or {}
        if not isinstance(no_go, dict):
            continue
        for gate_id, value in no_go.items():
            if value is False:
                false_by_gate.setdefault(str(gate_id), []).append(task)

    for gate_id, items in sorted(false_by_gate.items(), key=lambda x: (-len(x[1]), x[0])):
        count = len(items)
        sev = _gate_severity(gate_id, count)
        rows.append(_gate_row(
            f"task.no_go.{gate_id}",
            "task",
            f"{count} tasks",
            "blocked",
            sev,
            "coord_projection",
            f"false_on={count}; stages={_gate_stage_counts(items)}",
            gate_id,
            allowlist,
            source="task_projection",
            action="preserve gate until governed authority/preflight/receipt clears it",
        ))

    if missing_authority:
        rows.append(_gate_row(
            "task.authority_case",
            "task",
            f"{len(missing_authority)} tasks",
            "missing",
            "crit",
            "methodology",
            f"missing_on={len(missing_authority)}; stages={_gate_stage_counts(missing_authority)}",
            "authority_case",
            allowlist,
            source="task_projection",
            action="repair task metadata before claim/dispatch/close",
        ))

    blocker_counts: dict[str, int] = {}
    readiness_counts: dict[str, int] = {}
    for s in sessions:
        blocker = str(s.get("blocker") or "none")
        readiness = str(s.get("readiness") or "unknown")
        readiness_counts[readiness] = readiness_counts.get(readiness, 0) + 1
        if blocker and blocker != "none":
            blocker_counts[blocker] = blocker_counts.get(blocker, 0) + 1

    for blocker, count in sorted(blocker_counts.items(), key=lambda x: (-x[1], x[0])):
        rows.append(_gate_row(
            f"lane.blocker.{blocker}",
            "lane",
            f"{count} lanes",
            "blocked",
            _lane_gate_severity(blocker, count),
            "coordinator_state",
            f"count={count}; readiness=" + " ".join(f"{k}:{readiness_counts[k]}" for k in sorted(readiness_counts)),
            blocker,
            allowlist,
            source="session_state",
            action="inspect lane/session state before resume or dispatch",
        ))

    route_counts: dict[str, int] = {}
    route_ids_by_state: dict[str, dict[str, int]] = {}
    for s in sessions:
        state = str(s.get("route_binding_state") or "")
        route_id = str(s.get("route_id") or "")
        if not state and not route_id:
            continue
        if not state:
            state = "unbound"
        route_counts[state] = route_counts.get(state, 0) + 1
        if route_id:
            bucket = route_ids_by_state.setdefault(state, {})
            bucket[route_id] = bucket.get(route_id, 0) + 1

    route_missing_by_state = {
        "bound": "none",
        "policy_only": "launch receipt/session confirmation",
        "eligible_not_launched": "launch receipt/session binding",
        "mq_unbound": "merge-queue/session binding",
        "no_claim": "claimed_task",
        "unbound": "route decision",
        "policy_refused": "operator route override or new target",
        "policy_held": "policy unblock evidence",
        "platform_mismatch": "lane/platform route alignment",
        "source_missing": "route binding ledger",
        "source_unreadable": "readable route binding ledger",
    }
    route_state_by_state = {
        "bound": "observed",
        "policy_only": "preview-only",
        "eligible_not_launched": "preview-only",
        "mq_unbound": "preview-only",
        "no_claim": "preview-only",
        "unbound": "missing",
        "policy_refused": "blocked",
        "policy_held": "blocked",
        "platform_mismatch": "blocked",
        "source_missing": "missing",
        "source_unreadable": "blocked",
    }
    route_severity_by_state = {
        "bound": "ok",
        "policy_only": "warn",
        "eligible_not_launched": "warn",
        "mq_unbound": "warn",
        "no_claim": "warn",
        "unbound": "warn",
        "policy_refused": "major",
        "policy_held": "major",
        "platform_mismatch": "major",
        "source_missing": "major",
        "source_unreadable": "crit",
    }
    for state, count in sorted(route_counts.items(), key=lambda x: (-x[1], x[0])):
        route_evidence = route_ids_by_state.get(state) or {}
        routes_part = " ".join(f"{route}:{route_evidence[route]}" for route in sorted(route_evidence)[:4])
        evidence = f"count={count}"
        if routes_part:
            evidence += f"; routes={routes_part}"
        rows.append(_gate_row(
            f"route.binding.{state}",
            "route",
            f"{count} lanes",
            route_state_by_state.get(state, "preview-only"),
            route_severity_by_state.get(state, "warn"),
            "route_binding_ledger",
            evidence,
            route_missing_by_state.get(state, "route receipt"),
            allowlist,
            source="route_binding",
            action="inspect :sessions/:yard route evidence before resume or dispatch",
        ))

    command_rows = [
        ("command.dispatch", "dispatch", "preview-only", "warn", "methodology dispatch", "authority_case,parent_spec,preflight,receipt", ":intent dispatch"),
        ("command.claim", "claim", "preview-only", "warn", "cc-task route", "claim authority + receipt", ":intent claim"),
        ("command.close", "close", "preview-only", "warn", "cc-task route", "close evidence + receipt", ":intent close"),
        ("command.release", "release", "preview-only", "crit", "operator/governed route", "release_authorized=true receipt", ":intent approve"),
    ]
    for gate_id, subject, state, severity, authority, missing, action in command_rows:
        rows.append(_gate_row(
            gate_id,
            "command",
            subject,
            state,
            severity,
            authority,
            "verbs registered; mutation disabled in Reins",
            missing,
            allowlist,
            source="command_registry",
            action=action,
        ))

    blocked = sum(1 for row in rows if row.get("state") in {"blocked", "missing", "dark"})
    preview = sum(1 for row in rows if row.get("state") in {"preview-only", "preview_only"})
    totals.update({
        "sources": len(sources),
        "rows": len(rows),
        "blocked": blocked,
        "preview": preview,
        "tasks": len(tasks),
        "lanes": len(sessions),
        "commands": len(command_rows),
        "routes": sum(route_counts.values()),
        "false_gates": sum(len(v) for v in false_by_gate.values()),
        "events": len(events),
    })
    return {"sources": sources, "rows": rows, "totals": totals}


def _domain_pack_paths(cfg: dict | None = None) -> list[Path]:
    cfg = cfg or {}
    env = os.environ.get("REINS_DOMAIN_PACKS")
    if env:
        vals = [v for v in env.split(os.pathsep) if v]
    else:
        raw = cfg.get("domain_pack_paths") or []
        if isinstance(raw, list):
            vals = [str(v) for v in raw if str(v).strip()]
        elif raw:
            vals = [str(raw)]
        else:
            vals = []
    return [Path(os.path.expanduser(v)) for v in vals]


def _lifecycle_registry_paths(cfg: dict | None = None) -> list[Path]:
    cfg = cfg or {}
    env = os.environ.get("REINS_LIFECYCLE_REGISTRIES")
    if env:
        vals = [v for v in env.split(os.pathsep) if v]
    else:
        raw = cfg.get("lifecycle_registry_paths") or []
        if isinstance(raw, list):
            vals = [str(v) for v in raw if str(v).strip()]
        elif raw:
            vals = [str(raw)]
        else:
            vals = []
    return [Path(os.path.expanduser(v)) for v in vals]


def _domain_source(
    source_id: str,
    path: Path | None,
    status: str,
    count: int,
    allowlist: list[str],
    authority: str = "",
    detail: str = "",
) -> dict:
    p = path or Path("")
    fields = {
        "id": source_id,
        "path": str(p) if path else "",
        "exists": bool(path and path.exists()),
        "status": status,
        "count": count,
        "age_bucket": _path_age_bucket(p) if path else "missing",
        "authority": authority,
        "detail": detail,
        "privacy": "metadata-only",
        "raw_access": False,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _csvish(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, (list, tuple, set)):
        return ",".join(str(v) for v in value if str(v).strip())
    return str(value)


def _domain_evidence_count(row: dict) -> int:
    refs = row.get("source_refs") or row.get("evidence_refs") or row.get("sources")
    if isinstance(refs, list):
        return len(refs)
    if isinstance(refs, dict):
        return len(refs)
    return 1 if refs else 0


def _domain_row(row: dict, source_id: str, pack_authority: str, allowlist: list[str]) -> dict:
    domain_id = str(row.get("domain_id") or row.get("id") or row.get("name") or "")
    evidence_count = _domain_evidence_count(row)
    fields = {
        "domain_id": domain_id,
        "label": str(row.get("label") or row.get("title") or row.get("name") or domain_id),
        "lifecycle": str(row.get("lifecycle") or row.get("dlc") or row.get("domain") or ""),
        "terrain": str(row.get("terrain") or ""),
        "depth": str(row.get("depth") or ""),
        "scope": str(row.get("scope") or "instance"),
        "state": str(row.get("state") or row.get("status") or "observed"),
        "authority_ceiling": str(row.get("authority_ceiling") or row.get("authority") or pack_authority or "metadata-only"),
        "claim_ceiling": str(row.get("claim_ceiling") or row.get("claim") or "navigation"),
        "windows": _csvish(row.get("windows")),
        "surfaces": _csvish(row.get("surfaces")),
        "parity": str(row.get("parity") or row.get("coverage") or ""),
        "evidence_count": evidence_count,
        "blocker": str(row.get("blocker") or ""),
        "source_refs": f"{source_id}:{evidence_count} refs" if evidence_count else source_id,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _domain_relation(rel: dict, source_id: str, pack_authority: str, allowlist: list[str]) -> dict:
    refs = rel.get("source_refs") or rel.get("evidence_refs")
    count = len(refs) if isinstance(refs, list) else (1 if refs else 0)
    fields = {
        "source": str(rel.get("source") or rel.get("from") or ""),
        "target": str(rel.get("target") or rel.get("to") or ""),
        "relation": str(rel.get("relation") or rel.get("type") or "related"),
        "authority_ceiling": str(rel.get("authority_ceiling") or rel.get("authority") or pack_authority or "metadata-only"),
        "source_refs": f"{source_id}:{count} refs" if count else source_id,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _lifecycle_row(row: dict, source_id: str, registry_authority: str, allowlist: list[str]) -> dict:
    lifecycle_id = str(row.get("lifecycle_id") or row.get("id") or row.get("name") or "")
    evidence_count = _domain_evidence_count(row)
    fields = {
        "lifecycle_id": lifecycle_id,
        "label": str(row.get("label") or row.get("title") or row.get("name") or lifecycle_id),
        "owner": str(row.get("owner") or row.get("tenant") or ""),
        "scope": str(row.get("scope") or "tenant"),
        "plant": str(row.get("plant") or row.get("system") or ""),
        "posture": str(row.get("posture") or ""),
        "state": str(row.get("state") or row.get("status") or "observed"),
        "maturity": str(row.get("maturity") or ""),
        "adapter_id": str(row.get("adapter_id") or row.get("adapter") or ""),
        "authority_ceiling": str(row.get("authority_ceiling") or row.get("authority") or registry_authority or "metadata-only"),
        "claim_surface": _csvish(row.get("claim_surface")),
        "mutation_surface": _csvish(row.get("mutation_surface")),
        "dark_policy": str(row.get("dark_policy") or ""),
        "freshness_policy": str(row.get("freshness_policy") or ""),
        "air_class": str(row.get("air_class") or row.get("privacy") or ""),
        "windows": _csvish(row.get("windows")),
        "surfaces": _csvish(row.get("surfaces")),
        "commands": _csvish(row.get("commands")),
        "receipt_contracts": _csvish(row.get("receipt_contracts") or row.get("receipts")),
        "evidence_count": evidence_count,
        "blocker": str(row.get("blocker") or ""),
        "next_evidence": str(row.get("next_evidence") or row.get("next") or ""),
        "source_refs": f"{source_id}:{evidence_count} refs" if evidence_count else source_id,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def read_lifecycle_registry_summary(cfg: dict | None, allowlist: list[str]) -> dict:
    """Read optional authority-aware lifecycle contracts without source bodies.

    This is the tenant layer for SDLC/RDLC/LDLC/future n-DLCs. Domain packs remain lenses over the
    lifecycles; absence is explicit and keeps compiled navigation as fallback only.
    """
    paths = _lifecycle_registry_paths(cfg)
    sources: list[dict] = []
    rows: list[dict] = []
    hashes: list[str] = []
    generated_at = ""
    default_lens = ""
    authority = ""

    if not paths:
        sources.append(_domain_source(
            "lifecycle_registry_paths",
            None,
            "missing",
            0,
            allowlist,
            "compiled-fallback",
            "no REINS_LIFECYCLE_REGISTRIES/lifecycle_registry_paths configured",
        ))
        return {
            "sources": sources,
            "rows": rows,
            "totals": {"sources": 1, "rows": 0, "missing_sources": 1},
            "authority": "compiled-fallback",
            "generated_at": "",
            "package_hash": "",
            "default_lens": "",
        }

    for path in paths:
        source_id = path.stem or "lifecycle_registry"
        if not path.exists():
            sources.append(_domain_source(source_id, path, "missing", 0, allowlist, "compiled-fallback", "configured lifecycle registry absent"))
            continue
        try:
            doc = _read_json(path)
        except Exception as e:
            sources.append(_domain_source(source_id, path, "dark", 0, allowlist, "compiled-fallback", str(e)))
            continue
        registry_id = str(doc.get("registry_id") or doc.get("pack_id") or doc.get("id") or source_id)
        registry_authority = str(doc.get("authority_case") or doc.get("authority") or "")
        if registry_authority and not authority:
            authority = registry_authority
        if not generated_at:
            generated_at = str(doc.get("generated_at") or "")
        if not default_lens:
            default_lens = str(doc.get("default_lens") or doc.get("default_projection") or "")
        hashes.append(_file_sha256(path))
        lifecycle_rows = doc.get("lifecycles") or doc.get("rows") or []
        if isinstance(lifecycle_rows, dict):
            lifecycle_rows = list(lifecycle_rows.values())
        if not isinstance(lifecycle_rows, list):
            lifecycle_rows = []
        for row in lifecycle_rows:
            if isinstance(row, dict):
                rows.append(_lifecycle_row(row, registry_id, registry_authority, allowlist))
        status = "observed" if lifecycle_rows else "empty"
        sources.append(_domain_source(registry_id, path, status, len(lifecycle_rows), allowlist, registry_authority or "metadata-only", "lifecycle registry metadata"))

    package_hash = ""
    nonempty_hashes = [h for h in hashes if h]
    if len(nonempty_hashes) == 1:
        package_hash = nonempty_hashes[0]
    elif nonempty_hashes:
        package_hash = "sha256:" + hashlib.sha256("|".join(nonempty_hashes).encode("utf-8")).hexdigest()
    missing = sum(1 for s in sources if s.get("status") in {"missing", "dark"})
    return {
        "sources": sources,
        "rows": rows,
        "totals": {"sources": len(sources), "rows": len(rows), "missing_sources": missing},
        "authority": authority or ("source-backed" if rows else "compiled-fallback"),
        "generated_at": generated_at,
        "package_hash": package_hash,
        "default_lens": default_lens,
    }


def read_domain_pack_summary(cfg: dict | None, allowlist: list[str]) -> dict:
    """Read optional lifecycle/domain packs without reading note bodies or claiming authority.

    Domain packs are tenant/operator extensions over the compiled navigation registry. Absence is a
    first-class state: the UI should say source-backed ontology is missing, then fall back to the
    compiled registry as engine navigation hints.
    """
    lifecycle = read_lifecycle_registry_summary(cfg, allowlist)
    paths = _domain_pack_paths(cfg)
    sources: list[dict] = []
    rows: list[dict] = []
    relations: list[dict] = []
    hashes: list[str] = []
    generated_at = ""
    default_lens = ""
    authority = ""

    if not paths:
        sources.append(_domain_source(
            "domain_pack_paths",
            None,
            "missing",
            0,
            allowlist,
            "compiled-fallback",
            "no REINS_DOMAIN_PACKS/domain_pack_paths configured",
        ))
        return {
            "sources": sources,
            "rows": rows,
            "relations": relations,
            "totals": {"sources": 1, "rows": 0, "relations": 0, "missing_sources": 1},
            "authority": "compiled-fallback",
            "generated_at": "",
            "package_hash": "",
            "default_lens": "",
            "lifecycle_sources": lifecycle["sources"],
            "lifecycles": lifecycle["rows"],
            "lifecycle_totals": lifecycle["totals"],
            "lifecycle_authority": lifecycle["authority"],
            "lifecycle_generated_at": lifecycle["generated_at"],
            "lifecycle_package_hash": lifecycle["package_hash"],
            "lifecycle_default_lens": lifecycle["default_lens"],
        }

    for path in paths:
        source_id = path.stem or "domain_pack"
        if not path.exists():
            sources.append(_domain_source(source_id, path, "missing", 0, allowlist, "compiled-fallback", "configured source absent"))
            continue
        try:
            doc = _read_json(path)
        except Exception as e:
            sources.append(_domain_source(source_id, path, "dark", 0, allowlist, "compiled-fallback", str(e)))
            continue
        pack_id = str(doc.get("pack_id") or doc.get("id") or source_id)
        pack_authority = str(doc.get("authority_case") or doc.get("authority") or "")
        if pack_authority and not authority:
            authority = pack_authority
        if not generated_at:
            generated_at = str(doc.get("generated_at") or "")
        if not default_lens:
            default_lens = str(doc.get("default_lens") or doc.get("default_projection") or "")
        hashes.append(_file_sha256(path))
        domain_rows = doc.get("domains") or doc.get("rows") or []
        if isinstance(domain_rows, dict):
            domain_rows = list(domain_rows.values())
        if not isinstance(domain_rows, list):
            domain_rows = []
        relation_rows = doc.get("relations") or doc.get("edges") or []
        if isinstance(relation_rows, dict):
            relation_rows = list(relation_rows.values())
        if not isinstance(relation_rows, list):
            relation_rows = []
        for row in domain_rows:
            if isinstance(row, dict):
                rows.append(_domain_row(row, pack_id, pack_authority, allowlist))
        for rel in relation_rows:
            if isinstance(rel, dict):
                relations.append(_domain_relation(rel, pack_id, pack_authority, allowlist))
        status = "observed" if domain_rows or relation_rows else "empty"
        sources.append(_domain_source(pack_id, path, status, len(domain_rows) + len(relation_rows), allowlist, pack_authority or "metadata-only", "domain pack metadata"))

    package_hash = ""
    nonempty_hashes = [h for h in hashes if h]
    if len(nonempty_hashes) == 1:
        package_hash = nonempty_hashes[0]
    elif nonempty_hashes:
        package_hash = "sha256:" + hashlib.sha256("|".join(nonempty_hashes).encode("utf-8")).hexdigest()
    missing = sum(1 for s in sources if s.get("status") in {"missing", "dark"})
    return {
        "sources": sources,
        "rows": rows,
        "relations": relations,
        "totals": {"sources": len(sources), "rows": len(rows), "relations": len(relations), "missing_sources": missing},
        "authority": authority or ("source-backed" if rows or relations else "compiled-fallback"),
        "generated_at": generated_at,
        "package_hash": package_hash,
        "default_lens": default_lens,
        "lifecycle_sources": lifecycle["sources"],
        "lifecycles": lifecycle["rows"],
        "lifecycle_totals": lifecycle["totals"],
        "lifecycle_authority": lifecycle["authority"],
        "lifecycle_generated_at": lifecycle["generated_at"],
        "lifecycle_package_hash": lifecycle["package_hash"],
        "lifecycle_default_lens": lifecycle["default_lens"],
    }


def _frontmatter(path: Path) -> dict:
    """Parse simple YAML-ish frontmatter scalars without importing task tooling."""
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return {}
    if not lines or lines[0].strip() != "---":
        return {}
    out: dict[str, str] = {}
    for line in lines[1:]:
        if line.strip() == "---":
            break
        if ":" not in line or line.startswith((" ", "\t", "-")):
            continue
        k, v = line.split(":", 1)
        out[k.strip()] = v.strip().strip("'\"")
    return out


def _task_root(cfg: dict) -> Path | None:
    raw = os.environ.get("REINS_CC_TASKS_ACTIVE") or str(cfg.get("cc_tasks_active", ""))
    if not raw:
        return None
    return Path(os.path.expanduser(raw))


def _task_note_for(claimed_task: str, cfg: dict) -> tuple[Path | None, dict]:
    root = _task_root(cfg)
    if not root or not claimed_task or not root.exists():
        return None, {}
    for path in sorted(root.glob("*.md")):
        fm = _frontmatter(path)
        if fm.get("task_id") == claimed_task or claimed_task in path.name:
            return path, fm
    return None, {}


def _transcript_roots(cfg: dict) -> list[Path]:
    env = os.environ.get("REINS_SESSION_TRANSCRIPT_ROOTS")
    if env:
        vals = [x for x in env.split(os.pathsep) if x]
    else:
        raw = cfg.get("session_transcript_roots") or []
        vals = raw if isinstance(raw, list) else [str(raw)] if raw else []
    return [Path(os.path.expanduser(v)) for v in vals]


def _transcript_refs(role: str, platform: str, cfg: dict) -> tuple[list[dict], dict]:
    refs: list[dict] = []
    needles = [x.lower() for x in (role, platform) if x]
    roots = _transcript_roots(cfg)
    observed_roots = 0
    missing_roots = 0
    truncated = False
    for root in roots:
        if not root.exists():
            missing_roots += 1
            continue
        observed_roots += 1
        seen = 0
        for path in root.rglob("*"):
            if seen > 2000 or len(refs) >= 8:
                truncated = True
                break
            seen += 1
            if not path.is_file() or path.suffix not in {".jsonl", ".log", ".txt", ".md"}:
                continue
            name = path.name.lower()
            if needles and not any(n in name for n in needles):
                continue
            refs.append(_evidence_ref("transcript_candidate", path, "raw_transcript", raw_access=False))
    return refs, {
        "transcript_roots_observed": observed_roots,
        "transcript_roots_missing": missing_roots,
        "truncated": truncated,
    }


def _session_evidence_summary(refs: list[dict], scan: dict) -> dict:
    by_kind: dict[str, int] = {}
    raw_access = False
    privacy = "metadata-only"
    for ref in refs:
        kind = str(ref.get("kind") or "unknown")
        by_kind[kind] = by_kind.get(kind, 0) + 1
        raw_access = raw_access or bool(ref.get("raw_access"))
        if str(ref.get("privacy") or "") == "raw_transcript":
            privacy = "metadata-only; transcript candidates raw_access=false"
    return {
        "total": len(refs),
        "by_kind": by_kind,
        "transcript_roots_observed": _safe_int(scan.get("transcript_roots_observed")),
        "transcript_roots_missing": _safe_int(scan.get("transcript_roots_missing")),
        "truncated": bool(scan.get("truncated")),
        "privacy": privacy,
        "raw_access": raw_access,
    }


def to_session_detail(name: str, lane: dict, allowlist: list[str], cfg: dict | None = None, route_binding: dict | None = None) -> dict:
    cfg = cfg or {}
    base = to_session(name, lane, allowlist, route_binding)
    claimed_task = str(lane.get("claimed_task") or "")
    note_path, fm = _task_note_for(claimed_task, cfg)
    refs: list[dict] = []
    if note_path:
        refs.append(_evidence_ref("cc_task_note", note_path, "private", raw_access=False))
    transcript_refs, evidence_scan = _transcript_refs(base["role"], base["platform"], cfg)
    refs.extend(transcript_refs)
    evidence_summary = _session_evidence_summary(refs, evidence_scan)
    task = {
        "task_id": claimed_task,
        "status": str(fm.get("status", "")),
        "assigned_to": str(fm.get("assigned_to", "")),
        "authority_case": str(fm.get("authority_case", "")),
        "parent_spec": str(fm.get("parent_spec", "")),
        "mutation_surface": str(fm.get("mutation_surface", "")),
        "updated_at": str(fm.get("updated_at", "")),
    }
    fields = {
        "role": base["role"],
        "platform": base["platform"],
        "state": base["state"],
        "session": base["session"],
        "readiness": base["readiness"],
        "blocker": base["blocker"],
        "attention": str(base["attention"]),
        "task_id": task["task_id"],
        "status": task["status"],
        "assigned_to": task["assigned_to"],
        "authority_case": task["authority_case"],
        "parent_spec": task["parent_spec"],
        "mutation_surface": task["mutation_surface"],
        "updated_at": task["updated_at"],
        "path": refs[0]["path"] if refs else "",
        "evidence_count": str(len(refs)),
        "resume_ready": "false",
    }
    return {
        "role": base["role"],
        "platform": base["platform"],
        "state": base["state"],
        "readiness": base["readiness"],
        "blocker": base["blocker"],
        "attention": base["attention"],
        "health": {
            "alive": base["alive"],
            "idle": base["idle"],
            "stalled": base["stalled"],
            "output_age_s": base["output_age_s"],
            "relay_age_s": base["relay_age_s"],
        },
        "tmux": {
            "session": base["session"],
            "exists": bool(base["session"] and base["alive"]),
            "attached": False,
            "activity_age_s": base["output_age_s"],
        },
        "task": task,
        "evidence_refs": refs,
        "evidence_summary": evidence_summary,
        "resume": {
            "intent": "session.resume",
            "ready": False,
            "authority": "supervisor_or_methodology_dispatch",
            "blocked_reasons": ["not_wired"],
        },
        "air": classify_air(fields, allowlist),
    }



def _vault_root(cfg: dict | None = None) -> Path:
    cfg = cfg or {}
    raw = os.environ.get("REINS_VAULT_ROOT") or str(cfg.get("vault_root") or "")
    if raw:
        return Path(os.path.expanduser(raw))
    return Path.home() / "Documents" / "Personal"


def _vault_note(path: Path, vault_root: Path) -> dict:
    rel = path.relative_to(vault_root)
    parts = rel.parts
    return {
        "title": path.stem,
        "rel_path": rel.as_posix(),
        "obsidian_uri": "obsidian://open?path=" + quote(str(path.absolute()), safe=""),
        "folder": parts[0] if len(parts) > 1 else "",
        "modified": _iso_mtime(path),
    }


def read_vault_summary(cfg: dict | None = None) -> dict:
    """List Obsidian vault notes as metadata only.

    Bodies are default-deny: this only stats paths and never opens note files.
    """
    root = _vault_root(cfg)
    if not root.is_dir() or not os.access(root, os.R_OK):
        return {"vault_root": str(root), "dark": True, "notes": []}

    notes: list[tuple[float, dict]] = []
    try:
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [
                d for d in dirnames
                if not d.startswith(".") and d not in {".obsidian", ".trash"}
            ]
            base = Path(dirpath)
            for filename in filenames:
                if not filename.endswith(".md"):
                    continue
                path = base / filename
                try:
                    mtime = path.stat().st_mtime
                    notes.append((mtime, _vault_note(path, root)))
                except OSError:
                    continue
    except OSError:
        return {"vault_root": str(root), "dark": True, "notes": []}

    notes.sort(key=lambda row: row[0], reverse=True)
    return {"vault_root": str(root), "dark": False, "notes": [row for _, row in notes[:500]]}


_OBSERVE_DIMENSION_KEYS = (
    "health",
    "drift",
    "nudges",
    "agents",
    "governance",
    "consent",
    "profile",
    "cost",
    "gpu",
    "stimmung",
)

_OBSERVE_LIST_KEYS = {
    "health": ("checks", "services", "failed_checks", "failures"),
    "drift": ("drift", "drifts", "items", "rows"),
    "nudges": ("nudges", "items", "rows"),
    "agents": ("agents", "lanes", "sessions", "rows"),
    "governance": ("gates", "tasks", "rows"),
    "consent": ("consents", "rows", "items"),
    "profile": ("dimensions", "facts", "rows"),
    "cost": ("records", "traces", "rows"),
    "gpu": ("gpus", "devices", "rows"),
    "stimmung": ("events", "states", "rows"),
}


def _observe_dim(key: str, status: str, summary: str, count: int | None) -> dict:
    if status == "dark":
        count = None
    elif count is not None:
        try:
            count = int(count)
        except (TypeError, ValueError):
            count = None
    return {
        "key": key,
        "status": "live" if status == "live" else "dark",
        "summary": str(summary or ""),
        "count": count,
    }


def _observe_dark(key: str, reason: str) -> dict:
    return _observe_dim(key, "dark", f"source dark: {reason}", None)


def _observe_api_base_url(cfg: dict | None = None) -> str:
    cfg = cfg or {}
    return str(
        os.environ.get("REINS_OBSERVE_API_URL")
        or os.environ.get("REINS_COCKPIT_API_URL")
        or cfg.get("observe_api_url")
        or cfg.get("cockpit_api_url")
        or "http://localhost:8051"
    ).strip()


def _observe_timeout_s(cfg: dict | None = None) -> float:
    cfg = cfg or {}
    raw = os.environ.get("REINS_OBSERVE_TIMEOUT_S") or cfg.get("observe_timeout_s") or 0.2
    try:
        return max(0.01, min(2.0, float(raw)))
    except (TypeError, ValueError):
        return 0.2


def _observe_http_json(url: str, timeout: float) -> Any:
    import urllib.request

    req = urllib.request.Request(url, headers={"Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        raw = resp.read(1024 * 1024)
    if not raw:
        return {}
    return json.loads(raw.decode("utf-8", errors="replace"))


def _observe_payload_count(key: str, payload: Any) -> int | None:
    if isinstance(payload, list):
        return len(payload)
    if not isinstance(payload, dict):
        return None
    for direct in ("count", "total", "total_count"):
        value = payload.get(direct)
        if isinstance(value, bool):
            continue
        if isinstance(value, (int, float)):
            return int(value)
    dims = payload.get("dimensions")
    if isinstance(dims, list):
        for dim in dims:
            if isinstance(dim, dict) and str(dim.get("key") or "") == key:
                count = dim.get("count")
                if isinstance(count, bool):
                    return None
                if isinstance(count, (int, float)):
                    return int(count)
                return None
    for list_key in _OBSERVE_LIST_KEYS.get(key, ()):
        value = payload.get(list_key)
        if isinstance(value, list):
            return len(value)
        if isinstance(value, dict):
            return len(value)
        if isinstance(value, bool):
            continue
        if isinstance(value, (int, float)):
            return int(value)
    return None


def _observe_payload_summary(key: str, payload: Any, count: int | None) -> str:
    if isinstance(payload, dict):
        for field in ("summary", "detail", "message"):
            value = payload.get(field)
            if isinstance(value, str) and value.strip():
                return value.strip()
        status = payload.get("status") or payload.get("state")
        if status:
            if count is None:
                return f"cockpit/logos api live: {key} status={status}"
            return f"cockpit/logos api live: {key} status={status}; count={count}"
    if count is not None:
        return f"cockpit/logos api live: {key} count={count}"
    return f"cockpit/logos api live: {key}"


def _observe_from_http(cfg: dict | None, key: str) -> dict:
    base = _observe_api_base_url(cfg)
    if not base:
        return _observe_dark(key, "observe api url not configured")
    url = base.rstrip("/") + "/" + quote(key, safe="")
    try:
        payload = _observe_http_json(url, _observe_timeout_s(cfg))
    except Exception as e:
        return _observe_dark(key, str(e))
    if isinstance(payload, dict) and payload.get("dark") is True:
        return _observe_dark(key, str(payload.get("error") or payload.get("summary") or "api returned dark"))
    count = _observe_payload_count(key, payload)
    return _observe_dim(key, "live", _observe_payload_summary(key, payload, count), count)


def _observe_session_dimensions() -> tuple[dict, dict]:
    try:
        raw = _raw_sessions()
    except Exception as e:
        reason = str(e)
        return _observe_dark("health", reason), _observe_dark("agents", reason)

    total = len(raw)
    stalled = sum(1 for _, lane in raw if lane.get("stalled"))
    offline = sum(1 for _, lane in raw if not lane.get("alive"))
    idle = sum(1 for _, lane in raw if lane.get("idle") and lane.get("alive") and not lane.get("stalled"))
    active = max(0, total - stalled - offline - idle)
    health = _observe_dim(
        "health",
        "live",
        f"coordinator_state live: lanes={total}; stalled={stalled}; offline={offline}",
        stalled + offline,
    )
    agents = _observe_dim(
        "agents",
        "live",
        f"coordinator_state live: lanes={total}; active={active}; idle={idle}; stalled={stalled}; offline={offline}",
        total,
    )
    return health, agents


def _observe_governance(cfg: dict | None) -> dict:
    cfg = cfg or {}
    council_root = str(cfg.get("council_root") or "")
    if not council_root:
        return _observe_dark("governance", "council_root not configured")
    try:
        tasks = (_projection(council_root).get("tasks") or {})
        if not isinstance(tasks, dict):
            tasks = {}
        blocked = 0
        for task in tasks.values():
            no_go = task.get("no_go") if isinstance(task, dict) else {}
            if isinstance(no_go, dict) and any(value is False for value in no_go.values()):
                blocked += 1
        return _observe_dim(
            "governance",
            "live",
            f"coord projection live: tasks={len(tasks)}; blocked={blocked}",
            blocked,
        )
    except Exception as e:
        return _observe_dark("governance", str(e))


def _dispatch_ledger_path(cfg: dict | None = None) -> Path:
    cfg = cfg or {}
    raw = os.environ.get("REINS_DISPATCH_LEDGER_PATH") or str(cfg.get("dispatch_ledger_path") or "")
    if raw:
        return Path(os.path.expanduser(raw))
    return Path.home() / ".cache" / "hapax" / "sdlc-routing" / "dispatch-events.jsonl"


def _observe_cost(cfg: dict | None) -> dict:
    path = _dispatch_ledger_path(cfg)
    if path.exists():
        records = _read_jsonl_tail(path, 1000)
        measured = 0
        total_cost = 0.0
        for record in records:
            value = record.get("cost_usd")
            if value is None:
                continue
            try:
                total_cost += float(value)
                measured += 1
            except (TypeError, ValueError):
                continue
        return _observe_dim(
            "cost",
            "live",
            f"dispatch ledger live: records={len(records)}; measured_cost={measured}; total_cost_usd={round(total_cost, 6)}",
            len(records),
        )

    cfg = cfg or {}
    council_root = str(cfg.get("council_root") or "")
    if council_root:
        traces = read_traces(council_root, 40)
        if not traces.get("dark"):
            rows = traces.get("traces") or []
            total_cost = 0.0
            measured = 0
            for row in rows:
                if not isinstance(row, dict):
                    continue
                try:
                    total_cost += float(row.get("cost") or 0.0)
                    measured += 1
                except (TypeError, ValueError):
                    continue
            return _observe_dim(
                "cost",
                "live",
                f"langfuse traces live: traces={len(rows)}; measured_cost={measured}; total_cost={round(total_cost, 6)}",
                len(rows),
            )
        return _observe_dark("cost", str(traces.get("error") or "dispatch ledger missing and traces dark"))
    return _observe_dark("cost", f"dispatch ledger missing: {path}")


def read_observe_summary(cfg: dict | None = None) -> dict:
    """Aggregate whole-system observation dimensions without minting authority or AIR policy.

    This is a raw read projection for the :observe page. Each dimension is independently honest:
    if its source is unreachable, that dimension is dark and carries no fabricated count.
    """
    cfg = cfg or {}
    health, agents = _observe_session_dimensions()
    local = {
        "health": health,
        "agents": agents,
        "governance": _observe_governance(cfg),
        "cost": _observe_cost(cfg),
    }

    dimensions: list[dict] = []
    for key in _OBSERVE_DIMENSION_KEYS:
        local_dim = local.get(key)
        if local_dim and local_dim["status"] == "live":
            dimensions.append(local_dim)
            continue
        api_dim = _observe_from_http(cfg, key)
        if api_dim["status"] == "live":
            dimensions.append(api_dim)
        elif local_dim is not None:
            dimensions.append(local_dim)
        else:
            dimensions.append(api_dim)

    return {
        "dark": not any(dim["status"] == "live" for dim in dimensions),
        "dimensions": dimensions,
    }


def _seed(council_root: str) -> dict:
    """The curated system-dynamics map (council-root-relative; instance-config pattern).
    This is the source :dynamics renders — it obsoletes the standalone :8765 cytoscape viewer."""
    import json
    root = os.path.expanduser(council_root)
    path = os.path.join(root, "docs", "architecture", "system-dynamics-map.seed.json")
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def _sdm_arch_dir(council_root: str) -> Path:
    return Path(os.path.expanduser(council_root)) / "docs" / "architecture"


def _basename(path: Path) -> str:
    return path.name


def _count_json_array(path: Path, key: str) -> int:
    try:
        data = _read_json(path)
    except OSError:
        return 0
    rows = data.get(key)
    return len(rows) if isinstance(rows, list) else 0


def _read_jsonl(path: Path, limit: int = 5000) -> list[dict]:
    return _read_jsonl_tail(path, limit)


def _dyn_source_count(path: Path, key: str = "") -> int:
    if not path.exists():
        return 0
    if key:
        return _count_json_array(path, key)
    try:
        data = _read_json(path)
    except OSError:
        return 0
    return len(data) if data else 1


def _manifest_lenses(manifest: dict, lenses_doc: dict) -> list[dict]:
    rows = manifest.get("lenses")
    if isinstance(rows, list) and rows:
        return rows
    rows = lenses_doc.get("lenses")
    return rows if isinstance(rows, list) else []


def _claim_rollups(claims: list[dict]) -> dict[tuple[str, str, str, str], int]:
    out: dict[tuple[str, str, str, str], int] = {}
    for claim in claims:
        key = (
            str(claim.get("element_kind") or "unknown"),
            str(claim.get("claim_type") or "unknown"),
            str(((claim.get("provenance") or {}).get("authority_ceiling")) or "unknown"),
            str(((claim.get("freshness") or {}).get("state")) or "unknown"),
        )
        out[key] = out.get(key, 0) + 1
    return out


def _observation_rollups(observations: list[dict]) -> dict[tuple[str, str, str], int]:
    out: dict[tuple[str, str, str], int] = {}
    for obs in observations:
        key = (
            str(obs.get("state") or "unknown"),
            str(obs.get("source_type") or "unknown"),
            str(obs.get("freshness") or "unknown"),
        )
        out[key] = out.get(key, 0) + 1
    return out


def _relation_rollups(relations: list[dict]) -> dict[tuple[str, str], dict[str, int]]:
    out: dict[tuple[str, str], dict[str, int]] = {}
    for rel in relations:
        allowed = rel.get("allowed_claim_types") or []
        if isinstance(allowed, list):
            claim_types = "+".join(str(x) for x in allowed)
        else:
            claim_types = str(allowed)
        key = (str(rel.get("category") or "unknown"), claim_types or "unknown")
        row = out.setdefault(key, {"relations": 0, "edges": 0})
        row["relations"] += 1
        edges = rel.get("edge_ids") or []
        row["edges"] += len(edges) if isinstance(edges, list) else 0
    return out


def _command_ref(command: Any) -> str:
    text = str(command or "").strip()
    if not text:
        return "declared"
    return text.split()[0]


def _str_list(value: Any, limit: int = 12) -> list[str]:
    if not isinstance(value, list):
        return []
    out: list[str] = []
    for item in value:
        text = str(item or "").strip()
        if text:
            out.append(text)
        if len(out) >= limit:
            break
    return out


def _workbench_scene(scene: Any, allowlist: list[str]) -> dict:
    if not isinstance(scene, dict):
        scene = {}
    selection = scene.get("selection") if isinstance(scene.get("selection"), dict) else {}
    fields = {
        "title": str(scene.get("title") or ""),
        "lens": str(scene.get("lens") or ""),
        "selection_group": str(selection.get("group") or ""),
        "selection_id": str(selection.get("id") or ""),
        "takeaway": str(scene.get("takeaway") or ""),
        "caveat": str(scene.get("caveat") or ""),
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _dynamics_workbench_contract(manifest: dict, allowlist: list[str]) -> dict:
    raw = manifest.get("workbench_contract") if isinstance(manifest, dict) else {}
    if raw is None:
        raw = {}
    if not isinstance(raw, dict):
        fields = {"status": "malformed", "missing": "workbench_contract"}
        return {
            **fields,
            "defaults": {},
            "inquiry_modes": [],
            "audience_modes": [],
            "explanation_paths": [],
            "follow_on_tranches": [],
            "air": classify_air(fields, allowlist),
        }
    status = "observed" if raw else "missing"
    defaults_raw = raw.get("defaults") if isinstance(raw.get("defaults"), dict) else {}
    defaults = {
        "inquiry_mode": str(defaults_raw.get("inquiry_mode") or ""),
        "audience_mode": str(defaults_raw.get("audience_mode") or ""),
        "explanation_path": str(defaults_raw.get("explanation_path") or ""),
    }
    defaults["air"] = classify_air(defaults, allowlist)

    inquiry_modes: list[dict] = []
    for row in raw.get("inquiry_modes") or []:
        if not isinstance(row, dict):
            continue
        fields = {
            "id": str(row.get("id") or ""),
            "label": str(row.get("label") or ""),
            "lens": str(row.get("lens") or ""),
            "prompt": str(row.get("prompt") or ""),
            "answer_shape": _str_list(row.get("answer_shape")),
            "focus_node_ids": _str_list(row.get("focus_node_ids"), 20),
            "focus_edge_ids": _str_list(row.get("focus_edge_ids"), 20),
        }
        inquiry_modes.append({**fields, "air": classify_air(fields, allowlist)})

    audience_modes: list[dict] = []
    for row in raw.get("audience_modes") or []:
        if not isinstance(row, dict):
            continue
        fields = {
            "id": str(row.get("id") or ""),
            "label": str(row.get("label") or ""),
            "emphasis": str(row.get("emphasis") or ""),
        }
        audience_modes.append({**fields, "air": classify_air(fields, allowlist)})

    explanation_paths: list[dict] = []
    for row in raw.get("explanation_paths") or []:
        if not isinstance(row, dict):
            continue
        scenes = [_workbench_scene(scene, allowlist) for scene in (row.get("scenes") or []) if isinstance(scene, dict)]
        fields = {
            "id": str(row.get("id") or ""),
            "label": str(row.get("label") or ""),
            "summary": str(row.get("summary") or ""),
            "must_include": _str_list(row.get("must_include")),
            "scene_count": _safe_int(row.get("scene_count") or len(scenes)),
            "scenes": scenes,
        }
        explanation_paths.append({**fields, "air": classify_air(fields, allowlist)})

    fields = {
        "status": status,
        "missing": "" if status == "observed" else "workbench_contract",
    }
    return {
        **fields,
        "defaults": defaults,
        "inquiry_modes": inquiry_modes,
        "audience_modes": audience_modes,
        "explanation_paths": explanation_paths,
        "follow_on_tranches": _str_list(raw.get("follow_on_tranches"), 10),
        "air": classify_air(fields, allowlist),
    }


def read_dynamics_package(council_root: str, allowlist: list[str]) -> dict:
    arch = _sdm_arch_dir(council_root)
    package_path = arch / "system-dynamics-map.package.json"
    manifest_path = arch / "system-dynamics-map.view-manifest.json"
    lock_path = arch / "system-dynamics-map.lock.json"
    claims_path = arch / "system-dynamics-map.claims.json"
    lenses_path = arch / "system-dynamics-map.lenses.json"
    observations_path = arch / "system-dynamics-map.observations.jsonl"
    relations_path = arch / "system-dynamics-map.relations.json"
    shacl_path = arch / "system-dynamics-map.shacl.ttl"
    trig_path = arch / "system-dynamics-map.canonical.trig"

    package_doc = _read_json(package_path) if package_path.exists() else {}
    manifest = _read_json(manifest_path) if manifest_path.exists() else {}
    lock = _read_json(lock_path) if lock_path.exists() else {}
    claims_doc = _read_json(claims_path) if claims_path.exists() else {}
    lenses_doc = _read_json(lenses_path) if lenses_path.exists() else {}
    relations_doc = _read_json(relations_path) if relations_path.exists() else {}
    claims = claims_doc.get("claims") if isinstance(claims_doc.get("claims"), list) else []
    lenses = _manifest_lenses(manifest, lenses_doc)
    observations = _read_jsonl(observations_path)
    relations = relations_doc.get("relations") if isinstance(relations_doc.get("relations"), list) else []

    sources = [
        _dyn_source("package", "observed" if package_doc else "missing", len(package_doc), allowlist, "package metadata", _path_age_bucket(package_path), _basename(package_path)),
        _dyn_source("manifest", "observed" if manifest else "missing", len(manifest), allowlist, "view manifest/workbench", _path_age_bucket(manifest_path), _basename(manifest_path)),
        _dyn_source("lock", "observed" if lock else "missing", len(lock), allowlist, "hash lock/staleness policy", _path_age_bucket(lock_path), _basename(lock_path)),
        _dyn_source("claims", "observed" if claims else "missing", len(claims), allowlist, "claim fragments", _path_age_bucket(claims_path), _basename(claims_path)),
        _dyn_source("lenses", "observed" if lenses else "missing", len(lenses), allowlist, "declared view lenses", _path_age_bucket(lenses_path), _basename(lenses_path)),
        _dyn_source("observations", "observed" if observations else "missing", len(observations), allowlist, "temporal observation state", _path_age_bucket(observations_path), _basename(observations_path)),
        _dyn_source("relations", "observed" if relations else "missing", len(relations), allowlist, "relation vocabulary", _path_age_bucket(relations_path), _basename(relations_path)),
        _dyn_source("canonical_trig", "present" if trig_path.exists() else "missing", 1 if trig_path.exists() else 0, allowlist, "canonical RDF graph; not parsed here", _path_age_bucket(trig_path), _basename(trig_path)),
        _dyn_source("shacl", "present" if shacl_path.exists() else "missing", 1 if shacl_path.exists() else 0, allowlist, "validation shapes; not parsed here", _path_age_bucket(shacl_path), _basename(shacl_path)),
    ]

    validation: list[dict] = []
    val_doc = package_doc.get("validation") if isinstance(package_doc.get("validation"), dict) else {}
    for key, command in sorted(val_doc.items()):
        validation.append(_dyn_row("validation", str(key), "declared", 1, allowlist, f"command_ref={_command_ref(command)}", "package", "info"))
    manifest_val = manifest.get("validation") if isinstance(manifest.get("validation"), dict) else {}
    for key, command in sorted(manifest_val.items()):
        validation.append(_dyn_row("validation", str(key), "declared", 1, allowlist, f"command_ref={_command_ref(command)}", "manifest", "info"))

    lens_rows: list[dict] = []
    for lens in lenses:
        agg = lens.get("aggregation") if isinstance(lens.get("aggregation"), dict) else {}
        count = _safe_int(lens.get("visible_node_count", len(lens.get("visible_node_ids") or [])))
        edges = _safe_int(lens.get("visible_edge_count", len(lens.get("visible_edge_ids") or [])))
        status = "lossy" if agg.get("lossy") else "lossless"
        detail = f"label={lens.get('label', '')}; mode={lens.get('state_mode', '')}; layout={lens.get('layout', '')}; edges={edges}; reversible={bool(agg.get('reversible'))}"
        lens_rows.append(_dyn_row("lens", str(lens.get("id") or ""), status, count, allowlist, detail, "lenses", "warn" if status == "lossy" else "info"))

    claim_rows = [
        _dyn_row("claim", f"{kind}:{claim_type}", authority, count, allowlist, f"freshness={freshness}", "claims", "info")
        for (kind, claim_type, authority, freshness), count in sorted(_claim_rollups(claims).items())
    ]
    obs_rows = [
        _dyn_row("observation", f"{state}:{source_type}", freshness, count, allowlist, f"state={state}; source_type={source_type}", "observations", "warn" if freshness == "stale" else "info")
        for (state, source_type, freshness), count in sorted(_observation_rollups(observations).items())
    ]
    rel_rows = [
        _dyn_row("relation", f"{category}:{claim_types}", "declared", vals["relations"], allowlist, f"edge_count={vals['edges']}", "relations", "info")
        for (category, claim_types), vals in sorted(_relation_rollups(relations).items())
    ]

    source_snapshot = manifest.get("source_snapshot") if isinstance(manifest.get("source_snapshot"), dict) else {}
    totals = {
        "sources": len(sources),
        "artifacts": len(package_doc.get("artifacts") or []),
        "nodes": _safe_int(source_snapshot.get("node_count")),
        "edges": _safe_int(source_snapshot.get("edge_count")),
        "claims": len(claims),
        "lenses": len(lens_rows),
        "observations": len(observations),
        "relations": len(relations),
        "validation": len(validation),
        "missing_sources": sum(1 for s in sources if s.get("status") == "missing"),
    }
    return {
        "sources": sources,
        "validation": validation,
        "lenses": lens_rows,
        "claims": claim_rows,
        "observations": obs_rows,
        "relations": rel_rows,
        "totals": totals,
        "authority_case": str(package_doc.get("authority_case") or manifest.get("authority_case") or ""),
        "generated_at": str(package_doc.get("generated_at") or manifest.get("generated_at") or lock.get("generated_at") or ""),
        "package_hash": str(lock.get("package_hash") or ""),
        "default_lens": str((lenses_doc.get("default_lens") if isinstance(lenses_doc, dict) else "") or manifest.get("default_projection") or ""),
        "workbench_contract": _dynamics_workbench_contract(manifest, allowlist),
    }


def _empty_epistemics(scope: str = "dynamics") -> dict:
    return {
        "schema_version": "epistemics.read.v1",
        "scope": scope,
        "authority_case": "",
        "generated_at": "",
        "package_hash": "",
        "sources": [],
        "rows": [],
        "totals": {},
    }


def _doc_ref_labels(value: Any, limit: int = 8) -> list[str]:
    """Return bounded source-ref labels only; doc labels/URLs/summaries are raw body-adjacent."""
    if not isinstance(value, list):
        return []
    out: list[str] = []
    for item in value:
        refs: list[Any] = []
        if isinstance(item, dict):
            raw_refs = item.get("source_refs") or item.get("evidence_refs") or item.get("refs")
            if isinstance(raw_refs, list):
                refs.extend(raw_refs)
            elif raw_refs:
                refs.append(raw_refs)
            for key in ("source_ref", "ref", "path"):
                if item.get(key):
                    refs.append(item.get(key))
        else:
            refs.append(item)
        for ref in refs:
            label = _source_ref_label(ref)
            if label:
                out.append(label)
            if len(out) >= limit:
                return out
    return out


def _append_unique_labels(out: list[str], labels: list[str], limit: int = 8) -> None:
    for label in labels:
        label = str(label or "").strip()
        if not label or label in out:
            continue
        out.append(label)
        if len(out) >= limit:
            return


def _field_ref_labels(row: dict, limit: int = 8) -> list[str]:
    return _doc_ref_labels([row], limit)


def _map_kind_token(value: Any) -> str:
    kind = str(value or "").strip().lower()
    if kind.startswith("map-"):
        kind = kind[4:]
    if kind in {"nodes", "node"}:
        return "node"
    if kind in {"edges", "edge"}:
        return "edge"
    return kind


def _map_element_ids(row: dict) -> list[str]:
    ids: list[str] = []
    for key in ("element_id", "element_ref", "map_id", "subject_ref", "subject", "node_id", "edge_id", "id"):
        value = str(row.get(key) or "").strip()
        if value and value not in ids:
            ids.append(value)
    return ids


def _index_map_ref_labels(index: dict[tuple[str, str], list[str]], kind: str, ids: list[str], labels: list[str]) -> None:
    if not ids or not labels:
        return
    kind = _map_kind_token(kind)
    for element_id in ids:
        for key in ((kind, element_id), ("", element_id)):
            existing = index.setdefault(key, [])
            _append_unique_labels(existing, labels)


def _dynamics_claim_ref_index(claims: list[dict]) -> dict[tuple[str, str], list[str]]:
    index: dict[tuple[str, str], list[str]] = {}
    for claim in claims:
        if not isinstance(claim, dict):
            continue
        labels: list[str] = []
        provenance = claim.get("provenance") if isinstance(claim.get("provenance"), dict) else {}
        _append_unique_labels(labels, _field_ref_labels(provenance))
        _append_unique_labels(labels, _field_ref_labels(claim))
        kind = _map_kind_token(claim.get("element_kind") or claim.get("map_kind") or claim.get("subject_kind"))
        _index_map_ref_labels(index, kind, _map_element_ids(claim), labels)
    return index


def _observation_evidence_labels(obs: dict, limit: int = 8) -> list[str]:
    labels: list[str] = []
    _append_unique_labels(labels, _field_ref_labels(obs), limit)
    evidence = obs.get("evidence")
    if not isinstance(evidence, list):
        return labels
    for item in evidence:
        if isinstance(item, dict):
            _append_unique_labels(labels, _field_ref_labels(item), limit)
            label = str(item.get("label") or "").strip()
            if label:
                _append_unique_labels(labels, [_source_ref_label(label)], limit)
        else:
            _append_unique_labels(labels, [_source_ref_label(item)], limit)
        if len(labels) >= limit:
            break
    return labels


def _dynamics_observation_ref_index(observations: list[dict]) -> dict[tuple[str, str], list[str]]:
    index: dict[tuple[str, str], list[str]] = {}
    for obs in observations:
        if not isinstance(obs, dict):
            continue
        kind = _map_kind_token(obs.get("element_kind") or obs.get("map_kind") or obs.get("subject_kind"))
        _index_map_ref_labels(index, kind, _map_element_ids(obs), _observation_evidence_labels(obs))
    return index


def _map_element_source_labels(
    kind: str,
    element_id: str,
    doc_labels: list[str],
    claim_refs: dict[tuple[str, str], list[str]],
    observation_refs: dict[tuple[str, str], list[str]],
    limit: int = 8,
) -> list[str]:
    labels: list[str] = []
    kind = _map_kind_token(kind)
    _append_unique_labels(labels, doc_labels, limit)
    for index in (claim_refs, observation_refs):
        _append_unique_labels(labels, index.get((kind, element_id), []), limit)
        _append_unique_labels(labels, index.get(("", element_id), []), limit)
    return labels


def _epistemics_source_refs(source_id: str, labels: list[str]) -> str:
    return f"{source_id}:{len(labels)} refs" if labels else ""


def _doc_count(value: Any) -> int:
    return len(value) if isinstance(value, list) else 0


def _epistemics_row(
    row_id: str,
    family: str,
    subject_kind: str,
    subject_ref: str,
    status: str,
    authority_case: str,
    source_id: str,
    source_ref_labels: list[str],
    freshness: str,
    allowlist: list[str],
    map_kind: str,
    map_id: str = "",
    map_source: str = "",
    map_target: str = "",
    map_relation: str = "",
    docs_count: int = 0,
) -> dict:
    evidence_count = len(source_ref_labels)
    fields = {
        "row_id": row_id,
        "family": family,
        "subject_kind": subject_kind,
        "subject_ref": subject_ref,
        "subject": subject_ref,
        "status": status,
        "posture": "source-backed" if evidence_count else "declared",
        "authority": "metadata-only",
        "authority_case": authority_case,
        "evidence_count": evidence_count,
        "evidence": f"source_refs:{evidence_count}" if evidence_count else "none",
        "source": source_id,
        "source_refs": _epistemics_source_refs(source_id, source_ref_labels),
        "source_ref_labels": source_ref_labels,
        "freshness": freshness,
        "privacy": "metadata-only",
        "raw_access": False,
        "missing": "" if evidence_count else "source_refs",
        "action": "none" if evidence_count else "attach source_refs",
        "detail": f"docs={docs_count}; source_refs={evidence_count}",
        "map_kind": map_kind,
        "map_id": map_id,
        "map_source": map_source,
        "map_target": map_target,
        "map_relation": map_relation,
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _epistemics_package_row(row: dict, authority_case: str, freshness: str, allowlist: list[str]) -> dict:
    family = str(row.get("kind") or "package").strip() or "package"
    item_id = str(row.get("id") or family).strip() or family
    source_id = str(row.get("source") or family).strip() or family
    count = _safe_int(row.get("count"))
    source_refs = f"{source_id}:{count} records" if count else ""
    status = str(row.get("status") or "").strip()
    posture = "declared" if family == "validation" or status == "declared" else "source-backed"
    missing = "" if count else "source records"
    detail = _epistemics_package_detail(row)
    row_id = _epistemics_package_row_id(family, item_id, status, detail)
    fields = {
        "row_id": row_id,
        "family": family,
        "subject_kind": "package-row",
        "subject_ref": item_id,
        "subject": item_id,
        "status": status,
        "posture": posture,
        "authority": "metadata-only",
        "authority_case": authority_case,
        "evidence_count": count,
        "evidence": f"count:{count}",
        "source": source_id,
        "source_refs": source_refs,
        "source_ref_labels": [],
        "freshness": freshness or str(row.get("severity") or ""),
        "privacy": "metadata-only",
        "raw_access": False,
        "missing": missing,
        "action": "none" if count else f"restore {source_id} source",
        "detail": detail,
        "map_kind": "package-row",
        "map_id": "",
        "map_source": "",
        "map_target": "",
        "map_relation": "",
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _safe_row_token(value: str) -> str:
    token = re.sub(r"[^A-Za-z0-9_.-]+", "-", value.strip()).strip("-")
    return token[:80]


def _epistemics_package_row_id(family: str, item_id: str, status: str, detail: str) -> str:
    parts = [family, item_id]
    if family in {"claim", "observation"}:
        for part in (status, detail):
            token = _safe_row_token(part)
            if token:
                parts.append(token)
    return ":".join(parts)


def _epistemics_package_detail(row: dict) -> str:
    family = str(row.get("kind") or "").strip()
    detail = str(row.get("detail") or "").strip()
    if family != "lens":
        return detail
    safe_parts: list[str] = []
    for part in detail.split(";"):
        key, sep, value = part.strip().partition("=")
        if sep and key in {"mode", "layout", "edges", "reversible"}:
            safe_parts.append(f"{key}={value.strip()}")
    return "; ".join(safe_parts)


def read_epistemics(council_root: str, allowlist: list[str], scope: str = "dynamics") -> dict:
    if scope != "dynamics":
        raise ValueError(f"unsupported epistemics scope: {scope}")
    seed = _seed(council_root)
    package = read_dynamics_package(council_root, allowlist)
    arch = _sdm_arch_dir(council_root)
    claims_path = arch / "system-dynamics-map.claims.json"
    observations_path = arch / "system-dynamics-map.observations.jsonl"
    claims_doc = _read_json(claims_path) if claims_path.exists() else {}
    claims = claims_doc.get("claims") if isinstance(claims_doc.get("claims"), list) else []
    observations = _read_jsonl(observations_path) if observations_path.exists() else []
    claim_refs = _dynamics_claim_ref_index(claims)
    observation_refs = _dynamics_observation_ref_index(observations)
    nodes = [n for n in (seed.get("nodes") or []) if isinstance(n, dict)]
    edges = [e for e in (seed.get("edges") or []) if isinstance(e, dict)]
    authority_case = str(package.get("authority_case") or "")
    generated_at = str(package.get("generated_at") or "")
    freshness = generated_at or "unknown"
    source_id = "seed"
    rows: list[dict] = []

    for node in nodes:
        node_id = str(node.get("id") or "")
        refs = _map_element_source_labels("node", node_id, _doc_ref_labels(node.get("docs")), claim_refs, observation_refs)
        rows.append(_epistemics_row(
            f"map-node:{node_id}",
            "dynamics",
            "map-node",
            node_id,
            str(node.get("status") or ""),
            authority_case,
            source_id,
            refs,
            freshness,
            allowlist,
            "node",
            map_id=node_id,
            docs_count=_doc_count(node.get("docs")),
        ))

    for edge in edges:
        edge_id = str(edge.get("id") or "")
        map_source = str(edge.get("source") or "")
        map_target = str(edge.get("target") or "")
        map_relation = str(edge.get("relation") or "")
        if not edge_id:
            edge_id = f"{map_source}:{map_relation}:{map_target}"
        refs = _map_element_source_labels("edge", edge_id, _doc_ref_labels(edge.get("docs")), claim_refs, observation_refs)
        rows.append(_epistemics_row(
            f"map-edge:{edge_id}",
            "dynamics",
            "map-edge",
            edge_id,
            str(edge.get("status") or ""),
            authority_case,
            source_id,
            refs,
            freshness,
            allowlist,
            "edge",
            map_id=edge_id,
            map_source=map_source,
            map_target=map_target,
            map_relation=map_relation,
            docs_count=_doc_count(edge.get("docs")),
        ))

    package_rows: list[dict] = []
    for family in ("validation", "lenses", "claims", "observations", "relations"):
        for package_row in package.get(family) or []:
            if isinstance(package_row, dict):
                package_rows.append(_epistemics_package_row(package_row, authority_case, freshness, allowlist))
    rows.extend(package_rows)

    seed_source = _dyn_source(
        "seed",
        "observed" if nodes or edges else "missing",
        len(nodes) + len(edges),
        allowlist,
        "system dynamics map seed metadata",
        _path_age_bucket(arch / "system-dynamics-map.seed.json"),
        "system-dynamics-map.seed.json",
    )
    sources = [seed_source, *(package.get("sources") or [])]
    return {
        "schema_version": "epistemics.read.v1",
        "scope": "dynamics",
        "authority_case": authority_case,
        "generated_at": generated_at,
        "package_hash": str(package.get("package_hash") or ""),
        "sources": sources,
        "rows": rows,
        "totals": {
            "sources": len(sources),
            "rows": len(rows),
            "map_nodes": len(nodes),
            "map_edges": len(edges),
            "package_rows": len(package_rows),
            "validation": len(package.get("validation") or []),
            "lenses": len(package.get("lenses") or []),
            "claims": len(package.get("claims") or []),
            "observations": len(package.get("observations") or []),
            "relations": len(package.get("relations") or []),
            "evidence_refs": sum(_safe_int(row.get("evidence_count")) for row in rows),
            "missing_evidence": sum(1 for row in rows if row.get("missing")),
            "missing_sources": _safe_int((package.get("totals") or {}).get("missing_sources")) + (0 if nodes or edges else 1),
        },
    }


def to_trace_row(t: dict, allowlist: list[str] | None = None) -> dict:
    """Fold a Langfuse trace into an operational Reins row: model/tokens/cost/latency.
    Input/output (operator content = PII) NEVER enter the row — the livestream-safe
    projection of an LLM call. AIR is classified against the operator allowlist like every
    other row kind — trace fields (model ids, spend, latency) default-DENY on air until the
    operator allowlists them; nothing here hardcodes ok."""
    tu = t.get("tokenUsage") or {}
    md = t.get("metadata") or {}
    row = {
        "ts": str(t.get("timestamp", "")),
        "trace_id": str(t.get("id", "")),
        "model": str(md.get("model") or md.get("litellm_model_id") or "?"),
        "prompt_tok": int(tu.get("prompt") or tu.get("input") or 0),
        "completion_tok": int(tu.get("completion") or tu.get("output") or 0),
        "total_tok": int(tu.get("total") or 0),
        "cost": round(float(t.get("totalPrice") or 0.0), 6),
        "latency_ms": int(round(float(t.get("duration") or 0.0) * 1000)),
    }
    row["air"] = classify_air(row, allowlist or [])
    return row


def read_traces(council_root: str, limit: int = 40, allowlist: list[str] | None = None) -> dict:
    """Fold the existing Langfuse LLM-observability plane into a recent-traces list (WS#2).
    Honest-dark when Langfuse is unreachable. Reuses shared.langfuse_client — never
    re-instruments LLM calls."""
    try:
        from hapax.spine.langfuse_client import is_available, langfuse_get
        if not is_available():
            return {"dark": True, "error": "langfuse unavailable", "traces": []}
        resp = langfuse_get("/traces", {"limit": limit, "orderBy": "timestamp.desc"})
        traces = [to_trace_row(t, allowlist) for t in (resp.get("data") or [])[:limit]]
        return {"dark": False, "traces": traces}
    except Exception as e:  # honest-dark
        return {"dark": True, "error": str(e), "traces": []}


def _projection(council_root: str) -> dict:
    from hapax.spine.coord_event_log import default_event_log
    from hapax.spine.coord_projection import CoordProjection

    return CoordProjection.from_replay(default_event_log().replay(fail_open=True)).to_record()


def _page_before(records: list[dict], before: str | None, limit: int) -> list[dict]:
    """Scrollback cursor over ordered (oldest->newest) records: return up to ``limit``
    records strictly older than ``before`` (ISO-8601 ts), or the newest ``limit`` when
    ``before`` is None. A pure presentation window over the coord spine's retained
    history — the canonical BitchX/irssi /lastlog affordance, never new authority."""
    if before:
        records = [r for r in records if str(r.get("timestamp", r.get("ts", ""))) < before]
    return records[-limit:] if limit > 0 else []


def _raw_tail(council_root: str, limit: int, before: str | None = None) -> list[dict]:
    from hapax.spine.coord_event_log import default_event_log
    result = default_event_log().replay(fail_open=True)
    out = [e.to_record() for e in result.events]
    return _page_before(out, before, limit)


class _BundleMalformed(Exception):
    """A producer bundle is PRESENT but unreadable/malformed — distinct from an ABSENT producer, so
    'producer broken' never masquerades as 'producer not emitting'."""


def _load_context_bundle(council_root: str) -> dict | None:
    """Load the producer's reins_context_fact_bundle (spine/council-emitted). Returns None when the producer
    is genuinely ABSENT; raises _BundleMalformed when a bundle is PRESENT but broken (the two render as
    different honest-dark reasons). Read only — never mints or fabricates a bundle.

    If REINS_CONTEXT_BUNDLE is set, it is used EXCLUSIVELY (a broken bundle at the explicit path must not
    silently fall through to the council default — that would hide operator misconfiguration)."""
    env = os.environ.get("REINS_CONTEXT_BUNDLE")
    paths = [env] if env else [os.path.join(os.path.expanduser(council_root or ""), "reins-context-bundle.json")]
    for p in paths:
        if not (p and os.path.exists(p)):
            continue
        try:
            with open(p) as f:
                b = json.load(f)
        except Exception as e:
            raise _BundleMalformed(f"{p}: {e}") from e
        if not isinstance(b, dict):
            raise _BundleMalformed(f"{p}: bundle is not a JSON object")
        return b
    return None


def build_app(council_root: str, allowlist: list[str], session_cfg: dict | None = None) -> FastAPI:
    app = FastAPI()
    session_cfg = session_cfg or {}

    @app.get("/read/events")
    def read_events(limit: int = 80, before: str | None = None) -> dict:
        try:
            raws = _raw_tail(council_root, limit, before)
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "events": []}
        now = time.time()
        events = [to_event(r, allowlist, age_s=_age_s(str(r.get("timestamp", r.get("ts", ""))), now)) for r in raws]
        return {"dark": False, "events": events}

    @app.get("/read/traces")
    def traces(limit: int = 40) -> dict:
        return read_traces(council_root, limit, allowlist)

    @app.get("/read/tasks")
    def read_tasks() -> dict:
        try:
            proj = _projection(council_root)
            hist = _task_history(council_root)
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "tasks": []}
        now = time.time()
        tasks = [to_task(tid, t, allowlist, hist, now) for tid, t in (proj.get("tasks") or {}).items()]
        return {"dark": False, "tasks": tasks}

    @app.get("/read/dynamics")
    def read_dynamics() -> dict:
        try:
            seed = _seed(council_root)
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "layers": [], "nodes": [], "edges": [], "package": {"sources": [], "validation": [], "lenses": [], "claims": [], "observations": [], "relations": [], "totals": {}}}
        try:
            package = read_dynamics_package(council_root, allowlist)
        except Exception as e:
            package = {
                "sources": [_dyn_source("package", "dark", 0, allowlist, str(e), "", "")],
                "validation": [], "lenses": [], "claims": [], "observations": [], "relations": [],
                "totals": {"sources": 1, "missing_sources": 1},
                "workbench_contract": _dynamics_workbench_contract({}, allowlist),
            }
        layers = [{"id": str(L.get("id", "")), "label": str(L.get("label", ""))} for L in (seed.get("layers") or [])]
        return {
            "dark": False,
            "map_id": str(seed.get("map_id", "")),
            "thesis": str(seed.get("thesis", "")),
            "layers": layers,
            "nodes": [to_node(n, allowlist) for n in (seed.get("nodes") or [])],
            "edges": [to_edge(e, allowlist) for e in (seed.get("edges") or [])],
            "package": package,
        }

    @app.get("/read/epistemics")
    def read_epistemics_endpoint(scope: str = "dynamics") -> dict:
        try:
            return {"dark": False, "error": "", "epistemics": read_epistemics(council_root, allowlist, scope)}
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "epistemics": _empty_epistemics(scope)}

    @app.get("/read/sessions")
    def read_sessions() -> dict:
        try:
            raw = _raw_sessions()
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "sessions": []}
        bindings, binding_source, binding_path = _route_binding_index(session_cfg)
        sessions = [
            to_session(
                name,
                lane,
                allowlist,
                _session_route_binding(name, lane, bindings, binding_source, binding_path),
            )
            for name, lane in raw
        ]
        return {"dark": False, "sessions": sorted(sessions, key=_session_sort_key)}

    @app.get("/read/session/{role}")
    def read_session_detail(role: str) -> dict:
        try:
            raw = dict(_raw_sessions())
            lane = raw.get(role)
            if lane is None:
                return {"dark": True, "error": f"unknown session role: {role}", "detail": {}}
            bindings, binding_source, binding_path = _route_binding_index(session_cfg)
            route_binding = _session_route_binding(role, lane, bindings, binding_source, binding_path)
            return {"dark": False, "detail": to_session_detail(role, lane, allowlist, session_cfg, route_binding)}
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "detail": {}}

    @app.get("/read/session/{role}/turns")
    def read_session_turns(role: str, before: str | None = None, limit: int = 80) -> dict:
        try:
            fixtures = _TURN_FIXTURES.get(role)
            if fixtures is None:
                return {
                    "dark": True,
                    "error": f"no turn replay fixture for session role: {role}",
                    "turns": [],
                    "oldest_ts": "",
                }
            # never-mint / never-false-green: these are hand-authored replay FIXTURES, not a
            # live CapabilityIO feed. They ship dark:true + fixture_only so no consumer can
            # label them "live — streaming"; the cockpit's own demo-fixture path renders the
            # honest "demo fixture — live turn feed dark" label instead.
            page = _page_before(fixtures, before, limit)
            turns = [to_turn(row, allowlist) for row in page]
            return {
                "dark": True,
                "fixture_only": True,
                "error": f"fixture-only turn replay for {role} — no live CapabilityIO producer",
                "turns": turns,
                "oldest_ts": turns[0]["ts"] if turns else "",
            }
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "turns": [], "oldest_ts": ""}

    @app.get("/read/session/{role}/turns/{ts}/blocks")
    def read_session_turn_blocks(role: str, ts: str) -> dict:
        try:
            # Consumer-ahead read wire: no fixture block data exists, and CapabilityIO
            # capture-output has not yet promoted real per-turn block streams into the READ API.
            # Stay honest-empty for every role/turn instead of fabricating demo detail blocks.
            return {
                "dark": True,
                "error": f"no turn block stream for session role/turn: {role}/{ts}",
                "blocks": [],
            }
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "blocks": []}

    @app.get("/read/intake")
    def read_intake() -> dict:
        try:
            return {"dark": False, "intake": read_intake_summary(session_cfg, allowlist)}
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "intake": {"sources": [], "rows": [], "totals": {}}}

    @app.get("/read/context")
    def read_context(audience: str = "") -> dict:
        # The tri-audience context substrate (operator / the Yard Crow / Hapax), AIR-sealed per audience.
        # Sources the producer bundle (spine/council, via REINS_CONTEXT_BUNDLE or <root>/reins-context-
        # bundle.json); HONEST-DARK until the producer emits — never a fabricated projection. Readout only.
        # `audience` scopes the response to ONE projection (default operator_private — the only caller today
        # is the loopback cockpit) so operator-private facts are not handed to a non-operator consumer once
        # yard/hapax wire in; project_all requires an explicit audience=all.
        aud = audience or "operator_private"
        if aud != "all" and aud not in reins_context.AUDIENCES:
            return {"dark": True, "reason": f"unknown audience {aud!r}", "audiences": list(reins_context.AUDIENCES)}
        try:
            bundle = _load_context_bundle(council_root)
        except _BundleMalformed as e:  # PRESENT but broken — a DIFFERENT dark than absent (operator misconfig)
            return {"dark": True, "reason": f"producer bundle present but malformed: {e}",
                    "audiences": list(reins_context.AUDIENCES)}
        if bundle is None:
            return {"dark": True, "reason": "context-fact-bundle producer not emitting yet",
                    "audiences": list(reins_context.AUDIENCES)}
        try:
            if aud == "all":
                return {"dark": False, "projections": reins_context.project_all(bundle)}
            return {"dark": False, "audience": aud, "projections": {aud: reins_context.project(bundle, aud)}}
        except Exception as e:  # a projection failure degrades to honest-dark, never a half-projection
            return {"dark": True, "reason": f"projection failed: {e}", "audiences": list(reins_context.AUDIENCES)}

    @app.get("/read/capabilities")
    def read_capabilities() -> dict:
        try:
            return {"dark": False, "capabilities": read_capability_summary(council_root, allowlist, session_cfg)}
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "capabilities": {"sources": [], "rows": [], "routes": [], "totals": {}}}

    @app.get("/read/gates")
    def read_gates() -> dict:
        try:
            return {"dark": False, "gates": read_gate_summary(council_root, allowlist, session_cfg)}
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "gates": {"sources": [], "rows": [], "totals": {}}}

    @app.get("/read/domains")
    def read_domains() -> dict:
        try:
            return {"dark": False, "domains": read_domain_pack_summary(session_cfg, allowlist)}
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "domains": {"sources": [], "rows": [], "relations": [], "totals": {}, "lifecycle_sources": [], "lifecycles": [], "lifecycle_totals": {}}}

    @app.get("/read/vault")
    def read_vault() -> dict:
        return read_vault_summary(session_cfg)

    @app.get("/read/observe")
    def read_observe() -> dict:
        observe_cfg = dict(session_cfg)
        if council_root and not observe_cfg.get("council_root"):
            observe_cfg["council_root"] = council_root
        return read_observe_summary(observe_cfg)

    @app.get("/read/facets")
    def read_facets() -> dict:
        # the facet-cut SSOT served in-band (A6: the decoder travels with the artifact) — the 9
        # facets + their role/channel/gloss/air, the (domain,attr) classification, and the air policy.
        # The Go cell-grammar encoder + legend consume this; one source, no Go↔Python drift.
        return facet_registry.facets_payload()

    return app


# Neutral on-air default — STRUCTURAL fields only (no free-text subject/label/summary, which can
# carry PII on-air; the instance opts those in only after verifying). Mirrors config.Defaults() in Go.
_DEFAULT_ALLOW = (
    "kind,score,ts,task_id,stage,no_go,id,layer,status,source,target,relation,res,"
    "role,platform,state,alive,idle,stalled,output_age_s,relay_age_s,readiness,blocker,attention,"
    "evidence_count,resume_ready,evidence_summary,by_kind,transcript_roots_observed,transcript_roots_missing,truncated,"
    "count,age_bucket,coverage,task_link_state,severity,privacy,raw_access,exists,"
    "capability_id,capability_class,surface_family,spend_model,egress_class,receipt_requirement,"
    "route_count,ok_count,blocked_count,hkp_posture,source_refs,source_ref_labels,route_id,mode,profile,model_id,effort,context_mode,fast_mode,quantization,capacity_pool,demand_vector,hardening,eval_plane,review_obligation,learning_eligibility,benchmark_coverage,fixed_overhead,"
    "route_state,authority_ceiling,freshness_ok,quota_state,receipt_count,blockers,authority,"
    "route_binding_state,"
    "tool_id,available,authority_use,observed_at,stale_after,"
    "schema_version,row_id,family,subject_kind,subject_ref,posture,map_kind,map_id,map_source,map_target,map_relation,"
    "gate_id,domain,evidence,missing,action,detail,generated_at,package_hash,default_lens,"
    "domain_id,lifecycle,terrain,depth,scope,claim_ceiling,windows,surfaces,parity,source_refs,"
    "lifecycle_id,owner,plant,posture,maturity,adapter_id,claim_surface,mutation_surface,"
    "dark_policy,freshness_policy,air_class,commands,receipt_contracts,next_evidence"
)


def instance_config() -> dict:
    """Resolve instance config with precedence env > $REINS_CONFIG toml > neutral defaults — the SAME
    config.toml the cockpit reads, so the substrate root + on-air allowlist have ONE source of truth.
    No baked operator path: absent config, council_root is empty and the API honest-darks (never a
    fabricated path)."""
    import tomllib

    toml_cfg: dict = {}
    path = os.environ.get("REINS_CONFIG") or os.path.expanduser("~/.config/reins/config.toml")
    try:
        with open(path, "rb") as fh:
            toml_cfg = tomllib.load(fh)
    except (FileNotFoundError, tomllib.TOMLDecodeError):
        toml_cfg = {}

    council_root = os.environ.get("REINS_COUNCIL_ROOT") or str(toml_cfg.get("council_root", ""))
    cc_tasks_active = os.environ.get("REINS_CC_TASKS_ACTIVE") or str(toml_cfg.get("cc_tasks_active", ""))
    orchestration_ledger_dir = (
        os.environ.get("REINS_ORCHESTRATION_LEDGER_DIR")
        or str(toml_cfg.get("orchestration_ledger_dir", ""))
    )
    domain_env = os.environ.get("REINS_DOMAIN_PACKS")
    if domain_env:
        domain_pack_paths = [v for v in domain_env.split(os.pathsep) if v]
    else:
        raw_domains = toml_cfg.get("domain_pack_paths", [])
        if isinstance(raw_domains, list):
            domain_pack_paths = [str(v) for v in raw_domains if str(v).strip()]
        elif raw_domains:
            domain_pack_paths = [str(raw_domains)]
        else:
            domain_pack_paths = []
    lifecycle_env = os.environ.get("REINS_LIFECYCLE_REGISTRIES")
    if lifecycle_env:
        lifecycle_registry_paths = [v for v in lifecycle_env.split(os.pathsep) if v]
    else:
        raw_lifecycles = toml_cfg.get("lifecycle_registry_paths", [])
        if isinstance(raw_lifecycles, list):
            lifecycle_registry_paths = [str(v) for v in raw_lifecycles if str(v).strip()]
        elif raw_lifecycles:
            lifecycle_registry_paths = [str(raw_lifecycles)]
        else:
            lifecycle_registry_paths = []
    capability_surface_env = os.environ.get("REINS_CAPABILITY_SURFACE_PACKS")
    if capability_surface_env:
        capability_surface_pack_paths = [v for v in capability_surface_env.split(os.pathsep) if v]
    else:
        raw_capability_surfaces = toml_cfg.get("capability_surface_pack_paths", [])
        if isinstance(raw_capability_surfaces, list):
            capability_surface_pack_paths = [str(v) for v in raw_capability_surfaces if str(v).strip()]
        elif raw_capability_surfaces:
            capability_surface_pack_paths = [str(raw_capability_surfaces)]
        else:
            capability_surface_pack_paths = []
    hkp_shadow_root = os.environ.get("REINS_HKP_SHADOW_ROOT") or str(toml_cfg.get("hkp_shadow_root", ""))
    hkp_index_root = os.environ.get("REINS_HKP_INDEX_ROOT") or str(toml_cfg.get("hkp_index_root", ""))
    hkp_report_root = os.environ.get("REINS_HKP_REPORT_ROOT") or str(toml_cfg.get("hkp_report_root", ""))
    vault_root = os.environ.get("REINS_VAULT_ROOT") or str(toml_cfg.get("vault_root", ""))
    hkp_bundles_env = os.environ.get("REINS_HKP_BUNDLES")
    if hkp_bundles_env:
        hkp_bundles = [v.strip() for v in hkp_bundles_env.split(",") if v.strip()]
    else:
        raw_hkp_bundles = toml_cfg.get("hkp_bundles", [])
        if isinstance(raw_hkp_bundles, list):
            hkp_bundles = [str(v) for v in raw_hkp_bundles if str(v).strip()]
        elif raw_hkp_bundles:
            hkp_bundles = [str(raw_hkp_bundles)]
        else:
            hkp_bundles = []
    intake_paths = {
        "request_intake_state": os.environ.get("REINS_REQUEST_INTAKE_STATE") or str(toml_cfg.get("request_intake_state", "")),
        "planning_feed_state": os.environ.get("REINS_PLANNING_FEED_STATE") or str(toml_cfg.get("planning_feed_state", "")),
        "p0_incident_state": os.environ.get("REINS_P0_INCIDENT_STATE") or str(toml_cfg.get("p0_incident_state", "")),
        "p0_incident_events": os.environ.get("REINS_P0_INCIDENT_EVENTS") or str(toml_cfg.get("p0_incident_events", "")),
        "security_signal_state": os.environ.get("REINS_SECURITY_SIGNAL_STATE") or str(toml_cfg.get("security_signal_state", "")),
    }
    transcript_env = os.environ.get("REINS_SESSION_TRANSCRIPT_ROOTS")
    if transcript_env:
        session_transcript_roots = transcript_env.split(os.pathsep)
    else:
        raw_roots = toml_cfg.get("session_transcript_roots", [])
        if isinstance(raw_roots, list):
            session_transcript_roots = list(raw_roots)
        elif raw_roots:
            session_transcript_roots = [str(raw_roots)]
        else:
            session_transcript_roots = []

    allow_env = os.environ.get("REINS_AIR_ALLOWLIST")
    if allow_env:
        allowlist = allow_env.split(",")
    elif toml_cfg.get("air_allowlist"):
        allowlist = list(toml_cfg["air_allowlist"])
    else:
        # registry-derived default (operator-approved 2026-06-26): airs the structural skeleton
        # (incl. the criticality/freshness/magnitude/gate/… repair the flat list omitted), denies
        # free-text bodies + PII. Proven safe by test_facet_registry. _DEFAULT_ALLOW kept as the
        # documented prior; env/toml override above still wins. Takes effect on API restart.
        allowlist = facet_registry.air_allowlist()

    port = os.environ.get("REINS_PORT")
    if not port:
        api_url = str(toml_cfg.get("api_url", ""))
        port = api_url.rsplit(":", 1)[-1] if ":" in api_url else "8799"

    return {
        "council_root": council_root,
        "allowlist": allowlist,
        "port": int(port),
        "cc_tasks_active": cc_tasks_active,
        "orchestration_ledger_dir": orchestration_ledger_dir,
        "lifecycle_registry_paths": lifecycle_registry_paths,
        "domain_pack_paths": domain_pack_paths,
        "capability_surface_pack_paths": capability_surface_pack_paths,
        "hkp_shadow_root": hkp_shadow_root,
        "hkp_index_root": hkp_index_root,
        "hkp_report_root": hkp_report_root,
        "hkp_bundles": hkp_bundles,
        "vault_root": vault_root,
        "session_transcript_roots": session_transcript_roots,
        **intake_paths,
    }


if __name__ == "__main__":  # pragma: no cover
    import uvicorn

    cfg = instance_config()
    uvicorn.run(build_app(cfg["council_root"], cfg["allowlist"], cfg), host="127.0.0.1", port=cfg["port"])
