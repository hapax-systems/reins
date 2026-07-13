package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

const gate0ContextFixtureSHA256 = "594b6b96656cea2a46e4d50c2201152523bcf4530f0afcb0360425e76c17fae9"

var gate0ContextObservationTime = time.Date(2026, 7, 10, 17, 0, 0, 0, time.UTC)

type gate0ContextFixture struct {
	Compatibility       json.RawMessage `json:"compatibility"`
	LifecycleProjection json.RawMessage `json:"lifecycle_projection"`
	OperatorProjection  json.RawMessage `json:"operator_projection"`
	OperationProjection json.RawMessage `json:"operation_projection"`
	YardProjection      json.RawMessage `json:"yard_projection"`
}

func loadGate0ContextFixture(t *testing.T) gate0ContextFixture {
	t.Helper()
	path := filepath.Join("..", "..", "api", "fixtures", "context-canon-gate0-carriers.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != gate0ContextFixtureSHA256 {
		t.Fatalf("context fixture SHA-256 = %s", got)
	}
	var fixture gate0ContextFixture
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func canonicalOperatorContextReadout(t *testing.T) grammar.ContextReadout {
	t.Helper()
	fixture := loadGate0ContextFixture(t)
	projection := append(json.RawMessage(nil), fixture.OperatorProjection...)
	return holdContextReadout(
		[]string{
			grammar.ContextReadReasonCanonUnverified,
			"producer_receipt_missing",
		},
		&projection,
		nil,
	)
}

func gate0ContextModel(t *testing.T) Model {
	t.Helper()
	m := New("REINS")
	m.contextNow = func() time.Time { return gate0ContextObservationTime }
	return m
}

func rehashContextProjection(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := grammar.ContextProjectionContentHash(payload)
	if err != nil {
		t.Fatal(err)
	}
	value["projection_hash"] = hash
	value["projection_ref"] = "projection-envelope@sha256:" + hash
	payload, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return json.RawMessage(payload)
}

func copyJSONMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var copied map[string]any
	if err := json.Unmarshal(payload, &copied); err != nil {
		t.Fatal(err)
	}
	return copied
}

func rehashContextEvent(t *testing.T, value map[string]any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := grammar.ContextProjectionEventContentHash(payload)
	if err != nil {
		t.Fatal(err)
	}
	value["event_hash"] = hash
	value["event_ref"] = "epistemic-event@sha256:" + hash
}

func rawContextMessage(value string) *json.RawMessage {
	raw := json.RawMessage(value)
	return &raw
}

func holdContextReadout(
	reasons []string,
	projection *json.RawMessage,
	compatibility *json.RawMessage,
) grammar.ContextReadout {
	return grammar.ContextReadout{
		Schema:        grammar.ContextReadSchema,
		State:         grammar.ContextReadHold,
		Audience:      grammar.ContextReadAudience,
		ReasonCodes:   append([]string(nil), reasons...),
		Projection:    projection,
		Compatibility: compatibility,
		RawEnvelope:   json.RawMessage(`{"sentinel":"RAW-ENVELOPE-PRIVATE"}`),
	}
}

func assertContextBytesCleared(t *testing.T, m Model) {
	t.Helper()
	if m.ContextReadout.State != grammar.ContextReadDark {
		t.Fatalf("context state should be DARK, got %q", m.ContextReadout.State)
	}
	if m.ContextReadout.Projection != nil ||
		m.ContextReadout.Compatibility != nil ||
		len(m.ContextReadout.RawEnvelope) != 0 {
		t.Fatalf("private context bytes remain resident: %+v", m.ContextReadout)
	}
}

func allContextBytesZero(payload []byte) bool {
	for _, value := range payload {
		if value != 0 {
			return false
		}
	}
	return true
}

func TestFoldContextHoldDeepCopiesAndWipesIncomingCarrier(t *testing.T) {
	projection := rawContextMessage(
		`{"WHAT":"PRIVATE-WHAT","fact_count":17,"demand_correlation":"matched"}`,
	)
	wantProjection := string(*projection)
	readout := holdContextReadout(
		[]string{"canonical_verifier_unavailable", "producer_receipt_missing"},
		projection,
		nil,
	)
	incomingEnvelopeAlias := readout.RawEnvelope
	m := New("REINS").FoldContext(readout, "")

	if m.ContextReadout.State != grammar.ContextReadHold ||
		m.ContextReadout.Projection == nil ||
		string(*m.ContextReadout.Projection) != wantProjection {
		t.Fatalf("HOLD carrier was not retained: %+v", m.ContextReadout)
	}
	if len(*projection) != 0 || !allContextBytesZero(incomingEnvelopeAlias) {
		t.Fatal("accepted incoming carrier buffers were not overwritten after cloning")
	}

	readout.ReasonCodes[0] = "mutated"
	if m.ContextReadout.ReasonCodes[0] == "mutated" {
		t.Fatal("model aliases caller-owned reason codes")
	}
}

