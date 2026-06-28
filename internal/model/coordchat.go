package model

import "strings"

const (
	CoordChatPartText  = "text"
	CoordChatPartFile  = "file"
	CoordChatPartImage = "image"
)

// CoordChatPart is one structured piece of coordinator-chat context. FilePath is sensitive:
// renderers must AIR-redact it before it reaches the screen on-air.
type CoordChatPart struct {
	Type     string // "text" | "file" | "image"
	Text     string
	FilePath string // SENSITIVE — AIR-redact on render
	MimeType string
	AIRProv  string // provenance: operator | model
}

// CoordChatMessage is the structured local coordinator-chat model. Egress is intentionally still
// stubbed; this only captures renderable multimodal intent for later filebrowser injection.
type CoordChatMessage struct {
	Role  string
	Parts []CoordChatPart
}

// AppendOperatorText preserves the old flat-string behavior as one operator text part.
func (m Model) AppendOperatorText(msg string) Model {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return m
	}
	return m.AppendParts("operator", []CoordChatPart{{Type: CoordChatPartText, Text: msg, AIRProv: "operator"}})
}

// AppendParts appends one structured local coordinator-chat message. It performs only capture-time
// normalization; provider egress remains gated/stubbed elsewhere.
func (m Model) AppendParts(role string, parts []CoordChatPart) Model {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "operator"
	}
	clean := make([]CoordChatPart, 0, len(parts))
	for _, p := range parts {
		p.Type = strings.ToLower(strings.TrimSpace(p.Type))
		if p.Type == "" {
			if strings.TrimSpace(p.FilePath) != "" {
				p.Type = CoordChatPartFile
			} else {
				p.Type = CoordChatPartText
			}
		}
		p.AIRProv = strings.TrimSpace(p.AIRProv)
		if p.AIRProv == "" {
			p.AIRProv = role
		}
		if coordChatPartEmpty(p) {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return m
	}
	m.CoordChatLog = append(m.CoordChatLog, CoordChatMessage{Role: role, Parts: clean})
	return m
}

func coordChatPartEmpty(p CoordChatPart) bool {
	switch p.Type {
	case CoordChatPartFile, CoordChatPartImage:
		return strings.TrimSpace(p.FilePath) == "" && strings.TrimSpace(p.Text) == ""
	default:
		return strings.TrimSpace(p.Text) == "" && strings.TrimSpace(p.FilePath) == ""
	}
}
