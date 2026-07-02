// Package generation is the hot-plug generation substrate (U6, design pack §Design 1 A1.3/A1.10/A1.11).
//
// A generation is the WHOLE repo sha: generations/<sha>/ carries the `reins` binary, the `api/` tree, and
// a manifest.json binding their byte-hashes. Both the cockpit and reins-api.service resolve through the
// same `current` pointer, so a bare `git checkout` can no longer silently change what the next restart
// serves. The manifest BINDS BYTES: a generation whose binary or api-tree hash does not match its
// manifest is QUARANTINED and never selected for exec — a bad generation cannot break the swap path.
//
// This package is the PURE substrate: the store layout, byte-hash + quarantine, the consume-once Handoff,
// and the three-tier rollback resolver (current -> prev -> breakglass-manual). It performs NO syscall.Exec
// and touches no systemd unit — the live swap mechanism + the governed stage verb are U6b.
package generation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Manifest binds a generation's bytes (A1.10). BinarySHA256 + APITreeSHA256 are verified before the
// generation is ever selected for exec; a mismatch => QUARANTINED.
type Manifest struct {
	SHA           string `json:"sha"`
	BinarySHA256  string `json:"binary_sha256"`
	APITreeSHA256 string `json:"api_tree_sha256"`
	Created       string `json:"created"` // caller-supplied ISO (no wall-clock in this pkg — deterministic/testable)
	Prev          string `json:"prev"`
}

// Handoff is the consume-once swap baton (U6b relies on this): written before a swap with the posture
// path + target sha, consumed EXACTLY once on boot so a crash-loop cannot re-consume and re-swap.
type Handoff struct {
	PosturePath string `json:"posture_path"`
	TargetSHA   string `json:"target_sha"`
	Nonce       string `json:"nonce"`
}

// Tier names for the rollback resolver — the "which generation boots" decision.
const (
	TierCurrent    = "current"
	TierPrev       = "prev"
	TierBreakglass = "breakglass" // no verified generation resolves -> honest manual recovery
)

// Resolution is the resolver's verdict: a tier label + the chosen sha + an honest reason. NO scalar —
// there is no minted goodness/readiness score anywhere on the generation surface.
type Resolution struct {
	SHA    string
	Tier   string
	Reason string
}

// Store is a generation store rooted at a directory (e.g. ~/.local/share/reins).
type Store struct{ root string }

func NewStore(root string) *Store { return &Store{root: root} }

func (s *Store) genRoot() string          { return filepath.Join(s.root, "generations") }
func (s *Store) GenerationDir(sha string) string { return filepath.Join(s.genRoot(), sha) }
func (s *Store) manifestPath(sha string) string  { return filepath.Join(s.GenerationDir(sha), "manifest.json") }
func (s *Store) quarantinePath(sha string) string { return filepath.Join(s.root, "quarantine", sha) }
func (s *Store) currentPtr() string       { return filepath.Join(s.root, "current") }
func (s *Store) prevPtr() string          { return filepath.Join(s.root, "prev") }
func (s *Store) handoffPath() string      { return filepath.Join(s.root, "handoff.json") }

// hashBytes is the sha256 hex of a byte slice.
func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// APITreeHash is the DETERMINISTIC hash of an api/ tree: files hashed in sorted-name order, each as
// name-bytes then content-bytes (same construction as reins_serve.api_tree_sha, full digest here — the
// server truncates to :16 for the display witness). Independent of map iteration order.
func APITreeHash(tree map[string][]byte) string {
	names := make([]string, 0, len(tree))
	for n := range tree {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		h.Write([]byte(n))
		h.Write(tree[n])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writeAtomic writes data to path via a temp file + rename (no torn writes / partial pointers).
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Stage writes a new generation into the store: the binary, the api/ tree, and a manifest binding their
// byte-hashes. It does NOT flip the current pointer (staging != swapping). Returns the written manifest.
func (s *Store) Stage(sha string, binary []byte, apiTree map[string][]byte, created, prev string) (Manifest, error) {
	dir := s.GenerationDir(sha)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "reins"), binary, 0o755); err != nil {
		return Manifest{}, err
	}
	apiDir := filepath.Join(dir, "api")
	for name, content := range apiTree {
		p := filepath.Join(apiDir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return Manifest{}, err
		}
		if err := os.WriteFile(p, content, 0o644); err != nil {
			return Manifest{}, err
		}
	}
	m := Manifest{
		SHA:           sha,
		BinarySHA256:  hashBytes(binary),
		APITreeSHA256: APITreeHash(apiTree),
		Created:       created,
		Prev:          prev,
	}
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := writeAtomic(s.manifestPath(sha), mb); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// ReadManifest reads a generation's manifest.
func (s *Store) ReadManifest(sha string) (Manifest, error) {
	b, err := os.ReadFile(s.manifestPath(sha))
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("generation %s: corrupt manifest: %w", sha, err)
	}
	return m, nil
}

