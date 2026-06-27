package grammar

import (
	"fmt"
	"regexp"
	"strings"
)

// The injection core: the egress-safe model + composer for the Reins filebrowser->chat feature
// (research brief reins-design-ref, §1.3/§2). It deliberately
// reuses the package's Redact interlock — a file injection is "exactly a new raw value path", and
// existing wards leak precisely when given a parallel renderer. Two posture rules are load-bearing:
//   1. REFERENCE BY DEFAULT — a file/image part carries path + bounded head + token cost + source
//      AIR provenance, never the raw bytes; promotion (byte materialization) is a downstream,
//      explicit, governed send-confirm — never the Go write-path.
//   2. ALWAYS-GATED EGRESS — the composer never marks anything send-eligible and always shows the
//      default-deny posture + the not-wired governed-gate footer; on air, path and body redact.

// PartKind is the kind of an outbound chat-message part.
type PartKind int

const (
	PartText PartKind = iota
	PartFileRef
	PartImageRef
)

func (k PartKind) String() string {
	switch k {
	case PartFileRef:
		return "file"
	case PartImageRef:
		return "image"
	default:
		return "text"
	}
}

// ChatPart is one piece of an outbound chat message. A file/image part is a REFERENCE
// (Promoted=false) until the governed send gate promotes it; bytes are not held here.
type ChatPart struct {
	Kind     PartKind
	Text     string            // text body, or a file's bounded head preview
	Path     string            // SENSITIVE — redacted on air
	MIME     string            // images
	Bytes    int               // file size (cost/legibility; not the content)
	TokenEst int               // estimated token cost of this part
	AIR      map[string]string // source AIR provenance; nil/empty => default-deny (all fields redact on air)
	Promoted bool              // true ONLY after an explicit send-confirm; gates byte materialization
}

// EstimateTokens approximates token cost at ~4 characters per token (ceil).
func EstimateTokens(s string) int {
	n := len([]rune(s))
	return (n + 3) / 4
}

// TextPart builds an inline text part.
func TextPart(s string) ChatPart {
	return ChatPart{Kind: PartText, Text: s, TokenEst: EstimateTokens(s)}
}

// FileRef builds a non-promoted file reference: path + bounded head + size, token-estimated from
// the head (a reference, not a copy). airProv carries the source AIR provenance (nil => default-deny).
func FileRef(path, head string, bytes int, airProv map[string]string) ChatPart {
	return ChatPart{Kind: PartFileRef, Path: path, Text: head, Bytes: bytes, TokenEst: EstimateTokens(head), AIR: airProv}
}

// ImageRef builds a non-promoted image reference. Pixels never count as text tokens until the part
// is promoted and base64-encoded at the send gate; the estimate is a flat vision-tile placeholder.
func ImageRef(path, mime string, bytes int, airProv map[string]string) ChatPart {
	return ChatPart{Kind: PartImageRef, Path: path, MIME: mime, Bytes: bytes, TokenEst: 85, AIR: airProv}
}

// TotalTokens sums the estimated cost of a basket of parts.
func TotalTokens(parts []ChatPart) int {
	n := 0
	for _, p := range parts {
		n += p.TokenEst
	}
	return n
}

var secretPatterns = []struct {
	label string
	re    *regexp.Regexp
}{
	{"private key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"AWS key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"slack token", regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]{10,}`)},
	{"api key / secret", regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|passwd|bearer)\s*[:=]\s*\S{6,}`)},
	{"bearer token", regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`)},
}

// ScanSecrets returns the distinct labels of secret-like content found in s. It is PURE — it never
// mutates s. Detected secrets are SURFACED to inform the operator's send-confirm, never
// auto-cleaned-and-shipped (auto-strip is itself a leak class: it ships a "redacted" payload the
// operator never reviewed).
func ScanSecrets(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range secretPatterns {
		if p.re.MatchString(s) && !seen[p.label] {
			seen[p.label] = true
			out = append(out, p.label)
		}
	}
	return out
}

func injBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func injClip(s string, n int) string {
	r := []rune(strings.ReplaceAll(s, "\n", "⏎"))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

// RenderInjectionComposer renders the egress-safe hold-buffer: the present-at-hand preview of
// EXACTLY what an injection would carry, the default-deny posture, and the not-wired governed-gate
// footer. On air, the path and file/image body redact through the same Redact interlock the rows
// use (no parallel raw-value path); detected secrets are surfaced off-air only and never stripped.
// The renderer NEVER marks anything send-eligible — promotion is an explicit downstream confirm.
func RenderInjectionComposer(parts []ChatPart, airOn bool) string {
	var b strings.Builder
	b.WriteString(C("brt", "INJECTION HOLD-BUFFER") +
		C("mut", fmt.Sprintf(" — %d part(s) · ~%d tok · DEFAULT-DENY", len(parts), TotalTokens(parts))) + "\n")
	if len(parts) == 0 {
		b.WriteString(C("mut", "  (basket empty — nothing staged)") + "\n")
	}
	for i, p := range parts {
		switch p.Kind {
		case PartText:
			// A text part's body is content too: redact it on air through the SAME interlock as
			// file/image bodies — a pasted secret or path in chat text is exactly the leak the
			// file branch guards against, and must not ship verbatim because it is "just text".
			body := Redact(p.AIR, "body", injClip(p.Text, 48), airOn)
			b.WriteString(fmt.Sprintf("  %d %s ~%dtok · %s\n", i+1, C("2nd", "text"), p.TokenEst, body))
			b.WriteString(secretWarning(p.Text, airOn))
		default: // PartFileRef, PartImageRef
			path := Redact(p.AIR, "path", p.Path, airOn) // filesystem path is SENSITIVE — denied on air
			body := Redact(p.AIR, "body", bodyPreview(p), airOn)
			ref := C("2nd", "ref")
			if p.Promoted {
				ref = C("red", "PROMOTED")
			}
			b.WriteString(fmt.Sprintf("  %d %s[%s] %s · %s · ~%dtok · %s\n",
				i+1, C("yel", p.Kind.String()), ref, path, injBytes(p.Bytes), p.TokenEst, body))
			b.WriteString(secretWarning(p.Text, airOn))
		}
	}
	b.WriteString(C("mut", "  ── egress is always-gated (separate from execution trust) ──") + "\n")
	b.WriteString(C("org", "  NOT WIRED — provider send requires the governed CapabilityIO SESSION gate (explicit confirm; no Enter-sends)"))
	return b.String()
}

// secretWarning surfaces detected secrets — but ONLY in the operator's off-air present-at-hand
// frame (the air frame is shape-only and must not even hint where a secret sits). Pure: it scans,
// it never strips (auto-strip ships an unreviewed "redacted" payload, itself a leak class).
func secretWarning(text string, airOn bool) string {
	if airOn {
		return ""
	}
	if found := ScanSecrets(text); len(found) > 0 {
		return "    " + C("red", "⚠ secret detected: "+strings.Join(found, ", ")+
			" — surfaced, NOT stripped; review before any send") + "\n"
	}
	return ""
}

func bodyPreview(p ChatPart) string {
	if p.Kind == PartImageRef {
		return p.MIME + " (pixels egress on send)"
	}
	return injClip(p.Text, 40)
}
