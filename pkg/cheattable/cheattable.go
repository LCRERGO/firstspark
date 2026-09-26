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
	"strings"
)

// SchemaVersion identifies the firstspark cheat-table schema revision.
const SchemaVersion = "2"

// Entry is a single cheat table entry.
type Entry struct {
	ID          int    `xml:"ID,attr" json:"id"`
	Description string `xml:"Description,attr" json:"description"`
	Address     string `xml:"Address,attr" json:"address"`
	Type        string `xml:"Type,attr" json:"type"`
	Value       string `xml:",chardata" json:"value"`
	// Hotkey is the freeze toggle key (F1..F12 or a letter).
	Hotkey string `xml:"Hotkey,attr,omitempty" json:"hotkey,omitempty"`
	// Display is the value format: "decimal", "hex" or "binary".
	Display string `xml:"Display,attr,omitempty" json:"display,omitempty"`
	// ShowAsSigned renders integer values as signed instead of unsigned.
	ShowAsSigned bool `xml:"ShowAsSigned,attr,omitempty" json:"show_as_signed,omitempty"`
	// Frozen marks a locked value.
	Frozen bool `xml:"Frozen,attr,omitempty" json:"frozen,omitempty"`
	// Encoding is the string encoding for string types (e.g. "utf16le").
	Encoding string `xml:"Encoding,attr,omitempty" json:"encoding,omitempty"`
	// Pointer is a pointer chain, see FormatPointerChain.
	Pointer string `xml:"Pointer,attr,omitempty" json:"pointer,omitempty"`
	// BitSize, BitOffset, BitWidth and BitSigned describe a bitfield entry.
	BitSize   int  `xml:"BitSize,attr,omitempty" json:"bit_size,omitempty"`
	BitOffset int  `xml:"BitOffset,attr,omitempty" json:"bit_offset,omitempty"`
	BitWidth  int  `xml:"BitWidth,attr,omitempty" json:"bit_width,omitempty"`
	BitSigned bool `xml:"BitSigned,attr,omitempty" json:"bit_signed,omitempty"`
	// Group marks a group header: it has no address or value and only parents
	// children in the tree.
	Group bool `xml:"Group,attr,omitempty" json:"group,omitempty"`
	// Expr is an unresolved Cheat Engine address expression (symbolic or
	// parent-relative); Address is left empty when Expr is set.
	Expr string `xml:"Expr,attr,omitempty" json:"expr,omitempty"`
	// Offsets is the raw Cheat Engine offset list (comma-separated) that
	// applies after Expr resolves.
	Offsets string `xml:"Offsets,attr,omitempty" json:"offsets,omitempty"`
	// Script is the Auto Assembler source of a script record. It is written as
	// a child element and is not executed on import (ADR 0039).
	Script string `xml:"Script" json:"script,omitempty"`
	// Children are the nested records of a group or script.
	Children []Entry `xml:"CheatEntries>CheatEntry,omitempty" json:"children,omitempty"`
}

