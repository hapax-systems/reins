package grammar

import (
	"fmt"
	"strconv"
	"strings"
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

// TaskVerbs returns the governed verbs for a task with their SDLC-stage-gated legality. A done task
// offers none; an S7-hold task offers arm/rework. They route through the governed COMMAND surface — the
// cockpit never mints authority; the [v] menu only pre-seeds the preview.
// ContextAffordance is one classification→affordance→WHY entry from the tri-audience /read/context
// operator-cockpit projection: an affordance reins OFFERS on a subject and its honest state. Readout only
// (the never-injector doctrine) — the cockpit SHOWS it; action routes through the governed apply seam.
type ContextAffordance struct {
	Subject string `json:"subject_ref"`
	Kind    string `json:"affordance_kind"`
	State   string `json:"state"`
}

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
