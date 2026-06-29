package grammar

// VaultNote is one Obsidian vault note's METADATA — served by /read/vault. Bodies are default-deny
// (never fetched); only the title/path/link/folder/mtime cross the wire. The vault is operator-private
// life-planning (LDLC air_class "private-life"), so the :vault surface SEALS the list on air.
type VaultNote struct {
	Title       string `json:"title"`
	RelPath     string `json:"rel_path"`
	ObsidianURI string `json:"obsidian_uri"`
	Folder      string `json:"folder"`
	Modified    string `json:"modified"`
}
