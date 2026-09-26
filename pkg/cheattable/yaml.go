package cheattable

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format identifies a cheat-table serialization.
type Format int

const (
	// FormatCT is Firstspark's XML/Cheat Engine document.
	FormatCT Format = iota
	// FormatJSON is the JSON session form.
	FormatJSON
	// FormatYAML is the YAML session form.
	FormatYAML
)

// FormatFor returns the format implied by a file extension, defaulting to .CT.
func FormatFor(path string) Format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return FormatJSON
	case ".yaml", ".yml":
		return FormatYAML
	default:
		return FormatCT
	}
}

// LoadAny reads a table, choosing the format from the file extension.
func LoadAny(path string) (*Table, error) {
	switch FormatFor(path) {
	case FormatJSON:
		return ImportJSON(path)
	case FormatYAML:
		return ImportYAML(path)
	default:
		return Load(path)
	}
}

// SaveAs writes the table in the format implied by the extension.
func (t *Table) SaveAs(path string) error {
	switch FormatFor(path) {
	case FormatJSON:
		return t.ExportJSON(path)
	case FormatYAML:
		return t.ExportYAML(path)
	default:
		return t.Save(path)
	}
}

// MarshalYAML renders the table as YAML.
func (t *Table) MarshalYAML() ([]byte, error) {
	data, err := yaml.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("cheattable: marshal yaml: %w", err)
	}
	return data, nil
}

// ExportYAML writes the table as YAML.
func (t *Table) ExportYAML(path string) error {
	data, err := t.MarshalYAML()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("cheattable: write %s: %w", path, err)
	}
	return nil
}

// ParseYAML parses a YAML table.
func ParseYAML(data []byte) (*Table, error) {
	var t Table
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("cheattable: parse yaml: %w", err)
	}
	return &t, nil
}

// ImportYAML reads a YAML table.
func ImportYAML(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cheattable: read %s: %w", path, err)
	}
	return ParseYAML(data)
}