func TestFoldContextTransitionsClearPriorPrivateBytes(t *testing.T) {
	present := grammar.ContextReadout{
		Schema:      grammar.ContextReadSchema,
		State:       grammar.ContextReadPresent,
		Audience:    grammar.ContextReadAudience,
		Projection:  rawContextMessage(`{"sentinel":"PRESENT-PRIVATE"}`),
		RawEnvelope: json.RawMessage(`{"sentinel":"PRESENT-ENVELOPE-PRIVATE"}`),
	}
	producerDark := darkContextReadout("producer_absent")
	producerDark.RawEnvelope = json.RawMessage(`{"sentinel":"DARK-RAW-DISCARD"}`)
	safePresentDark := darkContextReadout(grammar.ContextReadReasonCanonUnverified)

	cases := []struct {
		name       string
		readout    grammar.ContextReadout
		fetchError string
		reason     string
	}{
		{"fetch_error", grammar.ContextReadout{}, "context_read_error", "context_read_error"},
		{"producer_dark", producerDark, "", "producer_absent"},
		{
			"safe_present_sentinel",
			safePresentDark,
			"context_read_error",
			grammar.ContextReadReasonCanonUnverified,
		},
		{
			"present_unverified",
			present,
			"context_read_error",
			grammar.ContextReadReasonCanonUnverified,
		},
		{"unknown_state", grammar.ContextReadout{State: "unknown"}, "", "context_read_invalid"},
		{
			"invalid_hold",
			holdContextReadout(
				[]string{"x"},
				rawContextMessage(`{"one":1}`),
				rawContextMessage(`{"two":2}`),
			),
			"",
			"context_read_invalid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			private := holdContextReadout(
				[]string{"compatibility_only"},
				nil,
				rawContextMessage(`{"sentinel":"COMPATIBILITY-PRIVATE"}`),
			)
			m := New("REINS").FoldContext(private, "")
			m = m.FoldContext(tc.readout, tc.fetchError)
			assertContextBytesCleared(t, m)
			if len(m.ContextReadout.ReasonCodes) != 1 ||
				m.ContextReadout.ReasonCodes[0] != tc.reason {
				t.Fatalf("wrong DARK reason: %+v", m.ContextReadout.ReasonCodes)
			}
			if strings.Contains(m.ContextError, "PRIVATE") {
				t.Fatalf("raw error text was retained: %q", m.ContextError)
			}
		})
	}
}

func TestSetAIRWipesCarrierAndOffDoesNotResurrect(t *testing.T) {
	m := New("REINS").FoldContext(
		holdContextReadout(
			[]string{"compatibility_only"},
			nil,
			rawContextMessage(`{"sentinel":"AIR-PRIVATE"}`),
		),
		"",
	)
	compatibilityAlias := *m.ContextReadout.Compatibility
	envelopeAlias := m.ContextReadout.RawEnvelope
	initialEpoch := m.ContextEpoch
	m = m.SetAIR(true)
	assertContextBytesCleared(t, m)
	if !allContextBytesZero(compatibilityAlias) || !allContextBytesZero(envelopeAlias) {
		t.Fatal("SetAIR did not overwrite model-owned private buffers")
	}
	if !m.AIR ||
		m.ContextEpoch != initialEpoch+1 ||
		len(m.ContextReadout.ReasonCodes) != 1 ||
		m.ContextReadout.ReasonCodes[0] != "operator_private_withheld_on_air" {
		t.Fatalf("AIR transition is not generic DARK: %+v", m.ContextReadout)
	}

	m = m.SetAIR(false)
	assertContextBytesCleared(t, m)
	if m.AIR || m.ContextEpoch != initialEpoch+2 {
		t.Fatalf("AIR off did not invalidate the prior epoch: air=%v epoch=%d", m.AIR, m.ContextEpoch)
	}
}

