package grammar

// Counts and status flags use pointers: absent/null is unknown, never an observed zero.
// ObligationsSummary is the aggregate count rollup from /read/obligations.
type ObligationsSummary struct {
	TotalOwes                 *int `json:"total_owes"`
	LanebusOwesCount          *int `json:"lanebus_owes_count"`
	GrantOwesCount            *int `json:"grant_owes_count"`
	CustodyOwesCount          *int `json:"custody_owes_count"`
	LocusAnomalyCount         *int `json:"locus_anomaly_count"`
	DarkSourcesCount          *int `json:"dark_sources_count"`
	ActionCount               *int `json:"action_count"`
	WarningCount              *int `json:"warning_count"`
	RoutesTotal               *int `json:"routes_total"`
	RoutesOccupiedInMemory    *int `json:"routes_occupied_in_memory"`
	LocalLociOccupiedInMemory *int `json:"local_loci_occupied_in_memory"`
	LocalLociTotal            *int `json:"local_loci_total"`
}

// ObligationRow is one unresolved ligatory obligation (creditor <- debtor owes subject).
type ObligationRow struct {
	Kind      string `json:"kind"`
	Relation  string `json:"relation"`
	Creditor  string `json:"creditor"`
	Debtor    string `json:"debtor"`
	Subject   string `json:"subject"`
	Count     *int   `json:"count"`
	State     string `json:"state"`
	Attention string `json:"attention"`
}

// LocusAnomalyRow is a substrate or source anomaly (dark host, lost endpoint, unexplained occupancy).
type LocusAnomalyRow struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	State     string `json:"state"`
	Subject   string `json:"subject"`
	Attention string `json:"attention"`
}

// SubstrateLocusRow is a local compute locus with memory occupancy vs. disk residency and topology.
type SubstrateLocusRow struct {
	ID                  string   `json:"id"`
	Host                string   `json:"host"`
	Port                string   `json:"port"`
	State               string   `json:"state"`
	LoadedModels        []string `json:"loaded_models"`
	ResidentModelsCount *int     `json:"resident_models_count"`
	AdmittedBy          []string `json:"admitted_by"`
	Topology            string   `json:"topology"`
}

// CapabilityFulfillmentRow joins a capability route to its fulfilling memory/disk loci.
type CapabilityFulfillmentRow struct {
	RouteID          string   `json:"route_id"`
	CapabilityID     string   `json:"capability_id"`
	Mode             string   `json:"mode"`
	ModelID          string   `json:"model_id"`
	RouteState       string   `json:"route_state"`
	FulfillmentState string   `json:"fulfillment_state"`
	OccupiedLoci     []string `json:"occupied_loci"`
	ResidentLoci     []string `json:"resident_loci"`
	Topology         string   `json:"topology"`
}

// ObligationsReport is the full /read/obligations read-only projection.
type ObligationsReport struct {
	Dark                  *bool                      `json:"dark"`
	PartialDark           *bool                      `json:"partial_dark"`
	SourceStatus          map[string]string          `json:"source_status"`
	Summary               ObligationsSummary         `json:"summary"`
	LanebusOwes           []ObligationRow            `json:"lanebus_owes"`
	GrantOwes             []ObligationRow            `json:"grant_owes"`
	CustodyOwes           []ObligationRow            `json:"custody_owes"`
	LocusAnomalies        []LocusAnomalyRow          `json:"locus_anomalies"`
	LocalSubstrateLoci    []SubstrateLocusRow        `json:"local_substrate_loci"`
	CapabilityFulfillment []CapabilityFulfillmentRow `json:"capability_fulfillment"`
}
