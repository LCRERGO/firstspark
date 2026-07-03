// Package cheattable imports and exports Cheat Engine .CT files. The .CT
// format is XML; this package implements the subset needed to round-trip a
// flat list of addresses and values. Sessions are also saved as JSON.
package cheattable

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
)

// Entry is a single cheat table entry.
type Entry struct {
	ID          int    `xml:"ID,attr" json:"id"`
	Description string `xml:"Description,attr" json:"description"`
	Address     string `xml:"Address,attr" json:"address"`
	Type        string `xml:"Type,attr" json:"type"`
	Value       string `xml:",chardata" json:"value"`
}

// Table is a flat list of cheat entries.
type Table struct {
	XMLName xml.Name `xml:"CheatTable" json:"-"`
	Entries []Entry  `xml:"CheatEntries>CheatEntry" json:"entries"`
}

// Add appends an entry, assigning the next ID.
func (t *Table) Add(description, address, typ, value string) {
	id := 1
	if len(t.Entries) > 0 {
		id = t.Entries[len(t.Entries)-1].ID + 1
	}
	t.Entries = append(t.Entries, Entry{
		ID:          id,
		Description: description,
		Address:     address,
		Type:        typ,
		Value:       value,
	})
}

// Load parses a .CT file.
func Load(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cheattable: read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses .CT XML.
func Parse(data []byte) (*Table, error) {
	var t Table
	if err := xml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("cheattable: parse xml: %w", err)
	}
	return &t, nil
}

// Save writes the table as a .CT XML file.
func (t *Table) Save(path string) error {
	data, err := t.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("cheattable: write %s: %w", path, err)
	}
	return nil
}

// Marshal renders the table as .CT XML.
func (t *Table) Marshal() ([]byte, error) {
	data, err := xml.MarshalIndent(t, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cheattable: marshal xml: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

// ExportJSON writes the table as JSON.
func (t *Table) ExportJSON(path string) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("cheattable: marshal json: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("cheattable: write %s: %w", path, err)
	}
	return nil
}

// ImportJSON reads a JSON table.
func ImportJSON(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cheattable: read %s: %w", path, err)
	}
	var t Table
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("cheattable: parse json: %w", err)
	}
	return &t, nil
}

// AddressValue parses an entry address as a hexadecimal number.
func (e Entry) AddressValue() (uint64, error) {
	s := e.Address
	if len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X") {
		s = s[2:]
	}
	return strconv.ParseUint(s, 16, 64)
}
