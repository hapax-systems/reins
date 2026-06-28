package grammar

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
)

// BroadcastStreamFrame renders a streaming turn's broadcast frame under Reins's two-frame doctrine.
// Off air (air=false): the present-at-hand frame — '▸ ' + the live partial text truncated to maxw cells.
// On air (air=true): it MUST disclose ONLY the abstract streaming SHAPE '▸ generating… [N tok]' with the
// token count, and NEVER any character of the partial text (the AIR egress floor for live generation).
func BroadcastStreamFrame(partial string, tokens int, air bool, maxw int) string {
	if air {
		return fmt.Sprintf("▸ generating… [%d tok]", tokens)
	}

	return ansi.Truncate("▸ "+partial, maxw, "")
}
