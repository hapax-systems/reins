"""Read-only ligatory obligations (`owes`) and capability-demand-fulfillment synthesis.

Projects unresolved obligations across lanebus inboxes, grant consumption, claim custody,
seat succession, locus anomalies, and capability route fulfillment without minting
authority, side-channels, or baked instance paths.
"""

from __future__ import annotations

from collections.abc import Callable
from typing import Any


def _classified(fields: dict[str, Any], allowlist: list[str], classify: Callable) -> dict[str, Any]:
    return {**fields, "air": classify(fields, allowlist)}


def _norm_model_token(val: str) -> str:
    s = str(val or "").strip().lower()
    if "/" in s:
        s = s.rsplit("/", 1)[-1]
    if ":" in s:
        s = s.split(":", 1)[0]
    for suf in (".gguf", "-instruct", "-fp8", "-int4", "-awq"):
        if s.endswith(suf):
            s = s[: -len(suf)]
    return s


def _extract_context_facts(context_bundle: dict[str, Any]) -> list[dict[str, Any]]:
    if not isinstance(context_bundle, dict):
        return []
    direct = context_bundle.get("facts")
    if isinstance(direct, list):
        return direct
    projs = context_bundle.get("projections")
    if isinstance(projs, dict):
        for key in ("operator_private", "hapax_substrate", "yard_context"):
            sub = projs.get(key)
            if isinstance(sub, dict) and isinstance(sub.get("facts"), list):
                return sub["facts"]
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
    lanebus_owes: list[dict[str, Any]] = []
    for inbox in lanebus.get("inboxes") or []:
        if not isinstance(inbox, dict):
            continue
        ack_mode = str(inbox.get("ack") or "")
        unacked = int(inbox.get("unacked_count") or inbox.get("unread_count") or 0)
        state = str(inbox.get("state") or "")
        lane_name = str(inbox.get("lane") or inbox.get("inbox") or "")
        age_s = inbox.get("oldest_unacked_age_s")
        if age_s is None:
            age_s = inbox.get("oldest_unread_age_s")
        if (unacked > 0 and ack_mode != "none") or state in ("missing", "unparseable", "stale"):
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
    grant_rows = grants.get("rows") if isinstance(grants.get("rows"), list) else grants.get("grants")
    for g in grant_rows or []:
        if not isinstance(g, dict):
            continue
        g_state = str(g.get("state") or g.get("spend_status") or "")
        attn = str(g.get("attention") or "")
        rec_present = g.get("grant_record_present")
        honors = int(g.get("honor_count") or 0)
        if g_state and g_state not in ("reconciled", "honored", "ok"):
            row = {
                "kind": f"grant.{g_state}",
                "relation": "owes",
                "creditor": "admission_authority",
                "debtor": str(g.get("lane") or g.get("minting_lane") or g.get("grant_id") or ""),
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
    for c in claim_custody.get("rows") or []:
        if not isinstance(c, dict):
            continue
        c_state = str(c.get("custody_state") or c.get("status") or c.get("state") or "")
        if c_state in ("stale", "orphaned", "blocked", "conflict"):
            row = {
                "kind": f"claim_custody.{c_state}",
                "relation": "owes",
                "creditor": "task_steward",
                "debtor": str(c.get("holder") or c.get("assigned_to") or c.get("task_id") or ""),
                "subject": str(c.get("task_id") or ""),
                "state": c_state,
                "attention": "warning",
            }
            custody_owes.append(_classified(row, allowlist, classify))

    for s in seat_succession.get("rows") or []:
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

    facts = _extract_context_facts(context_bundle)

    locus_anomalies: list[dict[str, Any]] = []
    live_loci: list[dict[str, Any]] = []
    local_substrate_loci: list[dict[str, Any]] = []
    for f in facts:
        if not isinstance(f, dict):
            continue
        f_kind = str(f.get("kind") or f.get("fact_type") or "")
        raw_state = f.get("state")
        if isinstance(raw_state, dict):
            f_state = str(raw_state.get("value_state") or "")
        else:
            f_state = str(raw_state or "")
        f_id = str(f.get("id") or f.get("subject_ref") or f.get("fact_id") or "")
        payload = f.get("payload") if isinstance(f.get("payload"), dict) else (f.get("value") if isinstance(f.get("value"), dict) else {})

        if f_kind in ("host", "resource_vram") and f_state not in ("live", "lit"):
            locus_anomalies.append(
                _classified(
                    {
                        "kind": "host.dark",
                        "id": f_id,
                        "state": f_state,
                        "subject": str(payload.get("host") or f_id),
                        "attention": "warning",
                    },
                    allowlist,
                    classify,
                )
            )
        elif f_kind == "declared_endpoint" and f_state == "lost":
            locus_anomalies.append(
                _classified(
                    {
                        "kind": "endpoint.lost",
                        "id": f_id,
                        "state": f_state,
                        "subject": str(payload.get("port") or f_id),
                        "attention": "action",
                    },
                    allowlist,
                    classify,
                )
            )
        elif f_kind in ("locus", "loaded_model"):
            if f_kind == "loaded_model":
                model_str = str(payload.get("model") or "")
                loaded = [model_str] if model_str else []
                resident = [str(m) for m in (payload.get("resident_models") or loaded)]
                admit = str(payload.get("admitting_row") or "")
                admitted_by = [admit] if admit else []
                host_str = str(payload.get("host") or "")
                port_str = str(payload.get("endpoint") or payload.get("port") or "")
                topo_str = str(payload.get("topology") or "single-node")
                norm_locus = {
                    "id": f_id,
                    "state": f_state,
                    "payload": {
                        "host": host_str,
                        "port": port_str,
                        "loaded_models": loaded,
                        "resident_models": resident,
                        "admitted_by": admitted_by,
                        "topology": topo_str,
                    },
                }
            else:
                loaded = [str(m) for m in (payload.get("loaded_models") or [])]
                resident = [str(m) for m in (payload.get("resident_models") or [])]
                admitted_by = [str(a) for a in (payload.get("admitted_by") or [])]
                host_str = str(payload.get("host") or "")
                port_str = str(payload.get("port") or "")
                topo_str = str(payload.get("topology") or "single-node")
                norm_locus = f

            live_loci.append(norm_locus)
            if (loaded and not admitted_by) or f_state == "unexplained":
                locus_anomalies.append(
                    _classified(
                        {
                            "kind": "locus.unexplained_occupancy",
                            "id": f_id,
                            "state": f_state,
                            "subject": ",".join(loaded),
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
                            "loaded_models": loaded,
                            "resident_models_count": len(resident),
                            "admitted_by": admitted_by,
                            "topology": topo_str,
                        },
                        allowlist,
                        classify,
                    )
                )

    capability_fulfillment: list[dict[str, Any]] = []
    cap_obj = capabilities.get("capabilities") if isinstance(capabilities.get("capabilities"), dict) else capabilities
    routes = cap_obj.get("routes") if isinstance(cap_obj, dict) else []
    if not isinstance(routes, list):
        routes = []

    for r in routes:
        if not isinstance(r, dict):
            continue
        r_id = str(r.get("route_id") or "")
        cap_id = str(r.get("capability_id") or "")
        model_id = str(r.get("model_id") or r.get("model") or "")
        mode = str(r.get("mode") or "")
        r_state = str(r.get("route_state") or r.get("state") or "")
        norm_target = _norm_model_token(model_id)

        occupied_loci: list[str] = []
        resident_loci: list[str] = []
        topologies: list[str] = []
        for lf in live_loci:
            lf_id = str(lf.get("id") or "")
            lp = lf.get("payload") if isinstance(lf.get("payload"), dict) else {}
            loaded = [str(m) for m in (lp.get("loaded_models") or [])]
            resident = [str(m) for m in (lp.get("resident_models") or [])]
            admitted_by = [str(a) for a in (lp.get("admitted_by") or [])]
            matched_admit = any(r_id and r_id in a for a in admitted_by)
            matched_loaded = matched_admit or any(
                norm_target and (norm_target in _norm_model_token(m) or _norm_model_token(m) in norm_target)
                for m in loaded
            )
            matched_resident = any(
                norm_target and (norm_target in _norm_model_token(m) or _norm_model_token(m) in norm_target)
                for m in resident
            )
            if matched_loaded and loaded:
                occupied_loci.append(lf_id)
                topo = str(lp.get("topology") or "")
                if topo and topo not in topologies:
                    topologies.append(topo)
            elif matched_resident:
                resident_loci.append(lf_id)

        if occupied_loci:
            f_state = "occupied_in_memory"
        elif resident_loci:
            f_state = "resident_on_disk_only"
        elif r_state in ("ok", "active"):
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
    action_count = sum(1 for x in all_owes + locus_anomalies if x.get("attention") == "action")
    warning_count = sum(1 for x in all_owes + locus_anomalies if x.get("attention") == "warning")
    occupied_routes = sum(
        1 for x in capability_fulfillment if x.get("fulfillment_state") == "occupied_in_memory"
    )
    occupied_loci_count = sum(1 for x in local_substrate_loci if x.get("loaded_models"))

    summary = _classified(
        {
            "total_owes": len(all_owes),
            "lanebus_owes_count": len(lanebus_owes),
            "grant_owes_count": len(grant_owes),
            "custody_owes_count": len(custody_owes),
            "locus_anomaly_count": len(locus_anomalies),
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
        "dark": False,
        "summary": summary,
        "lanebus_owes": lanebus_owes,
        "grant_owes": grant_owes,
        "custody_owes": custody_owes,
        "locus_anomalies": locus_anomalies,
        "local_substrate_loci": local_substrate_loci,
        "capability_fulfillment": capability_fulfillment,
    }
