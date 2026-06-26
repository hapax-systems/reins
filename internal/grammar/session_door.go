package grammar

import (
	"fmt"
	"strings"
)

// RenderSessionDoor renders the present-at-hand detail card for one live lane/session. It is still
// roster-only: no transcript tail, no terminal attach, no stdin/stdout bridge.
func RenderSessionDoor(s Session, detail SessionDetail, hasDetail, detailDark bool, detailErr string, airOn bool, w, h int) string {
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}

	val := func(field, raw string) string { return redact(s.AIR, field, raw, airOn) }
	lineVal := func(field, raw string) string {
		v := val(field, raw)
		if strings.TrimSpace(v) == "" {
			return "·"
		}
		return v
	}
	stateTok := sessionToken(s.State)
	if !sessionHealthVisible(s, airOn) {
		stateTok = "mut"
	}
	rdyTok := readinessToken(s.Readiness)
	if airOn && s.AIR["readiness"] != "ok" {
		rdyTok = "mut"
	}

	var lines []string
	add := func(segs ...whoisSeg) { lines = append(lines, whoisLine(w, segs...)) }
	blank := func() { lines = append(lines, "") }
	rule := func() { add(whoisSeg{"border", strings.Repeat("─", w)}) }

	add(
		whoisSeg{"brt", "◆ " + lineVal("role", s.Role)},
		whoisSeg{"2nd", "  " + lineVal("platform", s.Platform)},
		whoisSeg{stateTok, "  " + lineVal("state", s.State)},
	)
	add(whoisSeg{"mut", "DOOR /session — present-at-hand lane card; roster facts only."})
	blank()

	add(whoisSeg{"mut", "HEALTH — the resume question before any action"})
	add(whoisSeg{"mut", "  readiness  : "}, whoisSeg{rdyTok, lineVal("readiness", s.Readiness)})
	add(whoisSeg{"mut", "  state       : "}, whoisSeg{stateTok, sessionGlyph(s, airOn) + " " + lineVal("state", s.State)})
	add(whoisSeg{"mut", "  blocker     : "}, whoisSeg{"pri", lineVal("blocker", s.Blocker)})
	add(whoisSeg{"mut", "  attention   : "}, whoisSeg{"pri", lineVal("attention", compactAttention(s.Attention))})
	add(whoisSeg{"mut", "  output age  : "}, whoisSeg{"pri", lineVal("output_age_s", fmt.Sprintf("%.1fs", s.OutputAgeS))})
	add(whoisSeg{"mut", "  relay age   : "}, whoisSeg{"pri", lineVal("relay_age_s", fmt.Sprintf("%.1fs", s.RelayAgeS))})
	blank()

	add(whoisSeg{"mut", "IDENTITY — stable public role first; operational handles may redact"})
	add(whoisSeg{"mut", "  role        : "}, whoisSeg{"pri", lineVal("role", s.Role)})
	add(whoisSeg{"mut", "  tmux        : "}, whoisSeg{"2nd", lineVal("session", s.Session)})
	add(whoisSeg{"mut", "  claimed task: "}, whoisSeg{"2nd", lineVal("claimed_task", s.ClaimedTask)})
	blank()

	add(whoisSeg{"mut", "ROUTE BINDING — policy selection is not launch confirmation"})
	add(whoisSeg{"mut", "  route       : "}, whoisSeg{"pri", lineVal("route_id", s.RouteID)})
	add(whoisSeg{"mut", "  binding     : "}, whoisSeg{"pri", lineVal("route_binding_state", s.RouteBindingState)})
	add(whoisSeg{"mut", "  mode/profile: "}, whoisSeg{"2nd", lineVal("mode", s.RouteMode) + "/" + lineVal("profile", s.RouteProfile)})
	add(whoisSeg{"mut", "  evidence    : "}, whoisSeg{"2nd", lineVal("route_evidence_ref", s.RouteEvidenceRef)})
	blank()

	add(whoisSeg{"mut", "RESUME CONTRACT"})
	add(whoisSeg{"grn", "  ✓ "}, whoisSeg{"mut", "Reins can identify the lane and stage a governed resume intent."})
	if strings.TrimSpace(s.Blocker) != "" && s.Blocker != "none" {
		add(whoisSeg{"red", "  × "}, whoisSeg{"mut", "Visible cutover blocker: "}, whoisSeg{"pri", lineVal("blocker", s.Blocker)})
	}
	add(whoisSeg{"red", "  × "}, whoisSeg{"mut", "This slice does not read transcripts, attach terminals, or send input."})
	add(whoisSeg{"red", "  × "}, whoisSeg{"mut", "No command dispatch path is wired from this door."})
	rule()

	if !hasDetail && !detailDark {
		add(whoisSeg{"mut", "DETAIL — loading structured resume context…"})
		blank()
	} else if detailDark {
		msg := "DETAIL — dark; no fabricated resume context"
		if strings.TrimSpace(detailErr) != "" {
			if airOn {
				msg += " · reason hidden on AIR"
			} else {
				msg += " · " + detailErr
			}
		}
		add(whoisSeg{"mut", msg})
		blank()
	} else {
		add(whoisSeg{"mut", "TASK CONTEXT — frontmatter only; no body text"})
		add(whoisSeg{"mut", "  task        : "}, whoisSeg{"2nd", sessionDetailVal(detail, "task_id", detail.Task.TaskID, airOn)})
		add(whoisSeg{"mut", "  status      : "}, whoisSeg{"pri", sessionDetailVal(detail, "status", detail.Task.Status, airOn)})
		add(whoisSeg{"mut", "  case        : "}, whoisSeg{"2nd", sessionDetailVal(detail, "authority_case", detail.Task.AuthorityCase, airOn)})
		add(whoisSeg{"mut", "  parent spec : "}, whoisSeg{"2nd", sessionDetailVal(detail, "parent_spec", detail.Task.ParentSpec, airOn)})
		blank()

		add(whoisSeg{"mut", "EVIDENCE REFS — metadata only; raw_access=false"})
		add(whoisSeg{"mut", "  count       : "}, whoisSeg{"pri", fmt.Sprintf("%d", len(detail.EvidenceRefs))})
		for i, ref := range detail.EvidenceRefs {
			if i >= 3 {
				add(whoisSeg{"mut", fmt.Sprintf("  … %d more", len(detail.EvidenceRefs)-i)})
				break
			}
			path := sessionDetailVal(detail, "path", ref.Path, airOn)
			add(whoisSeg{"mut", fmt.Sprintf("  %-11s: ", ref.Kind)}, whoisSeg{"2nd", path}, whoisSeg{"mut", fmt.Sprintf("  %dB", ref.Size)})
		}
		blank()
	}

	dock := []string{
		whoisLine(w, whoisSeg{"mut", "VERB DOCK: "}, whoisSeg{"yel", "[r]"}, whoisSeg{"pri", " resume-intent"}, whoisSeg{"mut", "  unavailable: attach · compose · transcript"}),
		whoisLine(w, whoisSeg{"mut", "[Esc]/[Enter]/[q] back · [r] reports the governed COMMAND route only"}),
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

func sessionDetailVal(d SessionDetail, field, raw string, airOn bool) string {
	if airOn && d.AIR[field] != "ok" {
		return "▒▒▒"
	}
	if strings.TrimSpace(raw) == "" {
		return "·"
	}
	return raw
}
