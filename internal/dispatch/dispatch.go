// Package dispatch reads the SDLC dispatch ledger — the JSONL that cc-dispatch
// (shared/dispatch_record.py) appends one DispatchRecord per line to. This is the I/O layer that
// makes the Reins :dispatch surface LIVE: the grammar (internal/grammar) renders records, this reads
// them. Kept pure of any tea/model state.
package dispatch

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hapax-systems/reins/internal/grammar"
)

// LedgerPath resolves the canonical dispatch ledger location under the user's home (NOT tmpfs — the
// records must survive reboot). Empty string if the home dir cannot be resolved.
func LedgerPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "hapax", "sdlc-routing", "dispatch-events.jsonl")
}

// Read parses the dispatch ledger at path and returns up to lastN records, NEWEST FIRST. It is
// fail-open: a missing or unreadable ledger yields an empty slice and no error (the surface renders
// "ledger empty" honestly rather than erroring). Malformed/blank lines are skipped, not fatal — a
// corrupt append must not blind the whole surface.
func Read(path string, lastN int) ([]grammar.DispatchRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, nil // fail-open on any read error: the ledger is observability, not control
	}
	defer f.Close()

	var recs []grammar.DispatchRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // tolerate long lines
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r grammar.DispatchRecord
		if err := json.Unmarshal(line, &r); err != nil {
			continue // skip malformed lines
		}
		recs = append(recs, r)
	}

	// newest first
	for i, j := 0, len(recs)-1; i < j; i, j = i+1, j-1 {
		recs[i], recs[j] = recs[j], recs[i]
	}
	if lastN > 0 && len(recs) > lastN {
		recs = recs[:lastN]
	}
	return recs, nil
}