func TestCanonicalContextIndexIsCachedPerFoldAndWipedOnAIR(t *testing.T) {
	m := gate0ContextModel(t).FoldContext(canonicalOperatorContextReadout(t), "")
	if m.contextProjection == nil || m.contextProjectionState != "ready" {
		t.Fatalf("canonical projection was not indexed once at fold: state=%q", m.contextProjectionState)
	}
	cache := m.contextProjection
	eventPayloadAlias := cache.Events[0].Payload
	first := m.renderContextCarrier(100)
	second := m.renderContextCarrier(100)
	if first != second || m.contextProjection != cache {
		t.Fatal("rendering rebuilt or changed the fold-owned context index")
	}
	m = m.SetAIR(true)
	if m.contextProjection != nil ||
		cache.ProjectionRef != "" ||
		len(cache.Facts) != 0 ||
		len(cache.Actions) != 0 ||
		len(cache.Events) != 0 ||
		!allContextBytesZero(eventPayloadAlias) {
		t.Fatal("AIR transition did not clear the derived context index")
	}
	rendered := ansi.Strip(m.renderContextCarrier(100))
	if !strings.Contains(rendered, "DARK") ||
		!strings.Contains(rendered, "operator_private_withheld_on_air") ||
		strings.Contains(rendered, "task:rich") {
		t.Fatalf("AIR rendering exposed cached context:\n%s", rendered)
	}
}

func TestContextMessageArrivingWhileAIRStaysScrubbed(t *testing.T) {
	incoming := holdContextReadout(
		[]string{"producer_receipt_missing"},
		rawContextMessage(`{"sentinel":"RACING-PRIVATE"}`),
		nil,
	)
	m := New("REINS").SetAIR(true)
	updated, _ := m.Update(ContextMsg{Readout: incoming, Epoch: m.ContextEpoch})
	m = updated.(Model)

	assertContextBytesCleared(t, m)
	if strings.Contains(string(m.ContextReadout.RawEnvelope), "RACING-PRIVATE") {
		t.Fatal("an in-flight private response rehydrated AIR state")
	}
}

func TestContextEpochRejectsPreAIRResponseAfterAIRTurnsOff(t *testing.T) {
	m := New("REINS")
	preAIREpoch := m.ContextEpoch
	m = m.SetAIR(true)
	m = m.SetAIR(false)

	stale := holdContextReadout(
		[]string{"producer_receipt_missing"},
		rawContextMessage(`{"sentinel":"STALE-PRE-AIR-PRIVATE"}`),
		nil,
	)
	staleProjectionAlias := *stale.Projection
	updated, _ := m.Update(ContextMsg{Readout: stale, Epoch: preAIREpoch})
	m = updated.(Model)
	assertContextBytesCleared(t, m)
	if !allContextBytesZero(staleProjectionAlias) {
		t.Fatal("stale response buffers were not overwritten")
	}

	fresh := holdContextReadout(
		[]string{"producer_receipt_missing"},
		rawContextMessage(`{"sentinel":"FRESH-POST-AIR-PRIVATE"}`),
		nil,
	)
	updated, _ = m.Update(ContextMsg{Readout: fresh, Epoch: m.ContextEpoch})
	m = updated.(Model)
	if m.ContextReadout.State != grammar.ContextReadHold ||
		m.ContextReadout.Projection == nil ||
		!strings.Contains(string(*m.ContextReadout.Projection), "FRESH-POST-AIR-PRIVATE") {
		t.Fatalf("fresh current-epoch response was not accepted: %+v", m.ContextReadout)
	}
}
func TestAIRCommandPathsDestroyCarrier(t *testing.T) {
	seed := func() Model {
		return New("REINS").FoldContext(
			holdContextReadout(
				[]string{"producer_receipt_missing"},
				rawContextMessage(`{"sentinel":"COMMAND-PRIVATE"}`),
				nil,
			),
			"",
		)
	}

	execModel := seed().Exec("air on")
	assertContextBytesCleared(t, execModel)

	keyModel, _, handled := seed().updateGlobal(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'a'},
	})
	if !handled {
		t.Fatal("AIR key was not handled")
	}
	assertContextBytesCleared(t, keyModel)
}

