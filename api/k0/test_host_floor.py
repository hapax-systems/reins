"""The declared OS floor. An assumed dependency is one a stranger discovers by failing."""

from __future__ import annotations

import shutil

import pytest

from k0 import RefusalError
from k0.host_floor import FLOOR, probe, require


def test_the_floor_declares_ssh_keygen_with_a_reason_and_a_minimum():
    e = next(x for x in FLOOR if x.binary == "ssh-keygen")
    assert e.min_version == (9, 1), (
        "9.1 is where Z-suffixed UTC timestamps are accepted; validity windows and verify-time "
        "arrived in 8.7. Both modules emit the Z suffix, so 9.1 is the real floor."
    )
    assert "verify-time" in e.why or "rotation" in e.why


def test_probe_reports_what_is_actually_present():
    got = probe()
    assert set(got) == {e.binary for e in FLOOR}
    assert got["ssh-keygen"] is not None, "this host must have OpenSSH for the lane to run"


def test_this_host_satisfies_the_floor():
    require()  # no raise


def test_an_absent_dependency_REFUSES_rather_than_being_assumed(monkeypatch):
    """The arm that made declare_durable_root accept /dev/shm: unevaluable must not pass."""
    monkeypatch.setattr(shutil, "which", lambda _: None)
    with pytest.raises(RefusalError) as e:
        require()
    assert "DENIES rather than being assumed" in e.value.refusal.why
    assert e.value.refusal.legal_next


def test_a_too_old_dependency_refuses(monkeypatch):
    import k0.host_floor as hf
    monkeypatch.setattr(hf, "_detect", lambda entry: (7, 9) if entry.min_version else (2, 40))
    with pytest.raises(RefusalError) as e:
        hf.require()
    assert "below the declared floor" in e.value.refusal.why


def test_every_floor_entry_states_why_the_kernel_needs_it():
    for e in FLOOR:
        assert e.why.strip(), f"{e.binary} declared with no justification"
