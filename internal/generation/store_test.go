package generation

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func stageSample(t *testing.T, s *Store, sha, prev string) {
	t.Helper()
	if _, err := s.Stage(sha, []byte("binary-"+sha), map[string][]byte{
		"reins_serve.py": []byte("serve-" + sha),
		"reins_read.py":  []byte("read-" + sha),
	}, "2026-07-02T00:00:00Z", prev); err != nil {
		t.Fatalf("stage %s: %v", sha, err)
	}
}

// A staged generation round-trips: manifest binds byte-hashes, Verify passes, SetCurrent + Resolve pick it.
func TestStageVerifyResolveRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())
	stageSample(t, s, "sha-a", "")
	m, err := s.ReadManifest("sha-a")
	if err != nil {
		t.Fatal(err)
	}
	if m.BinarySHA256 == "" || m.APITreeSHA256 == "" {
		t.Fatal("manifest must bind binary + api-tree byte-hashes")
	}
	if err := s.Verify("sha-a"); err != nil {
		t.Fatalf("a freshly-staged generation must verify: %v", err)
	}
	if err := s.SetCurrent("sha-a"); err != nil {
		t.Fatal(err)
	}
	r := s.Resolve()
	if r.Tier != TierCurrent || r.SHA != "sha-a" {
		t.Fatalf("resolve should pick current sha-a, got %+v", r)
	}
}

