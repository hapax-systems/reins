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
	APIURL       string   `toml:"api_url"`
	CouncilRoot  string   `toml:"council_root"`  // READ API: the substrate root (folded by the API, not the cockpit)
	LedgerPath   string   `toml:"ledger_path"`   // READ API: the live coord ledger
	Palette      string   `toml:"palette"`       // gruvbox (R&D) | solarized (Research)
	AIRAllowlist []string `toml:"air_allowlist"` // READ API: on-air default-deny — fields NOT listed render ▒▒▒
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
	if v := os.Getenv("REINS_PALETTE"); v != "" {
		c.Palette = v
	}
	if v := os.Getenv("REINS_AIR_ALLOWLIST"); v != "" {
		c.AIRAllowlist = strings.Split(v, ",")
	}
}

func writeFile(p, body string) error { return os.WriteFile(p, []byte(body), 0o600) }
