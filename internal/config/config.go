package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	APIURL       string   `toml:"api_url"`
	CouncilRoot  string   `toml:"council_root"`
	LedgerPath   string   `toml:"ledger_path"`
	Palette      string   `toml:"palette"`
	AIRAllowlist []string `toml:"air_allowlist"`
}

func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("reins: cannot load config %q: %w (set REINS_CONFIG to a valid instance config)", path, err)
	}
	return &c, nil
}

func writeFile(p, body string) error { return os.WriteFile(p, []byte(body), 0o600) }
