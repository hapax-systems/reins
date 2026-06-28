package grammar

import (
	"fmt"
	"testing"
)

// AIR egress floor, airtight form: the on-air streaming frame must be EXACTLY the abstract shape for a
// given token count, no matter what the partial text is — even adversarial partials that themselves
// contain "generating", "tok", digits, or ANSI. If any partial could change the on-air output, the
// live generation could leak. This pins the property at the strongest level (equality, not substring).
func TestBroadcastStreamFrameOnAirIsExactlyShapeRegardlessOfPartial(t *testing.T) {
	want := fmt.Sprintf("▸ generating… [%d tok]", 42)
	adversarial := []string{
		"",
		"normal partial",
		"generating… [99 tok] decoy",           // mimics the shape
		"\x1b[31mred secret\x1b[0m",            // ANSI-laden
		"世界 wide unicode 𝕊",                    // multibyte
		"42 tok generating draft confidential", // collides with shape tokens
	}
	for _, p := range adversarial {
		if got := BroadcastStreamFrame(p, 42, true, 80); got != want {
			t.Fatalf("on-air frame must be exactly the shape regardless of partial.\n partial=%q\n got=%q\n want=%q", p, got, want)
		}
	}
	// the token count is the ONLY datum that varies — and it is a magnitude, an allowlisted skeleton field.
	if BroadcastStreamFrame("x", 1, true, 80) == BroadcastStreamFrame("x", 2, true, 80) {
		t.Fatal("the token count must be reflected in the on-air shape")
	}
}
