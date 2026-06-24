"""Thin READ service: folds the live coord ledger (via the council spine) into scored,
air-classified events. Engine code; the council root + ledger come from config (instance)."""
from __future__ import annotations
import math, os, re, sys, time
from typing import Any
from fastapi import FastAPI

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
    if not ts_str:
        return 0.0
    try:
        from datetime import datetime
        dt = datetime.fromisoformat(ts_str.replace("Z", "+00:00"))
        return max(0.0, now - dt.timestamp())
    except Exception:
        return 0.0


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
    root = os.path.expanduser(council_root)
    if root not in sys.path:
        sys.path.insert(0, root)
    from shared.coord_event_log import default_event_log  # noqa: E402

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
    fields = {
        "id": str(n.get("id", "")), "label": str(n.get("label", "")),
        "kind": str(n.get("kind", "")), "layer": str(n.get("layer", "")),
        "status": str(n.get("status", "")), "res": str(n.get("resolution", "")),
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def to_edge(e: dict, allowlist: list[str]) -> dict:
    fields = {
        "source": str(e.get("source", "")), "target": str(e.get("target", "")),
        "relation": str(e.get("relation", "")), "status": str(e.get("status", "")),
    }
    return {**fields, "air": classify_air(fields, allowlist)}


def _seed(council_root: str) -> dict:
    """The curated system-dynamics map (council-root-relative; instance-config pattern).
    This is the source :dynamics renders — it obsoletes the standalone :8765 cytoscape viewer."""
    import json
    root = os.path.expanduser(council_root)
    path = os.path.join(root, "docs", "architecture", "system-dynamics-map.seed.json")
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def _projection(council_root: str) -> dict:
    root = os.path.expanduser(council_root)
    if root not in sys.path:
        sys.path.insert(0, root)
    from shared.coord_event_log import default_event_log  # noqa: E402
    from shared.coord_projection import CoordProjection  # noqa: E402
    return CoordProjection.from_replay(default_event_log().replay(fail_open=True)).to_record()


def _raw_tail(council_root: str, limit: int) -> list[dict]:
    root = os.path.expanduser(council_root)
    if root not in sys.path:
        sys.path.insert(0, root)
    from shared.coord_event_log import default_event_log  # noqa: E402
    result = default_event_log().replay(fail_open=True)
    out: list[dict] = []
    for e in result.events[-limit:]:
        out.append(e.to_record())
    return out


def build_app(council_root: str, allowlist: list[str]) -> FastAPI:
    app = FastAPI()

    @app.get("/read/events")
    def read_events(limit: int = 80) -> dict:
        try:
            raws = _raw_tail(council_root, limit)
        except Exception as e:  # honest-dark
            return {"dark": True, "error": str(e), "events": []}
        now = time.time()
        events = [to_event(r, allowlist, age_s=_age_s(str(r.get("timestamp", r.get("ts", ""))), now)) for r in raws]
        return {"dark": False, "events": events}

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
            return {"dark": True, "error": str(e), "layers": [], "nodes": [], "edges": []}
        layers = [{"id": str(L.get("id", "")), "label": str(L.get("label", ""))} for L in (seed.get("layers") or [])]
        return {
            "dark": False,
            "map_id": str(seed.get("map_id", "")),
            "thesis": str(seed.get("thesis", "")),
            "layers": layers,
            "nodes": [to_node(n, allowlist) for n in (seed.get("nodes") or [])],
            "edges": [to_edge(e, allowlist) for e in (seed.get("edges") or [])],
        }

    return app


# Neutral on-air default — STRUCTURAL fields only (no free-text subject/label/summary, which can
# carry PII on-air; the instance opts those in only after verifying). Mirrors config.Defaults() in Go.
_DEFAULT_ALLOW = (
    "kind,score,ts,task_id,stage,no_go,id,layer,status,source,target,relation,res,"
    "prior_stage,predicted_stage,owner,freshness,criticality,rel_count"
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

    allow_env = os.environ.get("REINS_AIR_ALLOWLIST")
    if allow_env:
        allowlist = allow_env.split(",")
    elif toml_cfg.get("air_allowlist"):
        allowlist = list(toml_cfg["air_allowlist"])
    else:
        allowlist = _DEFAULT_ALLOW.split(",")

    port = os.environ.get("REINS_PORT")
    if not port:
        api_url = str(toml_cfg.get("api_url", ""))
        port = api_url.rsplit(":", 1)[-1] if ":" in api_url else "8799"

    return {"council_root": council_root, "allowlist": allowlist, "port": int(port)}


if __name__ == "__main__":  # pragma: no cover
    import uvicorn

    cfg = instance_config()
    uvicorn.run(build_app(cfg["council_root"], cfg["allowlist"]), host="127.0.0.1", port=cfg["port"])
