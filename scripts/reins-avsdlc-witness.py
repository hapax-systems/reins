#!/usr/bin/env python3
"""reins-avsdlc-witness — the AVSDLC predict-then-confirm witness for a Reins cockpit frame.

Given a captured frame PNG (the visual witness artifact from reins-shot.sh) and a PRE-AUTHORED
VisualIntentRecord, this computes the realized per-region {luma, edge_energy} vector using the
CANONICAL council eval (shared/avsdlc_realized_vector.py + avsdlc_visual_intent.py) and confirms the
intent predicates, emitting a witness receipt JSON + a per-predicate dossier.

It is a LOCAL self-witness (a terminal-frame capture, not an OBS broadcast) — the independence is in
the DETERMINISTIC council metric computation + intent_pass evaluator, not a minted assertion. The
signed-receipt / OBS-moving fields are the operator's release gate, not this self-witness.

Usage:
  reins-avsdlc-witness.py --frame f.png --intent i.json [--pov local-terminal] [--out DIR] [--source-head SHA]
Exit: 0 if the intent PASSED, 1 if it FAILED, 2 on a malformed intent.
"""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
import sys
import time

import numpy as np
from PIL import Image

# engine/instance separation (G9): the council eval location is INSTANCE config, not engine constant —
# env-overridable, ~-relative default, never a baked absolute home path.
COUNCIL = os.path.expanduser(os.environ.get("REINS_COUNCIL_SHARED", "~/projects/hapax-spine"))


def _load(name: str, path: str):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod  # register BEFORE exec so frozen-dataclass __module__ resolves (py3.14)
    spec.loader.exec_module(mod)
    return mod


def main() -> None:
    ap = argparse.ArgumentParser(description="AVSDLC predict-then-confirm witness for a Reins frame")
    ap.add_argument("--frame", required=True, help="captured frame PNG")
    ap.add_argument("--intent", required=True, help="pre-authored VisualIntentRecord JSON")
    ap.add_argument("--pov", default="local-terminal", help="POV label (must match the intent predicates)")
    ap.add_argument("--out", default="", help="optional dir to write witness-receipt.json")
    ap.add_argument("--source-head", default="", help="git SHA / provenance of the rendered binary")
    args = ap.parse_args()

    rv = _load("avsdlc_realized_vector", os.path.join(COUNCIL, "avsdlc_realized_vector.py"))
    vi = _load("avsdlc_visual_intent", os.path.join(COUNCIL, "avsdlc_visual_intent.py"))

    frame = np.asarray(Image.open(args.frame).convert("RGB"))
    realized = rv.realized_vector_from_frame(frame, args.pov)

    with open(args.intent) as fh:
        intent_data = json.load(fh)
    record = vi.parse_intent_record(intent_data)
    if record is None:
        print("MALFORMED intent record (failed parse / disallowed region|metric|op)", file=sys.stderr)
        sys.exit(2)

    verdict = bool(vi.intent_pass(record, realized))
    intent_hash = vi.intent_hash_from_record(record)

    # G8 fix (per-intent binding): the council canonical intent_hash covers ONLY predicates+floor, so two
    # pane intents with identical predicate sets COLLIDE (axes/relational shared 1a52cf77… — a receipt could
    # not prove WHICH pane's intent it witnessed; wrong-intent substitution was invisible). Bind the intent's
    # declared identity into the receipt: every intent file MUST carry an `intent_id` matching its basename;
    # the binding hash chains identity onto the canonical hash. Fail-closed: an anonymous or mislabeled
    # intent is not witnessable. The council module is untouched (its hash stays canonical for cross-checks).
    intent_id = str(intent_data.get("intent_id", "")).strip()
    expected_id = os.path.splitext(os.path.basename(args.intent))[0]
    if not intent_id:
        print("REFUSED: intent carries no intent_id — an anonymous intent is not witnessable (G8)", file=sys.stderr)
        sys.exit(2)
    if intent_id != expected_id:
        print(f"REFUSED: intent_id {intent_id!r} != intent file identity {expected_id!r} — substitution guard (G8)", file=sys.stderr)
        sys.exit(2)
    intent_binding_hash = hashlib.sha256(f"{intent_id}:{intent_hash}".encode("utf-8")).hexdigest()

    frame_bytes = open(args.frame, "rb").read()
    content_hash = hashlib.sha256(frame_bytes).hexdigest()
    perceptual_digest = hashlib.sha256(json.dumps(realized, sort_keys=True).encode()).hexdigest()

    now = time.time()
    receipt = {
        "kind": "av_witness",
        "witness_mode": "reins-local-terminal-self-witness",
        "receipt_id": hashlib.sha256(f"{content_hash}{now}".encode()).hexdigest()[:24],
        "content_hash": content_hash,
        "active_source_head": args.source_head,
        "status": "pass" if verdict else "fail",
        "obs_moving": bool(frame.size > 0 and float(np.asarray(frame).std()) > 1.0),  # TUI analog: the frame actually rendered content
        "collected_at": now,
        "expires_at": now + 1800,
        "via": "reins-shot",
        "perceptual_digest": perceptual_digest,
        "intent_hash": intent_hash,
        "intent_id": intent_id,                      # WHICH intent this receipt witnessed (G8)
        "intent_binding_hash": intent_binding_hash,  # sha256(intent_id:intent_hash) — per-pane, collision-proof
        "intent_pass": verdict,
        "signature": "",  # operator HMAC key is the release gate, not this self-witness
    }

    predicates = []
    for p in record.predicates:
        val = vi._resolve(realized, p.pov_label, p.region, p.metric)
        predicates.append({
            "pov": p.pov_label, "region": p.region, "metric": p.metric,
            "op": p.op, "target": p.target, "critical": p.critical,
            "realized": None if val is None else round(val, 3),
            "holds": bool(val is not None and p.holds(val)),
        })

    out = {"receipt": receipt, "predicates": predicates, "realized_vector": realized}
    print(json.dumps(out, indent=2, default=float))
    if args.out:
        os.makedirs(args.out, exist_ok=True)
        with open(os.path.join(args.out, "witness-receipt.json"), "w") as fh:
            json.dump(out, fh, indent=2, default=float)
        print(f"\nwrote {os.path.join(args.out, 'witness-receipt.json')}", file=sys.stderr)

    print(f"\nINTENT {'PASS' if verdict else 'FAIL'} (intent_hash {intent_hash[:12]})", file=sys.stderr)
    sys.exit(0 if verdict else 1)


if __name__ == "__main__":
    main()