func TestContextCarrierRendersCanonicalOrientationUnderOuterHold(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	readout := canonicalOperatorContextReadout(t)
	wantRaw := append([]byte(nil), (*readout.Projection)...)
	m := gate0ContextModel(t).FoldContext(readout, "")

	index, err := m.ContextReadout.ProjectionIndexAt(gate0ContextObservationTime)
	if err != nil {
		t.Fatal(err)
	}
	if index.Position.TaskRef != "task:rich" ||
		index.Position.StageToken != "S6" ||
		!slices.Equal(index.Position.LegalSuccessors, []string{"BLOCKED", "S7"}) ||
		index.FocusRef != "fact:capability-gap" ||
		len(index.Actions) != 2 ||
		len(index.Events) != 16 ||
		index.Actions[0].Operation != "source_mutation" ||
		index.Actions[1].Operation != "context.inspect" ||
		index.Orientation == nil {
		t.Fatalf("canonical context index is incomplete: %+v", index)
	}
	if !bytes.Equal(*m.ContextReadout.Projection, wantRaw) {
		t.Fatal("typed indexing changed the retained projection bytes")
	}

	var compatibility struct {
		Wire struct {
			Strata struct {
				FSM struct {
					What string `json:"what"`
					How  string `json:"how"`
					Must string `json:"must"`
				} `json:"fsm"`
			} `json:"strata"`
		} `json:"wire"`
	}
	if err := json.Unmarshal(fixture.Compatibility, &compatibility); err != nil {
		t.Fatal(err)
	}
	if index.LifecycleFSM.What != compatibility.Wire.Strata.FSM.What ||
		index.LifecycleFSM.How != compatibility.Wire.Strata.FSM.How ||
		index.LifecycleFSM.Must != compatibility.Wire.Strata.FSM.Must {
		t.Fatal("typed lifecycle WHAT/HOW/MUST differs from the locked compatibility carrier")
	}

	rendered := ansi.Strip(m.renderContextCarrier(110))
	linearized := strings.Join(strings.Fields(rendered), "")
	for _, want := range []string{
		"CONTEXT CARRIER",
		"operator-private readout; carrier admission dominates",
		"HOLD",
		grammar.ContextReadReasonCanonUnverified,
		"producer_receipt_missing",
		"source frame and producer receipt unverified",
		"task:rich",
		"S6 -> BLOCKED, S7",
		"fact:capability-gap [HOLD]",
		"Execution capability is held pending independent evidence.",
		"observed / support_non_authoritative / producer:observer",
		"carrier HOLD · action:execute [HOLD] projected unavailable",
		"source_mutation",
		"Independent measurement and an execution lease are absent.",
		"receipt:execute",
		"carrier HOLD · action:inspect [PRESENT] projected legal",
		"context.inspect",
		"Inspection is non-mutating and supported by the current position.",
		"receipt:inspect",
		"observation_recorded",
		"measurement_updated",
		"caused by",
		"execution_lease_missing",
		"can",
		"action:inspect",
		"cannot",
		"action:execute",
		index.Orientation.FacetID,
		index.Orientation.FocusRef,
		index.Orientation.ChangeAuthority,
		index.Orientation.Counterfactual.ActionID,
		index.Orientation.Counterfactual.PredictedStateDelta.CanonicalJSON,
		"no_effect=true may_authorize=false",
		"exact WHAT/HOW/MUST retained",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("canonical carrier readout omitted %q:\n%s", want, rendered)
		}
	}
	for _, want := range []string{
		index.Orientation.FacetRef,
		index.Orientation.FacetHash,
		index.Orientation.PositionRef,
		index.Orientation.Counterfactual.PredictedStateDelta.SHA256,
		index.Facts[0].Data.SHA256,
		index.Facts[0].ScopeRef,
		index.Facts[0].TemporalRef,
		index.Facts[0].ResolutionRef,
		index.Facts[0].DerivationRef,
		index.Facts[0].Confidence.EvidenceRefs[0],
		"event:observation",
		"event:context-frame-materialized",
		"event:effect-observed",
		"event:receipt-recorded",
		"event:measurement-updated",
	} {
		if !strings.Contains(linearized, want) {
			t.Fatalf("canonical carrier readout omitted wrapped identity %q:\n%s", want, rendered)
		}
	}
	for _, want := range []string{
		index.Facts[0].Provenance.Generation,
		index.Facts[0].Provenance.PolicyGeneration,
		"legal next",
		"prohibited next",
		"expected receipt",
		"action:inspect",
		"action:execute",
		"receipt:inspect",
		"receipt:execute",
		"10 causal events retained in",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("canonical carrier readout omitted evidence/context %q:\n%s", want, rendered)
		}
	}
	if !strings.Contains(linearized, index.ProjectionRef) {
		t.Fatalf("chronology omission receipt lost projection identity:\n%s", rendered)
	}
	if strings.Contains(rendered, "[enter]") ||
		strings.Contains(rendered, "[a] arm") {
		t.Fatalf("projected actions acquired a command affordance:\n%s", rendered)
	}

	yard := ansi.Strip(m.renderYardCockpit(120))
	if !strings.Contains(yard, "CONTEXT CARRIER") ||
		!strings.Contains(yard, "producer_receipt_missing") ||
		!strings.Contains(yard, "action:inspect") {
		t.Fatalf("Yard cockpit omitted contextual orientation:\n%s", yard)
	}
}

