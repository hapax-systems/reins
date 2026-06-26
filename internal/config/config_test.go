package config

import "testing"

var expectedDefaultAIRAllowlist = []string{
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
}

func TestLoadReadsInstanceValues(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/c.toml"
	if err := writeFile(p, "api_url='http://x:9'\ncouncil_root='/c'\nledger_path='/l'\nlifecycle_registry_paths=['/lc1']\ndomain_pack_paths=['/d1','/d2']\ncapability_surface_pack_paths=['/c1']\nhkp_shadow_root='/hkp-shadow'\nhkp_index_root='/hkp-index'\nhkp_report_root='/hkp-reports'\nhkp_bundles=['sdlc']\npalette='solarized'\nair_allowlist=['kind','subject']\n"); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.APIURL != "http://x:9" || c.Palette != "solarized" || len(c.AIRAllowlist) != 2 || c.CouncilRoot != "/c" || len(c.LifecycleRegistryPaths) != 1 || len(c.DomainPackPaths) != 2 || len(c.CapabilitySurfacePackPaths) != 1 || c.HKPShadowRoot != "/hkp-shadow" || c.HKPIndexRoot != "/hkp-index" || c.HKPReportRoot != "/hkp-reports" || len(c.HKPBundles) != 1 {
		t.Fatalf("bad config: %+v", c)
	}
}

func TestLoadMissingFileFallsBackToDefaults(t *testing.T) {
	c, err := Load("/no/such/file.toml")
	if err != nil {
		t.Fatalf("missing file must be zero-config (defaults), not an error: %v", err)
	}
	if c.APIURL != Defaults().APIURL || c.Palette != "gruvbox" {
		t.Fatalf("missing file should yield defaults: %+v", c)
	}
}

func TestDefaultAIRAllowlistIsConservativeStructuralSet(t *testing.T) {
	c := Defaults()
	if len(c.AIRAllowlist) != len(expectedDefaultAIRAllowlist) {
		t.Fatalf("default allowlist length drifted: got %v want %v", c.AIRAllowlist, expectedDefaultAIRAllowlist)
	}
	for i, want := range expectedDefaultAIRAllowlist {
		if c.AIRAllowlist[i] != want {
			t.Fatalf("default allowlist drift at %d: got %q want %q (%v)", i, c.AIRAllowlist[i], want, c.AIRAllowlist)
		}
	}
}

func TestPartialFileMergesOntoDefaults(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/c.toml"
	if err := writeFile(p, "api_url='http://only:1'\n"); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIURL != "http://only:1" {
		t.Fatalf("file value should win: %+v", c)
	}
	if c.Palette != "gruvbox" || len(c.AIRAllowlist) == 0 {
		t.Fatalf("absent keys should keep defaults: %+v", c)
	}
}

func TestEnvOverridesFileAndDefaults(t *testing.T) {
	t.Setenv("REINS_API_URL", "http://env:7")
	t.Setenv("REINS_COUNCIL_ROOT", "/env/root")
	t.Setenv("REINS_ORCHESTRATION_LEDGER_DIR", "/env/orch")
	t.Setenv("REINS_LIFECYCLE_REGISTRIES", "/env/lc1:/env/lc2")
	t.Setenv("REINS_DOMAIN_PACKS", "/env/dom1:/env/dom2")
	t.Setenv("REINS_CAPABILITY_SURFACE_PACKS", "/env/cap1:/env/cap2")
	t.Setenv("REINS_HKP_SHADOW_ROOT", "/env/hkp-shadow")
	t.Setenv("REINS_HKP_INDEX_ROOT", "/env/hkp-index")
	t.Setenv("REINS_HKP_REPORT_ROOT", "/env/hkp-reports")
	t.Setenv("REINS_HKP_BUNDLES", "sdlc,rdlc")
	c, err := Load("/no/such/file.toml") // no file -> defaults, then env overrides
	if err != nil {
		t.Fatal(err)
	}
	if c.APIURL != "http://env:7" || c.CouncilRoot != "/env/root" || c.OrchestrationLedgerDir != "/env/orch" || len(c.LifecycleRegistryPaths) != 2 || len(c.DomainPackPaths) != 2 || len(c.CapabilitySurfacePackPaths) != 2 || c.HKPShadowRoot != "/env/hkp-shadow" || c.HKPIndexRoot != "/env/hkp-index" || c.HKPReportRoot != "/env/hkp-reports" || len(c.HKPBundles) != 2 {
		t.Fatalf("env must override: %+v", c)
	}
}

func TestMalformedFileIsActionableError(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/c.toml"
	if err := writeFile(p, "api_url = = not valid toml ["); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("a malformed config must be a clear error, not silently ignored")
	}
}