// Verify recomputes the on-disk byte-hashes and compares them to the manifest. A mismatch (tampered or
// truncated binary/api-tree) is an error — the caller QUARANTINES; a verified generation may be exec'd.
func (s *Store) Verify(sha string) error {
	m, err := s.ReadManifest(sha)
	if err != nil {
		return err
	}
	bin, err := os.ReadFile(filepath.Join(s.GenerationDir(sha), "reins"))
	if err != nil {
		return fmt.Errorf("generation %s: binary unreadable: %w", sha, err)
	}
	if got := hashBytes(bin); got != m.BinarySHA256 {
		return fmt.Errorf("generation %s: binary hash mismatch (manifest %s, got %s)", sha, m.BinarySHA256, got)
	}
	tree, err := s.readAPITree(sha)
	if err != nil {
		return fmt.Errorf("generation %s: api tree unreadable: %w", sha, err)
	}
	if got := APITreeHash(tree); got != m.APITreeSHA256 {
		return fmt.Errorf("generation %s: api-tree hash mismatch (manifest %s, got %s)", sha, m.APITreeSHA256, got)
	}
	return nil
}

func (s *Store) readAPITree(sha string) (map[string][]byte, error) {
	apiDir := filepath.Join(s.GenerationDir(sha), "api")
	tree := map[string][]byte{}
	err := filepath.Walk(apiDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(apiDir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		tree[rel] = b
		return nil
	})
	return tree, err
}

// Quarantine marks a generation as unsafe-to-exec (byte mismatch or a failed probation). A quarantined
// generation is never selected by Resolve.
func (s *Store) Quarantine(sha, reason string) error {
	return writeAtomic(s.quarantinePath(sha), []byte(reason+"\n"))
}

func (s *Store) IsQuarantined(sha string) bool {
	_, err := os.Stat(s.quarantinePath(sha))
	return err == nil
}

// readPtr reads a pointer file's sha (empty string if absent).
func readPtr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(trimSpace(b))
}

func trimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\t' || b[j-1] == '\r') {
		j--
	}
	return b[i:j]
}

func (s *Store) Current() string { return readPtr(s.currentPtr()) }
func (s *Store) Prev() string    { return readPtr(s.prevPtr()) }

// SetCurrent atomically flips the current pointer to sha, demoting the old current to prev. The two
// writes are each atomic (temp+rename); prev is written first so a crash mid-flip never loses the
// recovery target.
func (s *Store) SetCurrent(sha string) error {
	if old := s.Current(); old != "" && old != sha {
		if err := writeAtomic(s.prevPtr(), []byte(old+"\n")); err != nil {
			return err
		}
	}
	return writeAtomic(s.currentPtr(), []byte(sha+"\n"))
}

// verified reports whether a sha names a present, hash-verified, non-quarantined generation.
func (s *Store) verified(sha string) bool {
	if sha == "" || s.IsQuarantined(sha) {
		return false
	}
	return s.Verify(sha) == nil
}

// Resolve is the three-tier rollback decision: boot the current generation if it verifies and is not
// quarantined; else fall back to prev if IT verifies; else breakglass-manual (no verified generation).
// Pure over the on-disk state — the exec decision without any exec.
func (s *Store) Resolve() Resolution {
	cur := s.Current()
	if s.verified(cur) {
		return Resolution{SHA: cur, Tier: TierCurrent, Reason: "current verified"}
	}
	prev := s.Prev()
	if s.verified(prev) {
		reason := "current unverified/quarantined -> prev"
		if cur == "" {
			reason = "no current pointer -> prev"
		}
		return Resolution{SHA: prev, Tier: TierPrev, Reason: reason}
	}
	return Resolution{SHA: "", Tier: TierBreakglass,
		Reason: "no verified generation (current + prev both absent/quarantined) — manual recovery"}
}

// WriteHandoff writes the consume-once swap baton.
func (s *Store) WriteHandoff(h Handoff) error {
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return writeAtomic(s.handoffPath(), b)
}

// ConsumeHandoff reads AND removes the handoff atomically. ok=false when none is present. Consume-once:
// the file is renamed away before the value is returned, so a crash-loop re-boot sees no handoff and the
// supervisor (U6b) falls through to a normal --resume instead of re-swapping.
func (s *Store) ConsumeHandoff() (Handoff, bool, error) {
	consumed := s.handoffPath() + ".consumed"
	if err := os.Rename(s.handoffPath(), consumed); err != nil {
		if os.IsNotExist(err) {
			return Handoff{}, false, nil
		}
		return Handoff{}, false, err
	}
	b, err := os.ReadFile(consumed)
	if err != nil {
		return Handoff{}, false, err
	}
	_ = os.Remove(consumed)
	var h Handoff
	if err := json.Unmarshal(b, &h); err != nil {
		return Handoff{}, false, fmt.Errorf("corrupt handoff: %w", err)
	}
	return h, true, nil
}
