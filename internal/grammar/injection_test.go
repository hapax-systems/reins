package grammar

import (
	"strings"
	"testing"
)

// The injection core is the egress-safe foundation for the filebrowser->chat feature
// (reins-design-ref). Files/images are REFERENCES by default,
// bytes never materialize on the Go write-path, the path/body redact on air through the SAME
// Redact interlock the rows use, secrets are surfaced (never stripped), and nothing the renderer
// produces is send-eligible — provider send is a downstream governed gate.

func TestPartsAreReferencesByDefault(t *testing.T) {
	f := FileRef("/srv/secret/config.toml", "head preview", 2048, nil)
	if f.Promoted {
		t.Fatal("a fresh file ref must NOT be promoted (bytes never materialize at mark time)")
	}
	if f.Kind != PartFileRef {
		t.Fatalf("kind = %v", f.Kind)
	}
	img := ImageRef("/srv/diagram.png", "image/png", 50000, nil)
	if img.Promoted || img.Kind != PartImageRef {
		t.Fatal("a fresh image ref must be an unpromoted image part")
	}
}

func TestEstimateTokensMonotonic(t *testing.T) {
	small := EstimateTokens("hello")
	big := EstimateTokens(strings.Repeat("hello world ", 100))
	if !(big > small && small > 0) {
		t.Fatalf("token estimate must grow with content: small=%d big=%d", small, big)
	}
	parts := []ChatPart{TextPart("abc"), FileRef("/p", "a longer head here", 10, nil)}
	if TotalTokens(parts) != parts[0].TokenEst+parts[1].TokenEst {
		t.Fatal("TotalTokens must sum the parts")
	}
}

func TestScanSecretsSurfacesNeverStrips(t *testing.T) {
	body := "port = 8080\napi_key = sk-abcdef0123456789\nok = true"
	found := ScanSecrets(body)
	if len(found) == 0 {
		t.Fatal("an api_key=... line must be detected")
	}
	// ScanSecrets is pure — it must not mutate or return a cleaned body.
	if !strings.Contains(body, "sk-abcdef0123456789") {
		t.Fatal("ScanSecrets must NOT strip the secret from the source")
	}
	if len(ScanSecrets("nothing sensitive here")) != 0 {
		t.Fatal("clean text yields no findings")
	}
}

func TestComposerRedactsPathAndBodyOnAir(t *testing.T) {
	parts := []ChatPart{
		TextPart("please review"),
		FileRef("/srv/secret/creds.toml", "api_key = sk-DEADBEEF01234567", 1024, nil),
	}
	// Off air (operator present-at-hand frame): path + secret surface to inform the confirm.
	off := RenderInjectionComposer(parts, false)
	if !strings.Contains(off, "creds.toml") {
		t.Fatalf("off-air composer should show the path for the operator:\n%s", off)
	}
	if !strings.Contains(off, "secret detected") {
		t.Fatalf("off-air composer must surface the detected secret:\n%s", off)
	}
	// On air (livestreamed frame): the path NEVER appears; body is shape-only (default-deny).
	on := RenderInjectionComposer(parts, true)
	if strings.Contains(on, "creds.toml") || strings.Contains(on, "secret") || strings.Contains(on, "sk-DEADBEEF") {
		t.Fatalf("on-air composer must redact path/body/secret entirely:\n%s", on)
	}
	if !strings.Contains(on, "▒▒▒") {
		t.Fatalf("on-air composer must show the redaction token:\n%s", on)
	}
}

func TestComposerTextPartIsEgressSafe(t *testing.T) {
	// GLM-via-CC review (2026-06-27) caught the PartText branch leaking on air — a text part is
	// content too: on air it must redact, off air it must surface secrets (never strip).
	parts := []ChatPart{TextPart("api_key = sk-DEADBEEF01234567 please ship")}
	off := RenderInjectionComposer(parts, false)
	if !strings.Contains(off, "secret detected") {
		t.Fatalf("off-air must surface a secret pasted into a TEXT part:\n%s", off)
	}
	on := RenderInjectionComposer(parts, true)
	if strings.Contains(on, "sk-DEADBEEF") || strings.Contains(on, "api_key") || strings.Contains(on, "secret") {
		t.Fatalf("on-air must redact a TEXT part's body — no verbatim leak:\n%s", on)
	}
	if !strings.Contains(on, "▒▒▒") {
		t.Fatalf("on-air text body must show the redaction token:\n%s", on)
	}
}

func TestComposerNeverMarksSendEligible(t *testing.T) {
	out := RenderInjectionComposer([]ChatPart{FileRef("/p/f.go", "package x", 10, nil)}, false)
	if !strings.Contains(out, "NOT WIRED") {
		t.Fatalf("composer must always carry the not-wired governed-gate footer:\n%s", out)
	}
	if !strings.Contains(strings.ToUpper(out), "DEFAULT-DENY") {
		t.Fatalf("composer must state the default-deny posture:\n%s", out)
	}
	// Empty basket is honest, not blank.
	if strings.TrimSpace(RenderInjectionComposer(nil, false)) == "" {
		t.Fatal("an empty basket must still render an honest hold-buffer header")
	}
}
