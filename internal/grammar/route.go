package grammar

// route.go — the ROUTE projection types (U5). Reins RENDERS the spine's routing evidence (U4's
// /route/posture + /route/candidates); it mints no decision and no display scalar. These types mirror
// the honest read-fold shapes exactly — a decision is always the spine's `NO SPINE DECISION ON FILE`
// until echoed, the reqvec range is the producer contract (never sample-inferred), and a candidate's
// demand vector is either a COMPLETE measured 8-dim vector or ABSENT (never a null-dict).

// RouteSource is one evidence feed and its honest live/dark state.
type RouteSource struct {
	Name   string `json:"name"`
	State  string `json:"state"` // "live" | "dark"
	Events int    `json:"events"`
}

// RouteKeyspace is the frozen-11 coverage: which routing_classes were observed vs the pinned anchor,
// and any observed class OUTSIDE the eleven (a keyspace-drift signal, surfaced not absorbed).
type RouteKeyspace struct {
	Pinned          []string `json:"pinned"`
	PinnedCount     int      `json:"pinned_count"`
	Observed        []string `json:"observed"`
	ObservedCount   int      `json:"observed_count"`
	UnknownObserved []string `json:"unknown_observed"`
}

// RouteReqvec is the requirement_vector contract: the 8 dims + the PRODUCER-declared range (0..5),
// never inferred from the sample.
type RouteReqvec struct {
	Dims        []string `json:"dims"`
	Min         int      `json:"min"`
	Max         int      `json:"max"`
	RangeSource string   `json:"range_source"`
}

// RoutePosture is the /route/posture projection. Decision is always NO SPINE DECISION ON FILE until the
// spine echoes one — reins never mints a band/floor/decision.
type RoutePosture struct {
	Dark     bool          `json:"dark"`
	Error    string        `json:"error"`
	Decision string        `json:"decision"`
	Keyspace RouteKeyspace `json:"keyspace"`
	Reqvec   RouteReqvec   `json:"reqvec"`
	Sources  []RouteSource `json:"sources"`
}

// RouteCandidate is one routing_class's measured DEMAND evidence. DispatchReqvec is populated ONLY when
// a complete measured vector exists (ReqvecMeasured=true); otherwise the render says ABSENT — a null
// dispatch_reqvec is never fabricated into zeros. MeasuredEvents is a structural admission count, not a
// ranking scalar.
type RouteCandidate struct {
	RoutingClass   string
	InKeyspace     bool
	MeasuredEvents int
	DispatchReqvec map[string]int
	ReqvecMeasured bool
}