func TestContextCarrierIndexesAndRendersCurrentPurposeProjections(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	tests := []struct {
		name       string
		projection json.RawMessage
		purpose    string
	}{
		{
			name:       "lifecycle possibility",
			projection: fixture.LifecycleProjection,
			purpose:    "lifecycle_possibility",
		},
		{
			name:       "operation",
			projection: fixture.OperationProjection,
			purpose:    "operation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := append(json.RawMessage(nil), test.projection...)
			readout := holdContextReadout(
				[]string{grammar.ContextReadReasonCanonUnverified, "producer_receipt_missing"},
				&projection,
				nil,
			)
			m := gate0ContextModel(t).FoldContext(readout, "")
			index, err := m.ContextReadout.ProjectionIndexAt(gate0ContextObservationTime)
			if err != nil {
				t.Fatal(err)
			}
			if index.Purpose != test.purpose || index.Orientation != nil {
				t.Fatalf("wrong purpose index: %+v", index)
			}
			rendered := ansi.Strip(m.renderContextCarrier(120))
			linearized := strings.Join(strings.Fields(rendered), "")
			compact := ansi.Strip(m.renderContextCarrierCompact(120))
			if strings.Contains(rendered, "[enter]") || strings.Contains(rendered, "[a] arm") {
				t.Fatalf("purpose projection acquired a command affordance:\n%s", rendered)
			}
			if test.purpose == "operation" {
				if index.LifecyclePossibility != nil || !strings.Contains(rendered, "context.inspect") {
					t.Fatalf("operation projection lost its typed action context:\n%s", rendered)
				}
				return
			}
			possibility := index.LifecyclePossibility
			if possibility == nil || possibility.MayAuthorize || !possibility.NoEffect {
				t.Fatalf("lifecycle possibility contract lost: %+v", possibility)
			}
			for _, want := range []string{
				possibility.FacetID,
				possibility.CandidateRef,
				possibility.WhyNow,
				possibility.Uncertainty,
				possibility.CandidatePlant.CanonicalJSON,
				possibility.EstimatedCost.CanonicalJSON,
				"plant_fields_missing",
				"harness_unbuilt",
				"measurement_threshold_missing",
				"no_effect=true may_authorize=false",
			} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("lifecycle readout omitted %q:\n%s", want, rendered)
				}
			}
			for _, want := range []string{
				possibility.FacetRef,
				possibility.FacetHash,
				possibility.CandidatePlant.SHA256,
				possibility.EstimatedCost.SHA256,
			} {
				if !strings.Contains(linearized, want) {
					t.Fatalf("lifecycle readout omitted wrapped identity %q:\n%s", want, rendered)
				}
			}
			if !strings.Contains(compact, possibility.FacetID) ||
				!strings.Contains(compact, "[Y] context") {
				t.Fatalf("compact lifecycle readout lost the full-facet door:\n%s", compact)
			}
		})
	}
}

func TestContextCarrierRendersEveryVisibleFactIncludingRefusal(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	var value map[string]any
	if err := json.Unmarshal(fixture.OperatorProjection, &value); err != nil {
		t.Fatal(err)
	}
	facts := value["facts"].([]any)
	refused := copyJSONMap(t, facts[0].(map[string]any))
	refused["fact_id"] = "fact:refused-canary"
	refused["subject_ref"] = "subject:refused-canary"
	refused["state"] = map[string]any{
		"value_state":  "refused",
		"reason_codes": []any{"policy_refused"},
	}
	refused["legal_next"] = []any{}
	refused["prohibited_next"] = []any{}
	refused["expected_receipt_refs"] = []any{}
	facts = append(facts, refused)
	value["facts"] = facts
	projection := rehashContextProjection(t, value)
	readout := holdContextReadout(
		[]string{grammar.ContextReadReasonCanonUnverified, "producer_receipt_missing"},
		&projection,
		nil,
	)
	m := gate0ContextModel(t).FoldContext(readout, "")
	if _, err := m.ContextReadout.ProjectionIndexAt(gate0ContextObservationTime); err != nil {
		t.Fatal(err)
	}
	rendered := ansi.Strip(m.renderContextCarrier(120))
	linearized := strings.Join(strings.Fields(rendered), "")
	for _, want := range []string{
		"fact:refused-canary [REFUSED]",
		"policy_refused",
		"no_effect=true may_authorize=false",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("visible refused fact is unreachable (%q):\n%s", want, rendered)
		}
	}
	for _, want := range []string{
		refused["data"].(map[string]any)["sha256"].(string),
		refused["scope_ref"].(string),
		refused["temporal_ref"].(string),
		refused["resolution_ref"].(string),
		refused["derivation_ref"].(string),
	} {
		if !strings.Contains(linearized, want) {
			t.Fatalf("visible refused fact lost evidence/context %q:\n%s", want, rendered)
		}
	}
}

