package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the ONE instance contract, read by BOTH halves: the cockpit consumes APIURL; the READ
// API (api/reins_read.py) reads the SAME file for CouncilRoot/LedgerPath/AIRAllowlist. Single source
// of truth — the launcher points both at it via $REINS_CONFIG.
type Config struct {
	APIURL                     string   `toml:"api_url"`
	CouncilRoot                string   `toml:"council_root"`                  // READ API: the substrate root (folded by the API, not the cockpit)
	LedgerPath                 string   `toml:"ledger_path"`                   // READ API: the live coord ledger
	CCTasksActive              string   `toml:"cc_tasks_active"`               // READ API: optional active task-note root for session detail refs
	SessionTranscriptRoots     []string `toml:"session_transcript_roots"`      // READ API: optional metadata-only transcript roots
	RequestIntakeState         string   `toml:"request_intake_state"`          // READ API: optional durable request-intake snapshot
	PlanningFeedState          string   `toml:"planning_feed_state"`           // READ API: optional request planning-feed snapshot
	P0IncidentState            string   `toml:"p0_incident_state"`             // READ API: optional coalesced P0 incident state
	P0IncidentEvents           string   `toml:"p0_incident_events"`            // READ API: optional P0 incident JSONL ledger
	SecuritySignalState        string   `toml:"security_signal_state"`         // READ API: optional security signal intake snapshot
	OrchestrationLedgerDir     string   `toml:"orchestration_ledger_dir"`      // READ API: optional dispatch/route-decision ledger directory
	LifecycleRegistryPaths     []string `toml:"lifecycle_registry_paths"`      // READ API: optional source-backed SDLC/RDLC/n-DLC lifecycle contracts
	DomainPackPaths            []string `toml:"domain_pack_paths"`             // READ API: optional source-backed SDLC/RDLC/n-DLC domain packs
	CapabilitySurfacePackPaths []string `toml:"capability_surface_pack_paths"` // READ API: optional capability-surface discovery packs
	HKPShadowRoot              string   `toml:"hkp_shadow_root"`               // READ API: optional HKP cache-only shadow bundle root
	HKPIndexRoot               string   `toml:"hkp_index_root"`                // READ API: optional HKP derived index root
	HKPReportRoot              string   `toml:"hkp_report_root"`               // READ API: optional HKP support report root
	HKPBundles                 []string `toml:"hkp_bundles"`                   // READ API: optional HKP bundle IDs to summarize
	Palette                    string   `toml:"palette"`                       // gruvbox (R&D) | solarized (Research)
	AIRAllowlist               []string `toml:"air_allowlist"`                 // READ API: on-air default-deny — fields NOT listed render ▒▒▒
}

// Defaults are NEUTRAL — zero instance/operator paths. A fresh binary runs zero-config against a
// local API on the default port; an instance config (or env) supplies the substrate paths. The
// default allowlist is the SAFE on-air set (structural fields only — see the on-air note in README).
func Defaults() Config {
	return Config{
		APIURL:  "http://127.0.0.1:8799",
		Palette: "gruvbox",
		AIRAllowlist: []string{
			"kind", "score", "ts", "task_id", "stage", "no_go",
			"id", "layer", "status", "source", "target", "relation", "res",
			"role", "platform", "state", "alive", "idle", "stalled", "output_age_s", "relay_age_s",
			"readiness", "blocker", "attention",
			"evidence_count", "resume_ready",
			"evidence_summary", "by_kind", "transcript_roots_observed", "transcript_roots_missing", "truncated",
			"count", "age_bucket", "coverage", "task_link_state", "severity", "privacy", "raw_access", "exists",
			"capability_id", "capability_class", "surface_family", "spend_model", "egress_class", "receipt_requirement",
			"route_count", "ok_count", "blocked_count", "hkp_posture", "source_refs", "source_ref_labels",
			"route_id", "mode", "profile", "model_id", "effort", "context_mode", "fast_mode", "quantization", "capacity_pool", "demand_vector", "hardening", "eval_plane", "review_obligation", "learning_eligibility", "benchmark_coverage", "fixed_overhead", "route_state", "authority_ceiling",
			"freshness_ok", "quota_state", "receipt_count", "blockers", "authority",
			"route_binding_state",
			"tool_id", "available", "authority_use", "observed_at", "stale_after",
			"schema_version", "row_id", "family", "subject_kind", "subject_ref", "posture", "map_kind", "map_id", "map_source", "map_target", "map_relation",
			"gate_id", "domain", "evidence", "missing", "action",
			"detail", "generated_at", "package_hash", "default_lens",
			"domain_id", "lifecycle", "terrain", "depth", "scope", "claim_ceiling", "windows", "surfaces", "parity", "source_refs",
			"lifecycle_id", "owner", "plant", "posture", "maturity", "adapter_id", "claim_surface", "mutation_surface", "dark_policy", "freshness_policy", "air_class", "commands", "receipt_contracts", "next_evidence",
		},
	}
}

