package grammar

import (
	_ "embed"
	"fmt"

	"github.com/BurntSushi/toml"
)

//go:embed formats.toml
var formatsTOML string

// FieldSpec is one auditable row-grammar field. Width 0 marks the single greedy body
// field for a row kind; Fill marks the BitchX-style %| expansion point; EmptyDots
// preserves structured silence by rendering empty cells as dots.
type FieldSpec struct {
	Name      string `toml:"name"`
	Width     int    `toml:"width"`
	Align     string `toml:"align"`
	Fill      bool   `toml:"fill"`
	EmptyDots bool   `toml:"empty_dots"`
}

// FormatSpec is the ordered field grammar for one row kind.
type FormatSpec struct {
	Fields []FieldSpec `toml:"field"`
}

// FormatTable is the embedded, versioned row-grammar table keyed by row kind.
type FormatTable struct {
	Version string
	Formats map[string]FormatSpec
}

type rawFormatTable struct {
	Version string     `toml:"version"`
	Event   FormatSpec `toml:"event"`
	Task    FormatSpec `toml:"task"`
	Trace   FormatSpec `toml:"trace"`
	Session FormatSpec `toml:"session"`
	Turn    FormatSpec `toml:"turn"`
}

// LoadFormats returns the package-embedded row-grammar table.
func LoadFormats() FormatTable {
	var raw rawFormatTable
	if _, err := toml.Decode(formatsTOML, &raw); err != nil {
		panic(fmt.Errorf("reins: embedded grammar formats.toml is malformed: %w", err))
	}
	return FormatTable{
		Version: raw.Version,
		Formats: map[string]FormatSpec{
			"event":   raw.Event,
			"task":    raw.Task,
			"trace":   raw.Trace,
			"session": raw.Session,
			"turn":    raw.Turn,
		},
	}
}