func TestContextProjectionIndexFailsClosedWithoutChangingOuterHold(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	var canonical map[string]any
	if err := json.Unmarshal(fixture.OperatorProjection, &canonical); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(map[string]any){
		"root_authorizes": func(value map[string]any) {
			value["may_authorize"] = true
		},
		"effect_claimed": func(value map[string]any) {
			value["no_effect"] = false
		},
		"missing_meaning": func(value map[string]any) {
			delete(value, "meaning")
		},
		"position_mismatch": func(value map[string]any) {
			actions := value["actions"].([]any)
			actions[0].(map[string]any)["position_ref"] = "context-position@sha256:" + strings.Repeat("0", 64)
		},
		"receipt_missing": func(value map[string]any) {
			actions := value["actions"].([]any)
			actions[0].(map[string]any)["expected_receipt_ref"] = ""
		},
		"provenance_missing": func(value map[string]any) {
			facts := value["facts"].([]any)
			delete(facts[0].(map[string]any), "provenance")
		},
		"orientation_mismatch": func(value map[string]any) {
			value["orientation"].(map[string]any)["position_ref"] = "context-position@sha256:" + strings.Repeat("0", 64)
		},
		"action_index_mismatch": func(value map[string]any) {
			value["legal_next"] = []any{}
		},
		"fact_data_tampered": func(value map[string]any) {
			facts := value["facts"].([]any)
			facts[0].(map[string]any)["data"].(map[string]any)["canonical_json"] = `{"forged":true}`
		},
		"terminal_control": func(value map[string]any) {
			facts := value["facts"].([]any)
			facts[0].(map[string]any)["proves"] = []any{"forged\x1b[31m"}
		},
		"legal_hold_action": func(value map[string]any) {
			actions := value["actions"].([]any)
			actions[0].(map[string]any)["disposition"] = "legal"
		},
		"unsupported_lifecycle_facet": func(value map[string]any) {
			value["purpose"] = "lifecycle_possibility"
			value["orientation"] = nil
			value["lifecycle_possibility"] = map[string]any{"forged": true}
		},
		"missing_current_events": func(value map[string]any) {
			delete(value, "events")
		},
		"action_operation_mismatch": func(value map[string]any) {
			actions := value["actions"].([]any)
			actions[0].(map[string]any)["operation"] = "context.inspect"
		},
		"demand_canon_mismatch": func(value map[string]any) {
			demand := value["demand_shape"].(map[string]any)
			descriptor := demand["descriptor"].(map[string]any)
			descriptor["canon"].(map[string]any)["canonical_json"] = `{"forged":true}`
		},
		"demand_position_basis_mismatch": func(value map[string]any) {
			demand := value["demand_shape"].(map[string]any)
			descriptor := demand["descriptor"].(map[string]any)
			descriptor["position_basis"].(map[string]any)["canonical_json"] = `{"forged":true}`
		},
		"event_order_mismatch": func(value map[string]any) {
			events := value["events"].([]any)
			events[0], events[1] = events[1], events[0]
		},
		"event_lineage_mismatch": func(value map[string]any) {
			lineage := value["lineage_refs"].([]any)
			value["lineage_refs"] = lineage[:len(lineage)-1]
		},
		"event_audience_mismatch": func(value map[string]any) {
			event := value["events"].([]any)[0].(map[string]any)
			event["privacy_class"] = "yard_context"
			rehashContextEvent(t, event)
		},
		"event_authority_escalation": func(value map[string]any) {
			event := value["events"].([]any)[1].(map[string]any)
			event["authority_ceiling"] = "constitutional_evidence"
			rehashContextEvent(t, event)
		},
		"event_causal_forward_reference": func(value map[string]any) {
			events := value["events"].([]any)
			event := events[1].(map[string]any)
			event["caused_by"] = []any{events[2].(map[string]any)["event_ref"]}
			rehashContextEvent(t, event)
		},
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			payload, err := json.Marshal(canonical)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(payload, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			raw := rehashContextProjection(t, value)
			readout := holdContextReadout(
				[]string{
					grammar.ContextReadReasonCanonUnverified,
					"producer_receipt_missing",
				},
				&raw,
				nil,
			)
			if _, err := readout.ProjectionIndexAt(gate0ContextObservationTime); err == nil {
				t.Fatal("malformed projection produced a semantic index")
			}
			m := gate0ContextModel(t).FoldContext(readout, "")
			rendered := ansi.Strip(m.renderContextCarrier(100))
			if !strings.Contains(rendered, "state") ||
				!strings.Contains(rendered, "HOLD") ||
				!strings.Contains(rendered, "DARK semantic view") ||
				strings.Contains(rendered, "Inspect exact evidence gap") {
				t.Fatalf("invalid projection was not semantically DARK under outer HOLD:\n%s", rendered)
			}
		})
	}
}

