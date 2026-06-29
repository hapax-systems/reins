package grammar

// ObserveDimension is one whole-system signal (health/drift/nudges/agents/governance/consent/profile/
// cost/gpu/stimmung) for the :observe page — served by /read/observe. Status is "live" or "dark"
// (per-dimension honest-dark); Count is nil when the source is dark or has no count (never fabricated).
type ObserveDimension struct {
	Key     string `json:"key"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Count   *int   `json:"count"`
}
