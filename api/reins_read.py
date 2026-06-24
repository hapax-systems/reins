"""Thin READ service: folds the live coord ledger (via the council spine) into scored,
air-classified events. Engine code; the council root + ledger come from config (instance)."""
from __future__ import annotations
import os, sys, time
from typing import Any
from fastapi import FastAPI

_KIND_SEVERITY = {  # kind -> base severity in [0,1]; escalations rank above routine
    "review.fail": 0.95, "session.ended": 0.4, "pr.merged": 0.6,
    "task.closed": 0.55, "stage": 0.45, "status": 0.4,
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

    return app


if __name__ == "__main__":  # pragma: no cover
    import uvicorn
    root = os.environ.get("REINS_COUNCIL_ROOT", "~/projects/hapax-spine")
    allow = os.environ.get("REINS_AIR_ALLOWLIST", "kind,subject,score,ts").split(",")
    uvicorn.run(build_app(root, allow), host="127.0.0.1", port=int(os.environ.get("REINS_PORT", "8799")))
