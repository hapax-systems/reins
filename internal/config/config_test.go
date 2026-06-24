package config

import "testing"

func TestLoadReadsInstanceValues(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/c.toml"
	if err := writeFile(p, "api_url='http://x:9'\ncouncil_root='/c'\nledger_path='/l'\npalette='gruvbox'\nair_allowlist=['kind','subject']\n"); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.APIURL != "http://x:9" || c.Palette != "gruvbox" || len(c.AIRAllowlist) != 2 {
		t.Fatalf("bad config: %+v", c)
	}
}

func TestLoadMissingFileIsClearError(t *testing.T) {
	if _, err := Load("/no/such/file.toml"); err == nil {
		t.Fatal("expected error for missing config")
	}
}
