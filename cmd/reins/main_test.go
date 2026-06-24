package main

import (
	"testing"

	"github.com/hapax-systems/reins/internal/model"
)

func TestTickCmdProducesEventsMsg(t *testing.T) {
	// against an unreachable url, the tick must still yield an EventsMsg (dark=true), never panic.
	msg := fetchOnce("http://127.0.0.1:0")
	em, ok := msg.(model.EventsMsg)
	if !ok {
		t.Fatalf("tick must yield model.EventsMsg, got %T", msg)
	}
	if !em.Dark {
		t.Fatal("unreachable api must fold to dark (honest), not empty-success")
	}
}