// The manifest BINDS BYTES: a tampered binary fails Verify and, once quarantined, is never resolved.
func TestTamperedBinaryFailsVerifyAndQuarantines(t *testing.T) {
	s := NewStore(t.TempDir())
	stageSample(t, s, "sha-a", "")
	// tamper the binary on disk (a truncation/swap attack, or bit-rot).
	if err := os.WriteFile(filepath.Join(s.GenerationDir("sha-a"), "reins"), []byte("EVIL"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify("sha-a"); err == nil {
		t.Fatal("a tampered binary MUST fail Verify (bytes no longer match the manifest)")
	}
	_ = s.SetCurrent("sha-a")
	if r := s.Resolve(); r.SHA == "sha-a" {
		t.Fatalf("an unverifiable current must NOT be selected for exec, got %+v", r)
	}
}

// A tampered api/ tree also fails Verify (the whole-sha generation, not just the binary).
func TestTamperedAPITreeFailsVerify(t *testing.T) {
	s := NewStore(t.TempDir())
	stageSample(t, s, "sha-a", "")
	if err := os.WriteFile(filepath.Join(s.GenerationDir("sha-a"), "api", "reins_serve.py"), []byte("mutated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify("sha-a"); err == nil {
		t.Fatal("a tampered api tree MUST fail Verify")
	}
}

// APITreeHash is deterministic regardless of map iteration order.
func TestAPITreeHashDeterministic(t *testing.T) {
	a := APITreeHash(map[string][]byte{"z.py": []byte("1"), "a.py": []byte("2"), "m.py": []byte("3")})
	b := APITreeHash(map[string][]byte{"a.py": []byte("2"), "m.py": []byte("3"), "z.py": []byte("1")})
	if a != b {
		t.Fatal("APITreeHash must be order-independent")
	}
	if a == APITreeHash(map[string][]byte{"a.py": []byte("2"), "m.py": []byte("3"), "z.py": []byte("CHANGED")}) {
		t.Fatal("APITreeHash must change when content changes")
	}
}

// Verify must IGNORE __pycache__/*.pyc and non-.py files a running interpreter or build drops into a
// served generation's api/ — the canonical set is {top-level *.py}, so pollution must not spuriously
// quarantine a valid generation (the U6 review MEDIUM).
func TestVerifyIgnoresPycachePollution(t *testing.T) {
	s := NewStore(t.TempDir())
	stageSample(t, s, "sha-a", "")
	apiDir := filepath.Join(s.GenerationDir("sha-a"), "api")
	// a running python interpreter writes bytecode + a cache dir into the served tree
	if err := os.MkdirAll(filepath.Join(apiDir, "__pycache__"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "__pycache__", "reins_serve.cpython-312.pyc"), []byte("BYTECODE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "uv.lock"), []byte("lock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify("sha-a"); err != nil {
		t.Fatalf("pycache/lockfile pollution must NOT fail Verify (only top-level .py is byte-bound): %v", err)
	}
	// but a tampered .py in the SAME dir still fails (the canonical set is still checked)
	if err := os.WriteFile(filepath.Join(apiDir, "reins_serve.py"), []byte("mutated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify("sha-a"); err == nil {
		t.Fatal("a tampered .py must still fail Verify despite the ignored pollution")
	}
}

// Cross-language parity: APITreeHash must equal the digest reins_serve._framed/api_tree_sha computes
// for the SAME tree (test_reins_serve.py pins the identical constant). If either side's construction
// drifts, GEN-SKEW (U6b) would false-positive; this constant is the shared contract.
func TestAPITreeHashCrossLanguageParity(t *testing.T) {
	const want = "1cc45033fa146f2fbbef2a1bdb0d9e0f651e5a9949d63a6ea0bb1d77ebd9e540"
	got := APITreeHash(map[string][]byte{"reins_read.py": []byte("READ-BODY"), "reins_serve.py": []byte("SERVE-BODY")})
	if got != want {
		t.Fatalf("APITreeHash drifted from the cross-language contract:\n got  %s\n want %s", got, want)
	}
	if got[:16] != "1cc45033fa146f2f" {
		t.Fatal("the :16 prefix (the server's api_tree_sha witness) must match")
	}
}

// APITreeHash length-framing: a boundary shift between name and content must NOT collide.
func TestAPITreeHashLengthFramed(t *testing.T) {
	a := APITreeHash(map[string][]byte{"ab": []byte("c")})
	b := APITreeHash(map[string][]byte{"a": []byte("bc")})
	if a == b {
		t.Fatal("length-framing must prevent name/content boundary-shift collisions")
	}
}

// Three-tier rollback: verified current wins; a quarantined/broken current falls to a verified prev;
// neither verified => breakglass.
func TestThreeTierRollbackResolver(t *testing.T) {
	s := NewStore(t.TempDir())
	stageSample(t, s, "gen1", "")
	stageSample(t, s, "gen2", "gen1")
	// current=gen2 verified -> tier current
	_ = s.SetCurrent("gen1")
	_ = s.SetCurrent("gen2") // demotes gen1 -> prev
	if r := s.Resolve(); r.Tier != TierCurrent || r.SHA != "gen2" {
		t.Fatalf("verified current should win: %+v", r)
	}
	// quarantine gen2 -> falls back to prev gen1
	if err := s.Quarantine("gen2", "probation failed"); err != nil {
		t.Fatal(err)
	}
	if r := s.Resolve(); r.Tier != TierPrev || r.SHA != "gen1" {
		t.Fatalf("quarantined current should fall to prev gen1: %+v", r)
	}
	// quarantine gen1 too -> breakglass
	_ = s.Quarantine("gen1", "also bad")
	if r := s.Resolve(); r.Tier != TierBreakglass || r.SHA != "" {
		t.Fatalf("no verified generation should resolve to breakglass: %+v", r)
	}
}

// SetCurrent demotes the old current to prev (the recovery target survives a flip).
func TestSetCurrentDemotesPrev(t *testing.T) {
	s := NewStore(t.TempDir())
	stageSample(t, s, "old", "")
	stageSample(t, s, "new", "old")
	_ = s.SetCurrent("old")
	_ = s.SetCurrent("new")
	if s.Current() != "new" || s.Prev() != "old" {
		t.Fatalf("expected current=new prev=old, got current=%s prev=%s", s.Current(), s.Prev())
	}
}

// Handoff is consumed EXACTLY once — a second boot (crash-loop) sees nothing, so no re-swap.
func TestHandoffConsumeOnce(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.WriteHandoff(Handoff{PosturePath: "/tmp/p", TargetSHA: "gen2", Nonce: "n1"}); err != nil {
		t.Fatal(err)
	}
	h, ok, err := s.ConsumeHandoff()
	if err != nil || !ok {
		t.Fatalf("first consume should succeed: ok=%v err=%v", ok, err)
	}
	if h.TargetSHA != "gen2" || h.Nonce != "n1" {
		t.Fatalf("handoff round-trip wrong: %+v", h)
	}
	_, ok2, err2 := s.ConsumeHandoff()
	if err2 != nil {
		t.Fatalf("second consume should be a clean miss, got err %v", err2)
	}
	if ok2 {
		t.Fatal("handoff must be consumed EXACTLY once — a crash-loop must not re-consume/re-swap")
	}
}

// no-display-scalar: the resolver's verdict + the manifest carry NO numeric goodness/readiness scalar.
// Enforced by reflection so a future numeric field is actually REJECTED (not silently compiled past).
func TestResolutionHasNoScalar(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(Resolution{}), reflect.TypeOf(Manifest{})} {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			switch f.Type.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Float32, reflect.Float64:
				t.Fatalf("%s.%s is numeric (%s) — the generation surface must mint no scalar",
					typ.Name(), f.Name, f.Type.Kind())
			}
		}
	}
}
