"""K0 — the Stage-0 kernel, as extracted code (R0.3).

Membership is ratified and pinned; see first-init-k0-kernel-fixed-point-spec-2026-08-01.md
(pin b604b52bfdd9e267b7a5b68f42d020f233065f3c6d77eeb9f244de2d78ee6d59).

  core  fail-closed-default     -> fail_closed.py           BUILT
  core  refusal-as-data         -> refusal.py               BUILT
  core  receipt-primitive       -> ../bootstrap_receipt.py  BUILT
  core  fsm-phase-legality-law  -> ../bootstrap_receipt.py  BUILT (PHASE_LADDER, drift-pinned)
  core  identity-seed           -> identity.py              BUILT (non-PII, CSPRNG, never re-minted)
  seed  ratification-act        -> ratification.py          BUILT (R2.8 — performed, not merely recorded)

R0.11 is COMPLETE: ratifier.py binds a ratification to the sovereign (SSHSIG), recovery.py handles
key rotation and loss. R2.8 lands the act itself: ratification.py proposes a stipulation, obtains
the sovereign's signature, and witnesses it as an `act=ratified` row a stranger can verify later.
Ceremony progress is DERIVED from the chain (pending = proposed - ratified), so there is no wizard
cursor to corrupt, desynchronise, or fail to resume.

host_floor.py declares the kernel's OS-dependency floor as DATA (R1.4), so a stranger's box is told
what is required instead of discovering it by failing.

Estate-independent by construction: stdlib only, no paths, no host names, no operator identity.

## Rechecking every claim on this page

Each line above asserts something a reader would otherwise have to take on trust. None of it is
self-evident from the source, and a docstring that ages into a lie is worse than no docstring — so
each claim carries the command that re-establishes it, runnable from `api/`:

  BUILT, and the manifest agrees with the files
    uv run --with pytest pytest k0/ -q

  membership and the drift pin are the RATIFIED ones (fails if either moved)
    uv run python -c "import k0; print(len(k0.K0.members), k0.K0_DRIFT_PIN == k0.RATIFIED_PIN)"

  ceremony progress really is DERIVED (no cursor field exists to desynchronise)
    uv run python -c "import k0.ratification as r; print(r.pending.__doc__)"

  estate-independence, over the whole package, exempting nothing (ARMED — fails rather than
  skipping if the token file is absent, so green is always evidence the scan ran)
    K0_REQUIRE_ESTATE_SCAN=1 uv run --with pytest pytest k0/test_k0.py -q -k estate_independent

  the export list agrees with the module in BOTH directions
    uv run --with pytest pytest k0/test_k0.py -q -k all_agrees

The estate-independence scan needs the token file described in `api/conftest.py`; without it the
test SKIPS rather than passing, so an UNARMED green run on a stranger's checkout is not evidence
the scan ran. The armed form above sets `K0_REQUIRE_ESTATE_SCAN=1`, which converts the skip into a
FAIL — that is the only invocation whose green is a witness. `pytest -rs` shows the skip reason.
"""

from .fail_closed import Evaluation, decide, evaluate_optional
from .manifest import (
    K0,
    K0_DRIFT_PIN,
    RATIFIED_PIN,
    KernelManifestError,
    kernel_identity,
    verify,
    verify_minimality,
)
from .identity import EstateIdentity, IdentitySeedError, load_or_mint, mint_estate_id
from .host_floor import FLOOR, probe as probe_host_floor, require as require_host_floor
from .degradation import (
    Degradation,
    DegradationError,
    Lifecycle,
    accept as accept_degradation,
    declare as declare_degradation,
    lift as lift_degradation,
    render as render_degradation,
    state as degradation_state,
)
from .ratification import (
    RatificationError,
    RatificationVerdict,
    Stipulation,
    pending,
    propose,
    ratify,
    verify_ratifications,
)
from .ratifier import (
    RATIFICATION_NAMESPACE,
    RatifierError,
    sign_ratification,
    verify_ratification,
    write_allowed_signers,
)
from .recovery import SignerEntry, rotate, rotation_record, write_signers
from .refusal import DeadEndRefusalError, Refusal, RefusalError

__all__ = [
    "K0",
    "K0_DRIFT_PIN",
    "RATIFICATION_NAMESPACE",
    "FLOOR",
    "RATIFIED_PIN",
    "DeadEndRefusalError",
    "EstateIdentity",
    "Evaluation",
    "IdentitySeedError",
    "KernelManifestError",
    "Refusal",
    "SignerEntry",
    "RefusalError",
    "RatifierError",
    "decide",
    "kernel_identity",
    "probe_host_floor",
    "require_host_floor",
    "rotate",
    "rotation_record",
    "evaluate_optional",
    "load_or_mint",
    "mint_estate_id",
    "sign_ratification",
    "verify_ratification",
    "verify",
    "verify_minimality",
    "write_allowed_signers",
    "write_signers",
    # R2.6 degradation ledger and R2.8 ratification act. The branch that built these added their
    # IMPORTS and not their __all__ entries, so `from k0 import *` exposed the whole kernel except
    # its completing member. An export list that disagrees with what the module actually exports is
    # the same two-records-disagree defect the rest of this work has been removing; a test below
    # pins them together so the two cannot drift again.
    "Degradation",
    "DegradationError",
    "Lifecycle",
    "RatificationError",
    "RatificationVerdict",
    "Stipulation",
    "accept_degradation",
    "declare_degradation",
    "degradation_state",
    "lift_degradation",
    "pending",
    "propose",
    "ratify",
    "render_degradation",
    "verify_ratifications",
]
