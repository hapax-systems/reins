package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The send-gate evidence consumer (the reins half of council #4440's SESSION bus).
//
// The contract (docs/runbooks/capabilityio-session-gate.md): the send-gate lights
// LIVE only on a governed receipt with receipt_schema=1, op=session_send,
// outcome=sent. Write-ahead "attempted" lines, failed relays, missing files,
// corrupt lines, and unknown schemas stay dark. Reins never provider-sends — the
// gate renders governed evidence, never a capability claim.

// sendReceiptsPath mirrors the council writer's default (HAPAX_SESSION_SEND_RECEIPTS
// override, resolved at call time so post-import sets take effect).
func sendReceiptsPath() string {
	if override := os.Getenv("HAPAX_SESSION_SEND_RECEIPTS"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "hapax", "sdlc-routing", "session-send-receipts.jsonl")
}

// sendReceiptEvidence is the latest genuinely-sent governed receipt, or nil.
type sendReceiptEvidence struct {
	ReceiptID string
	Lane      string
	Outcome   string
	CreatedAt string
}

// latestSendEvidence reads the bus and returns the newest outcome=sent receipt.
// Every failure mode is dark: missing/unreadable file, corrupt lines (skipped,
// not fatal), attempted-only pairs, failed outcomes, unknown schemas or ops.
func latestSendEvidence(path string) *sendReceiptEvidence {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var latest *sendReceiptEvidence
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ReceiptSchema int    `json:"receipt_schema"`
			Op            string `json:"op"`
			Outcome       string `json:"outcome"`
			ReceiptID     string `json:"receipt_id"`
			Lane          string `json:"lane"`
			CreatedAt     string `json:"created_at"`
		}
		if json.Unmarshal([]byte(line), &row) != nil {
			continue // corrupt lines stay dark, never fatal
		}
		if row.ReceiptSchema != 1 || row.Op != "session_send" || row.Outcome != "sent" {
			continue
		}
		latest = &sendReceiptEvidence{
			ReceiptID: row.ReceiptID,
			Lane:      row.Lane,
			Outcome:   row.Outcome,
			CreatedAt: row.CreatedAt,
		}
	}
	return latest
}

// sendGateEvidence is the model's read of the bus for the send-gate footer.
func (m Model) sendGateEvidence() *sendReceiptEvidence {
	return latestSendEvidence(sendReceiptsPath())
}
