"""R0.9 — the corpus, and the check that keeps it honest.

THE TEST THAT MATTERS IS `test_no_dangling_doctrine_reference`.

Without it this corpus decays exactly like every other record in this estate: someone adds a
`teaches="doctrine/new-thing"` to a refusal, nobody writes the entry, and the reference dangles
silently until a stranger hits it in a receipt and learns nothing. That failure mode has been
measured repeatedly here — work lands in code, the record is not written back, and the next reader
pays full price.

So the invariant is enforced where it can fail loudly and early: **every slug-addressed `teaches`
reference in kernel production code must resolve against the shipped corpus.** Adding a reference
without adding its entry breaks the build, which is the only mechanism that has ever reliably kept
a record current here.
"""

from __future__ import annotations

import pathlib
import re

import pytest

from k0 import doctrine
from k0.refusal import Refusal

K0_DIR = pathlib.Path(__file__).resolve().parent
TEACHES_RX = re.compile(r'teaches\s*=\s*"([^"]+)"')


def _production_sources() -> list[pathlib.Path]:
    """Kernel production modules. Tests are excluded deliberately: a test may reference a
    deliberately-absent slug to prove the degraded path, and that is not a dangling reference."""
    return sorted(p for p in K0_DIR.glob("*.py") if not p.name.startswith("test_"))


def test_no_dangling_doctrine_reference() -> None:
    dangling: list[tuple[str, str]] = []
    seen = 0
    for src in _production_sources():
        for ref in TEACHES_RX.findall(src.read_text(encoding="utf-8")):
            if not ref.startswith(doctrine.REF_PREFIX):
                continue  # inline lesson: carries its own text, needs no corpus
            seen += 1
            if doctrine.resolve(ref) is None:
                dangling.append((src.name, ref))

    assert seen > 0, (
        "no slug-addressed doctrine references found at all — the scanner is looking in the wrong "
        "place, which would let this test pass while enforcing nothing"
    )
    assert not dangling, (
        "doctrine references that resolve against nothing:\n"
        + "\n".join(f"  {f}: {r}" for f, r in dangling)
        + "\n\nAdd the entry to api/k0/doctrine.py. A `teaches` reference the surface cannot "
        "render teaches nobody anything."
    )


def test_resolve_returns_none_rather_than_raising_for_unknown() -> None:
    """Acts are legal without the corpus, merely less legible (manifest.py, R0.9). A missing entry
    must degrade the surface, never refuse the act."""
    assert doctrine.resolve("doctrine/not-a-real-slug") is None
    assert doctrine.resolve("") is None
    assert doctrine.resolve("k0.receipt-primitive: the chain is the audit") is None


def test_a_refusal_carrying_a_real_ref_can_be_rendered() -> None:
    r = Refusal(gate="host-floor", why="python3 not observable",
                legal_next="install python3 and re-probe", teaches="doctrine/host-floor")
    entry = doctrine.resolve(r.teaches)
    assert entry is not None
    assert entry.title
    assert entry.body
    assert entry.slug == "host-floor"


def test_corpus_is_read_only_at_runtime() -> None:
    """The corpus is data the kit carries, not state a caller may edit."""
    with pytest.raises(TypeError):
        doctrine.ENTRIES["host-floor"] = None  # type: ignore[index]


# A `test_no_estate_paths_in_the_corpus` was written here and REMOVED. It listed the forbidden
# tokens inline so it could scan for them — which put the guard's data inside the tree the guard
# reads, and `test_the_kernel_package_is_uncoupled_from_the_substrate_it_was_extracted_from`
# failed on this very file for carrying substrate vocabulary.
#
# That test's docstring already states the rule: "a guard's data must not sit inside what the
# guard reads", and records that a reviewer caught the identical shape before. The estate's
# existing guard is better designed than the one being added, so the addition goes rather than
# the rule.
#
# The property is not lost. `SUBSTRATE_TOKENS` covers substrate vocabulary from conftest.py, which
# is outside the scanned package; and review/closure_check.py verifies "no estate paths anywhere
# under api/k0" as R0.3's absent-in-tree predicate, from outside the tree — correct by the same
# rule that made the inline version wrong.