// MarshalXML writes an entry without emitting an empty <CheatEntries> wrapper.
// A plain struct with a chardata field and a nested slice would always emit the
// wrapper, which corrupts the value on re-parse.
func (e Entry) MarshalXML(enc *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{Name: xml.Name{Local: "CheatEntry"}}
	set := func(name, value string) {
		if value != "" {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: value})
		}
	}
	boolean := func(name string, v bool) {
		if v {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: "true"})
		}
	}
	integer := func(name string, v int) {
		if v != 0 {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: strconv.Itoa(v)})
		}
	}
	integer("ID", e.ID)
	set("Description", e.Description)
	set("Address", e.Address)
	set("Type", e.Type)
	set("Hotkey", e.Hotkey)
	set("Display", e.Display)
	boolean("Frozen", e.Frozen)
	set("Encoding", e.Encoding)
	set("Pointer", e.Pointer)
	integer("BitSize", e.BitSize)
	integer("BitOffset", e.BitOffset)
	integer("BitWidth", e.BitWidth)
	boolean("BitSigned", e.BitSigned)
	boolean("Group", e.Group)
	set("Expr", e.Expr)
	set("Offsets", e.Offsets)

	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	if e.Value != "" {
		if err := enc.EncodeToken(xml.CharData(e.Value)); err != nil {
			return err
		}
	}
	if e.Script != "" {
		se := xml.StartElement{Name: xml.Name{Local: "Script"}}
		if err := enc.EncodeToken(se); err != nil {
			return err
		}
		if err := enc.EncodeToken(xml.CharData(e.Script)); err != nil {
			return err
		}
		if err := enc.EncodeToken(se.End()); err != nil {
			return err
		}
	}
	if len(e.Children) > 0 {
		wrap := xml.StartElement{Name: xml.Name{Local: "CheatEntries"}}
		if err := enc.EncodeToken(wrap); err != nil {
			return err
		}
		for _, c := range e.Children {
			if err := enc.Encode(c); err != nil {
				return err
			}
		}
		if err := enc.EncodeToken(wrap.End()); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

// Table is a flat list of cheat entries.
type Table struct {
	XMLName xml.Name `xml:"CheatTable" json:"-"`
	Version string   `xml:"Version,attr,omitempty" json:"version,omitempty"`
	Entries []Entry  `xml:"CheatEntries>CheatEntry" json:"entries"`
	// Stats summarises a Cheat Engine conversion. It is not serialized.
	Stats ImportStats `xml:"-" json:"-"`
	// CustomTypes lists Cheat Engine custom type definitions found in the
	// table's scripts. It is not serialized.
	CustomTypes []CustomTypeDef `xml:"-" json:"-"`
}

// ImportStats reports the outcome of converting a Cheat Engine .CT file into
// Firstspark's flat table. Reasons counts skipped entries by cause ("group",
// "script", "address", "type").
type ImportStats struct {
	Imported int
	Skipped  int
	Reasons  map[string]int
}

// CustomTypeDef is a Cheat Engine custom type definition extracted from a
// table's scripts (ADR 0037 S5).
type CustomTypeDef struct {
	Name string
	Size int
}

// PointerChain is a parsed pointer path. When Module is set, Offset is relative
// to that module's load base; otherwise Base is an absolute address.
type PointerChain struct {
	Module  string
	Base    uint64
	Offset  uint64
	Offsets []int64
}

// FormatPointerChain renders a chain as "module+0xoffset:+0x10,-0x8" or
// "0xbase:+0x10". It returns "" when there is nothing to store.
func FormatPointerChain(c PointerChain) string {
	if c.Module == "" && c.Base == 0 && c.Offset == 0 && len(c.Offsets) == 0 {
		return ""
	}
	var b strings.Builder
	if c.Module != "" {
		fmt.Fprintf(&b, "%s+0x%x", c.Module, c.Offset)
	} else {
		fmt.Fprintf(&b, "0x%x", c.Base)
	}
	for _, off := range c.Offsets {
		if off < 0 {
			fmt.Fprintf(&b, ":-0x%x", uint64(-off))
		} else {
			fmt.Fprintf(&b, ":+0x%x", uint64(off))
		}
	}
	return b.String()
}

// ParsePointerChain parses a Pointer attribute written by FormatPointerChain.
func ParsePointerChain(s string) (PointerChain, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return PointerChain{}, false
	}
	head, rest, _ := strings.Cut(s, ":")
	var c PointerChain
	if i := strings.LastIndex(head, "+0x"); i > 0 {
		c.Module = head[:i]
		v, err := strconv.ParseUint(head[i+3:], 16, 64)
		if err != nil {
			return PointerChain{}, false
		}
		c.Offset = v
	} else {
		v, err := strconv.ParseUint(strings.TrimPrefix(head, "0x"), 16, 64)
		if err != nil {
			return PointerChain{}, false
		}
		c.Base = v
	}
	if rest != "" {
		for _, part := range strings.Split(rest, ":") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			neg := strings.HasPrefix(part, "-")
			part = strings.TrimPrefix(strings.TrimPrefix(part, "-"), "+")
			part = strings.TrimPrefix(strings.TrimPrefix(part, "0x"), "0X")
			v, err := strconv.ParseInt(part, 16, 64)
			if err != nil {
				return PointerChain{}, false
			}
			if neg {
				v = -v
			}
			c.Offsets = append(c.Offsets, v)
		}
	}
	return c, true
}

// Add appends an entry, assigning the next ID.
func (t *Table) Add(description, address, typ, value string) {
	if t.Version == "" {
		t.Version = SchemaVersion
	}
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

// Parse parses .CT XML. Cheat Engine files (detected by their
// CheatEngineTableVersion attribute) are converted to Firstspark's flat model;
// the conversion is best-effort and its outcome is reported in Table.Stats.
func Parse(data []byte) (*Table, error) {
	var probe struct {
		XMLName                 xml.Name
		CheatEngineTableVersion string `xml:"CheatEngineTableVersion,attr"`
	}
	if err := xml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("cheattable: parse xml: %w", err)
	}
	if probe.CheatEngineTableVersion != "" {
		return parseCE(data)
	}
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
