"""Read-only ligatory obligations (`owes`) and capability-demand-fulfillment synthesis.

Projects unresolved obligations across lanebus inboxes, grant consumption, claim custody,
seat succession, locus anomalies, and capability route fulfillment without minting
authority, side-channels, or baked instance paths.
"""

from __future__ import annotations

import re
from collections.abc import Callable
from datetime import UTC, datetime
from typing import Any

REDACTION_TOKEN = "▒▒▒"

_SECRET_PATTERN = re.compile(
    r"(?i)(?:"
    r"\bsk-[A-Za-z0-9_-]{8,}\b"
    r"|\bgh[pousr]_[A-Za-z0-9_]{8,}\b"
    r"|\bbearer\s+[A-Za-z0-9._~+/=-]{6,}"
    r"|\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|secret|password|passwd|authorization)\s*[:=]\s*\S+"
    r"|\b\w*(?:secret[_-]?marker|sensitive[_-]?marker|synthetic[_-]?secret|canary[_-]?token|opaque[_-]?canary|synthetic[_-]?token|secret[_-]?token)\w*\b"
    r")"
)

_FS_PATH_ROOTS = frozenset(
    {
        "var",
        "tmp",
        "home",
        "mnt",
        "models",
        "opt",
        "usr",
        "srv",
        "data",
        "root",
        "etc",
    }
)

_LOCAL_ROUTE_INDICATORS = frozenset(
    {
        "local",
        "vllm",
        "ollama",
        "llama",
        "llama.cpp",
        "llama-server",
        "tabbyapi",
        "sglang",
        "substrate",
        "local_compute",
    }
)

_OBLIGATIONS_STRUCTURAL_ALLOW = (
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
    "fulfillment_state",
    "resident_models_count",
    "topology",
    "age_s",
    "creditor",
)


def _sanitize_scalar(val: Any) -> Any:
    if isinstance(val, str):
        cleaned = val.replace("\x00", "")
        return _SECRET_PATTERN.sub(REDACTION_TOKEN, cleaned)
    if isinstance(val, list):
        return [_sanitize_scalar(item) for item in val]
    if isinstance(val, dict):
        return {k: _sanitize_scalar(v) for k, v in val.items()}
    return val


def _redact_for_deny(val: Any) -> Any:
    if isinstance(val, str):
        return REDACTION_TOKEN
    if isinstance(val, list):
        return [REDACTION_TOKEN for _ in val]
    if isinstance(val, dict):
        return {k: _redact_for_deny(v) for k, v in val.items()}
    return val


def _classified(
    fields: dict[str, Any], allowlist: list[str], classify: Callable
) -> dict[str, Any]:
    sanitized = {k: _sanitize_scalar(v) for k, v in fields.items()}
    is_default_air = getattr(classify, "__name__", "") == "classify_air"
    eff_allow = (
        list(dict.fromkeys([*allowlist, *_OBLIGATIONS_STRUCTURAL_ALLOW]))
        if (allowlist and is_default_air)
        else allowlist
    )
    raw_air = classify(sanitized, eff_allow)
    air: dict[str, str] = {}
    for k in sanitized:
        dec = raw_air.get(k) if isinstance(raw_air, dict) else "deny"
        air[k] = "ok" if dec in ("ok", "allow") else "deny"
    deny_all = bool(air) and not any(v == "ok" for v in air.values())
    enforce_redact = deny_all or not allowlist or not is_default_air
    out: dict[str, Any] = {}
    for k, v in sanitized.items():
        if enforce_redact and air[k] != "ok":
            out[k] = _redact_for_deny(v)
        else:
            out[k] = v
    out["air"] = air
    return out


def _safe_int(val: Any) -> tuple[int, bool]:
    if val is None:
        return 0, True
    if isinstance(val, bool):
        return 0, False
    if isinstance(val, int):
        if val < 0 or val > 10**18:
            return 0, False
        return val, True
    if isinstance(val, str):
        s = val.strip()
        if not s:
            return 0, True
        if re.fullmatch(r"[+-]?\d{1,18}", s):
            try:
                parsed = int(s)
            except ValueError:
                return 0, False
            if parsed < 0:
                return 0, False
            return parsed, True
        return 0, False
    return 0, False


