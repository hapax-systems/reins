package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The reader parses the dispatch ledger JSONL NEWEST-FIRST, preserves the null/measured distinction
// end-to-end (a null cost stays nil → renders UNMEASURED, never 0.0), clamps to lastN, and fails OPEN
// on a missing ledger (empty, no error — the surface renders "ledger empty" honestly).
func TestReadParsesNewestFirstAndFailsOpen(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dispatch-events.jsonl")
	lines := []string{
		`{"ts":"t1","capability":"glm-via-cc","route_id":"claude.full","launched":true,"dispatch_latency_ms":1180,"cost_usd":null,"quality_signal":null,"outcome":null}`,
		`{"ts":"t2","capability":"codex.full","route_id":"codex.spark.full","launched":true,"dispatch_latency_ms":940,"cost_usd":0.0123,"quality_signal":"pass","outcome":"succeeded"}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	recs, err := Read(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].TS != "t2" {
		t.Fatalf("newest first: want t2 first, got %q", recs[0].TS)
	}
	if recs[0].CostUSD == nil || *recs[0].CostUSD != 0.0123 {
		t.Fatalf("a populated cost must parse to its real value")
	}
	if recs[1].CostUSD != nil {
		t.Fatalf("a null cost must stay nil (UNMEASURED, never 0.0)")
	}

	one, _ := Read(p, 1)
	if len(one) != 1 || one[0].TS != "t2" {
		t.Fatalf("lastN=1 must return only the newest record")
	}

	none, err := Read(filepath.Join(dir, "nonexistent.jsonl"), 10)
	if err != nil || len(none) != 0 {
		t.Fatalf("a missing ledger must fail open (empty, no error), got %v err=%v", none, err)
	}
}

// Malformed lines are skipped, not fatal — a corrupt append must not blind the whole surface.
func TestReadSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dispatch-events.jsonl")
	body := `{"ts":"a","capability":"glm-via-cc"}` + "\n" +
		`{ this is not json` + "\n" +
		`` + "\n" +
		`{"ts":"b","capability":"codex.full"}` + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, err := Read(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 valid records (malformed + blank skipped), got %d", len(recs))
	}
}
