package grammar

import (
	"strings"
	"testing"
)

// airAllOK is every field RenderTaskRow consults, allowed. Tests below flip ONE key so the
// assertion isolates that field instead of tripping over an unrelated redaction.
func airAllOK() map[string]string {
	m := map[string]string{}
	for _, f := range []string{
		"task_id", "stage", "authority_case", "no_go", "prior_stage", "prior_stage_state",
		"predicted_stage", "owner", "freshness", "criticality", "rel_count",
	} {
		m[f] = "ok"
	}
	return m
}

// AIR gates the STATE field, not just the value. prior_stage_state is allowlist-classified like any
// other field; when it is denied, its value must not choose the glyph. Rendering "○ absent" would
// leak the very fact the deny withheld — that the producer looked and found nothing.
func TestDeniedPriorStageStateCannotRenderAMeasuredNegative(t *testing.T) {
	// PredictedStage is non-empty so the SilenceDark assertion below can only be satisfied by the
	// prior-stage cell — an empty predicted stage renders its own silence and would mask the miss.
	base := Task{TaskID: "t1", Stage: "build", PredictedStage: "ship", PriorStage: "", PriorStageState: "origin"}

	allowed := base
	allowed.AIR = airAllOK()
	if got := RenderTaskRow(allowed, true); !strings.Contains(got, SilenceAbsent) {
		t.Fatalf("allowed prior_stage_state should render the measured negative %q:\n%s", SilenceAbsent, got)
	}

	denied := base
	denied.AIR = airAllOK()
	denied.AIR["prior_stage_state"] = "deny"
	got := RenderTaskRow(denied, true)
	if strings.Contains(got, SilenceAbsent) {
		t.Fatalf("denied prior_stage_state leaked a measured negative %q:\n%s", SilenceAbsent, got)
	}
	if !strings.Contains(got, SilenceDark) {
		t.Fatalf("denied prior_stage_state must fail closed to %q:\n%s", SilenceDark, got)
	}
}

// With AIR off entirely there is nothing to gate, so the producer's own state still governs.
func TestPriorStageStateGovernsWhenAirIsOff(t *testing.T) {
	task := Task{TaskID: "t1", Stage: "build", PriorStageState: "origin"}
	task.AIR = map[string]string{"prior_stage_state": "deny"}
	if got := RenderTaskRow(task, false); !strings.Contains(got, SilenceAbsent) {
		t.Fatalf("air off: producer state should govern, want %q:\n%s", SilenceAbsent, got)
	}
}