def _norm_model_token(val: str) -> str:
    s = str(val or "").strip().lower()
    if not s:
        return ""
    if "/" in s:
        first_seg = s.lstrip("/").split("/", 1)[0]
        if s.startswith("/") or first_seg in _FS_PATH_ROOTS or s.endswith(".gguf"):
            s = s.rsplit("/", 1)[-1]
    if s.endswith(":latest"):
        s = s[: -len(":latest")]
    if s.endswith(".gguf"):
        s = s[: -len(".gguf")]
    return s.strip()


def _parse_iso_epoch(ts: Any) -> float | None:
    if not isinstance(ts, str) or not ts.strip():
        return None
    try:
        dt = datetime.fromisoformat(ts.strip().replace("Z", "+00:00"))
    except (ValueError, AttributeError):
        return None
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=UTC)
    else:
        dt = dt.astimezone(UTC)
    return dt.timestamp()


def _norm_route_id(val: str) -> str:
    s = str(val or "").strip()
    if not s:
        return ""
    return s.split()[0].strip()


def _is_local_route(route: dict[str, Any]) -> bool:
    for key in (
        "locality",
        "provider",
        "platform",
        "mode",
        "route_id",
        "capability_id",
        "capacity_pool",
    ):
        raw = str(route.get(key) or "").strip().lower()
        if not raw:
            continue
        tokens = set(re.split(r"[^a-z0-9.]+", raw))
        if (
            tokens.intersection(_LOCAL_ROUTE_INDICATORS)
            or raw.startswith("local.")
            or ".local." in raw
        ):
            return True
    return False


def _source_is_dark(facet: Any) -> bool:
    if not isinstance(facet, dict) or not facet:
        return True
    if bool(facet.get("dark")):
        return True
    if str(facet.get("bundle_state") or "").lower() == "dark":
        return True
    eval_obj = facet.get("evaluation")
    if (
        isinstance(eval_obj, dict)
        and str(eval_obj.get("bundle_state") or "").lower() == "dark"
    ):
        return True
    return False


def _extract_context_facts(context_bundle: dict[str, Any]) -> list[dict[str, Any]]:
    if not isinstance(context_bundle, dict):
        return []
    direct = context_bundle.get("facts")
    if isinstance(direct, list):
        return [f for f in direct if isinstance(f, dict)]
    if isinstance(direct, dict):
        flattened: list[dict[str, Any]] = []
        for lst in direct.values():
            if isinstance(lst, list):
                flattened.extend(f for f in lst if isinstance(f, dict))
        return flattened
    projs = context_bundle.get("projections")
    if isinstance(projs, dict):
        for key in ("operator_private", "hapax_substrate", "yard_context"):
            sub = projs.get(key)
            if isinstance(sub, dict) and isinstance(sub.get("facts"), list):
                return [f for f in sub["facts"] if isinstance(f, dict)]
    return []


