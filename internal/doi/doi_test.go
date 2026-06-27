package doi

import "testing"

func TestImportanceIsMultiplicative(t *testing.T) {
	s := NewSalience()
	if s.Importance() != 1.0 {
		t.Fatalf("neutral salience should be 1.0, got %v", s.Importance())
	}
	s.Severity = 0.5
	s.Freshness = 0.5
	if got := s.Importance(); got != 0.25 {
		t.Fatalf("expected 0.25, got %v", got)
	}
	s.Classification = 0 // classified irrelevant -> earns no cells
	if s.Importance() != 0 {
		t.Fatal("a zero factor must annihilate importance")
	}
}

func TestDOISubtractsDistance(t *testing.T) {
	if DOI(0.9, 0.2) != 0.7 {
		t.Fatalf("DOI = importance - distance")
	}
	// nearer-to-focus (smaller distance) ranks higher
	if DOI(0.5, 0.0) <= DOI(0.5, 0.3) {
		t.Fatal("nearer to focus must score higher")
	}
}

func TestFoldRanksAndBudgets(t *testing.T) {
	items := []Scored{
		{"a", 0.1}, {"b", 0.9}, {"c", 0.5}, {"d", 0.8}, {"e", 0.2},
	}
	placements, aggregated := Fold(items, 3, 0.7)
	if aggregated != 2 {
		t.Fatalf("5 items, budget 3 -> 2 aggregated, got %d", aggregated)
	}
	// visible in DOI order: b(0.9), d(0.8), c(0.5)
	wantOrder := []string{"b", "d", "c"}
	for i, p := range placements {
		if p.ID != wantOrder[i] {
			t.Fatalf("placement %d: want %s got %s (must be DOI-ranked)", i, wantOrder[i], p.ID)
		}
	}
	// b,d >= 0.7 -> Full ; c < 0.7 -> Collapsed
	if placements[0].Tier != Full || placements[1].Tier != Full {
		t.Fatal("DOI >= fullThreshold must render Full")
	}
	if placements[2].Tier != Collapsed {
		t.Fatal("DOI < fullThreshold must render Collapsed")
	}
}

func TestFoldZeroBudgetAggregatesAll(t *testing.T) {
	items := []Scored{{"a", 0.9}, {"b", 0.5}}
	placements, aggregated := Fold(items, 0, 0.5)
	if len(placements) != 0 || aggregated != 2 {
		t.Fatalf("budget 0 -> all aggregated, got %d visible / %d aggregated", len(placements), aggregated)
	}
}

func TestFoldStableOnTies(t *testing.T) {
	items := []Scored{{"x", 0.5}, {"y", 0.5}, {"z", 0.5}}
	placements, _ := Fold(items, 3, 0.4)
	for i, want := range []string{"x", "y", "z"} {
		if placements[i].ID != want {
			t.Fatalf("ties must keep input order, got %s at %d", placements[i].ID, i)
		}
	}
}
