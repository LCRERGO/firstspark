package cheattable

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

// CE schema version written by MarshalCE.
const ceSchemaVersion = 45

// ceOutTable is the Cheat Engine document written by MarshalCE.
type ceOutTable struct {
	XMLName                 xml.Name `xml:"CheatTable"`
	CheatEngineTableVersion int      `xml:"CheatEngineTableVersion,attr"`
	CheatEntries            []ceOut  `xml:"CheatEntries>CheatEntry"`
}

// ceOut is one Cheat Engine record.
type ceOut struct {
	ID              int      `xml:"ID,omitempty"`
	Description     string   `xml:"Description,omitempty"`
	GroupHeader     int      `xml:"GroupHeader,omitempty"`
	ShowAsHex       int      `xml:"ShowAsHex,omitempty"`
	ShowAsSigned    int      `xml:"ShowAsSigned,omitempty"`
	VariableType    string   `xml:"VariableType,omitempty"`
	Address         string   `xml:"Address,omitempty"`
	Offsets         []string `xml:"Offsets>Offset,omitempty"`
	Length          int      `xml:"Length,omitempty"`
	Unicode         int      `xml:"Unicode,omitempty"`
	ByteLength      int      `xml:"ByteLength,omitempty"`
	CustomType      string   `xml:"CustomType,omitempty"`
	AssemblerScript string   `xml:"AssemblerScript,omitempty"`
	Children        []ceOut  `xml:"CheatEntries>CheatEntry,omitempty"`
}

// MarshalCE renders the table as a Cheat Engine .CT document.
func (t *Table) MarshalCE() ([]byte, error) {
	doc := ceOutTable{CheatEngineTableVersion: ceSchemaVersion}
	for i := range t.Entries {
		doc.CheatEntries = append(doc.CheatEntries, entryToCE(&t.Entries[i]))
	}
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cheattable: marshal CE xml: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

// ExportCE writes the table as a Cheat Engine .CT file.
func (t *Table) ExportCE(path string) error {
	data, err := t.MarshalCE()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("cheattable: write %s: %w", path, err)
	}
	return nil
}

func entryToCE(e *Entry) ceOut {
	out := ceOut{ID: e.ID, Description: ceQuote(e.Description)}
	switch {
	case e.Group && e.Script != "":
		out.GroupHeader = 1
		out.VariableType = "Auto Assembler Script"
		out.AssemblerScript = e.Script
	case e.Group:
		out.GroupHeader = 1
	default:
		out.VariableType, out.CustomType = ceVariableType(e)
		out.Length, out.Unicode, out.ByteLength = ceTypeExtras(e)
	}
	if e.Display == "hex" {
		out.ShowAsHex = 1
	}
	if e.ShowAsSigned {
		out.ShowAsSigned = 1
	}
	out.Address, out.Offsets = ceAddress(e)
	if len(e.Children) > 0 {
		for i := range e.Children {
			out.Children = append(out.Children, entryToCE(&e.Children[i]))
		}
	}
	return out
}

// ceVariableType maps a firstspark Entry to CE's VariableType (and CustomType).
func ceVariableType(e *Entry) (string, string) {
	typ := e.Type
	if strings.EqualFold(typ, "bitfield") {
		return "Binary", ""
	}
	switch typ {
	case "byte":
		return "Byte", ""
	case "word":
		return "2 Bytes", ""
	case "dword":
		return "4 Bytes", ""
	case "qword":
		return "8 Bytes", ""
	case "float":
		return "Float", ""
	case "double":
		return "Double", ""
	case "string", "utf16le", "utf16be":
		return "String", ""
	case "aob":
		return "Array of byte", ""
	case "binary":
		return "Binary", ""
	default:
		if typ == "" {
			return "4 Bytes", ""
		}
		return "Custom", typ
	}
}

// ceAddress renders an entry's address and offset chain in CE form.
func ceAddress(e *Entry) (string, []string) {
	if e.Expr != "" {
		return e.Expr, splitOffsets(e.Offsets)
	}
	if e.Pointer != "" {
		if pc, ok := ParsePointerChain(e.Pointer); ok {
			addr := fmt.Sprintf("0x%x", pc.Base)
			if pc.Module != "" {
				addr = fmt.Sprintf("%q+0x%x", pc.Module, pc.Offset)
			}
			var offsets []string
			for _, off := range pc.Offsets {
				if off < 0 {
					offsets = append(offsets, fmt.Sprintf("-0x%x", uint64(-off)))
				} else {
					offsets = append(offsets, fmt.Sprintf("+0x%x", uint64(off)))
				}
			}
			return addr, offsets
		}
	}
	if a, err := e.AddressValue(); err == nil {
		return fmt.Sprintf("0x%x", a), nil
	}
	return "", nil
}

// ceTypeExtras fills the per-type length/unicode fields. It is separate from
// ceVariableType so the length can come from the current value.
func ceTypeExtras(e *Entry) (length, unicode, byteLength int) {
	switch e.Type {
	case "utf16le", "utf16be":
		unicode = 1
		length = 2 * len([]rune(e.Value))
	case "string":
		length = len(e.Value)
	case "aob":
		byteLength = len(strings.Fields(e.Value))
	}
	return
}

func ceQuote(s string) string {
	if s == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func splitOffsets(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
