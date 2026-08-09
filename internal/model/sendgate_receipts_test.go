package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func _writeBus(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "receipts.jsonl")
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const _sentLine = `{"receipt_schema":1,"receipt_id":"r1","op":"session_send","outcome":"sent","lane":"eta","created_at":"2026-08-08T12:00:00Z"}`
const _attemptedLine = `{"receipt_schema":1,"receipt_id":"r1","op":"session_send","outcome":"attempted","lane":"eta","created_at":"2026-08-08T11:59:59Z"}`
const _failedLine = `{"receipt_schema":1,"receipt_id":"r2","op":"session_send","outcome":"failed","lane":"eta","created_at":"2026-08-08T12:01:00Z"}`

func Test_sendgate_lights_only_on_a_genuine_sent_receipt(t *testing.T) {
	path := _writeBus(t, _attemptedLine, _sentLine)
	ev := latestSendEvidence(path)
	if ev == nil {
		t.Fatal("a sent receipt must light the gate")
	}
	if ev.ReceiptID != "r1" || ev.Lane != "eta" {
		t.Fatalf("wrong receipt surfaced: %+v", ev)
	}
}

func Test_sendgate_dark_on_attempted_only(t *testing.T) {
	// the write-ahead line alone never lights (an egress in flight is not evidence of send)
	if ev := latestSendEvidence(_writeBus(t, _attemptedLine)); ev != nil {
		t.Fatalf("attempted-only must stay dark, got %+v", ev)
	}
}

func Test_sendgate_dark_on_failed_relay(t *testing.T) {
	if ev := latestSendEvidence(_writeBus(t, _attemptedLine, _failedLine)); ev != nil {
		t.Fatalf("failed outcome must stay dark, got %+v", ev)
	}
}

func Test_sendgate_dark_on_missing_file(t *testing.T) {
	if ev := latestSendEvidence(filepath.Join(t.TempDir(), "absent.jsonl")); ev != nil {
		t.Fatalf("missing bus must stay dark, got %+v", ev)
	}
}

func Test_sendgate_dark_on_corrupt_lines(t *testing.T) {
	path := _writeBus(t, "not json", `{"receipt_schema":1,`, _attemptedLine)
	if ev := latestSendEvidence(path); ev != nil {
		t.Fatalf("corrupt lines must stay dark, got %+v", ev)
	}
}

func Test_sendgate_dark_on_unknown_schema_or_op(t *testing.T) {
	unknownSchema := `{"receipt_schema":2,"receipt_id":"r9","op":"session_send","outcome":"sent"}`
	wrongOp := `{"receipt_schema":1,"receipt_id":"r9","op":"session_launch","outcome":"sent"}`
	if ev := latestSendEvidence(_writeBus(t, unknownSchema, wrongOp)); ev != nil {
		t.Fatalf("unknown schema/op must stay dark, got %+v", ev)
	}
}

func Test_sendgate_corrupt_lines_do_not_hide_a_later_valid_sent(t *testing.T) {
	path := _writeBus(t, "garbage", _attemptedLine, _sentLine)
	ev := latestSendEvidence(path)
	if ev == nil || ev.ReceiptID != "r1" {
		t.Fatalf("a valid sent receipt after corrupt lines must still light, got %+v", ev)
	}
}

func Test_sendgate_footer_renders_live_through_the_real_ui_path(t *testing.T) {
	bus := _writeBus(t, _attemptedLine, _sentLine)
	t.Setenv("HAPAX_SESSION_SEND_RECEIPTS", bus)
	m := New("REINS")
	m.Mode = ModeSendGate
	m.CoordChatInput = "status check"
	out := ansi.Strip(m.coordinatorChatPane(200, 24))
	if !strings.Contains(out, "session gate: LIVE") {
		t.Fatalf("the footer must render LIVE through coordinatorChatPane:\n%s", out)
	}
	if !strings.Contains(out, "eta") {
		t.Fatalf("the LIVE line must name the governed lane:\n%s", out)
	}
}

func Test_sendgate_footer_renders_dark_through_the_real_ui_path(t *testing.T) {
	bus := _writeBus(t, _attemptedLine) // attempted-only: never evidence of send
	t.Setenv("HAPAX_SESSION_SEND_RECEIPTS", bus)
	m := New("REINS")
	m.Mode = ModeSendGate
	m.CoordChatInput = "status check"
	out := ansi.Strip(m.coordinatorChatPane(200, 24))
	if !strings.Contains(out, "session gate: NOT WIRED") {
		t.Fatalf("the footer must render NOT WIRED with no genuine receipt:\n%s", out)
	}
	if strings.Contains(out, "session gate: LIVE") {
		t.Fatalf("attempted-only must never render LIVE:\n%s", out)
	}
}
