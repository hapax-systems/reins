package grammar

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var whoisStageMeanings = map[int]string{
	0:  "intake",
	1:  "triage",
	2:  "scope",
	3:  "plan",
	4:  "design",
	5:  "implementation gate",
	6:  "verification gate",
	7:  "release gate",
	8:  "ship / deploy",
	9:  "observe",
	10: "closeout",
	11: "archive",
}

type whoisSeg struct {
	token string
	text  string
}

// RenderWhoisDoor renders the present-at-hand /whois drill-in surface for one task. It is a
// self-contained decode: macro SDLC ladder first, then the seven labeled task dimensions, then
// authorization/relationship detail, with only governed-command verbs signified at the bottom.
func RenderWhoisDoor(t Task, airOn bool, w, h int) string {
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}

	current := whoisStageIndex(t.Stage)
	prior := -1
	if whoisFieldVisible(t, "prior_stage", airOn) {
		prior = whoisStageIndex(t.PriorStage)
	}
	predStage := -1
	if whoisFieldVisible(t, "predicted_stage", airOn) {
		predStage = whoisStageIndex(t.PredictedStage)
	}
	crit := t.Criticality
	if strings.TrimSpace(crit) == "" {
		crit = "ok"
	}
	ctok := SeverityToken(crit)
	if airOn && !whoisFieldVisible(t, "criticality", airOn) {
		ctok = "mut"
	}

	val := func(field, raw string) string { return redact(t.AIR, field, raw, airOn) }
	stageVisible := whoisFieldVisible(t, "stage", airOn)
	currentForLadder := current
	if !stageVisible {
		currentForLadder = -1
	}

	var lines []string
	add := func(segs ...whoisSeg) { lines = append(lines, whoisLine(w, segs...)) }
	blank := func() { lines = append(lines, "") }

	add(
		whoisSeg{"brt", "◆ " + val("task_id", t.TaskID)},
		whoisSeg{"2nd", "  AUTHORITY CASE: " + val("authority_case", t.AuthorityCase)},
	)
	add(whoisSeg{"mut", "DOOR /whois — present-at-hand task decode; values may redact, structure remains."})
	blank()

	add(whoisSeg{"mut", "SDLC LADDER: "}, whoisSeg{"2nd", "macro lifecycle frame (current is heavy-bracketed)"})
	lines = append(lines, whoisLadderLine(w, currentForLadder, prior, predStage, ctok))
	add(whoisStageCaption(t, airOn, current, currentForLadder))
	blank()

	add(whoisSeg{"mut", "SEVEN DIMENSIONS — labels carry the decode; no recall required"})
	whoisDimensions(&lines, t, w, airOn, val, ctok)
	blank()

	add(whoisSeg{"mut", "GRANTED AUTHORIZATIONS:"})
	lines = append(lines, whoisAuthorizationLines(t, w, airOn)...)
	blank()

	add(whoisSeg{"mut", "RELATIONSHIPS:"})
	add(whoisSeg{"mut", "  (no task-edge source yet)"})

	dock := []string{
		whoisVerbDock(w, t, current),
		whoisLine(w, whoisSeg{"mut", "[Esc]/[Enter] back · verbs route through the governed COMMAND surface (cockpit never mints authority)"}),
	}

	if h <= len(dock) {
		return strings.Join(dock[:h], "\n")
	}
	maxBody := h - len(dock)
	lines = doorBodyWithOverflow(lines, maxBody, w, "door")
	for len(lines) < maxBody {
		blank()
	}
	lines = append(lines, dock...)
	return strings.Join(lines, "\n")
}

func doorBodyWithOverflow(lines []string, maxBody, w int, label string) []string {
	if maxBody <= 0 {
		return nil
	}
	if len(lines) <= maxBody {
		return lines
	}
	hidden := len(lines) - maxBody + 1
	out := append([]string(nil), lines[:maxBody]...)
	out[maxBody-1] = whoisLine(w, whoisSeg{"mut", fmt.Sprintf("… %d %s rows hidden; taller frame", hidden, label)})
	return out
}

func whoisDimensions(lines *[]string, t Task, w int, airOn bool, val func(string, string) string, ctok string) {
	crit := t.Criticality
	if strings.TrimSpace(crit) == "" {
		crit = "ok"
	}
	glyph := critGlyph[crit]
	if glyph == "" {
		glyph = "·"
	}
	if airOn && !whoisFieldVisible(t, "criticality", airOn) {
		glyph = "▒"
	}
	stage := val("stage", shortStage(t.Stage))
	prior := val("prior_stage", shortStage(t.PriorStage))
	pred := whoisPredictedDisplay(t.PredictedStage)
	pred = val("predicted_stage", pred)
	critVal := val("criticality", crit)
	bar := critBar(crit)
	if airOn && !whoisFieldVisible(t, "criticality", airOn) {
		bar = "▒▒▒▒"
	}
	fresh := val("freshness", fmt.Sprintf("%.2f", t.Freshness))
	rel := whoisRedactRel(t, airOn, fmt.Sprintf("●%d", t.RelCount))
	predTok := whoisPredictedToken(t.PredictedStage)
	if airOn && !whoisFieldVisible(t, "predicted_stage", airOn) {
		predTok = "mut"
	}
	ownerTok := LaneToken(t.Owner)
	if airOn && !whoisFieldVisible(t, "owner", airOn) {
		ownerTok = "mut"
	}
	freshTok := "pri"
	if airOn && !whoisFieldVisible(t, "freshness", airOn) {
		freshTok = "mut"
	}
	relTok := "blu"
	if airOn && !whoisFieldVisible(t, "rel_count", airOn) {
		relTok = "mut"
	}

	addDim := func(label string, segs ...whoisSeg) {
		base := []whoisSeg{{"mut", "  " + pad(label, 12) + ": "}}
		base = append(base, segs...)
		*lines = append(*lines, whoisLine(w, base...))
	}
	addDim("state", whoisSeg{ctok, glyph + " " + stage}, whoisSeg{"mut", " — current lifecycle state"})
	addDim("was", whoisSeg{"mut", "◀ " + dotsOr(prior, 4)}, whoisSeg{"mut", " — prior stage"})
	addDim("now→next", whoisSeg{"pri", stage}, whoisSeg{"mut", " → "}, whoisSeg{predTok, pred})
	addDim("criticality", whoisSeg{ctok, critVal + " " + bar})
	addDim("owner", whoisSeg{ownerTok, val("owner", dotsOr(t.Owner, 8))})
	addDim("freshness", whoisSeg{freshTok, fresh})
	addDim("relations", whoisSeg{relTok, rel})
}

func whoisAuthorizationLines(t Task, w int, airOn bool) []string {
	if airOn && !whoisFieldVisible(t, "no_go", airOn) {
		return []string{whoisLine(w, whoisSeg{"grn", "  ✓ "}, whoisSeg{"mut", "▒▒▒"})}
	}
	var out []string
	for _, part := range strings.Split(t.NoGo, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		part = strings.TrimSuffix(part, "_authorized")
		out = append(out, whoisLine(w, whoisSeg{"grn", "  ✓ "}, whoisSeg{"pri", part}))
	}
	if len(out) == 0 {
		out = append(out, whoisLine(w, whoisSeg{"mut", "  (none granted yet)"}))
	}
	return out
}

// TaskVerb is one governed action on a task with its current state-legality. The /whois verb dock AND
// the [v] object-verb menu read the SAME legality (verbs attach to OBJECTS, not to memory) — one source.
type TaskVerb struct {
	Key, Name string
	Legal     bool
}

const (
	ContextReadSchema                = "hapax.reins-context-read.v1"
	ContextReadAudience              = "operator_private"
	ContextReadMaxReasonCodes        = 64
	ContextReadMaxReasonCodeBytes    = 128
	ContextReadReasonCanonUnverified = "canonical_projection_verification_unavailable"
)

type ContextReadState string

const (
	ContextReadPresent ContextReadState = "present"
	ContextReadHold    ContextReadState = "hold"
	ContextReadDark    ContextReadState = "dark"
)

// ContextReadout is the strict outer carrier. Nested payload semantics remain opaque until the
// installed canonical verifier and governed producer receipt both admit them.
type ContextReadout struct {
	Schema        string           `json:"schema"`
	State         ContextReadState `json:"state"`
	Audience      string           `json:"audience"`
	ReasonCodes   []string         `json:"reason_codes"`
	Projection    *json.RawMessage `json:"projection"`
	Compatibility *json.RawMessage `json:"compatibility"`
	RawEnvelope   json.RawMessage  `json:"-"`
}

var contextReadReasonCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]*$`)

// ValidContextReasonCodes enforces the bounded terminal-safe wire vocabulary shared by the
// transport decoder and model. It rejects rather than normalizes non-canonical input.
func ValidContextReasonCodes(codes []string, allowEmpty bool) bool {
	if (!allowEmpty && len(codes) == 0) || len(codes) > ContextReadMaxReasonCodes {
		return false
	}
	if !sort.StringsAreSorted(codes) {
		return false
	}
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if len(code) > ContextReadMaxReasonCodeBytes ||
			!contextReadReasonCodePattern.MatchString(code) {
			return false
		}
		if _, duplicate := seen[code]; duplicate {
			return false
		}
		seen[code] = struct{}{}
	}
	return true
}

// ContextProjectionIndex is a disposable, non-authorizing view over one retained
// ProjectionEnvelope. It is rebuilt from raw bytes and is never serialized as a
// second wire or stored as an independent truth plane.
type ContextProjectionIndex struct {
	ProjectionRef        string
	ProjectionHash       string
	Audience             string
	Purpose              string
	Depth                string
	DeviceClass          string
	Register             string
	FocusRef             string
	State                ContextProjectionState
	Position             ContextProjectionPosition
	Facts                []ContextProjectionFact
	RedactedFacts        []ContextRedactedFact
	RedactedObjects      []ContextRedactedObject
	FocusedFacts         []ContextProjectionFact
	Actions              []ContextProjectionAction
	Events               []ContextProjectionEvent
	Impingements         []ContextProjectionImpingement
	Signals              []ContextProjectionSignal
	Portals              []ContextProjectionPortal
	Orientation          *ContextBoundaryOrientation
	LifecyclePossibility *ContextLifecyclePossibility
	Meaning              []string
	Implications         []string
	BlindSpots           []string
	LegalNext            []string
	ProhibitedNext       []string
	LineageRefs          []string
	SupersedesRefs       []string
	ProducerRef          string
	GeneratedAt          string
	StaleAfter           string
	LifecycleFSM         ContextLifecycleFSM
}

type ContextProjectionState struct {
	Value       string   `json:"value_state"`
	ReasonCodes []string `json:"reason_codes"`
}

type ContextAuthorizationFlag struct {
	Name       string `json:"name"`
	Authorized bool   `json:"authorized"`
	SourceRef  string `json:"source_ref"`
}

type ContextProjectionPosition struct {
	TaskRef                   string                     `json:"task_ref"`
	StageToken                string                     `json:"stage_token"`
	LegalSuccessors           []string                   `json:"legal_successors"`
	AuthorityCase             string                     `json:"authority_case"`
	AuthorizedFlags           []ContextAuthorizationFlag `json:"authorized_flags"`
	MutationScopeRefs         []string                   `json:"mutation_scope_refs"`
	ClaimRef                  string                     `json:"claim_ref"`
	RouteDecisionRef          string                     `json:"route_decision_ref"`
	CanonBundleRef            string                     `json:"canon_bundle_ref"`
	CanonBundleHash           string                     `json:"canon_bundle_hash"`
	CanonID                   string                     `json:"canon_id"`
	CanonVersion              int64                      `json:"canon_version"`
	CanonLevel                string                     `json:"canon_level"`
	CanonImageHash            string                     `json:"canon_image_hash"`
	LifecycleDefinition       json.RawMessage            `json:"lifecycle_definition"`
	LifecycleDefinitionRef    string                     `json:"lifecycle_definition_ref"`
	LifecycleDefinitionHash   string                     `json:"lifecycle_definition_hash"`
	LifecycleFSMDataSHA256    string                     `json:"lifecycle_fsm_data_sha256"`
	DemandShapeFingerprint    string                     `json:"demand_shape_fingerprint"`
	EffectiveConstraintDigest string                     `json:"effective_constraint_digest"`
	ImpingementDigest         string                     `json:"impingement_digest"`
	PortalSetDigest           string                     `json:"portal_set_digest"`
	ReceiptLineage            []string                   `json:"receipt_lineage"`
	PositionRef               string                     `json:"position_ref"`
	PositionHash              string                     `json:"position_hash"`
	MayAuthorize              bool                       `json:"may_authorize"`
}

type ContextCanonicalJSON struct {
	CanonicalJSON string `json:"canonical_json"`
	SHA256        string `json:"sha256"`
}

type ContextProjectionConfidence struct {
	Word               string          `json:"word"`
	Method             string          `json:"method"`
	EvidenceRefs       []string        `json:"evidence_refs"`
	CalibrationRef     json.RawMessage `json:"calibration_ref"`
	CalibrationMetric  json.RawMessage `json:"calibration_metric"`
	ValidityDomainRefs []string        `json:"validity_domain_refs"`
	DistributionState  string          `json:"distribution_state"`
	Abstained          bool            `json:"abstained"`
}

type ContextProjectionProvenance struct {
	Kind             string   `json:"kind"`
	AuthorityLevel   string   `json:"authority_level"`
	Derivation       string   `json:"derivation"`
	ProducerRef      string   `json:"producer_ref"`
	SourceRefs       []string `json:"source_refs"`
	ObservedAt       string   `json:"observed_at"`
	ProducedAt       string   `json:"produced_at"`
	StaleAfter       string   `json:"stale_after"`
	Generation       string   `json:"generation"`
	PolicyGeneration string   `json:"policy_generation"`
}

type ContextProjectionFact struct {
	ProjectionKind      string                      `json:"projection_kind"`
	FactID              string                      `json:"fact_id"`
	FactType            string                      `json:"fact_type"`
	SubjectRef          string                      `json:"subject_ref"`
	Data                ContextCanonicalJSON        `json:"data"`
	Unit                json.RawMessage             `json:"unit"`
	Meaning             string                      `json:"meaning"`
	Implications        []string                    `json:"implications"`
	Proves              []string                    `json:"proves"`
	DoesNotProve        []string                    `json:"does_not_prove"`
	BlindSpots          []string                    `json:"blind_spots"`
	Provenance          ContextProjectionProvenance `json:"provenance"`
	FreshnessState      string                      `json:"freshness_state"`
	Confidence          ContextProjectionConfidence `json:"confidence"`
	State               ContextProjectionState      `json:"state"`
	RelationRefs        []string                    `json:"relation_refs"`
	LegalNext           []string                    `json:"legal_next"`
	ProhibitedNext      []string                    `json:"prohibited_next"`
	ExpectedReceiptRefs []string                    `json:"expected_receipt_refs"`
	ScopeRef            string                      `json:"scope_ref"`
	TemporalRef         string                      `json:"temporal_ref"`
	ResolutionRef       string                      `json:"resolution_ref"`
	DerivationRef       string                      `json:"derivation_ref"`
	SupersedesRefs      []string                    `json:"supersedes_refs"`
	NoEffect            bool                        `json:"no_effect"`
	MayAuthorize        bool                        `json:"may_authorize"`
}

type ContextRedactedFact struct {
	ProjectionKind string                 `json:"projection_kind"`
	FactID         string                 `json:"fact_id"`
	State          ContextProjectionState `json:"state"`
	NoEffect       bool                   `json:"no_effect"`
	MayAuthorize   bool                   `json:"may_authorize"`
}

type ContextRedactedObject struct {
	ObjectKind   string                 `json:"object_kind"`
	ObjectID     string                 `json:"object_id"`
	State        ContextProjectionState `json:"state"`
	NoEffect     bool                   `json:"no_effect"`
	MayAuthorize bool                   `json:"may_authorize"`
}

type ContextGuardEvidence struct {
	Guard        string   `json:"guard"`
	Disposition  string   `json:"disposition"`
	EvidenceRefs []string `json:"evidence_refs"`
	MayAuthorize bool     `json:"may_authorize"`
}

type ContextProjectionAction struct {
	ActionID           string                 `json:"action_id"`
	Label              string                 `json:"label"`
	Disposition        string                 `json:"disposition"`
	PositionRef        string                 `json:"position_ref"`
	ActionClass        string                 `json:"action_class"`
	Operation          string                 `json:"operation"`
	LifecycleOperation *string                `json:"lifecycle_operation"`
	TransitionTo       *string                `json:"transition_to"`
	TransitionEdge     *string                `json:"transition_edge"`
	AdmissionRef       *string                `json:"admission_ref"`
	GuardEvidence      []ContextGuardEvidence `json:"guard_evidence"`
	SourceFactRefs     []string               `json:"source_fact_refs"`
	Why                string                 `json:"why"`
	PredictedEffect    string                 `json:"predicted_effect"`
	Recovery           string                 `json:"recovery"`
	ExpectedReceiptRef string                 `json:"expected_receipt_ref"`
	State              ContextProjectionState `json:"state"`
	NoEffect           bool                   `json:"no_effect"`
	MayAuthorize       bool                   `json:"may_authorize"`
}

// ContextProjectionEvent is one content-addressed, audience-sealed event in the
// projection's causal chronology. Payload remains exact private bytes so AIR can
// overwrite it alongside the retained carrier.
type ContextProjectionEvent struct {
	EventRef         string                 `json:"event_ref"`
	EventHash        string                 `json:"event_hash"`
	EventID          string                 `json:"event_id"`
	Kind             string                 `json:"kind"`
	SessionRef       string                 `json:"session_ref"`
	TaskRef          string                 `json:"task_ref"`
	TraceRef         string                 `json:"trace_ref"`
	PositionRef      string                 `json:"position_ref"`
	ScopeRef         string                 `json:"scope_ref"`
	TemporalRef      string                 `json:"temporal_ref"`
	ResolutionRef    string                 `json:"resolution_ref"`
	Generation       int64                  `json:"generation"`
	SubjectRef       string                 `json:"subject_ref"`
	OccurredAt       string                 `json:"occurred_at"`
	ExpiresAt        string                 `json:"expires_at"`
	ProducerRef      string                 `json:"producer_ref"`
	MethodRef        string                 `json:"method_ref"`
	PrivacyClass     string                 `json:"privacy_class"`
	AuthorityCeiling string                 `json:"authority_ceiling"`
	SourceRefs       []string               `json:"source_refs"`
	CausedBy         []string               `json:"caused_by"`
	SupersedesRefs   []string               `json:"supersedes_refs"`
	DerivationDepth  int64                  `json:"derivation_depth"`
	Payload          json.RawMessage        `json:"payload"`
	State            ContextProjectionState `json:"state"`
	MayAuthorize     bool                   `json:"may_authorize"`
}

type ContextProjectionImpingement struct {
	ImpingementID  string                 `json:"impingement_id"`
	Kind           string                 `json:"kind"`
	Summary        string                 `json:"summary"`
	SourceFactRefs []string               `json:"source_fact_refs"`
	Protects       []string               `json:"protects"`
	LegalNext      []string               `json:"legal_next"`
	State          ContextProjectionState `json:"state"`
	MayAuthorize   bool                   `json:"may_authorize"`
}

type ContextProjectionSignal struct {
	SignalID         string                 `json:"signal_id"`
	SignalRef        string                 `json:"signal_ref"`
	SignalHash       string                 `json:"signal_hash"`
	Kind             string                 `json:"kind"`
	Label            string                 `json:"label"`
	WhyNow           string                 `json:"why_now"`
	Uncertainty      string                 `json:"uncertainty"`
	DoesNotProve     []string               `json:"does_not_prove"`
	PrivacyClass     string                 `json:"privacy_class"`
	PositionRef      string                 `json:"position_ref"`
	SourceFactRefs   []string               `json:"source_fact_refs"`
	EstimateRefs     []string               `json:"estimate_refs"`
	LensRef          string                 `json:"lens_ref"`
	ConstellationRef string                 `json:"constellation_ref"`
	PortalRef        *string                `json:"portal_ref"`
	ValueVector      json.RawMessage        `json:"value_vector"`
	State            ContextProjectionState `json:"state"`
	NoEffect         bool                   `json:"no_effect"`
	MayAuthorize     bool                   `json:"may_authorize"`
}

type ContextProjectionPortal struct {
	PortalRef        string                 `json:"portal_ref"`
	Kind             string                 `json:"kind"`
	Purpose          string                 `json:"purpose"`
	PrivacyClass     string                 `json:"privacy_class"`
	SourceFactRefs   []string               `json:"source_fact_refs"`
	BudgetRef        string                 `json:"budget_ref"`
	EffectivityBasis []string               `json:"effectivity_basis"`
	State            ContextProjectionState `json:"state"`
	NoEffect         bool                   `json:"no_effect"`
	MayAuthorize     bool                   `json:"may_authorize"`
}

type ContextCounterfactual struct {
	ActionID            string               `json:"action_id"`
	PredictedStateDelta ContextCanonicalJSON `json:"predicted_state_delta"`
	NoEffect            bool                 `json:"no_effect"`
	MayAuthorize        bool                 `json:"may_authorize"`
}

type ContextBoundaryOrientation struct {
	FacetID         string                `json:"facet_id"`
	FacetRef        string                `json:"facet_ref"`
	FacetHash       string                `json:"facet_hash"`
	FocusRef        string                `json:"focus_ref"`
	PositionRef     string                `json:"position_ref"`
	BoundaryKind    string                `json:"boundary_kind"`
	WhyNowRefs      []string              `json:"why_now_refs"`
	Protects        []string              `json:"protects"`
	Can             []string              `json:"can"`
	Cannot          []string              `json:"cannot"`
	Until           []string              `json:"until"`
	IFF             []string              `json:"iff"`
	ChangeAuthority string                `json:"change_authority"`
	Counterfactual  ContextCounterfactual `json:"counterfactual"`
	NoEffect        bool                  `json:"no_effect"`
	MayAuthorize    bool                  `json:"may_authorize"`
}

type ContextLifecyclePossibility struct {
	FacetID                 string                 `json:"facet_id"`
	FacetRef                string                 `json:"facet_ref"`
	FacetHash               string                 `json:"facet_hash"`
	CandidateRef            string                 `json:"candidate_ref"`
	SourceFactRefs          []string               `json:"source_fact_refs"`
	WhyNow                  string                 `json:"why_now"`
	DoesNotProve            []string               `json:"does_not_prove"`
	Uncertainty             string                 `json:"uncertainty"`
	AlternativeDispositions []string               `json:"alternative_dispositions"`
	UnknownFields           []string               `json:"unknown_fields"`
	CandidatePlant          ContextCanonicalJSON   `json:"candidate_plant"`
	EstimatedCost           ContextCanonicalJSON   `json:"estimated_cost"`
	PlantGap                ContextProjectionState `json:"plant_gap"`
	HarnessGap              ContextProjectionState `json:"harness_gap"`
	MeasurementGap          ContextProjectionState `json:"measurement_gap"`
	LawfulNext              []string               `json:"lawful_next"`
	NoEffect                bool                   `json:"no_effect"`
	MayAuthorize            bool                   `json:"may_authorize"`
}

type ContextLifecycleFSM struct {
	What string
	How  string
	Must string
}

type contextDemandShape struct {
	Fingerprint  string                 `json:"fingerprint"`
	Descriptor   json.RawMessage        `json:"descriptor"`
	State        ContextProjectionState `json:"state"`
	MayAuthorize bool                   `json:"may_authorize"`
}

type contextDemandShapeDescriptor struct {
	Schema                 string               `json:"schema"`
	DescriptorRef          string               `json:"descriptor_ref"`
	SessionRef             string               `json:"session_ref"`
	Strategy               ContextCanonicalJSON `json:"strategy"`
	Strata                 ContextCanonicalJSON `json:"strata"`
	Canon                  ContextCanonicalJSON `json:"canon"`
	PositionBasis          ContextCanonicalJSON `json:"position_basis"`
	OfferedAffordances     []string             `json:"offered_affordances"`
	ProvenanceGeneration   string               `json:"provenance_generation"`
	PolicyGeneration       string               `json:"policy_generation"`
	AudiencePolicy         ContextCanonicalJSON `json:"audience_policy"`
	Kernel                 ContextCanonicalJSON `json:"kernel"`
	Budget                 ContextCanonicalJSON `json:"budget"`
	DemandShapeFingerprint string               `json:"demand_shape_fingerprint"`
	MayAuthorize           bool                 `json:"may_authorize"`
}

type contextLifecycleOperationAdmission struct {
	Actions             []string `json:"actions"`
	AuthorityCapability string   `json:"authority_capability"`
	Enforcement         string   `json:"enforcement"`
	EnforcementRef      *string  `json:"enforcement_ref"`
	Guards              []string `json:"guards"`
	Operation           string   `json:"operation"`
}

type contextLifecycleTransition struct {
	Actions             []string `json:"actions"`
	AuthorityCapability string   `json:"authority_capability"`
	Enforcement         string   `json:"enforcement"`
	EnforcementRef      *string  `json:"enforcement_ref"`
	Guards              []string `json:"guards"`
	ProjectionRole      string   `json:"projection_role"`
	To                  string   `json:"to"`
}

type contextLifecycleStage struct {
	Token               string                               `json:"token"`
	Next                []contextLifecycleTransition         `json:"next"`
	Fall                []contextLifecycleTransition         `json:"fall"`
	OperationAdmissions []contextLifecycleOperationAdmission `json:"operation_admissions"`
}

type contextProjectionEnvelope struct {
	Schema                       string                         `json:"schema"`
	ProjectionRef                string                         `json:"projection_ref"`
	ProjectionHash               string                         `json:"projection_hash"`
	Position                     ContextProjectionPosition      `json:"position"`
	DemandShape                  json.RawMessage                `json:"demand_shape"`
	Audience                     string                         `json:"audience"`
	Purpose                      string                         `json:"purpose"`
	Depth                        string                         `json:"depth"`
	DeviceClass                  string                         `json:"device_class"`
	Register                     string                         `json:"register"`
	DecoderRef                   string                         `json:"decoder_ref"`
	FocusRef                     string                         `json:"focus_ref"`
	State                        ContextProjectionState         `json:"state"`
	Meaning                      []string                       `json:"meaning"`
	Implications                 []string                       `json:"implications"`
	BlindSpots                   []string                       `json:"blind_spots"`
	Scopes                       json.RawMessage                `json:"scopes"`
	TemporalCoordinates          json.RawMessage                `json:"temporal_coordinates"`
	ResolutionCoordinates        json.RawMessage                `json:"resolution_coordinates"`
	SourceAdmissions             json.RawMessage                `json:"source_admissions"`
	Observations                 json.RawMessage                `json:"observations"`
	Derivations                  json.RawMessage                `json:"derivations"`
	Events                       []json.RawMessage              `json:"events"`
	Facts                        []json.RawMessage              `json:"facts"`
	RedactedObjects              []ContextRedactedObject        `json:"redacted_objects"`
	Relations                    json.RawMessage                `json:"relations"`
	Actions                      []ContextProjectionAction      `json:"actions"`
	Impingements                 []ContextProjectionImpingement `json:"impingements"`
	SignalEstimates              json.RawMessage                `json:"signal_estimates"`
	SignalLenses                 json.RawMessage                `json:"signal_lenses"`
	SignalConstellations         json.RawMessage                `json:"signal_constellations"`
	OrientingSignals             []ContextProjectionSignal      `json:"orienting_signals"`
	PortalOffers                 []ContextProjectionPortal      `json:"portal_offers"`
	SignalLearningReceipts       json.RawMessage                `json:"signal_learning_receipts"`
	LegalNext                    []string                       `json:"legal_next"`
	ProhibitedNext               []string                       `json:"prohibited_next"`
	LineageRefs                  []string                       `json:"lineage_refs"`
	SupersedesRefs               []string                       `json:"supersedes_refs"`
	ProducerRef                  string                         `json:"producer_ref"`
	VerificationScope            string                         `json:"verification_scope"`
	ProducerVerificationRequired bool                           `json:"producer_verification_required"`
	GeneratedAt                  string                         `json:"generated_at"`
	StaleAfter                   string                         `json:"stale_after"`
	AudiencePolicyDigest         string                         `json:"audience_policy_digest"`
	MappingManifest              json.RawMessage                `json:"mapping_manifest"`
	Loss                         json.RawMessage                `json:"loss"`
	Orientation                  json.RawMessage                `json:"orientation"`
	LifecyclePossibility         json.RawMessage                `json:"lifecycle_possibility"`
	NoEffect                     bool                           `json:"no_effect"`
	MayAuthorize                 bool                           `json:"may_authorize"`
}

var contextProjectionHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var ErrContextProjectionStale = errors.New("context projection is stale")

const (
	ContextProjectionMaxDisplayBytes = 1 << 20
	contextProjectionMaxItems        = 1024
	contextProjectionMaxCrossLinks   = 65536
	contextProjectionMaxTextBytes    = 64 << 10
	contextProjectionMaxTextItems    = 4096
)

var contextProjectionRequiredFields = []string{
	"actions", "audience", "audience_policy_digest", "blind_spots", "decoder_ref",
	"demand_shape", "depth", "derivations", "device_class", "events", "facts", "focus_ref",
	"generated_at", "impingements", "implications", "legal_next",
	"lifecycle_possibility", "lineage_refs", "loss", "mapping_manifest",
	"may_authorize", "meaning", "no_effect", "observations", "orientation",
	"orienting_signals", "portal_offers", "position", "producer_ref",
	"producer_verification_required", "prohibited_next", "projection_hash",
	"projection_ref", "purpose", "redacted_objects", "register", "relations",
	"resolution_coordinates", "schema", "scopes", "signal_constellations",
	"signal_estimates", "signal_learning_receipts", "signal_lenses",
	"source_admissions", "stale_after", "state", "supersedes_refs",
	"temporal_coordinates", "verification_scope",
}

// ProjectionIndex validates the structure and current freshness needed for
// contextual rendering while retaining the carrier's outer HOLD.
func (r ContextReadout) ProjectionIndex() (ContextProjectionIndex, error) {
	return r.ProjectionIndexAt(time.Now().UTC())
}

// ProjectionIndexAt supplies an explicit clock for deterministic replay and
// tests. Historical envelopes remain evidence but do not produce a semantic
// display index after stale_after.
func (r ContextReadout) ProjectionIndexAt(now time.Time) (ContextProjectionIndex, error) {
	index, err := r.projectionIndex()
	if err != nil {
		return ContextProjectionIndex{}, err
	}
	staleAfter, err := time.Parse(time.RFC3339, index.StaleAfter)
	if err != nil {
		return ContextProjectionIndex{}, fmt.Errorf("context projection stale_after is invalid")
	}
	if !now.Before(staleAfter) {
		return ContextProjectionIndex{}, ErrContextProjectionStale
	}
	return index, nil
}

// projectionIndex proves only structure and internal content addressing. It
// proves neither producer authenticity nor a live frame receipt.
func (r ContextReadout) projectionIndex() (ContextProjectionIndex, error) {
	if r.Schema != ContextReadSchema || r.State != ContextReadHold ||
		r.Audience != ContextReadAudience || r.Projection == nil ||
		r.Compatibility != nil {
		return ContextProjectionIndex{}, fmt.Errorf("context projection is not an operator HOLD carrier")
	}
	wantReasons := []string{
		ContextReadReasonCanonUnverified,
		"producer_receipt_missing",
	}
	if !equalContextStrings(r.ReasonCodes, wantReasons) {
		return ContextProjectionIndex{}, fmt.Errorf("context projection has an unexpected admission reason set")
	}
	if len(*r.Projection) > ContextProjectionMaxDisplayBytes {
		return ContextProjectionIndex{}, fmt.Errorf(
			"context projection exceeds the semantic display budget",
		)
	}

	var fields map[string]json.RawMessage
	if err := decodeContextJSON(*r.Projection, &fields, false); err != nil {
		return ContextProjectionIndex{}, err
	}
	if len(fields) != len(contextProjectionRequiredFields) {
		return ContextProjectionIndex{}, fmt.Errorf("context projection has an incomplete field set")
	}
	for _, field := range contextProjectionRequiredFields {
		if _, ok := fields[field]; !ok {
			return ContextProjectionIndex{}, fmt.Errorf("context projection is missing %s", field)
		}
	}

	var envelope contextProjectionEnvelope
	if err := decodeContextJSON(*r.Projection, &envelope, true); err != nil {
		return ContextProjectionIndex{}, err
	}
	contentHash, err := ContextProjectionContentHash(*r.Projection)
	if err != nil {
		return ContextProjectionIndex{}, err
	}
	if contentHash != envelope.ProjectionHash {
		return ContextProjectionIndex{}, fmt.Errorf("context projection hash does not bind its content")
	}
	return validateContextProjectionEnvelope(envelope)
}

func decodeContextJSON(payload []byte, target any, strict bool) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode context projection: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("context projection contains trailing JSON")
		}
		return fmt.Errorf("decode trailing context projection: %w", err)
	}
	return nil
}

// ContextProjectionContentHash reproduces the canonical package domain hash.
// It proves content identity only, never producer authenticity or authority.
func ContextProjectionContentHash(payload []byte) (string, error) {
	return contextProjectionDomainHash(
		payload,
		"hapax.projection-envelope.v1",
		"projection_ref",
		"projection_hash",
	)
}

// ContextProjectionEventContentHash reproduces the event domain hash for
// deterministic conformance tests without treating the hash as authenticity.
func ContextProjectionEventContentHash(payload []byte) (string, error) {
	return contextProjectionDomainHash(
		payload,
		"hapax.epistemic-flow-event.v1",
		"event_ref",
		"event_hash",
	)
}

func contextProjectionDomainHash(payload []byte, domainName string, omitted ...string) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode context projection for hashing: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("context projection contains trailing hash input")
		}
		return "", fmt.Errorf("decode trailing context hash input: %w", err)
	}
	body, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Errorf("context projection hash input is not an object")
	}
	for _, field := range omitted {
		delete(body, field)
	}
	canonical, err := appendContextCanonicalJSON(nil, body)
	if err != nil {
		return "", err
	}
	domain := []byte(domainName + "\x00")
	hashInput := make([]byte, 0, len(domain)+len(canonical))
	hashInput = append(hashInput, domain...)
	hashInput = append(hashInput, canonical...)
	return fmt.Sprintf("%x", sha256.Sum256(hashInput)), nil
}

func appendContextCanonicalJSON(dst []byte, value any) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return append(dst, "null"...), nil
	case bool:
		if typed {
			return append(dst, "true"...), nil
		}
		return append(dst, "false"...), nil
	case string:
		return appendContextCanonicalString(dst, typed), nil
	case json.Number:
		raw := typed.String()
		if strings.ContainsAny(raw, ".eE") {
			return nil, fmt.Errorf("context canonical JSON forbids non-integer numbers")
		}
		integer, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("context canonical JSON number is invalid: %w", err)
		}
		return strconv.AppendInt(dst, integer, 10), nil
	case []any:
		dst = append(dst, '[')
		for i, item := range typed {
			if i > 0 {
				dst = append(dst, ',')
			}
			var err error
			dst, err = appendContextCanonicalJSON(dst, item)
			if err != nil {
				return nil, err
			}
		}
		return append(dst, ']'), nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		dst = append(dst, '{')
		for i, key := range keys {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendContextCanonicalString(dst, key)
			dst = append(dst, ':')
			var err error
			dst, err = appendContextCanonicalJSON(dst, typed[key])
			if err != nil {
				return nil, err
			}
		}
		return append(dst, '}'), nil
	default:
		return nil, fmt.Errorf("context canonical JSON contains %T", value)
	}
}

func appendContextCanonicalString(dst []byte, value string) []byte {
	const hex = "0123456789abcdef"
	dst = append(dst, '"')
	for _, r := range value {
		switch r {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\r':
			dst = append(dst, '\\', 'r')
		default:
			switch {
			case r >= 0x20 && r <= 0x7e:
				dst = append(dst, byte(r))
			case r <= 0xffff:
				dst = appendContextUnicodeEscape(dst, uint16(r), hex)
			default:
				r -= 0x10000
				high := uint16(0xd800 + (r >> 10))
				low := uint16(0xdc00 + (r & 0x3ff))
				dst = appendContextUnicodeEscape(dst, high, hex)
				dst = appendContextUnicodeEscape(dst, low, hex)
			}
		}
	}
	return append(dst, '"')
}

func appendContextUnicodeEscape(dst []byte, value uint16, hex string) []byte {
	return append(
		dst,
		'\\',
		'u',
		hex[(value>>12)&0xf],
		hex[(value>>8)&0xf],
		hex[(value>>4)&0xf],
		hex[value&0xf],
	)
}

func validateContextProjectionEnvelope(
	envelope contextProjectionEnvelope,
) (ContextProjectionIndex, error) {
	if envelope.Schema != "hapax.projection-envelope.v1" ||
		envelope.Audience != ContextReadAudience ||
		envelope.VerificationScope != "structure_and_content_address_only" ||
		!envelope.ProducerVerificationRequired ||
		envelope.MayAuthorize || !envelope.NoEffect {
		return ContextProjectionIndex{}, fmt.Errorf("context projection root contract is invalid")
	}
	if !validContextProjectionHash(envelope.ProjectionHash) ||
		envelope.ProjectionRef != "projection-envelope@sha256:"+envelope.ProjectionHash {
		return ContextProjectionIndex{}, fmt.Errorf("context projection identity is invalid")
	}
	if err := validateContextEnum(
		envelope.Purpose,
		"context projection purpose",
		"orientation",
		"lifecycle_possibility",
		"operation",
	); err != nil {
		return ContextProjectionIndex{}, fmt.Errorf(
			"context projection purpose %q has no lossless display index: %w",
			envelope.Purpose, err,
		)
	}
	if err := validateContextEnum(envelope.Depth, "depth",
		"immediate", "expanded", "inspectable", "raw"); err != nil {
		return ContextProjectionIndex{}, err
	}
	if err := validateContextEnum(envelope.DeviceClass, "device_class",
		"monitor", "handheld", "compact", "accessible_linear"); err != nil {
		return ContextProjectionIndex{}, err
	}
	if err := validateContextEnum(envelope.Register, "register",
		"plain", "labeled", "formal", "raw"); err != nil {
		return ContextProjectionIndex{}, err
	}
	for label, value := range map[string]string{
		"decoder_ref":  envelope.DecoderRef,
		"focus_ref":    envelope.FocusRef,
		"producer_ref": envelope.ProducerRef,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return ContextProjectionIndex{}, err
		}
	}
	if err := validateContextState(envelope.State); err != nil {
		return ContextProjectionIndex{}, err
	}
	if err := validateContextTexts(envelope.Meaning, "meaning", false); err != nil {
		return ContextProjectionIndex{}, err
	}
	if err := validateContextTexts(envelope.Implications, "implications", false); err != nil {
		return ContextProjectionIndex{}, err
	}
	if err := validateContextTexts(envelope.BlindSpots, "blind_spots", false); err != nil {
		return ContextProjectionIndex{}, err
	}
	if err := validateContextTexts(envelope.LineageRefs, "lineage_refs", false); err != nil {
		return ContextProjectionIndex{}, err
	}
	if err := validateContextTexts(envelope.SupersedesRefs, "supersedes_refs", true); err != nil {
		return ContextProjectionIndex{}, err
	}
	for _, redacted := range envelope.RedactedObjects {
		if err := validateContextRedactedObject(redacted); err != nil {
			return ContextProjectionIndex{}, err
		}
	}
	generated, err := time.Parse(time.RFC3339, envelope.GeneratedAt)
	if err != nil {
		return ContextProjectionIndex{}, fmt.Errorf("context projection generated_at is invalid")
	}
	stale, err := time.Parse(time.RFC3339, envelope.StaleAfter)
	if err != nil || !generated.Before(stale) {
		return ContextProjectionIndex{}, fmt.Errorf("context projection stale_after is invalid")
	}
	if !validContextProjectionHash(envelope.AudiencePolicyDigest) {
		return ContextProjectionIndex{}, fmt.Errorf("context audience policy digest is invalid")
	}
	if len(envelope.Facts) > contextProjectionMaxItems ||
		len(envelope.RedactedObjects) > contextProjectionMaxItems ||
		len(envelope.Actions) > contextProjectionMaxItems ||
		len(envelope.Impingements) > contextProjectionMaxItems ||
		len(envelope.OrientingSignals) > contextProjectionMaxItems ||
		len(envelope.PortalOffers) > contextProjectionMaxItems {
		return ContextProjectionIndex{}, fmt.Errorf(
			"context projection exceeds a semantic collection budget",
		)
	}
	if len(envelope.Actions) > 0 &&
		len(envelope.Facts) > contextProjectionMaxCrossLinks/len(envelope.Actions) {
		return ContextProjectionIndex{}, fmt.Errorf(
			"context projection exceeds the fact/action cross-link budget",
		)
	}

	var demand contextDemandShape
	if err := decodeContextJSON(envelope.DemandShape, &demand, true); err != nil {
		return ContextProjectionIndex{}, err
	}
	if demand.MayAuthorize || !validContextProjectionHash(demand.Fingerprint) {
		return ContextProjectionIndex{}, fmt.Errorf("context demand shape is invalid")
	}
	if err := validateContextState(demand.State); err != nil {
		return ContextProjectionIndex{}, err
	}
	demandDescriptor, err := validateContextDemandDescriptor(
		demand,
		envelope.Position,
		envelope.Actions,
	)
	if err != nil {
		return ContextProjectionIndex{}, err
	}
	if err := validateContextPosition(envelope.Position, demand.Fingerprint); err != nil {
		return ContextProjectionIndex{}, err
	}

	facts := make([]ContextProjectionFact, 0, len(envelope.Facts))
	redactedFacts := make([]ContextRedactedFact, 0)
	factByID := make(map[string]ContextProjectionFact, len(envelope.Facts))
	seenFactIDs := make(map[string]struct{}, len(envelope.Facts))
	var lifecycle *ContextProjectionFact
	for _, rawFact := range envelope.Facts {
		var discriminator struct {
			ProjectionKind string `json:"projection_kind"`
		}
		if err := decodeContextJSON(rawFact, &discriminator, false); err != nil {
			return ContextProjectionIndex{}, err
		}
		switch discriminator.ProjectionKind {
		case "fact":
			var fact ContextProjectionFact
			if err := decodeContextJSON(rawFact, &fact, true); err != nil {
				return ContextProjectionIndex{}, err
			}
			if err := validateContextFact(fact); err != nil {
				return ContextProjectionIndex{}, err
			}
			if _, duplicate := seenFactIDs[fact.FactID]; duplicate {
				return ContextProjectionIndex{}, fmt.Errorf("duplicate projected fact %s", fact.FactID)
			}
			seenFactIDs[fact.FactID] = struct{}{}
			facts = append(facts, fact)
			factByID[fact.FactID] = fact
			if fact.FactType == "lifecycle_fsm" {
				if lifecycle != nil {
					return ContextProjectionIndex{}, fmt.Errorf("multiple lifecycle_fsm facts")
				}
				copyOfFact := fact
				lifecycle = &copyOfFact
			}
		case "redacted":
			var fact ContextRedactedFact
			if err := decodeContextJSON(rawFact, &fact, true); err != nil {
				return ContextProjectionIndex{}, err
			}
			if err := validateContextRedactedFact(fact); err != nil {
				return ContextProjectionIndex{}, err
			}
			if _, duplicate := seenFactIDs[fact.FactID]; duplicate {
				return ContextProjectionIndex{}, fmt.Errorf("duplicate projected fact %s", fact.FactID)
			}
			seenFactIDs[fact.FactID] = struct{}{}
			redactedFacts = append(redactedFacts, fact)
		default:
			return ContextProjectionIndex{}, fmt.Errorf(
				"context projection fact has unknown projection_kind %q",
				discriminator.ProjectionKind,
			)
		}
	}
	if lifecycle == nil {
		return ContextProjectionIndex{}, fmt.Errorf("context projection has no lifecycle_fsm fact")
	}
	lifecycleFSM, err := decodeContextLifecycleFSM(
		lifecycle.Data,
		envelope.Position.LifecycleFSMDataSHA256,
	)
	if err != nil {
		return ContextProjectionIndex{}, err
	}

	actionByID := make(map[string]ContextProjectionAction, len(envelope.Actions))
	legal := make([]string, 0, len(envelope.Actions))
	prohibited := make([]string, 0, len(envelope.Actions))
	for i := range envelope.Actions {
		action := envelope.Actions[i]
		if err := validateContextAction(action, envelope.Position, factByID); err != nil {
			return ContextProjectionIndex{}, err
		}
		if _, duplicate := actionByID[action.ActionID]; duplicate {
			return ContextProjectionIndex{}, fmt.Errorf("duplicate projected action %s", action.ActionID)
		}
		actionByID[action.ActionID] = action
		if action.Disposition == "legal" {
			legal = append(legal, action.ActionID)
		} else {
			prohibited = append(prohibited, action.ActionID)
		}
	}
	sort.Strings(legal)
	sort.Strings(prohibited)
	if !equalContextStrings(envelope.LegalNext, legal) ||
		!equalContextStrings(envelope.ProhibitedNext, prohibited) {
		return ContextProjectionIndex{}, fmt.Errorf("context action indexes do not match dispositions")
	}
	if err := validateFactActionIndexes(facts, envelope.Actions); err != nil {
		return ContextProjectionIndex{}, err
	}
	events, err := validateContextProjectionEvents(
		envelope,
		demandDescriptor,
		factByID,
		actionByID,
	)
	if err != nil {
		return ContextProjectionIndex{}, err
	}

	portalByRef := make(map[string]struct{}, len(envelope.PortalOffers))
	for i := range envelope.PortalOffers {
		portal := envelope.PortalOffers[i]
		if err := validateContextPortal(portal, factByID); err != nil {
			return ContextProjectionIndex{}, err
		}
		if _, duplicate := portalByRef[portal.PortalRef]; duplicate {
			return ContextProjectionIndex{}, fmt.Errorf("duplicate context portal %s", portal.PortalRef)
		}
		portalByRef[portal.PortalRef] = struct{}{}
	}
	for i := range envelope.Impingements {
		if err := validateContextImpingement(
			envelope.Impingements[i], factByID, actionByID,
		); err != nil {
			return ContextProjectionIndex{}, err
		}
	}
	for i := range envelope.OrientingSignals {
		if err := validateContextSignal(
			envelope.OrientingSignals[i],
			envelope.Position.PositionRef,
			factByID,
			portalByRef,
		); err != nil {
			return ContextProjectionIndex{}, err
		}
	}
	var orientation *ContextBoundaryOrientation
	var lifecyclePossibility *ContextLifecyclePossibility
	switch envelope.Purpose {
	case "orientation":
		if contextProjectionRawIsNull(envelope.Orientation) ||
			!contextProjectionRawIsNull(envelope.LifecyclePossibility) {
			return ContextProjectionIndex{}, fmt.Errorf(
				"orientation projection has an invalid purpose facet set",
			)
		}
		var err error
		orientation, err = validateContextOrientationRaw(
			envelope.Orientation,
			envelope.FocusRef,
			envelope.Position.PositionRef,
			envelope.LegalNext,
			envelope.ProhibitedNext,
			envelope.Position.ReceiptLineage,
			factByID,
			actionByID,
		)
		if err != nil {
			return ContextProjectionIndex{}, err
		}
	case "lifecycle_possibility":
		if !contextProjectionRawIsNull(envelope.Orientation) ||
			contextProjectionRawIsNull(envelope.LifecyclePossibility) {
			return ContextProjectionIndex{}, fmt.Errorf(
				"lifecycle possibility projection has an invalid purpose facet set",
			)
		}
		var err error
		lifecyclePossibility, err = validateContextLifecyclePossibility(
			envelope.LifecyclePossibility,
			envelope.LegalNext,
			factByID,
		)
		if err != nil {
			return ContextProjectionIndex{}, err
		}
	case "operation":
		if !contextProjectionRawIsNull(envelope.Orientation) ||
			!contextProjectionRawIsNull(envelope.LifecyclePossibility) {
			return ContextProjectionIndex{}, fmt.Errorf(
				"operation projection has an invalid purpose facet set",
			)
		}
	}

	focused := make([]ContextProjectionFact, 0, 1)
	for _, fact := range facts {
		if envelope.FocusRef == fact.FactID || envelope.FocusRef == fact.SubjectRef {
			focused = append(focused, fact)
		}
	}
	if len(focused) == 0 {
		return ContextProjectionIndex{}, fmt.Errorf("context focus does not resolve to a fact")
	}
	expectedFocusState := focused[0].State
	if len(focused) > 1 {
		allPresent := true
		reasons := make(map[string]struct{})
		for _, fact := range focused {
			if fact.State.Value != "present" {
				allPresent = false
				for _, reason := range fact.State.ReasonCodes {
					reasons[reason] = struct{}{}
				}
			}
		}
		if allPresent {
			expectedFocusState = ContextProjectionState{Value: "present", ReasonCodes: []string{}}
		} else {
			merged := make([]string, 0, len(reasons))
			for reason := range reasons {
				merged = append(merged, reason)
			}
			sort.Strings(merged)
			if len(merged) == 0 {
				merged = []string{"mixed_context_state"}
			}
			expectedFocusState = ContextProjectionState{Value: "partial", ReasonCodes: merged}
		}
	}
	if !equalContextState(envelope.State, expectedFocusState) {
		return ContextProjectionIndex{}, fmt.Errorf("context root state differs from focused facts")
	}

	return ContextProjectionIndex{
		ProjectionRef:        envelope.ProjectionRef,
		ProjectionHash:       envelope.ProjectionHash,
		Audience:             envelope.Audience,
		Purpose:              envelope.Purpose,
		Depth:                envelope.Depth,
		DeviceClass:          envelope.DeviceClass,
		Register:             envelope.Register,
		FocusRef:             envelope.FocusRef,
		State:                envelope.State,
		Position:             envelope.Position,
		Facts:                facts,
		RedactedFacts:        redactedFacts,
		RedactedObjects:      envelope.RedactedObjects,
		FocusedFacts:         focused,
		Actions:              envelope.Actions,
		Events:               events,
		Impingements:         envelope.Impingements,
		Signals:              envelope.OrientingSignals,
		Portals:              envelope.PortalOffers,
		Orientation:          orientation,
		LifecyclePossibility: lifecyclePossibility,
		Meaning:              envelope.Meaning,
		Implications:         envelope.Implications,
		BlindSpots:           envelope.BlindSpots,
		LegalNext:            envelope.LegalNext,
		ProhibitedNext:       envelope.ProhibitedNext,
		LineageRefs:          envelope.LineageRefs,
		SupersedesRefs:       envelope.SupersedesRefs,
		ProducerRef:          envelope.ProducerRef,
		GeneratedAt:          envelope.GeneratedAt,
		StaleAfter:           envelope.StaleAfter,
		LifecycleFSM:         lifecycleFSM,
	}, nil
}

func validateContextDemandDescriptor(
	demand contextDemandShape,
	position ContextProjectionPosition,
	actions []ContextProjectionAction,
) (contextDemandShapeDescriptor, error) {
	if len(demand.Descriptor) == 0 || bytes.Equal(bytes.TrimSpace(demand.Descriptor), []byte("null")) {
		return contextDemandShapeDescriptor{}, fmt.Errorf("context demand descriptor is required")
	}
	var descriptor contextDemandShapeDescriptor
	if err := decodeContextJSON(demand.Descriptor, &descriptor, true); err != nil {
		return contextDemandShapeDescriptor{}, err
	}
	if descriptor.Schema != "hapax.demand-shape-descriptor.v1" ||
		descriptor.MayAuthorize || descriptor.DemandShapeFingerprint != demand.Fingerprint ||
		descriptor.DemandShapeFingerprint != position.DemandShapeFingerprint ||
		descriptor.DescriptorRef != "demand-shape@sha256:"+descriptor.DemandShapeFingerprint {
		return contextDemandShapeDescriptor{}, fmt.Errorf("context demand descriptor identity is invalid")
	}
	for label, value := range map[string]string{
		"demand session_ref":           descriptor.SessionRef,
		"demand provenance_generation": descriptor.ProvenanceGeneration,
		"demand policy_generation":     descriptor.PolicyGeneration,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return contextDemandShapeDescriptor{}, err
		}
	}
	for label, value := range map[string]ContextCanonicalJSON{
		"demand strategy":        descriptor.Strategy,
		"demand strata":          descriptor.Strata,
		"demand canon":           descriptor.Canon,
		"demand position_basis":  descriptor.PositionBasis,
		"demand audience_policy": descriptor.AudiencePolicy,
		"demand kernel":          descriptor.Kernel,
		"demand budget":          descriptor.Budget,
	} {
		if err := validateContextCanonicalObject(value, label); err != nil {
			return contextDemandShapeDescriptor{}, err
		}
	}
	descriptorHash, err := contextProjectionDomainHash(
		demand.Descriptor,
		"hapax.demand-shape-descriptor.v1",
		"descriptor_ref",
		"demand_shape_fingerprint",
	)
	if err != nil || descriptorHash != descriptor.DemandShapeFingerprint {
		return contextDemandShapeDescriptor{}, fmt.Errorf("context demand descriptor hash is invalid")
	}
	affordances := make([]string, 0, len(actions))
	for _, action := range actions {
		affordances = append(affordances, action.ActionID)
	}
	sort.Strings(affordances)
	if !equalContextStrings(descriptor.OfferedAffordances, affordances) {
		return contextDemandShapeDescriptor{}, fmt.Errorf("context demand affordances differ from actions")
	}
	wantCanon, err := buildContextCanonicalObject(map[string]any{
		"bundle_hash": position.CanonBundleHash,
		"bundle_ref":  position.CanonBundleRef,
		"canon_id":    position.CanonID,
		"image_hash":  position.CanonImageHash,
		"level":       position.CanonLevel,
		"version":     position.CanonVersion,
	})
	if err != nil || descriptor.Canon != wantCanon {
		return contextDemandShapeDescriptor{}, fmt.Errorf("context demand canon differs from position")
	}
	wantPosition, err := buildContextCanonicalObject(map[string]any{
		"legal_successors":          position.LegalSuccessors,
		"lifecycle_definition_hash": position.LifecycleDefinitionHash,
		"lifecycle_definition_ref":  position.LifecycleDefinitionRef,
		"stage_token":               position.StageToken,
	})
	if err != nil || descriptor.PositionBasis != wantPosition {
		return contextDemandShapeDescriptor{}, fmt.Errorf("context demand position basis differs from position")
	}
	return descriptor, nil
}

func validateContextCanonicalObject(value ContextCanonicalJSON, label string) error {
	if !validContextProjectionHash(value.SHA256) ||
		fmt.Sprintf("%x", sha256.Sum256([]byte(value.CanonicalJSON))) != value.SHA256 {
		return fmt.Errorf("%s hash is invalid", label)
	}
	decoder := json.NewDecoder(strings.NewReader(value.CanonicalJSON))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("%s JSON is invalid", label)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return fmt.Errorf("%s must encode an object", label)
	}
	canonical, err := appendContextCanonicalJSON(nil, decoded)
	if err != nil || string(canonical) != value.CanonicalJSON {
		return fmt.Errorf("%s is not canonical JSON", label)
	}
	return nil
}

func buildContextCanonicalObject(value map[string]any) (ContextCanonicalJSON, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return ContextCanonicalJSON{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return ContextCanonicalJSON{}, err
	}
	canonical, err := appendContextCanonicalJSON(nil, decoded)
	if err != nil {
		return ContextCanonicalJSON{}, err
	}
	return ContextCanonicalJSON{
		CanonicalJSON: string(canonical),
		SHA256:        fmt.Sprintf("%x", sha256.Sum256(canonical)),
	}, nil
}

func validateContextPosition(position ContextProjectionPosition, demandHash string) error {
	for label, value := range map[string]string{
		"task_ref":                 position.TaskRef,
		"stage_token":              position.StageToken,
		"authority_case":           position.AuthorityCase,
		"claim_ref":                position.ClaimRef,
		"route_decision_ref":       position.RouteDecisionRef,
		"canon_bundle_ref":         position.CanonBundleRef,
		"canon_id":                 position.CanonID,
		"canon_level":              position.CanonLevel,
		"lifecycle_definition_ref": position.LifecycleDefinitionRef,
		"position_ref":             position.PositionRef,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	if position.MayAuthorize || position.CanonVersion < 1 {
		return fmt.Errorf("context position may not authorize and needs a canon version")
	}
	for label, value := range map[string]string{
		"canon_bundle_hash":           position.CanonBundleHash,
		"canon_image_hash":            position.CanonImageHash,
		"lifecycle_definition_hash":   position.LifecycleDefinitionHash,
		"lifecycle_fsm_data_sha256":   position.LifecycleFSMDataSHA256,
		"demand_shape_fingerprint":    position.DemandShapeFingerprint,
		"effective_constraint_digest": position.EffectiveConstraintDigest,
		"impingement_digest":          position.ImpingementDigest,
		"portal_set_digest":           position.PortalSetDigest,
		"position_hash":               position.PositionHash,
	} {
		if !validContextProjectionHash(value) {
			return fmt.Errorf("%s is not a canonical digest", label)
		}
	}
	if position.PositionRef != "context-position@sha256:"+position.PositionHash ||
		position.CanonBundleRef != "canon-bundle@sha256:"+position.CanonBundleHash ||
		position.LifecycleDefinitionRef !=
			"lifecycle-definition@sha256:"+position.LifecycleDefinitionHash ||
		position.DemandShapeFingerprint != demandHash {
		return fmt.Errorf("context position identity binding is invalid")
	}
	var definition struct {
		Schema         string `json:"schema"`
		DefinitionRef  string `json:"definition_ref"`
		DefinitionHash string `json:"definition_hash"`
		MayAuthorize   bool   `json:"may_authorize"`
	}
	if err := decodeContextJSON(position.LifecycleDefinition, &definition, false); err != nil {
		return err
	}
	definitionHash, err := contextProjectionDomainHash(
		position.LifecycleDefinition,
		"hapax.lifecycle-definition.v1",
		"definition_ref",
		"definition_hash",
	)
	if err != nil || definition.Schema != "hapax.lifecycle-definition.v1" ||
		definition.MayAuthorize || definitionHash != position.LifecycleDefinitionHash ||
		definition.DefinitionHash != position.LifecycleDefinitionHash ||
		definition.DefinitionRef != position.LifecycleDefinitionRef {
		return fmt.Errorf("context lifecycle definition does not bind the position")
	}
	if err := validateContextTexts(position.LegalSuccessors, "legal_successors", true); err != nil {
		return err
	}
	if !sort.StringsAreSorted(position.LegalSuccessors) {
		return fmt.Errorf("context legal successors are not sorted")
	}
	if err := validateContextTexts(position.MutationScopeRefs, "mutation_scope_refs", true); err != nil {
		return err
	}
	if !sort.StringsAreSorted(position.MutationScopeRefs) {
		return fmt.Errorf("context mutation scope refs are not sorted")
	}
	if err := validateContextTexts(position.ReceiptLineage, "receipt_lineage", false); err != nil {
		return err
	}
	flagNames := make([]string, 0, len(position.AuthorizedFlags))
	seenFlags := make(map[string]struct{}, len(position.AuthorizedFlags))
	for _, flag := range position.AuthorizedFlags {
		if err := validateContextText(flag.Name, "authorization flag", false); err != nil {
			return err
		}
		if err := validateContextText(flag.SourceRef, "authorization source", false); err != nil {
			return err
		}
		if _, duplicate := seenFlags[flag.Name]; duplicate {
			return fmt.Errorf("duplicate context authorization flag %s", flag.Name)
		}
		seenFlags[flag.Name] = struct{}{}
		flagNames = append(flagNames, flag.Name)
	}
	if !sort.StringsAreSorted(flagNames) {
		return fmt.Errorf("context authorization flags are not sorted")
	}
	constraintPayload, err := json.Marshal(map[string]any{
		"authority_case":      position.AuthorityCase,
		"authorized_flags":    position.AuthorizedFlags,
		"mutation_scope_refs": position.MutationScopeRefs,
	})
	if err != nil {
		return err
	}
	constraintDigest, err := contextProjectionDomainHash(
		constraintPayload,
		"hapax.effective-constraints.v1",
	)
	if err != nil || constraintDigest != position.EffectiveConstraintDigest {
		return fmt.Errorf("context effective constraint digest is invalid")
	}
	positionPayload, err := json.Marshal(position)
	if err != nil {
		return err
	}
	positionHash, err := contextProjectionDomainHash(
		positionPayload,
		"hapax.context-position.v1",
		"position_ref",
		"position_hash",
	)
	if err != nil || positionHash != position.PositionHash {
		return fmt.Errorf("context position hash does not bind its content")
	}
	return nil
}

func validateContextFact(fact ContextProjectionFact) error {
	if fact.ProjectionKind != "fact" || fact.MayAuthorize || !fact.NoEffect {
		return fmt.Errorf("context fact %s is not a non-authorizing full fact", fact.FactID)
	}
	for label, value := range map[string]string{
		"fact_id":        fact.FactID,
		"fact_type":      fact.FactType,
		"subject_ref":    fact.SubjectRef,
		"meaning":        fact.Meaning,
		"scope_ref":      fact.ScopeRef,
		"temporal_ref":   fact.TemporalRef,
		"resolution_ref": fact.ResolutionRef,
		"derivation_ref": fact.DerivationRef,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	if err := validateContextState(fact.State); err != nil {
		return err
	}
	if err := validateContextTexts(fact.Implications, "fact implications", false); err != nil {
		return err
	}
	if err := validateContextTexts(fact.Proves, "fact proves", true); err != nil {
		return err
	}
	if err := validateContextTexts(fact.DoesNotProve, "fact does_not_prove", false); err != nil {
		return err
	}
	if err := validateContextTexts(fact.BlindSpots, "fact blind_spots", false); err != nil {
		return err
	}
	if !validContextProjectionHash(fact.Data.SHA256) ||
		fmt.Sprintf("%x", sha256.Sum256([]byte(fact.Data.CanonicalJSON))) != fact.Data.SHA256 {
		return fmt.Errorf("context fact %s data hash is invalid", fact.FactID)
	}
	if err := validateContextProvenance(fact.Provenance); err != nil {
		return err
	}
	if err := validateContextText(fact.FreshnessState, "freshness_state", false); err != nil {
		return err
	}
	if err := validateContextText(fact.Confidence.Word, "confidence word", false); err != nil {
		return err
	}
	if err := validateContextText(fact.Confidence.Method, "confidence method", false); err != nil {
		return err
	}
	if err := validateContextTexts(
		fact.Provenance.SourceRefs,
		"provenance source_refs",
		false,
	); err != nil {
		return err
	}
	return nil
}

func validateContextRedactedFact(fact ContextRedactedFact) error {
	if fact.ProjectionKind != "redacted" || fact.MayAuthorize || !fact.NoEffect ||
		fact.State.Value != "dark" {
		return fmt.Errorf("redacted fact %s is not explicitly DARK and non-authorizing", fact.FactID)
	}
	if err := validateContextText(fact.FactID, "redacted fact_id", false); err != nil {
		return err
	}
	return validateContextState(fact.State)
}

func validateContextRedactedObject(object ContextRedactedObject) error {
	if object.MayAuthorize || !object.NoEffect || object.State.Value != "dark" {
		return fmt.Errorf(
			"redacted context object %s is not explicitly DARK and non-authorizing",
			object.ObjectID,
		)
	}
	if err := validateContextEnum(
		object.ObjectKind,
		"redacted object kind",
		"scope",
		"temporal",
		"resolution",
		"source_admission",
		"observation",
		"derivation",
		"relation",
		"action",
		"estimate",
		"lens",
		"constellation",
		"signal",
		"learning_receipt",
		"event",
	); err != nil {
		return err
	}
	if err := validateContextText(object.ObjectID, "redacted object_id", false); err != nil {
		return err
	}
	return validateContextState(object.State)
}

func validateContextProvenance(provenance ContextProjectionProvenance) error {
	for label, value := range map[string]string{
		"provenance kind":              provenance.Kind,
		"provenance authority":         provenance.AuthorityLevel,
		"provenance derivation":        provenance.Derivation,
		"provenance producer":          provenance.ProducerRef,
		"provenance generation":        provenance.Generation,
		"provenance policy generation": provenance.PolicyGeneration,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	observed, err := time.Parse(time.RFC3339, provenance.ObservedAt)
	if err != nil {
		return fmt.Errorf("context provenance observed_at is invalid")
	}
	produced, err := time.Parse(time.RFC3339, provenance.ProducedAt)
	if err != nil {
		return fmt.Errorf("context provenance produced_at is invalid")
	}
	stale, err := time.Parse(time.RFC3339, provenance.StaleAfter)
	if err != nil || observed.After(produced) || produced.After(stale) {
		return fmt.Errorf("context provenance time ordering is invalid")
	}
	return nil
}

func validateContextAction(
	action ContextProjectionAction,
	position ContextProjectionPosition,
	facts map[string]ContextProjectionFact,
) error {
	if action.MayAuthorize || !action.NoEffect || action.PositionRef != position.PositionRef {
		return fmt.Errorf("context action %s is not bound non-authorizing context", action.ActionID)
	}
	if err := validateContextEnum(action.Disposition, "action disposition",
		"legal", "prohibited", "unavailable"); err != nil {
		return err
	}
	if err := validateContextEnum(
		action.ActionClass,
		"action class",
		"inspection",
		"portal_pull",
		"inquiry",
		"counterfactual",
		"lifecycle_operation",
		"lifecycle_transition",
	); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"action_id":               action.ActionID,
		"action label":            action.Label,
		"action class":            action.ActionClass,
		"action operation":        action.Operation,
		"action why":              action.Why,
		"action predicted effect": action.PredictedEffect,
		"action recovery":         action.Recovery,
		"action expected receipt": action.ExpectedReceiptRef,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	if err := validateContextState(action.State); err != nil {
		return err
	}
	if (action.Disposition == "legal") != (action.State.Value == "present") {
		return fmt.Errorf("context action disposition differs from its state")
	}
	for label, value := range map[string]*string{
		"lifecycle operation": action.LifecycleOperation,
		"transition target":   action.TransitionTo,
		"transition edge":     action.TransitionEdge,
		"admission ref":       action.AdmissionRef,
	} {
		if value != nil {
			if err := validateContextText(*value, label, false); err != nil {
				return err
			}
		}
	}
	switch action.ActionClass {
	case "lifecycle_operation":
		if action.LifecycleOperation == nil || action.AdmissionRef == nil ||
			action.Operation != *action.LifecycleOperation ||
			action.TransitionTo != nil || action.TransitionEdge != nil {
			return fmt.Errorf("lifecycle operation action fields are inconsistent")
		}
	case "lifecycle_transition":
		if action.LifecycleOperation != nil || action.AdmissionRef == nil ||
			action.Operation != "lifecycle.transition" ||
			action.TransitionTo == nil || action.TransitionEdge == nil {
			return fmt.Errorf("lifecycle transition action fields are inconsistent")
		}
		if *action.TransitionEdge != "next" && *action.TransitionEdge != "fall" {
			return fmt.Errorf("lifecycle transition edge is invalid")
		}
	default:
		if action.Operation == "lifecycle.transition" {
			return fmt.Errorf("lifecycle transition operation requires its action class")
		}
		if action.LifecycleOperation != nil || action.AdmissionRef != nil ||
			action.TransitionTo != nil || action.TransitionEdge != nil ||
			len(action.GuardEvidence) != 0 {
			return fmt.Errorf("non-lifecycle action carries lifecycle admission fields")
		}
	}
	if err := validateContextLifecycleActionAdmission(action, position); err != nil {
		return err
	}
	if action.ActionClass == "lifecycle_operation" ||
		action.ActionClass == "lifecycle_transition" {
		if len(action.GuardEvidence) == 0 {
			return fmt.Errorf("lifecycle action has no typed guard evidence")
		}
		allSatisfied := true
		lastGuard := ""
		for _, guard := range action.GuardEvidence {
			if guard.MayAuthorize {
				return fmt.Errorf("context guard evidence may authorize")
			}
			if err := validateContextText(guard.Guard, "guard", false); err != nil {
				return err
			}
			if guard.Guard <= lastGuard {
				return fmt.Errorf("context guard evidence is not sorted and unique")
			}
			lastGuard = guard.Guard
			if err := validateContextEnum(
				guard.Disposition,
				"guard disposition",
				"satisfied",
				"unsatisfied",
				"unknown",
			); err != nil {
				return err
			}
			if err := validateContextTexts(guard.EvidenceRefs, "guard evidence_refs", false); err != nil {
				return err
			}
			allSatisfied = allSatisfied && guard.Disposition == "satisfied"
		}
		if (action.Disposition == "legal") != allSatisfied {
			return fmt.Errorf("lifecycle action disposition differs from guard evidence")
		}
	}
	if len(action.SourceFactRefs) == 0 {
		return fmt.Errorf("context action %s has no source facts", action.ActionID)
	}
	for _, ref := range action.SourceFactRefs {
		if _, ok := facts[ref]; !ok {
			return fmt.Errorf("context action %s references an unknown fact", action.ActionID)
		}
	}
	return nil
}

func validateContextLifecycleActionAdmission(
	action ContextProjectionAction,
	position ContextProjectionPosition,
) error {
	var definition struct {
		Stages []contextLifecycleStage `json:"stages"`
	}
	if err := decodeContextJSON(position.LifecycleDefinition, &definition, false); err != nil {
		return err
	}
	var stage *contextLifecycleStage
	for i := range definition.Stages {
		if definition.Stages[i].Token != position.StageToken {
			continue
		}
		if stage != nil {
			return fmt.Errorf("context lifecycle definition repeats the current stage")
		}
		stage = &definition.Stages[i]
	}
	if stage == nil {
		return fmt.Errorf("context lifecycle definition omits the current stage")
	}
	legalSuccessors := make([]string, 0, len(stage.Next)+len(stage.Fall))
	for _, edge := range append(append([]contextLifecycleTransition(nil), stage.Next...), stage.Fall...) {
		legalSuccessors = append(legalSuccessors, edge.To)
	}
	sort.Strings(legalSuccessors)
	legalSuccessors = compactContextStrings(legalSuccessors)
	if !equalContextStrings(legalSuccessors, position.LegalSuccessors) {
		return fmt.Errorf("context lifecycle stage differs from legal successors")
	}

	guardNames := make([]string, 0, len(action.GuardEvidence))
	for _, evidence := range action.GuardEvidence {
		guardNames = append(guardNames, evidence.Guard)
	}
	switch action.ActionClass {
	case "lifecycle_operation":
		var admission *contextLifecycleOperationAdmission
		for i := range stage.OperationAdmissions {
			if stage.OperationAdmissions[i].Operation == action.Operation {
				admission = &stage.OperationAdmissions[i]
				break
			}
		}
		if admission == nil || !equalContextStrings(guardNames, admission.Guards) {
			return fmt.Errorf("context lifecycle action differs from its stage admission")
		}
		body, err := json.Marshal(map[string]any{
			"stage_token": position.StageToken,
			"admission":   admission,
		})
		if err != nil {
			return err
		}
		digest, err := contextProjectionDomainHash(
			body,
			"hapax.lifecycle-operation-admission.v1",
		)
		if err != nil || action.AdmissionRef == nil ||
			*action.AdmissionRef != "lifecycle-operation-admission@sha256:"+digest {
			return fmt.Errorf("context lifecycle operation admission identity is invalid")
		}
	case "lifecycle_transition":
		var transitions []contextLifecycleTransition
		if action.TransitionEdge != nil && *action.TransitionEdge == "fall" {
			transitions = stage.Fall
		} else {
			transitions = stage.Next
		}
		var transition *contextLifecycleTransition
		for i := range transitions {
			if action.TransitionTo != nil && transitions[i].To == *action.TransitionTo {
				transition = &transitions[i]
				break
			}
		}
		if transition == nil || !equalContextStrings(guardNames, transition.Guards) {
			return fmt.Errorf("context lifecycle transition differs from its stage admission")
		}
		body, err := json.Marshal(map[string]any{
			"stage_token":     position.StageToken,
			"transition_edge": *action.TransitionEdge,
			"transition":      transition,
		})
		if err != nil {
			return err
		}
		digest, err := contextProjectionDomainHash(
			body,
			"hapax.lifecycle-transition-admission.v1",
		)
		if err != nil || action.AdmissionRef == nil ||
			*action.AdmissionRef != "lifecycle-transition-admission@sha256:"+digest {
			return fmt.Errorf("context lifecycle transition admission identity is invalid")
		}
	default:
		for _, admission := range stage.OperationAdmissions {
			if admission.Operation == action.Operation {
				return fmt.Errorf("context lifecycle operation requires its action class")
			}
		}
	}
	return nil
}

func compactContextStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

type contextEventTemporal struct {
	TemporalRef    string                 `json:"temporal_ref"`
	EventStart     string                 `json:"event_time_start"`
	ProcessingTime string                 `json:"processing_time"`
	ValidUntil     string                 `json:"valid_until"`
	Completeness   ContextProjectionState `json:"completeness"`
}

type contextEventResolution struct {
	ResolutionRef string `json:"resolution_ref"`
	ScopeRef      string `json:"scope_ref"`
	TemporalRef   string `json:"temporal_ref"`
}

func validateContextProjectionEvents(
	envelope contextProjectionEnvelope,
	descriptor contextDemandShapeDescriptor,
	facts map[string]ContextProjectionFact,
	actions map[string]ContextProjectionAction,
) ([]ContextProjectionEvent, error) {
	if len(envelope.Events) == 0 || len(envelope.Events) > contextProjectionMaxItems {
		return nil, fmt.Errorf("context projection requires a bounded event chronology")
	}
	universe := map[string]struct{}{
		envelope.Position.PositionRef:                              {},
		"demand-shape@sha256:" + descriptor.DemandShapeFingerprint: {},
	}
	states := make(map[string]ContextProjectionState)
	authority := make(map[string]int)
	for _, ref := range envelope.Position.ReceiptLineage {
		universe[ref] = struct{}{}
	}
	for ref, fact := range facts {
		universe[ref] = struct{}{}
		states[ref] = fact.State
		authority[ref] = contextProvenanceAuthorityRank(fact.Provenance.AuthorityLevel)
	}
	for ref, action := range actions {
		universe[ref] = struct{}{}
		states[ref] = action.State
	}

	temporals := make(map[string]contextEventTemporal)
	temporalItems, err := decodeContextRawCollection(envelope.TemporalCoordinates)
	if err != nil {
		return nil, err
	}
	for _, raw := range temporalItems {
		var item contextEventTemporal
		if err := decodeContextJSON(raw, &item, false); err != nil {
			return nil, err
		}
		if item.TemporalRef == "" {
			return nil, fmt.Errorf("context temporal coordinate has no ref")
		}
		if _, duplicate := temporals[item.TemporalRef]; duplicate {
			return nil, fmt.Errorf("duplicate context temporal coordinate")
		}
		if err := validateContextState(item.Completeness); err != nil {
			return nil, err
		}
		for label, value := range map[string]string{
			"event temporal start":      item.EventStart,
			"event temporal processing": item.ProcessingTime,
			"event temporal validity":   item.ValidUntil,
		} {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return nil, fmt.Errorf("%s is invalid", label)
			}
		}
		temporals[item.TemporalRef] = item
		universe[item.TemporalRef] = struct{}{}
		states[item.TemporalRef] = item.Completeness
	}

	resolutions := make(map[string]contextEventResolution)
	resolutionItems, err := decodeContextRawCollection(envelope.ResolutionCoordinates)
	if err != nil {
		return nil, err
	}
	for _, raw := range resolutionItems {
		var item contextEventResolution
		if err := decodeContextJSON(raw, &item, false); err != nil {
			return nil, err
		}
		if item.ResolutionRef == "" || item.ScopeRef == "" || item.TemporalRef == "" {
			return nil, fmt.Errorf("context resolution coordinate is incomplete")
		}
		if _, duplicate := resolutions[item.ResolutionRef]; duplicate {
			return nil, fmt.Errorf("duplicate context resolution coordinate")
		}
		resolutions[item.ResolutionRef] = item
		universe[item.ResolutionRef] = struct{}{}
	}

	collections := []struct {
		payload        json.RawMessage
		refField       string
		stateField     string
		authorityField string
	}{
		{envelope.Scopes, "scope_ref", "", ""},
		{envelope.SourceAdmissions, "admission_ref", "availability", "authority_ceiling"},
		{envelope.Observations, "observation_ref", "state", "authority_ceiling"},
		{envelope.Derivations, "derivation_ref", "state", ""},
		{envelope.Relations, "relation_id", "state", ""},
		{envelope.SignalEstimates, "estimate_ref", "state", ""},
		{envelope.SignalLenses, "lens_ref", "", ""},
		{envelope.SignalConstellations, "constellation_ref", "state", ""},
		{envelope.SignalLearningReceipts, "learning_ref", "state", ""},
	}
	for _, collection := range collections {
		if err := indexContextEventSources(
			collection.payload,
			collection.refField,
			collection.stateField,
			collection.authorityField,
			universe,
			states,
			authority,
		); err != nil {
			return nil, err
		}
	}
	for _, impingement := range envelope.Impingements {
		universe[impingement.ImpingementID] = struct{}{}
		states[impingement.ImpingementID] = impingement.State
	}
	for _, signal := range envelope.OrientingSignals {
		universe[signal.SignalRef] = struct{}{}
		states[signal.SignalRef] = signal.State
	}
	for _, portal := range envelope.PortalOffers {
		universe[portal.PortalRef] = struct{}{}
		states[portal.PortalRef] = portal.State
	}

	events := make([]ContextProjectionEvent, 0, len(envelope.Events))
	byRef := make(map[string]ContextProjectionEvent, len(envelope.Events))
	ids := make(map[string]struct{}, len(envelope.Events))
	for _, raw := range envelope.Events {
		var event ContextProjectionEvent
		if err := decodeContextJSON(raw, &event, true); err != nil {
			return nil, err
		}
		if err := validateContextProjectionEvent(
			event,
			raw,
			envelope,
			descriptor,
			temporals,
			resolutions,
			universe,
			states,
			authority,
			byRef,
		); err != nil {
			return nil, err
		}
		if _, duplicate := byRef[event.EventRef]; duplicate {
			return nil, fmt.Errorf("duplicate context event ref")
		}
		if _, duplicate := ids[event.EventID]; duplicate {
			return nil, fmt.Errorf("duplicate context event id")
		}
		if len(events) > 0 && !contextEventPrecedes(events[len(events)-1], event) {
			return nil, fmt.Errorf("context events are not in canonical causal order")
		}
		ids[event.EventID] = struct{}{}
		byRef[event.EventRef] = event
		events = append(events, event)
	}
	wantLineage := append([]string(nil), envelope.Position.ReceiptLineage...)
	for _, event := range events {
		wantLineage = append(wantLineage, event.EventRef)
	}
	if !equalContextStrings(envelope.LineageRefs, wantLineage) {
		return nil, fmt.Errorf("context event lineage differs from typed chronology")
	}
	return events, nil
}

func decodeContextRawCollection(payload json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := decodeContextJSON(payload, &items, false); err != nil {
		return nil, err
	}
	return items, nil
}

func indexContextEventSources(
	payload json.RawMessage,
	refField string,
	stateField string,
	authorityField string,
	universe map[string]struct{},
	states map[string]ContextProjectionState,
	authority map[string]int,
) error {
	items, err := decodeContextRawCollection(payload)
	if err != nil {
		return err
	}
	for _, raw := range items {
		var object map[string]json.RawMessage
		if err := decodeContextJSON(raw, &object, false); err != nil {
			return err
		}
		var ref string
		if err := json.Unmarshal(object[refField], &ref); err != nil || ref == "" {
			return fmt.Errorf("context object is missing %s", refField)
		}
		if _, duplicate := universe[ref]; duplicate {
			return fmt.Errorf("duplicate context object ref %s", ref)
		}
		universe[ref] = struct{}{}
		if stateField != "" {
			var state ContextProjectionState
			if err := json.Unmarshal(object[stateField], &state); err != nil {
				return fmt.Errorf("context object %s has no state", ref)
			}
			if err := validateContextState(state); err != nil {
				return err
			}
			states[ref] = state
		}
		if authorityField != "" {
			var ceiling string
			if err := json.Unmarshal(object[authorityField], &ceiling); err != nil {
				return fmt.Errorf("context object %s has no authority ceiling", ref)
			}
			rank, ok := contextAuthorityRank(ceiling)
			if !ok {
				return fmt.Errorf("context object %s has an invalid authority ceiling", ref)
			}
			authority[ref] = rank
		}
	}
	return nil
}

var contextEventPayloadFields = map[string][]string{
	"observation_recorded":       {"kind", "observation_ref", "observation_state"},
	"context_fact_derived":       {"derivation_ref", "fact_ref", "kind"},
	"context_frame_materialized": {"frame_ref", "frame_state", "kind"},
	"projection_materialized":    {"kind", "projection_ref", "projection_state"},
	"orienting_signal_offered":   {"kind", "offer_state", "signal_ref"},
	"portal_pull_requested":      {"kind", "portal_ref", "request_state"},
	"portal_consumed":            {"consumption_receipt_ref", "consumption_state", "kind", "portal_ref"},
	"inquiry":                    {"inquiry_ref", "inquiry_state", "kind"},
	"counterfactual":             {"action_ref", "counterfactual_state", "kind"},
	"intent_expressed":           {"action_ref", "intent_kind", "intent_state", "kind"},
	"stipulation_recorded":       {"kind", "stipulation_ref", "stipulation_state"},
	"consent_recorded":           {"consent_ref", "consent_state", "kind"},
	"lease_referenced":           {"kind", "lease_ref", "lease_state"},
	"effect_observed":            {"effect_ref", "kind", "outcome_state"},
	"measurement_updated":        {"kind", "learning_target_ref", "measurement_ref", "measurement_state"},
	"receipt_recorded":           {"kind", "receipt_ref", "receipt_state"},
	"correction":                 {"corrected_ref", "correction_ref", "kind"},
	"supersession":               {"kind", "superseded_ref", "superseding_ref"},
}

func validateContextProjectionEvent(
	event ContextProjectionEvent,
	raw json.RawMessage,
	envelope contextProjectionEnvelope,
	descriptor contextDemandShapeDescriptor,
	temporals map[string]contextEventTemporal,
	resolutions map[string]contextEventResolution,
	universe map[string]struct{},
	states map[string]ContextProjectionState,
	authority map[string]int,
	prior map[string]ContextProjectionEvent,
) error {
	if event.MayAuthorize || event.Generation < 1 || event.DerivationDepth < 0 {
		return fmt.Errorf("context event %s has invalid effect or generation", event.EventID)
	}
	for label, value := range map[string]string{
		"event ref": event.EventRef, "event id": event.EventID, "event kind": event.Kind,
		"event session": event.SessionRef, "event task": event.TaskRef, "event trace": event.TraceRef,
		"event position": event.PositionRef, "event scope": event.ScopeRef,
		"event temporal": event.TemporalRef, "event resolution": event.ResolutionRef,
		"event subject": event.SubjectRef, "event producer": event.ProducerRef,
		"event method": event.MethodRef, "event privacy": event.PrivacyClass,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	if !validContextProjectionHash(event.EventHash) ||
		event.EventRef != "epistemic-event@sha256:"+event.EventHash {
		return fmt.Errorf("context event %s identity is invalid", event.EventID)
	}
	hash, err := contextProjectionDomainHash(
		raw,
		"hapax.epistemic-flow-event.v1",
		"event_ref",
		"event_hash",
	)
	if err != nil || hash != event.EventHash {
		return fmt.Errorf("context event %s hash does not bind its content", event.EventID)
	}
	if event.SessionRef != descriptor.SessionRef || event.TaskRef != envelope.Position.TaskRef ||
		event.PositionRef != envelope.Position.PositionRef || event.PrivacyClass != envelope.Audience {
		return fmt.Errorf("context event %s differs from its audience or position", event.EventID)
	}
	if err := validateContextState(event.State); err != nil {
		return err
	}
	rank, ok := contextAuthorityRank(event.AuthorityCeiling)
	if !ok {
		return fmt.Errorf("context event %s authority ceiling is invalid", event.EventID)
	}
	if err := validateContextTexts(event.SourceRefs, "event source_refs", false); err != nil {
		return err
	}
	if err := validateContextTexts(event.CausedBy, "event caused_by", true); err != nil {
		return err
	}
	if err := validateContextTexts(event.SupersedesRefs, "event supersedes_refs", true); err != nil {
		return err
	}
	for _, refs := range [][]string{event.SourceRefs, event.CausedBy, event.SupersedesRefs} {
		if !sort.StringsAreSorted(refs) {
			return fmt.Errorf("context event %s refs are not sorted", event.EventID)
		}
	}
	temporal, ok := temporals[event.TemporalRef]
	if !ok || temporal.EventStart != event.OccurredAt || temporal.ValidUntil != event.ExpiresAt {
		return fmt.Errorf("context event %s differs from its temporal coordinate", event.EventID)
	}
	resolution, ok := resolutions[event.ResolutionRef]
	if !ok || resolution.ScopeRef != event.ScopeRef || resolution.TemporalRef != event.TemporalRef {
		return fmt.Errorf("context event %s differs from its resolution coordinate", event.EventID)
	}
	if _, err := time.Parse(time.RFC3339, event.OccurredAt); err != nil {
		return fmt.Errorf("context event %s occurrence is invalid", event.EventID)
	}
	if _, err := time.Parse(time.RFC3339, event.ExpiresAt); err != nil || event.OccurredAt > event.ExpiresAt {
		return fmt.Errorf("context event %s expiry is invalid", event.EventID)
	}
	for _, ref := range event.SourceRefs {
		if _, ok := universe[ref]; !ok {
			return fmt.Errorf("context event %s has an unresolved source", event.EventID)
		}
		if event.State.Value == "present" {
			if state, ok := states[ref]; ok && state.Value != "present" {
				return fmt.Errorf("present context event %s has a non-present source", event.EventID)
			}
		}
		if rank > authority[ref] {
			return fmt.Errorf("context event %s authority exceeds its source", event.EventID)
		}
	}
	ancestry := make(map[string]struct{}, len(prior)+len(envelope.Position.ReceiptLineage))
	for ref := range prior {
		ancestry[ref] = struct{}{}
	}
	for _, ref := range envelope.Position.ReceiptLineage {
		ancestry[ref] = struct{}{}
	}
	for _, ref := range append(append([]string(nil), event.CausedBy...), event.SupersedesRefs...) {
		if _, ok := ancestry[ref]; !ok {
			return fmt.Errorf("context event %s ancestry is unresolved or forward-referencing", event.EventID)
		}
		ancestor, local := prior[ref]
		if !local {
			continue
		}
		ancestorRank, _ := contextAuthorityRank(ancestor.AuthorityCeiling)
		ancestorTemporal := temporals[ancestor.TemporalRef]
		if rank > ancestorRank || ancestor.OccurredAt > event.OccurredAt ||
			ancestorTemporal.ProcessingTime > temporal.ProcessingTime ||
			ancestor.Generation > event.Generation {
			return fmt.Errorf("context event %s exceeds or precedes its ancestry", event.EventID)
		}
		if containsContextString(event.CausedBy, ref) {
			if ancestor.DerivationDepth >= event.DerivationDepth ||
				(event.State.Value == "present" && ancestor.State.Value != "present") {
				return fmt.Errorf("context event %s causal binding is invalid", event.EventID)
			}
		}
	}
	return validateContextEventPayload(event)
}

func validateContextEventPayload(event ContextProjectionEvent) error {
	wantFields, ok := contextEventPayloadFields[event.Kind]
	if !ok {
		return fmt.Errorf("context event %s kind is unsupported", event.EventID)
	}
	var payload map[string]json.RawMessage
	if err := decodeContextJSON(event.Payload, &payload, false); err != nil {
		return err
	}
	if len(payload) != len(wantFields) {
		return fmt.Errorf("context event %s payload field set is invalid", event.EventID)
	}
	for _, field := range wantFields {
		raw, present := payload[field]
		if !present {
			return fmt.Errorf("context event %s payload omits %s", event.EventID, field)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("context event %s payload %s is invalid", event.EventID, field)
		}
		if err := validateContextText(value, "event payload", false); err != nil {
			return err
		}
		if field == "kind" && value != event.Kind {
			return fmt.Errorf("context event %s payload kind differs", event.EventID)
		}
	}
	return nil
}

func contextEventPrecedes(left, right ContextProjectionEvent) bool {
	if left.OccurredAt != right.OccurredAt {
		return left.OccurredAt < right.OccurredAt
	}
	if left.Generation != right.Generation {
		return left.Generation < right.Generation
	}
	if left.DerivationDepth != right.DerivationDepth {
		return left.DerivationDepth < right.DerivationDepth
	}
	return left.EventRef < right.EventRef
}

func contextAuthorityRank(value string) (int, bool) {
	switch value {
	case "projection_only":
		return 0, true
	case "observation_only":
		return 1, true
	case "constitutional_evidence":
		return 2, true
	default:
		return 0, false
	}
}

func contextProvenanceAuthorityRank(value string) int {
	switch value {
	case "authoritative":
		return 2
	case "support_non_authoritative":
		return 1
	default:
		return 0
	}
}

func validateFactActionIndexes(
	facts []ContextProjectionFact,
	actions []ContextProjectionAction,
) error {
	for _, fact := range facts {
		var legal, prohibited, receipts []string
		for _, action := range actions {
			if !containsContextString(action.SourceFactRefs, fact.FactID) {
				continue
			}
			if action.Disposition == "legal" {
				legal = append(legal, action.ActionID)
			} else {
				prohibited = append(prohibited, action.ActionID)
			}
			receipts = append(receipts, action.ExpectedReceiptRef)
		}
		sort.Strings(legal)
		sort.Strings(prohibited)
		sort.Strings(receipts)
		if !equalContextStrings(fact.LegalNext, legal) ||
			!equalContextStrings(fact.ProhibitedNext, prohibited) ||
			!equalContextStrings(fact.ExpectedReceiptRefs, receipts) {
			return fmt.Errorf("context fact %s action or receipt index is invalid", fact.FactID)
		}
	}
	return nil
}

func validateContextImpingement(
	impingement ContextProjectionImpingement,
	facts map[string]ContextProjectionFact,
	actions map[string]ContextProjectionAction,
) error {
	if impingement.MayAuthorize {
		return fmt.Errorf("context impingement %s may authorize", impingement.ImpingementID)
	}
	for label, value := range map[string]string{
		"impingement_id":      impingement.ImpingementID,
		"impingement kind":    impingement.Kind,
		"impingement summary": impingement.Summary,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	if err := validateContextState(impingement.State); err != nil {
		return err
	}
	if err := validateContextTexts(
		impingement.SourceFactRefs,
		"impingement source_fact_refs",
		false,
	); err != nil {
		return err
	}
	if err := validateContextTexts(impingement.Protects, "impingement protects", false); err != nil {
		return err
	}
	if err := validateContextTexts(impingement.LegalNext, "impingement legal_next", true); err != nil {
		return err
	}
	for _, ref := range impingement.SourceFactRefs {
		if _, ok := facts[ref]; !ok {
			return fmt.Errorf("context impingement references an unknown fact")
		}
	}
	for _, ref := range impingement.LegalNext {
		if _, ok := actions[ref]; !ok {
			return fmt.Errorf("context impingement references an unknown action")
		}
	}
	return nil
}

func validateContextSignal(
	signal ContextProjectionSignal,
	positionRef string,
	facts map[string]ContextProjectionFact,
	portals map[string]struct{},
) error {
	if signal.MayAuthorize || !signal.NoEffect || signal.PositionRef != positionRef {
		return fmt.Errorf("context signal %s is not position-bound and non-authorizing", signal.SignalID)
	}
	if !validContextProjectionHash(signal.SignalHash) ||
		signal.SignalRef != "orienting-signal@sha256:"+signal.SignalHash {
		return fmt.Errorf("context signal %s identity is invalid", signal.SignalID)
	}
	for label, value := range map[string]string{
		"signal_id":            signal.SignalID,
		"signal kind":          signal.Kind,
		"signal label":         signal.Label,
		"signal why_now":       signal.WhyNow,
		"signal uncertainty":   signal.Uncertainty,
		"signal privacy":       signal.PrivacyClass,
		"signal lens":          signal.LensRef,
		"signal constellation": signal.ConstellationRef,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	if err := validateContextState(signal.State); err != nil {
		return err
	}
	if err := validateContextTexts(signal.DoesNotProve, "signal does_not_prove", false); err != nil {
		return err
	}
	if err := validateContextTexts(signal.SourceFactRefs, "signal source_fact_refs", false); err != nil {
		return err
	}
	for _, ref := range signal.SourceFactRefs {
		if _, ok := facts[ref]; !ok {
			return fmt.Errorf("context signal references an unknown fact")
		}
	}
	if signal.PortalRef != nil {
		if err := validateContextText(*signal.PortalRef, "signal portal", false); err != nil {
			return err
		}
		if _, ok := portals[*signal.PortalRef]; !ok {
			return fmt.Errorf("context signal references an unknown portal")
		}
	}
	return nil
}

func validateContextPortal(
	portal ContextProjectionPortal,
	facts map[string]ContextProjectionFact,
) error {
	if portal.MayAuthorize || !portal.NoEffect {
		return fmt.Errorf("context portal %s is not a no-effect offer", portal.PortalRef)
	}
	for label, value := range map[string]string{
		"portal_ref":     portal.PortalRef,
		"portal kind":    portal.Kind,
		"portal purpose": portal.Purpose,
		"portal privacy": portal.PrivacyClass,
		"portal budget":  portal.BudgetRef,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	if err := validateContextState(portal.State); err != nil {
		return err
	}
	if err := validateContextTexts(portal.SourceFactRefs, "portal source_fact_refs", false); err != nil {
		return err
	}
	if err := validateContextTexts(portal.EffectivityBasis, "portal effectivity_basis", false); err != nil {
		return err
	}
	for _, ref := range portal.SourceFactRefs {
		if _, ok := facts[ref]; !ok {
			return fmt.Errorf("context portal references an unknown fact")
		}
	}
	return nil
}

func contextProjectionRawIsNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func validateContextRawObjectFields(
	raw json.RawMessage,
	label string,
	required []string,
) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := decodeContextJSON(raw, &fields, false); err != nil {
		return nil, err
	}
	if len(fields) != len(required) {
		return nil, fmt.Errorf("%s has an incomplete field set", label)
	}
	for _, field := range required {
		if _, ok := fields[field]; !ok {
			return nil, fmt.Errorf("%s is missing %s", label, field)
		}
	}
	return fields, nil
}

func validateContextOrientationRaw(
	raw json.RawMessage,
	focusRef string,
	positionRef string,
	legal []string,
	prohibited []string,
	receiptLineage []string,
	facts map[string]ContextProjectionFact,
	actions map[string]ContextProjectionAction,
) (*ContextBoundaryOrientation, error) {
	fields, err := validateContextRawObjectFields(
		raw,
		"context orientation",
		[]string{
			"facet_id", "facet_ref", "facet_hash", "focus_ref", "position_ref",
			"boundary_kind", "why_now_refs", "protects", "can", "cannot",
			"until", "iff", "change_authority", "counterfactual", "no_effect",
			"may_authorize",
		},
	)
	if err != nil {
		return nil, err
	}
	if _, err := validateContextRawObjectFields(
		fields["counterfactual"],
		"context orientation counterfactual",
		[]string{"action_id", "predicted_state_delta", "no_effect", "may_authorize"},
	); err != nil {
		return nil, err
	}
	var orientation ContextBoundaryOrientation
	if err := decodeContextJSON(raw, &orientation, true); err != nil {
		return nil, err
	}
	if err := validateContextOrientation(
		orientation,
		focusRef,
		positionRef,
		legal,
		prohibited,
		receiptLineage,
		facts,
		actions,
	); err != nil {
		return nil, err
	}
	digest, err := contextProjectionDomainHash(
		raw,
		"hapax.boundary-orientation-facet.v1",
		"facet_ref",
		"facet_hash",
	)
	if err != nil || digest != orientation.FacetHash {
		return nil, fmt.Errorf("context orientation hash does not bind its content")
	}
	return &orientation, nil
}

func validateContextOrientation(
	orientation ContextBoundaryOrientation,
	focusRef string,
	positionRef string,
	legal []string,
	prohibited []string,
	receiptLineage []string,
	facts map[string]ContextProjectionFact,
	actions map[string]ContextProjectionAction,
) error {
	if orientation.MayAuthorize || !orientation.NoEffect ||
		orientation.Counterfactual.MayAuthorize || !orientation.Counterfactual.NoEffect ||
		orientation.FocusRef != focusRef || orientation.PositionRef != positionRef {
		return fmt.Errorf("context orientation is not a no-effect position-bound facet")
	}
	if !validContextProjectionHash(orientation.FacetHash) ||
		orientation.FacetRef != "boundary-orientation@sha256:"+orientation.FacetHash {
		return fmt.Errorf("context orientation identity is invalid")
	}
	for label, value := range map[string]string{
		"orientation facet_id":              orientation.FacetID,
		"orientation boundary":              orientation.BoundaryKind,
		"orientation change authority":      orientation.ChangeAuthority,
		"orientation counterfactual action": orientation.Counterfactual.ActionID,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
	}
	if err := validateContextTexts(orientation.Can, "orientation can", true); err != nil {
		return err
	}
	if err := validateContextTexts(orientation.Cannot, "orientation cannot", true); err != nil {
		return err
	}
	if err := validateContextTexts(orientation.Until, "orientation until", false); err != nil {
		return err
	}
	if err := validateContextTexts(orientation.IFF, "orientation iff", false); err != nil {
		return err
	}
	if err := validateContextTexts(orientation.Protects, "orientation protects", false); err != nil {
		return err
	}
	if err := validateContextTexts(orientation.WhyNowRefs, "orientation why_now_refs", false); err != nil {
		return err
	}
	if _, ok := actions[orientation.Counterfactual.ActionID]; !ok {
		return fmt.Errorf("context orientation counterfactual references an unknown action")
	}
	for _, ref := range orientation.WhyNowRefs {
		if _, ok := facts[ref]; !ok && !containsContextString(receiptLineage, ref) {
			return fmt.Errorf("context orientation references an unknown fact")
		}
	}
	for _, ref := range orientation.Can {
		if !containsContextString(legal, ref) {
			return fmt.Errorf("context orientation can-list is not legal")
		}
	}
	for _, ref := range orientation.Cannot {
		if !containsContextString(prohibited, ref) {
			return fmt.Errorf("context orientation cannot-list is not prohibited")
		}
	}
	return nil
}

func validateContextLifecyclePossibility(
	raw json.RawMessage,
	legal []string,
	facts map[string]ContextProjectionFact,
) (*ContextLifecyclePossibility, error) {
	if _, err := validateContextRawObjectFields(
		raw,
		"context lifecycle possibility",
		[]string{
			"facet_id", "facet_ref", "facet_hash", "candidate_ref", "source_fact_refs",
			"why_now", "does_not_prove", "uncertainty", "alternative_dispositions",
			"unknown_fields", "candidate_plant", "estimated_cost", "plant_gap",
			"harness_gap", "measurement_gap", "lawful_next", "no_effect",
			"may_authorize",
		},
	); err != nil {
		return nil, err
	}
	var possibility ContextLifecyclePossibility
	if err := decodeContextJSON(raw, &possibility, true); err != nil {
		return nil, err
	}
	if possibility.MayAuthorize || !possibility.NoEffect {
		return nil, fmt.Errorf("context lifecycle possibility is not a no-effect facet")
	}
	for label, value := range map[string]string{
		"lifecycle possibility facet_id":      possibility.FacetID,
		"lifecycle possibility candidate_ref": possibility.CandidateRef,
		"lifecycle possibility why_now":       possibility.WhyNow,
		"lifecycle possibility uncertainty":   possibility.Uncertainty,
	} {
		if err := validateContextText(value, label, false); err != nil {
			return nil, err
		}
	}
	for label, values := range map[string][]string{
		"lifecycle possibility source facts":             possibility.SourceFactRefs,
		"lifecycle possibility does_not_prove":           possibility.DoesNotProve,
		"lifecycle possibility alternative dispositions": possibility.AlternativeDispositions,
		"lifecycle possibility lawful_next":              possibility.LawfulNext,
	} {
		if err := validateContextTexts(values, label, false); err != nil {
			return nil, err
		}
		if !sort.StringsAreSorted(values) {
			return nil, fmt.Errorf("%s is not sorted", label)
		}
	}
	if err := validateContextTexts(
		possibility.UnknownFields,
		"lifecycle possibility unknown fields",
		true,
	); err != nil {
		return nil, err
	}
	if !sort.StringsAreSorted(possibility.UnknownFields) {
		return nil, fmt.Errorf("lifecycle possibility unknown fields are not sorted")
	}
	for _, disposition := range possibility.AlternativeDispositions {
		if err := validateContextEnum(
			disposition,
			"lifecycle possibility alternative disposition",
			"one_shot_task",
			"checklist_or_workflow",
			"lifecycle_candidate",
			"insufficient_evidence",
		); err != nil {
			return nil, err
		}
	}
	if err := validateContextCanonicalObject(
		possibility.CandidatePlant,
		"lifecycle possibility candidate plant",
	); err != nil {
		return nil, err
	}
	if err := validateContextCanonicalObject(
		possibility.EstimatedCost,
		"lifecycle possibility estimated cost",
	); err != nil {
		return nil, err
	}
	for _, state := range []ContextProjectionState{
		possibility.PlantGap,
		possibility.HarnessGap,
		possibility.MeasurementGap,
	} {
		if err := validateContextState(state); err != nil {
			return nil, err
		}
	}
	for _, ref := range possibility.SourceFactRefs {
		if _, ok := facts[ref]; !ok {
			return nil, fmt.Errorf("context lifecycle possibility references an unknown fact")
		}
	}
	for _, actionID := range possibility.LawfulNext {
		if !containsContextString(legal, actionID) {
			return nil, fmt.Errorf("context lifecycle possibility lawful_next is not legal")
		}
	}
	digest, err := contextProjectionDomainHash(
		raw,
		"hapax.lifecycle-possibility-facet.v1",
		"facet_ref",
		"facet_hash",
	)
	if err != nil || digest != possibility.FacetHash ||
		possibility.FacetRef != "lifecycle-possibility@sha256:"+digest {
		return nil, fmt.Errorf("context lifecycle possibility identity is invalid")
	}
	return &possibility, nil
}

func decodeContextLifecycleFSM(
	data ContextCanonicalJSON,
	wantSHA string,
) (ContextLifecycleFSM, error) {
	if data.SHA256 != wantSHA ||
		fmt.Sprintf("%x", sha256.Sum256([]byte(data.CanonicalJSON))) != wantSHA {
		return ContextLifecycleFSM{}, fmt.Errorf("lifecycle_fsm data does not bind the position")
	}
	var payload struct {
		Schema         json.RawMessage `json:"schema"`
		Canon          json.RawMessage `json:"canon"`
		Lifecycle      json.RawMessage `json:"lifecycle"`
		Stage          json.RawMessage `json:"stage"`
		What           string          `json:"what"`
		How            string          `json:"how"`
		Must           string          `json:"must"`
		Kernel         json.RawMessage `json:"kernel"`
		Representation json.RawMessage `json:"representation"`
	}
	if err := decodeContextJSON([]byte(data.CanonicalJSON), &payload, true); err != nil {
		return ContextLifecycleFSM{}, err
	}
	for label, value := range map[string]string{
		"lifecycle WHAT": payload.What,
		"lifecycle HOW":  payload.How,
		"lifecycle MUST": payload.Must,
	} {
		if err := validateContextText(value, label, true); err != nil {
			return ContextLifecycleFSM{}, err
		}
	}
	return ContextLifecycleFSM{What: payload.What, How: payload.How, Must: payload.Must}, nil
}

func validateContextState(state ContextProjectionState) error {
	if err := validateContextEnum(
		state.Value,
		"context state",
		"present", "partial", "absent", "dark", "hold", "stale", "refused", "uncertain",
	); err != nil {
		return err
	}
	if (state.Value == "present") != (len(state.ReasonCodes) == 0) {
		return fmt.Errorf("context state reason contract is invalid")
	}
	if err := validateContextTexts(
		state.ReasonCodes,
		"context state reasons",
		state.Value == "present",
	); err != nil {
		return err
	}
	if !sort.StringsAreSorted(state.ReasonCodes) {
		return fmt.Errorf("context state reasons are not sorted")
	}
	return nil
}

func validateContextEnum(value, label string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s is invalid", label)
}

func validateContextTexts(values []string, label string, allowEmpty bool) error {
	if !allowEmpty && len(values) == 0 {
		return fmt.Errorf("%s may not be empty", label)
	}
	if len(values) > contextProjectionMaxTextItems {
		return fmt.Errorf("%s exceeds the semantic item budget", label)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateContextText(value, label, false); err != nil {
			return err
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s contains a duplicate", label)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateContextText(value, label string, allowMultiline bool) error {
	if value == "" || len(value) > contextProjectionMaxTextBytes ||
		value != strings.TrimSpace(value) {
		return fmt.Errorf("%s is blank or has edge whitespace", label)
	}
	for _, r := range value {
		if unicode.Is(unicode.Cf, r) ||
			unicode.Is(unicode.Zl, r) ||
			unicode.Is(unicode.Zp, r) {
			return fmt.Errorf("%s contains directional or line-format control data", label)
		}
		if !unicode.IsControl(r) {
			continue
		}
		if allowMultiline && (r == '\n' || r == '\t') {
			continue
		}
		return fmt.Errorf("%s contains terminal control data", label)
	}
	return nil
}

func validContextProjectionHash(value string) bool {
	return contextProjectionHashPattern.MatchString(value)
}

func equalContextStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalContextState(left, right ContextProjectionState) bool {
	return left.Value == right.Value &&
		equalContextStrings(left.ReasonCodes, right.ReasonCodes)
}

func containsContextString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// TaskVerbs returns the governed verbs for a task with their SDLC-stage-gated legality. A done task
// offers none; an S7-hold task offers arm/rework. They route through the governed COMMAND surface — the
// cockpit never mints authority; the [v] menu only pre-seeds the preview.
func TaskVerbs(t Task) []TaskVerb {
	pred := strings.ToLower(strings.TrimSpace(t.PredictedStage))
	stage := whoisStageIndex(t.Stage)
	return []TaskVerb{
		{Key: "a", Name: "arm", Legal: stage >= 7 && pred == "hold"},
		{Key: "r", Name: "rework", Legal: pred == "hold"},
		{Key: "f", Name: "refute", Legal: stage >= 5},
		{Key: "c", Name: "close", Legal: pred == "ship"},
		// focus: the operator's attention/prioritization — always legal, wired + operator-attested (the
		// reins frontdoor primitive the spine consumes; not a lifecycle transition).
		{Key: "F", Name: "focus", Legal: true},
	}
}

func whoisVerbDock(w int, t Task, stage int) string {
	segs := []whoisSeg{{"mut", "VERB DOCK: "}}
	for i, v := range TaskVerbs(t) {
		if i > 0 {
			segs = append(segs, whoisSeg{"mut", " "})
		}
		if v.Legal {
			segs = append(segs, whoisSeg{"yel", "[" + v.Key + "]"}, whoisSeg{"pri", " " + v.Name})
		} else {
			segs = append(segs, whoisSeg{"mut", " " + v.Name + " "})
		}
	}
	return whoisLine(w, segs...)
}

func whoisLadderLine(w int, current, prior, predicted int, currentToken string) string {
	segs := []whoisSeg{{"mut", "  "}}
	for i := 0; i <= 11; i++ {
		if i > 0 {
			segs = append(segs, whoisSeg{"border", "─"})
		}
		cell := fmt.Sprintf("S%d", i)
		tok := "2nd"
		if i == prior {
			cell = "◀" + cell
			tok = "mut"
		}
		if i == predicted {
			cell = "→" + cell
			tok = "grn"
		}
		if i == current {
			cell = "【" + cell + "】"
			tok = currentToken
		}
		segs = append(segs, whoisSeg{tok, cell})
	}
	return whoisLine(w, segs...)
}

func whoisStageCaption(t Task, airOn bool, current, renderedCurrent int) whoisSeg {
	if renderedCurrent < 0 {
		return whoisSeg{"mut", "CURRENT STAGE: " + redact(t.AIR, "stage", shortStage(t.Stage), airOn) + " — stage value redacted; ladder frame remains S0 through S11"}
	}
	name := whoisStageMeaning(current)
	pred := whoisPredictedDisplay(t.PredictedStage)
	if !whoisFieldVisible(t, "predicted_stage", airOn) {
		pred = redact(t.AIR, "predicted_stage", pred, airOn)
	}
	return whoisSeg{"2nd", fmt.Sprintf("CURRENT STAGE: %s — %s = %s · predicted: %s", redact(t.AIR, "stage", shortStage(t.Stage), airOn), shortStage(t.Stage), name, pred)}
}

func whoisStageMeaning(stage int) string {
	if m := whoisStageMeanings[stage]; m != "" {
		return m
	}
	return "unknown stage"
}

func whoisPredictedDisplay(pred string) string {
	pred = strings.TrimSpace(pred)
	if pred == "" {
		return "····"
	}
	switch strings.ToLower(pred) {
	case "hold":
		return "→hold"
	case "ship":
		return "·ship"
	}
	return "→" + shortStage(pred)
}

func whoisPredictedToken(pred string) string {
	switch strings.ToLower(strings.TrimSpace(pred)) {
	case "hold":
		return "red"
	case "ship":
		return "grn"
	case "":
		return "mut"
	}
	return "grn"
}

func whoisRedactRel(t Task, airOn bool, val string) string {
	if !airOn {
		return val
	}
	if state, ok := t.AIR["rel_count"]; ok {
		if state == "ok" {
			return val
		}
		return "▒▒▒"
	}
	if state, ok := t.AIR["relations"]; ok {
		if state == "ok" {
			return val
		}
		return "▒▒▒"
	}
	return "▒▒▒"
}

func whoisFieldVisible(t Task, field string, airOn bool) bool {
	return !airOn || t.AIR[field] == "ok"
}

func whoisStageIndex(stage string) int {
	s := strings.TrimSpace(shortStage(stage))
	if s == "" {
		return -1
	}
	if len(s) > 0 && (s[0] == 'S' || s[0] == 's') {
		s = s[1:]
	}
	var digits []rune
	for _, r := range s {
		if !unicode.IsDigit(r) {
			break
		}
		digits = append(digits, r)
	}
	if len(digits) == 0 {
		return -1
	}
	n, err := strconv.Atoi(string(digits))
	if err != nil || n < 0 || n > 11 {
		return -1
	}
	return n
}

func whoisLine(w int, segs ...whoisSeg) string {
	if w <= 0 {
		w = 80
	}
	remaining := w
	var b strings.Builder
	for _, seg := range segs {
		if remaining <= 0 {
			break
		}
		text := whoisClip(seg.text, remaining)
		if text == "" {
			continue
		}
		if seg.token == "" {
			b.WriteString(text)
		} else {
			b.WriteString(C(seg.token, text))
		}
		remaining -= len([]rune(text))
	}
	return b.String()
}

func whoisClip(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