func TestContextProjectionHashBindsUnhashedSemanticText(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	var value map[string]any
	if err := json.Unmarshal(fixture.OperatorProjection, &value); err != nil {
		t.Fatal(err)
	}
	actions := value["actions"].([]any)
	actions[0].(map[string]any)["why"] = "forged but still well-formed"
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(payload)
	readout := holdContextReadout(
		[]string{grammar.ContextReadReasonCanonUnverified, "producer_receipt_missing"},
		&raw,
		nil,
	)
	if _, err := readout.ProjectionIndexAt(gate0ContextObservationTime); err == nil ||
		!strings.Contains(err.Error(), "hash does not bind") {
		t.Fatalf("unhashed semantic mutation was not rejected: %v", err)
	}
}

func TestContextProjectionIndexIsJSONEncodingIndependent(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	var value any
	if err := json.Unmarshal(fixture.OperatorProjection, &value); err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(pretty)
	readout := holdContextReadout(
		[]string{grammar.ContextReadReasonCanonUnverified, "producer_receipt_missing"},
		&raw,
		nil,
	)
	index, err := readout.ProjectionIndexAt(gate0ContextObservationTime)
	if err != nil {
		t.Fatal(err)
	}
	if index.Position.TaskRef != "task:rich" || index.Position.StageToken != "S6" {
		t.Fatalf("pretty/reordered projection changed semantics: %+v", index.Position)
	}
}

func TestRawDepthLifecycleTextUsesBijectiveSingleLineEscaping(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	var value map[string]any
	if err := json.Unmarshal(fixture.OperatorProjection, &value); err != nil {
		t.Fatal(err)
	}
	value["depth"] = "raw"
	raw := rehashContextProjection(t, value)
	readout := holdContextReadout(
		[]string{grammar.ContextReadReasonCanonUnverified, "producer_receipt_missing"},
		&raw,
		nil,
	)
	m := gate0ContextModel(t).FoldContext(readout, "")
	rendered := ansi.Strip(m.renderContextCarrier(100))
	if !strings.Contains(rendered, `\n  what.grounding.stage-vocabulary`) {
		t.Fatalf("raw lifecycle newline was not represented losslessly:\n%s", rendered)
	}
	if strings.Contains(rendered, "\n  what.grounding.stage-vocabulary") {
		t.Fatalf("raw lifecycle text injected an unlabeled terminal line:\n%s", rendered)
	}
}

func TestContextCarrierGlobalRenderBudgetIsBoundedAndReceipted(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	var value map[string]any
	if err := json.Unmarshal(fixture.OperatorProjection, &value); err != nil {
		t.Fatal(err)
	}
	implications := make([]any, 32)
	for i := range implications {
		implications[i] = fmt.Sprintf(
			"adversarial implication %02d %s",
			i,
			strings.Repeat(string(rune('a'+i%26)), 4096),
		)
	}
	facts := value["facts"].([]any)
	facts[0].(map[string]any)["implications"] = implications
	raw := rehashContextProjection(t, value)
	projectionRef := value["projection_ref"].(string)
	readout := holdContextReadout(
		[]string{grammar.ContextReadReasonCanonUnverified, "producer_receipt_missing"},
		&raw,
		nil,
	)
	m := gate0ContextModel(t).FoldContext(readout, "")
	for _, width := range []int{80, 60} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			rendered := m.renderContextCarrier(width)
			plain := ansi.Strip(rendered)
			if strings.Contains(plain, "DARK semantic view") {
				t.Fatalf("valid adversarial carrier failed semantic indexing:\n%s", plain)
			}
			if lines := strings.Count(rendered, "\n") + 1; lines > contextRenderLineLimit {
				t.Fatalf("carrier rendered %d lines; limit is %d", lines, contextRenderLineLimit)
			}
			if len(rendered) > contextRenderByteLimit {
				t.Fatalf("carrier rendered %d bytes; limit is %d", len(rendered), contextRenderByteLimit)
			}
			collapsed := strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(plain)
			if !strings.Contains(plain, "display omission") ||
				!strings.Contains(collapsed, "exactcontextretainedin"+projectionRef) ||
				!strings.Contains(collapsed, "valuesclipped") ||
				!strings.Contains(plain, "carrier HOLD") {
				t.Fatalf("bounded rendering omitted its complete receipt:\n%s", plain)
			}
		})
	}
}

