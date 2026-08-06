#!/usr/bin/env python3
"""Build a SHRUNKEN copy of the example intake, internally consistent in every respect but size.

The negative control it feeds must fail on the CENSUS FLOOR and nothing else. An earlier version
edited only the program header and the orphan marker and left the coverage matrix untouched — so
the engine could reject it for an internal-consistency mismatch *before* ever comparing the census,
and the test would pass while proving nothing about re-narrowing detection. Every derived count is
updated here; only the census sidecar stays at the baseline, which is the single difference under
test.
"""

from __future__ import annotations

import pathlib
import sys

SRC = pathlib.Path("scripts/fixtures/example-intake.md")


def main(dest: str) -> None:
    t = SRC.read_text(encoding="utf-8")
    start = t.index("- **ex-beta**")
    end = t.index("### example-program-two")
    t = t[:start] + t[end:]
    t = t.replace("### example-program-one  (2 items)", "### example-program-one  (1 items)")
    t = t.replace("| example-program-one | 2 |", "| example-program-one | 1 |")
    t = t.replace("| obligation | 1 |\n", "")          # ex-beta was the only obligation item
    t = t.replace("| critic:program | 1 |\n", "")      # and its only critic:program source
    t = t.replace("**Orphan-check:** 3/3", "**Orphan-check:** 2/2")

    out = pathlib.Path(dest)
    out.write_text(t, encoding="utf-8")
    # Census stays at the BASELINE — that mismatch is the whole point of the control.
    pathlib.Path(str(out) + ".census.json").write_text(
        pathlib.Path(str(SRC) + ".census.json").read_text(encoding="utf-8"), encoding="utf-8"
    )


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: make_shrunk_fixture.py <dest.md>")
    main(sys.argv[1])