// Load resolves instance config with precedence env > file > defaults. A MISSING file is NOT an
// error (zero-config): the cockpit always launches and renders honest-dark absent a substrate —
// never a crash. Only a MALFORMED file fails (executive-function: the error is actionable).
func Load(path string) (*Config, error) {
	c := Defaults()
	if _, statErr := os.Stat(path); statErr == nil {
		if _, err := toml.DecodeFile(path, &c); err != nil {
			return nil, fmt.Errorf("reins: config %q is malformed (fix the TOML, or remove it to run on defaults): %w", path, err)
		}
	}
	applyEnv(&c)
	return &c, nil
}

// applyEnv lets any field be overridden without a config file (env > file). Mirrors the API side.
func applyEnv(c *Config) {
	if v := os.Getenv("REINS_API_URL"); v != "" {
		c.APIURL = v
	}
	if v := os.Getenv("REINS_COUNCIL_ROOT"); v != "" {
		c.CouncilRoot = v
	}
	if v := os.Getenv("REINS_LEDGER_PATH"); v != "" {
		c.LedgerPath = v
	}
	if v := os.Getenv("REINS_CC_TASKS_ACTIVE"); v != "" {
		c.CCTasksActive = v
	}
	if v := os.Getenv("REINS_SESSION_TRANSCRIPT_ROOTS"); v != "" {
		c.SessionTranscriptRoots = strings.Split(v, string(os.PathListSeparator))
	}
	if v := os.Getenv("REINS_REQUEST_INTAKE_STATE"); v != "" {
		c.RequestIntakeState = v
	}
	if v := os.Getenv("REINS_PLANNING_FEED_STATE"); v != "" {
		c.PlanningFeedState = v
	}
	if v := os.Getenv("REINS_P0_INCIDENT_STATE"); v != "" {
		c.P0IncidentState = v
	}
	if v := os.Getenv("REINS_P0_INCIDENT_EVENTS"); v != "" {
		c.P0IncidentEvents = v
	}
	if v := os.Getenv("REINS_SECURITY_SIGNAL_STATE"); v != "" {
		c.SecuritySignalState = v
	}
	if v := os.Getenv("REINS_ORCHESTRATION_LEDGER_DIR"); v != "" {
		c.OrchestrationLedgerDir = v
	}
	if v := os.Getenv("REINS_LIFECYCLE_REGISTRIES"); v != "" {
		c.LifecycleRegistryPaths = strings.Split(v, string(os.PathListSeparator))
	}
	if v := os.Getenv("REINS_DOMAIN_PACKS"); v != "" {
		c.DomainPackPaths = strings.Split(v, string(os.PathListSeparator))
	}
	if v := os.Getenv("REINS_CAPABILITY_SURFACE_PACKS"); v != "" {
		c.CapabilitySurfacePackPaths = strings.Split(v, string(os.PathListSeparator))
	}
	if v := os.Getenv("REINS_HKP_SHADOW_ROOT"); v != "" {
		c.HKPShadowRoot = v
	}
	if v := os.Getenv("REINS_HKP_INDEX_ROOT"); v != "" {
		c.HKPIndexRoot = v
	}
	if v := os.Getenv("REINS_HKP_REPORT_ROOT"); v != "" {
		c.HKPReportRoot = v
	}
	if v := os.Getenv("REINS_HKP_BUNDLES"); v != "" {
		parts := strings.Split(v, ",")
		c.HKPBundles = c.HKPBundles[:0]
		for _, part := range parts {
			if s := strings.TrimSpace(part); s != "" {
				c.HKPBundles = append(c.HKPBundles, s)
			}
		}
	}
	if v := os.Getenv("REINS_PALETTE"); v != "" {
		c.Palette = v
	}
	if v := os.Getenv("REINS_AIR_ALLOWLIST"); v != "" {
		c.AIRAllowlist = strings.Split(v, ",")
	}
}

func writeFile(p, body string) error { return os.WriteFile(p, []byte(body), 0o600) }
