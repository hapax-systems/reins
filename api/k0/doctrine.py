"""K0 member `portable-doctrine-corpus` — the teaching material, shipped in the kit. (R0.9)

WHAT THIS IS FOR
----------------
`Refusal` carries a `teaches` field: "the doctrine reference the projecting surface renders". Three
such references exist in kernel production code — `doctrine/host-floor`, `doctrine/identity-seed`,
`doctrine/ratifier-key` — and **none of them resolved against anything**. A surface rendering
`doctrine/host-floor` had nowhere to render it from, and a stranger reading that receipt learned
only that a slug existed.

R0.9's gap names the fix exactly: *"versioned doctrine corpus shipped in the kit; teaches: refs
resolve ONLY against it (never /home/hapax paths)."*

WHY A MODULE AND NOT A DATA DIRECTORY
--------------------------------------
The requirement is that references resolve against the kit and **never** against an estate path.
A data directory satisfies that by discipline — someone must remember not to write an absolute
path — and this estate has measured what happens to properties maintained by memory.

A module satisfies it **by construction**. There is no path to get wrong: the corpus travels inside
the package, `resolve()` is a dict lookup, and there is no filesystem access that could point at an
estate path even by mistake. Estate-independence stops being a rule someone follows and becomes a
thing that cannot be expressed.

LEGIBILITY, NOT LEGALITY
------------------------
`manifest.py` is precise about this member: *"R0.9 — installed by `install`. Teaching material
shipped in the kit; acts are legal without it, merely less legible."* A missing entry must
therefore never refuse an act. `resolve()` returns `None` for an unknown reference and the caller
renders the bare slug — degraded, honest, not fatal. The check that catches dangling references is
a **test**, which is where a legibility invariant belongs, rather than a runtime gate that could
turn a documentation gap into an outage.
"""

from __future__ import annotations

from dataclasses import dataclass
from types import MappingProxyType

#: Bumped when an entry's meaning changes, not when prose is polished. A receipt may record which
#: corpus version taught it, so a later reader can tell whether the lesson they are reading now is
#: the lesson that was rendered then.
CORPUS_VERSION = "1.0.0"

#: The prefix marking a slug-addressed reference. Anything else in a `teaches` field is an inline
#: lesson carrying its own text (`k0.<module>: <lesson>`) and needs no corpus.
REF_PREFIX = "doctrine/"


@dataclass(frozen=True)
class Entry:
    """One doctrine entry. `title` is what a surface shows; `body` is what it shows on request."""

    slug: str
    title: str
    body: str


def _e(slug: str, title: str, body: str) -> tuple[str, Entry]:
    return slug, Entry(slug=slug, title=title, body=body.strip())


_ENTRIES: dict[str, Entry] = dict(
    (
        _e(
            "host-floor",
            "The host floor is probed, never assumed",
            """
A host floor is the set of facts about this machine that later acts depend on: which interpreters
exist, which of them are usable, what the clock says, whether the durable root is actually durable.

The kernel does not assume any of it. It probes, records what it observed, and refuses when an
observation is missing rather than proceeding on a default. A default here would be a guess wearing
the costume of a fact, and every act downstream would inherit it without knowing.

If you are reading this in a refusal: something the floor requires was not observable. The refusal
names it, and names what you may legally do next. Install it, or grant access to it, and probe
again. The floor is data — it can be re-probed as often as you like, and a probe that now succeeds
is a real change in the world rather than a retry that got lucky.
""",
        ),
        _e(
            "identity-seed",
            "The estate identity is minted once, locally, and is not a name",
            """
An estate mints its own identifier at genesis, with an exclusive create: if the identifier already
exists, minting refuses rather than overwriting. Two estates sharing an identity would make every
receipt chain ambiguous about which estate it describes.

The identifier is deliberately not personal. It is not your name, your email, or your hostname, and
it carries no information about who you are — so receipts can be shown to someone else without
disclosing you along with them.

You cannot re-mint it to fix a mistake. That is the point: an identity replaceable on request would
not identify anything. If the seed is genuinely lost, that is a recovery ceremony, not a re-mint,
and it is recorded as one.
""",
        ),
        _e(
            "ratifier-key",
            "A ratification is bound to a key, not to a click",
            """
When you ratify something, the kernel records a signature over the exact bytes you were shown, made
with a key only you hold. It does not record that a button was pressed.

That distinction is the whole mechanism. A recorded click proves software reached a code path. A
signature proves the holder of a specific key saw specific content and consented to it — and it
keeps proving that later, to someone who does not trust the software that collected it.

Consequences worth knowing before relying on it:

  * Consent binds the bytes, never the identifier. Re-ratify if the content changes at all. A
    signature over an id alone would let the content be swapped underneath it.
  * The key can be rotated, and rotation is itself a ratified act, so the chain records who could
    sign at each point in time.
  * If the key is lost, past ratifications remain verifiable — they were valid when made. Only
    future ones need the recovery ceremony.
""",
        ),
    )
)

#: Read-only view: the corpus is data the kit carries, not state a caller may edit at runtime.
ENTRIES = MappingProxyType(_ENTRIES)


def resolve(ref: str) -> Entry | None:
    """Resolve a `teaches` reference against the shipped corpus.

    Returns `None` for an unknown slug, and for an inline lesson (anything without the
    `doctrine/` prefix), which carries its own text. **Never raises**: acts are legal without the
    corpus, merely less legible, so a missing entry degrades the surface rather than refusing.
    """
    if not ref or not ref.startswith(REF_PREFIX):
        return None
    return ENTRIES.get(ref[len(REF_PREFIX):])


def slugs() -> frozenset[str]:
    """Every slug the corpus can resolve. Used by the dangling-reference test."""
    return frozenset(ENTRIES)
