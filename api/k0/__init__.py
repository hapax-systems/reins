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
]
