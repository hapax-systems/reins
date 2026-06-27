package grammar

import "testing"

// Reactions (GLM-lane-generated, verified here): parse the /on syntax, match case-insensitive
// substrings, honor the * wildcard, error on missing braces, and fire the matching set.
func TestParseReactionAndMatch(t *testing.T) {
	r, err := ParseReaction("/on review.fail #blocked { flash }")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.EventKind != "review.fail" || r.Match != "#blocked" || r.Effect != "flash" {
		t.Fatalf("parsed wrong: %+v", r)
	}
	if !r.Matches("REVIEW.FAIL.gate", "task #blocked now") {
		t.Fatalf("should match by case-insensitive substring on kind+text")
	}
	if r.Matches("pr.merged", "task #blocked") {
		t.Fatalf("a kind mismatch must not match")
	}
	any, err := ParseReaction("/on * * { ntfy }")
	if err != nil || !any.Matches("anything", "whatever") {
		t.Fatalf("wildcard * must match any event (err=%v)", err)
	}
	if _, err := ParseReaction("/on x y no braces here"); err == nil {
		t.Fatalf("missing braces must error")
	}
	if got := (ReactionSet{r, any}).Fired("review.fail", "#blocked"); len(got) != 2 {
		t.Fatalf("both armed reactions should fire on a matching event, got %d", len(got))
	}
}