func TestExpiredContextProjectionIsSemanticStaleUnderOuterHold(t *testing.T) {
	readout := canonicalOperatorContextReadout(t)
	staleAt := time.Date(2026, 7, 10, 18, 0, 0, 0, time.UTC)
	if _, err := readout.ProjectionIndexAt(staleAt); !errors.Is(err, grammar.ErrContextProjectionStale) {
		t.Fatalf("expired projection error = %v", err)
	}
	m := New("REINS")
	m.contextNow = func() time.Time { return staleAt }
	m = m.FoldContext(readout, "")
	rendered := ansi.Strip(m.renderContextCarrier(100))
	if !strings.Contains(rendered, "state") ||
		!strings.Contains(rendered, "HOLD") ||
		!strings.Contains(rendered, "STALE semantic view") ||
		strings.Contains(rendered, "action:inspect") {
		t.Fatalf("expired semantic context was rendered as current:\n%s", rendered)
	}
}

func TestOversizedProjectionRetainsOuterHoldButSemanticDark(t *testing.T) {
	payload := bytes.Repeat([]byte{' '}, grammar.ContextProjectionMaxDisplayBytes+1)
	raw := json.RawMessage(payload)
	readout := holdContextReadout(
		[]string{grammar.ContextReadReasonCanonUnverified, "producer_receipt_missing"},
		&raw,
		nil,
	)
	if _, err := readout.ProjectionIndexAt(gate0ContextObservationTime); err == nil ||
		!strings.Contains(err.Error(), "display budget") {
		t.Fatalf("oversized semantic projection error = %v", err)
	}
	m := gate0ContextModel(t).FoldContext(readout, "")
	rendered := ansi.Strip(m.renderContextCarrier(100))
	if !strings.Contains(rendered, "HOLD") ||
		!strings.Contains(rendered, "DARK semantic view") {
		t.Fatalf("oversized projection did not fail closed:\n%s", rendered)
	}
}

func TestCompatibilityHoldHasNoSemanticProjectionIndex(t *testing.T) {
	fixture := loadGate0ContextFixture(t)
	compatibility := append(json.RawMessage(nil), fixture.Compatibility...)
	readout := holdContextReadout(
		[]string{"compatibility_only"},
		nil,
		&compatibility,
	)
	if _, err := readout.ProjectionIndexAt(gate0ContextObservationTime); err == nil {
		t.Fatal("lossy compatibility carrier produced a rich semantic index")
	}
	rendered := ansi.Strip(gate0ContextModel(t).FoldContext(readout, "").renderContextCarrier(100))
	if !strings.Contains(rendered, "OPAQUE compatibility HOLD") ||
		strings.Contains(rendered, "task:rich") {
		t.Fatalf("compatibility carrier was interpreted as rich context:\n%s", rendered)
	}
}

func TestContextCarrierIsIndependentOfTaskFocus(t *testing.T) {
	m := New("REINS").FoldContext(
		holdContextReadout(
			[]string{"compatibility_only"},
			nil,
			rawContextMessage(`{"task_ref":"task-a","sentinel":"NO-TASK-BINDING"}`),
		),
		"",
	)
	m.Tasks = []grammar.Task{{TaskID: "task-a"}, {TaskID: "task-b"}}

	m.Focus = 0
	first := m.renderContextCarrier(100)
	m.Focus = 1
	second := m.renderContextCarrier(100)
	if first != second {
		t.Fatalf("global carrier changed with task focus:\n%s\n---\n%s", first, second)
	}

	pane := m.taskWorkDomainPane(100)
	for _, forbidden := range []string{"NO-TASK-BINDING", "CONTEXT CARRIER", "locked context"} {
		if strings.Contains(pane, forbidden) {
			t.Fatalf("task pane acquired global context %q:\n%s", forbidden, pane)
		}
	}
}

func TestContextCarrierAIRRendererIsDefensivelyGeneric(t *testing.T) {
	m := New("REINS")
	m.ContextReadout = holdContextReadout(
		[]string{"PRIVATE-REASON"},
		rawContextMessage(`{"sentinel":"AIR-PRIVATE"}`),
		nil,
	)
	m.AIR = true // bypass SetAIR to exercise the rendering defense directly

	rendered := m.renderContextCarrier(100)
	if !strings.Contains(rendered, "DARK") ||
		!strings.Contains(rendered, "operator_private_withheld_on_air") {
		t.Fatalf("AIR context status is not generic DARK:\n%s", rendered)
	}
	for _, forbidden := range []string{"PRIVATE-REASON", "AIR-PRIVATE"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("AIR context status leaked %q:\n%s", forbidden, rendered)
		}
	}
}

func TestContextCarrierStateVisibleOnCompactYardView(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{80, 24},
		{180, 16},
	} {
		m := New("REINS")
		m.Page = PageYard
		m.Width = size.width
		m.Height = size.height
		frame := ansi.Strip(m.View())
		if !strings.Contains(frame, "ctx DARK") {
			t.Fatalf(
				"compact %dx%d Yard viewport omitted ambient context state:\n%s",
				size.width,
				size.height,
				frame,
			)
		}
	}
}