def read_obligations_summary(
    *,
    allowlist: list[str],
    classify: Callable,
    lanebus: dict[str, Any],
    grants: dict[str, Any],
    claim_custody: dict[str, Any],
    seat_succession: dict[str, Any],
    capabilities: dict[str, Any],
    context_bundle: dict[str, Any],
) -> dict[str, Any]:
    """Synthesize ligatory obligations (`owes`) and capability fulfillment across read facets."""
    sources = {
        "lanebus": lanebus,
        "grants": grants,
        "claim_custody": claim_custody,
        "seat_succession": seat_succession,
        "capabilities": capabilities,
        "context_bundle": context_bundle,
    }
    source_dark_map = {name: _source_is_dark(obj) for name, obj in sources.items()}
    overall_dark = all(source_dark_map.values())

    lanebus_owes: list[dict[str, Any]] = []
    inboxes = lanebus.get("inboxes") if isinstance(lanebus, dict) else None
    for inbox in inboxes or []:
        if not isinstance(inbox, dict):
            continue
        ack_mode = str(inbox.get("ack") or "")
        has_explicit_unacked = (
            "unacked_count" in inbox and inbox["unacked_count"] is not None
        )
        raw_count = (
            inbox["unacked_count"]
            if has_explicit_unacked
            else inbox.get("unread_count")
        )
        unacked, count_ok = _safe_int(raw_count)
        state = str(inbox.get("state") or "")
        if not count_ok:
            state = "unparseable"
            unacked = 0
        lane_name = str(inbox.get("lane") or inbox.get("inbox") or "")
        age_s = inbox.get("oldest_unacked_age_s")
        if age_s is None and not has_explicit_unacked:
            age_s = inbox.get("oldest_unread_age_s")
        if (unacked > 0 and ack_mode != "none") or state in (
            "missing",
            "unparseable",
            "stale",
        ):
            row = {
                "kind": "lanebus.unacked" if unacked > 0 else f"lanebus.{state}",
                "relation": "owes",
                "creditor": "lanebus",
                "debtor": lane_name,
                "subject": f"{unacked} unacked message(s)" if unacked > 0 else state,
                "count": unacked,
                "age_s": age_s,
                "state": state or "unacked",
                "attention": str(inbox.get("attention") or "warning"),
            }
            lanebus_owes.append(_classified(row, allowlist, classify))

    grant_owes: list[dict[str, Any]] = []
    grant_rows = (
        (
            grants.get("rows")
            if isinstance(grants.get("rows"), list)
            else grants.get("grants")
        )
        if isinstance(grants, dict)
        else None
    )
    for g in grant_rows or []:
        if not isinstance(g, dict):
            continue
        g_state = str(g.get("state") or g.get("spend_status") or "")
        attn = str(g.get("attention") or "")
        rec_present = g.get("grant_record_present")
        honors, honors_ok = _safe_int(g.get("honor_count"))
        if not honors_ok:
            g_state = "unparseable_honor_count"
        if g_state and g_state not in ("reconciled", "honored", "ok"):
            row = {
                "kind": f"grant.{g_state}",
                "relation": "owes",
                "creditor": "admission_authority",
                "debtor": str(
                    g.get("lane") or g.get("minting_lane") or g.get("grant_id") or ""
                ),
                "subject": str(g.get("grant_id") or ""),
                "route_id": str(g.get("route_id") or ""),
                "state": g_state,
                "attention": attn or "warning",
            }
            grant_owes.append(_classified(row, allowlist, classify))
        elif rec_present is True and honors == 0:
            row = {
                "kind": "grant.retained_unhonored",
                "relation": "owes",
                "creditor": "admission_authority",
                "debtor": str(g.get("minting_lane") or g.get("grant_id") or ""),
                "subject": str(g.get("grant_id") or ""),
                "route_id": "",
                "state": "retained_unhonored",
                "attention": "info",
            }
            grant_owes.append(_classified(row, allowlist, classify))

    custody_owes: list[dict[str, Any]] = []
    custody_rows = (
        claim_custody.get("rows") if isinstance(claim_custody, dict) else None
    )
    for c in custody_rows or []:
        if not isinstance(c, dict):
            continue
        c_state = str(c.get("custody_state") or c.get("status") or c.get("state") or "")
        if c_state in ("stale", "orphaned", "blocked", "conflict"):
            row = {
                "kind": f"claim_custody.{c_state}",
                "relation": "owes",
                "creditor": "task_steward",
                "debtor": str(
                    c.get("holder") or c.get("assigned_to") or c.get("task_id") or ""
                ),
                "subject": str(c.get("task_id") or ""),
                "state": c_state,
                "attention": "warning",
            }
            custody_owes.append(_classified(row, allowlist, classify))

    seat_rows = (
        seat_succession.get("rows") if isinstance(seat_succession, dict) else None
    )
    for s in seat_rows or []:
        if not isinstance(s, dict):
            continue
        pane_state = str(s.get("pane_state") or "")
        claims_obj = s.get("claims") if isinstance(s.get("claims"), dict) else {}
        if pane_state == "absent" and claims_obj.get("present") is True:
            row = {
                "kind": "seat_succession.claimed_without_pane",
                "relation": "owes",
                "creditor": "seat_steward",
                "debtor": str(s.get("role") or ""),
                "subject": str(s.get("session") or ""),
                "state": pane_state,
                "attention": "action",
            }
            custody_owes.append(_classified(row, allowlist, classify))

    locus_anomalies: list[dict[str, Any]] = []
    for src_name, is_dark in source_dark_map.items():
        if is_dark:
            locus_anomalies.append(
                _classified(
                    {
                        "kind": "source.dark",
                        "id": f"source:{src_name}",
                        "state": "dark",
                        "subject": src_name,
                        "attention": "warning",
                    },
                    allowlist,
                    classify,
                )
            )

    facts = (
        []
        if source_dark_map["context_bundle"]
        else _extract_context_facts(context_bundle)
    )
    bundle_ref_ts = (
        context_bundle.get("generated_at")
        or (
            (context_bundle.get("provenance") or {}).get("generated_at")
            if isinstance(context_bundle.get("provenance"), dict)
            else None
        )
        if isinstance(context_bundle, dict)
        else None
    )
    bundle_ref_epoch = _parse_iso_epoch(bundle_ref_ts)

    live_loci: list[dict[str, Any]] = []
    local_substrate_loci: list[dict[str, Any]] = []
    host_disk_resident_models: dict[str, list[str]] = {}

    for f in facts:
        if not isinstance(f, dict):
            continue
        f_kind = str(f.get("kind") or f.get("fact_type") or "")
        is_producer_fact = "fact_type" in f or isinstance(f.get("state"), dict)
        raw_state = f.get("state")
        reason_codes: list[str] = []
        if isinstance(raw_state, dict):
            f_state = str(raw_state.get("value_state") or "")
            raw_rc = raw_state.get("reason_codes")
            if isinstance(raw_rc, list):
                reason_codes = [str(x) for x in raw_rc]
        else:
            f_state = str(raw_state or "")
            raw_rc = f.get("reason_codes")
            if isinstance(raw_rc, list):
                reason_codes = [str(x) for x in raw_rc]

        raw_fresh = f.get("freshness_state")
        if raw_fresh is not None and str(raw_fresh).strip():
            freshness = str(raw_fresh).strip()
        else:
            freshness = "absent" if is_producer_fact else "fresh"

        obs_ts = (
            f.get("observed_at")
            or (
                (f.get("provenance") or {}).get("observed_at")
                if isinstance(f.get("provenance"), dict)
                else None
            )
            or (
                (f.get("confidence_basis") or {}).get("observed_at")
                if isinstance(f.get("confidence_basis"), dict)
                else None
            )
        )
        if obs_ts is not None:
            obs_ep = _parse_iso_epoch(obs_ts)
            if obs_ep is None:
                freshness = "absent"
            elif bundle_ref_epoch is not None:
                age_diff = bundle_ref_epoch - obs_ep
                if age_diff > 300 or age_diff < -60:
                    freshness = "stale"

        if freshness in ("stale", "aging") and f_state in (
            "live",
            "lit",
            "ok",
            "present",
        ):
            f_state = "stale"
        elif freshness == "absent" and f_state in ("live", "lit", "ok", "present"):
            f_state = "absent"
            if "observation_timestamp_absent" not in reason_codes:
                reason_codes.append("observation_timestamp_absent")

        f_id = str(f.get("id") or f.get("subject_ref") or f.get("fact_id") or "")
        payload = (
            f.get("payload")
            if isinstance(f.get("payload"), dict)
            else (f.get("value") if isinstance(f.get("value"), dict) else {})
        )

        if f_kind in ("host", "resource_vram"):
            host_name = str(
                payload.get("host")
                or f_id.removeprefix("resource_vram:")
                .removeprefix("host:")
                .split("/", 1)[0]
            )
            disk_models = payload.get("ollama_disk_resident_models")
            if isinstance(disk_models, list) and disk_models:
                host_disk_resident_models[host_name] = [
                    str(m) for m in disk_models if str(m).strip()
                ]

            if f_state == "dark" or "host_unreachable" in reason_codes:
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": "host.dark",
                            "id": f_id,
                            "state": "dark",
                            "subject": host_name or f_id,
                            "attention": "warning",
                        },
                        allowlist,
                        classify,
                    )
                )
            elif (
                f_state == "partial"
                or "partial_memory_observation" in reason_codes
                or "per_card_free_util_absent_backcompat_gpus" in reason_codes
            ):
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": "host.partial_observation",
                            "id": f_id,
                            "state": "partial",
                            "subject": host_name or f_id,
                            "attention": "info",
                        },
                        allowlist,
                        classify,
                    )
                )
            elif f_state not in ("live", "lit", "ok"):
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": f"host.{f_state or 'anomaly'}",
                            "id": f_id,
                            "state": f_state or "anomaly",
                            "subject": host_name or f_id,
                            "attention": "warning",
                        },
                        allowlist,
                        classify,
                    )
                )
        elif f_kind == "declared_endpoint":
            if (
                f_state == "absent"
                and "observation_timestamp_absent" in reason_codes
                and "LOST" not in reason_codes
                and payload.get("alive") is not False
            ):
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": "endpoint.unverified_freshness",
                            "id": f_id,
                            "state": "absent",
                            "subject": str(payload.get("port") or f_id),
                            "attention": "warning",
                        },
                        allowlist,
                        classify,
                    )
                )
            elif (
                f_state == "lost"
                or "LOST" in reason_codes
                or (f_state == "absent" and "not_declared" not in reason_codes)
            ):
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": "endpoint.lost",
                            "id": f_id,
                            "state": "lost",
                            "subject": str(payload.get("port") or f_id),
                            "attention": "action",
                        },
                        allowlist,
                        classify,
                    )
                )
        elif f_kind in ("locus", "loaded_model"):
            if f_kind == "loaded_model":
                model_str = str(payload.get("model") or "").strip()
                loaded = [model_str] if model_str else []
                raw_res = payload.get("resident_models")
                resident = (
                    [str(m) for m in raw_res if str(m).strip()]
                    if isinstance(raw_res, list)
                    else list(loaded)
                )
                admit = str(payload.get("admitting_row") or "").strip()
                admitted_by = [admit] if admit else []
                host_str = str(payload.get("host") or "")
                port_str = str(payload.get("endpoint") or payload.get("port") or "")
                topo_str = str(payload.get("topology") or "single-node")
            else:
                loaded = [
                    str(m)
                    for m in (payload.get("loaded_models") or [])
                    if str(m).strip()
                ]
                resident = [
                    str(m)
                    for m in (payload.get("resident_models") or [])
                    if str(m).strip()
                ]
                admitted_by = [
                    str(a) for a in (payload.get("admitted_by") or []) if str(a).strip()
                ]
                host_str = str(payload.get("host") or "")
                port_str = str(payload.get("port") or "")
                topo_str = str(payload.get("topology") or "single-node")

            norm_locus = {
                "id": f_id,
                "state": f_state,
                "freshness_state": freshness,
                "reason_codes": reason_codes,
                "payload": {
                    "host": host_str,
                    "port": port_str,
                    "loaded_models": loaded,
                    "resident_models": resident,
                    "admitted_by": admitted_by,
                    "topology": topo_str,
                },
            }
            live_loci.append(norm_locus)

            if f_state == "stale":
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": "locus.stale",
                            "id": f_id,
                            "state": "stale",
                            "subject": ",".join(loaded or resident) or f_id,
                            "attention": "warning",
                        },
                        allowlist,
                        classify,
                    )
                )
            elif "OLLAMA_PS_PROBE_FAILED" in reason_codes:
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": "locus.probe_failed",
                            "id": f_id,
                            "state": f_state,
                            "subject": f"{host_str}:{port_str}",
                            "attention": "warning",
                        },
                        allowlist,
                        classify,
                    )
                )
            elif (
                (loaded and not admitted_by)
                or f_state in ("unexplained", "hold")
                or "UNEXPLAINED" in reason_codes
                or "UNVERIFIED_WORKER_SHARD" in reason_codes
            ):
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": "locus.unexplained_occupancy",
                            "id": f_id,
                            "state": f_state,
                            "subject": ",".join(loaded) or f_id,
                            "attention": "warning",
                        },
                        allowlist,
                        classify,
                    )
                )
            if loaded or resident:
                local_substrate_loci.append(
                    _classified(
                        {
                            "id": f_id,
                            "host": host_str,
                            "port": port_str,
                            "state": f_state,
                            "loaded_models": loaded
                            if f_state in ("live", "lit", "ok")
                            else [],
                            "resident_models_count": len(resident),
                            "admitted_by": admitted_by,
                            "topology": topo_str,
                        },
                        allowlist,
                        classify,
                    )
                )

    for h_name, disk_models in host_disk_resident_models.items():
        already_resident = {
            _norm_model_token(m)
            for lf in live_loci
            if str((lf.get("payload") or {}).get("host") or "") == h_name
            for m in ((lf.get("payload") or {}).get("resident_models") or [])
        }
        missing_disk = [
            m for m in disk_models if _norm_model_token(m) not in already_resident
        ]
        if missing_disk:
            disk_locus_id = f"host:{h_name}:11434/disk-resident"
            live_loci.append(
                {
                    "id": disk_locus_id,
                    "state": "resident_only",
                    "freshness_state": "fresh",
                    "reason_codes": [],
                    "payload": {
                        "host": h_name,
                        "port": "11434",
                        "loaded_models": [],
                        "resident_models": missing_disk,
                        "admitted_by": [],
                        "topology": "single-node",
                    },
                }
            )

    capability_fulfillment: list[dict[str, Any]] = []
    cap_obj = (
        capabilities.get("capabilities")
        if isinstance(capabilities, dict)
        and isinstance(capabilities.get("capabilities"), dict)
        else (capabilities if isinstance(capabilities, dict) else {})
    )
    routes = (
        []
        if source_dark_map["capabilities"]
        else (cap_obj.get("routes") if isinstance(cap_obj, dict) else [])
    )
    if not isinstance(routes, list):
        routes = []

    for r in routes:
        if not isinstance(r, dict):
            continue
        r_id = _norm_route_id(str(r.get("route_id") or ""))
        cap_id = str(r.get("capability_id") or "")
        model_id = str(r.get("model_id") or r.get("model") or "")
        mode = str(r.get("mode") or "")
        r_state = str(r.get("route_state") or r.get("state") or "")
        norm_target = _norm_model_token(model_id)
        is_local = _is_local_route(r)

        occupied_loci: list[str] = []
        stale_loci: list[str] = []
        resident_loci: list[str] = []
        topologies: list[str] = []

        for lf in live_loci:
            lf_id = str(lf.get("id") or "")
            lf_state = str(lf.get("state") or "")
            lf_fresh = str(lf.get("freshness_state") or "absent")
            lp = lf.get("payload") if isinstance(lf.get("payload"), dict) else {}
            loaded = [
                str(m)
                for m in (lp.get("loaded_models") or [])
                if _norm_model_token(str(m))
            ]
            resident = [
                str(m)
                for m in (lp.get("resident_models") or [])
                if _norm_model_token(str(m))
            ]
            admitted_by = [
                _norm_route_id(str(a))
                for a in (lp.get("admitted_by") or [])
                if _norm_route_id(str(a))
            ]

            exact_admit = bool(r_id) and (r_id in admitted_by)
            exact_model_loaded = bool(norm_target) and any(
                _norm_model_token(m) == norm_target for m in loaded
            )
            exact_model_resident = bool(norm_target) and any(
                _norm_model_token(m) == norm_target for m in resident
            )

            matched_loaded = exact_model_loaded or (exact_admit and not norm_target)
            matched_resident = exact_model_resident or (
                exact_admit and not norm_target and not loaded
            )

            is_locus_live = (
                (not source_dark_map["context_bundle"])
                and lf_state in ("live", "lit", "ok", "present")
                and lf_fresh == "fresh"
            )

            if matched_loaded and loaded:
                if is_locus_live:
                    occupied_loci.append(lf_id)
                    topo = str(lp.get("topology") or "")
                    if topo and topo not in topologies:
                        topologies.append(topo)
                elif lf_state == "stale" or lf_fresh in ("stale", "aging"):
                    stale_loci.append(lf_id)
            elif matched_resident and lf_state not in ("dark", "absent", "stale"):
                resident_loci.append(lf_id)

        if r_state in ("blocked", "error", "dark", "refused"):
            f_state = "route_blocked_locus_occupied" if occupied_loci else r_state
        elif occupied_loci:
            f_state = "occupied_in_memory"
        elif stale_loci:
            f_state = "stale_locus"
        elif resident_loci:
            f_state = "resident_on_disk_only"
        elif is_local:
            f_state = "unfulfilled_local_locus"
        elif r_state in ("ok", "active", "observed", "live", "ready"):
            f_state = "admitted_remote_or_cloud"
        else:
            f_state = r_state or "unfulfilled"

        row = {
            "route_id": r_id,
            "capability_id": cap_id,
            "mode": mode,
            "model_id": model_id,
            "route_state": r_state,
            "fulfillment_state": f_state,
            "occupied_loci": occupied_loci,
            "resident_loci": resident_loci,
            "topology": ",".join(topologies),
        }
        capability_fulfillment.append(_classified(row, allowlist, classify))

    all_owes = lanebus_owes + grant_owes + custody_owes
    action_count = sum(
        1 for x in all_owes + locus_anomalies if x.get("attention") == "action"
    )
    warning_count = sum(
        1 for x in all_owes + locus_anomalies if x.get("attention") == "warning"
    )
    occupied_routes = sum(
        1
        for x in capability_fulfillment
        if x.get("fulfillment_state") == "occupied_in_memory"
    )
    occupied_loci_count = sum(1 for x in local_substrate_loci if x.get("loaded_models"))
    dark_sources_count = sum(1 for v in source_dark_map.values() if v)

    summary = _classified(
        {
            "total_owes": len(all_owes),
            "lanebus_owes_count": len(lanebus_owes),
            "grant_owes_count": len(grant_owes),
            "custody_owes_count": len(custody_owes),
            "locus_anomaly_count": len(locus_anomalies),
            "dark_sources_count": dark_sources_count,
            "action_count": action_count,
            "warning_count": warning_count,
            "routes_total": len(capability_fulfillment),
            "routes_occupied_in_memory": occupied_routes,
            "local_loci_occupied_in_memory": occupied_loci_count,
            "local_loci_total": len(local_substrate_loci),
        },
        allowlist,
        classify,
    )

    return {
        "dark": overall_dark,
        "partial_dark": dark_sources_count > 0,
        "dark_sources_count": dark_sources_count,
        "source_status": {
            k: ("dark" if v else "live") for k, v in source_dark_map.items()
        },
        "summary": summary,
        "lanebus_owes": lanebus_owes,
        "grant_owes": grant_owes,
        "custody_owes": custody_owes,
        "locus_anomalies": locus_anomalies,
        "local_substrate_loci": local_substrate_loci,
        "capability_fulfillment": capability_fulfillment,
    }
