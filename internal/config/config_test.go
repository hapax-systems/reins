package config

import "testing"

func TestLoadReadsInstanceValues(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/c.toml"
	if err := writeFile(p, "api_url='http://x:9'\ncouncil_root='/c'\nledger_path='/l'\npalette='solarized'\nair_allowlist=['kind','subject']\n"); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.APIURL != "http://x:9" || c.Palette != "solarized" || len(c.AIRAllowlist) != 2 || c.CouncilRoot != "/c" {
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
	c, err := Load("/no/such/file.toml") // no file -> defaults, then env overrides
	if err != nil {
		t.Fatal(err)
	}
	if c.APIURL != "http://env:7" || c.CouncilRoot != "/env/root" {
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
