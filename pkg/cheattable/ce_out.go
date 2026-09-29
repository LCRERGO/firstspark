package cheattable

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// .CT schema version written by MarshalCE.
const ceSchemaVersion = 45

// ceOutTable is the .CT document written by MarshalCE.
type ceOutTable struct {
	XMLName                 xml.Name `xml:"CheatTable"`
	CheatEngineTableVersion int
	CheatEntries            []ceOut
	LuaScript               string
	Comments                string
	Extras                  []RawElement
}

// MarshalXML writes the table's top-level elements, keeping unmodelled ones.
func (d ceOutTable) MarshalXML(enc *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{Name: xml.Name{Local: "CheatTable"}}
	start.Attr = append(start.Attr, xml.Attr{
		Name:  xml.Name{Local: "CheatEngineTableVersion"},
		Value: strconv.Itoa(d.CheatEngineTableVersion),
	})
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	wrap := xml.StartElement{Name: xml.Name{Local: "CheatEntries"}}
	if err := enc.EncodeToken(wrap); err != nil {
		return err
	}
	for _, e := range d.CheatEntries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	if err := enc.EncodeToken(wrap.End()); err != nil {
		return err
	}
	if err := ceElem(enc, "Comments", d.Comments); err != nil {
		return err
	}
	if err := ceElem(enc, "LuaScript", d.LuaScript); err != nil {
		return err
	}
	for _, ex := range d.Extras {
		if err := ceRawElem(enc, ex.Name, ex.Inner, ex.Text); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

// ceOut is one record written by MarshalCE. It marshals itself so
// the optional LastState/Color/Hotkeys elements and preserved unknown elements
// can be emitted in the reference tool's order.
type ceOut struct {
	ID              int
	Description     string
	GroupHeader     int
	ShowAsHex       int
	ShowAsBinary    int
	ShowAsSigned    int
	Color           string
	VariableType    string
	Address         string
	Offsets         []string
	Length          int
	Unicode         int
	ByteLength      int
	CustomType      string
	AssemblerScript string
	Comments        string
	DontSaveValue   int
	LastState       *ceOutLastState
	Hotkeys         []ceOutHotkey
	Extras          []RawElement
	Children        []ceOut
}

type ceOutLastState struct {
	RealAddress string
	Value       string
	Activated   bool
}

type ceOutHotkey struct {
	Action        string
	Active        bool
	OnlyWhileDown bool
	Keys          []int
	Value         string
	Description   string
	ID            int
}

// MarshalXML writes the record's child elements.
func (o ceOut) MarshalXML(enc *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{Name: xml.Name{Local: "CheatEntry"}}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	if err := o.encodeChildren(enc); err != nil {
		return err
	}
	return enc.EncodeToken(start.End())
}

func (o ceOut) encodeChildren(enc *xml.Encoder) error {
	if err := ceElemInt(enc, "ID", o.ID); err != nil {
		return err
	}
	if err := ceElem(enc, "Description", o.Description); err != nil {
		return err
	}
	if err := ceElem(enc, "Address", o.Address); err != nil {
		return err
	}
	if len(o.Offsets) > 0 {
		if err := ceContainer(enc, "Offsets", "Offset", o.Offsets); err != nil {
			return err
		}
	}
	if o.LastState != nil {
		if err := o.LastState.encode(enc); err != nil {
			return err
		}
	}
	if err := ceElemInt(enc, "ShowAsHex", o.ShowAsHex); err != nil {
		return err
	}
	if err := ceElemInt(enc, "ShowAsBinary", o.ShowAsBinary); err != nil {
		return err
	}
	if o.ShowAsSigned == 1 {
		if err := ceElem(enc, "ShowAsSigned", "1"); err != nil {
			return err
		}
	}
	if err := ceElem(enc, "Color", o.Color); err != nil {
		return err
	}
	if err := ceElemInt(enc, "GroupHeader", o.GroupHeader); err != nil {
		return err
	}
	if err := ceElem(enc, "VariableType", o.VariableType); err != nil {
		return err
	}
	if err := ceElem(enc, "CustomType", o.CustomType); err != nil {
		return err
	}
	if err := ceElemInt(enc, "Length", o.Length); err != nil {
		return err
	}
	if err := ceElemInt(enc, "Unicode", o.Unicode); err != nil {
		return err
	}
	if err := ceElemInt(enc, "ByteLength", o.ByteLength); err != nil {
		return err
	}
	if err := ceElem(enc, "AssemblerScript", o.AssemblerScript); err != nil {
		return err
	}
	if err := ceElem(enc, "Comments", o.Comments); err != nil {
		return err
	}
	if err := ceElemInt(enc, "DontSaveValue", o.DontSaveValue); err != nil {
		return err
	}
	if len(o.Hotkeys) > 0 {
		if err := encodeHotkeys(enc, o.Hotkeys); err != nil {
			return err
		}
	}
	for _, ex := range o.Extras {
		if err := ceRawElem(enc, ex.Name, ex.Inner, ex.Text); err != nil {
			return err
		}
	}
	if len(o.Children) > 0 {
		wrap := xml.StartElement{Name: xml.Name{Local: "CheatEntries"}}
		if err := enc.EncodeToken(wrap); err != nil {
			return err
		}
		for _, c := range o.Children {
			if err := enc.Encode(c); err != nil {
				return err
			}
		}
		if err := enc.EncodeToken(wrap.End()); err != nil {
			return err
		}
	}
	return nil
}

func (ls ceOutLastState) encode(enc *xml.Encoder) error {
	start := xml.StartElement{Name: xml.Name{Local: "LastState"}}
	if ls.RealAddress != "" {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "RealAddress"}, Value: ls.RealAddress})
	}
	if ls.Value != "" {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "Value"}, Value: ls.Value})
	}
	if ls.Activated {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "Activated"}, Value: "1"})
	}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	return enc.EncodeToken(start.End())
}

func encodeHotkeys(enc *xml.Encoder, hks []ceOutHotkey) error {
	wrap := xml.StartElement{Name: xml.Name{Local: "Hotkeys"}}
	if err := enc.EncodeToken(wrap); err != nil {
		return err
	}
	for _, hk := range hks {
		start := xml.StartElement{Name: xml.Name{Local: "Hotkey"}}
		if !hk.Active {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "Active"}, Value: "0"})
		}
		if hk.OnlyWhileDown {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "OnlyWhileDown"}, Value: "1"})
		}
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		if err := ceElem(enc, "Action", hk.Action); err != nil {
			return err
		}
		if len(hk.Keys) > 0 {
			keys := make([]string, len(hk.Keys))
			for i, k := range hk.Keys {
				keys[i] = strconv.Itoa(k)
			}
			if err := ceContainer(enc, "Keys", "Key", keys); err != nil {
				return err
			}
		}
		if err := ceElem(enc, "Value", hk.Value); err != nil {
			return err
		}
		if err := ceElem(enc, "Description", hk.Description); err != nil {
			return err
		}
		if err := ceElemInt(enc, "ID", hk.ID); err != nil {
			return err
		}
		if err := enc.EncodeToken(start.End()); err != nil {
			return err
		}
	}
	return enc.EncodeToken(wrap.End())
}

// ceRawElem writes an element Firstspark does not model. When inner is set it
// is re-parsed and copied through so nested markup survives; otherwise text is
// written as-is.
func ceRawElem(enc *xml.Encoder, name, inner, text string) error {
	start := xml.StartElement{Name: xml.Name{Local: name}}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	if inner != "" {
		dec := xml.NewDecoder(strings.NewReader(inner))
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if err := enc.EncodeToken(tok); err != nil {
				return err
			}
		}
	} else if text != "" {
		if err := enc.EncodeToken(xml.CharData(text)); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

// ceElem writes <name>text</name> when text is non-empty.
func ceElem(enc *xml.Encoder, name, text string) error {
	if text == "" {
		return nil
	}
	start := xml.StartElement{Name: xml.Name{Local: name}}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	if err := enc.EncodeToken(xml.CharData(text)); err != nil {
		return err
	}
	return enc.EncodeToken(start.End())
}

func ceElemInt(enc *xml.Encoder, name string, v int) error {
	if v == 0 {
		return nil
	}
	return ceElem(enc, name, strconv.Itoa(v))
}

// ceContainer writes <outer><inner>v</inner>...</outer>.
func ceContainer(enc *xml.Encoder, outer, inner string, values []string) error {
	start := xml.StartElement{Name: xml.Name{Local: outer}}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	for _, v := range values {
		if err := ceElem(enc, inner, v); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

func entryToCE(e *Entry) ceOut {
	out := ceOut{ID: e.ID, Description: ceQuote(e.Description), Color: e.Color}
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
	if e.Display == "binary" {
		out.ShowAsBinary = 1
	}
	if e.ShowAsSigned {
		out.ShowAsSigned = 1
	}
	if e.LastValue != "" || e.LastAddress != "" || e.Activated {
		out.LastState = &ceOutLastState{
			RealAddress: strings.TrimPrefix(e.LastAddress, "0x"),
			Value:       e.LastValue,
			Activated:   e.Activated,
		}
	}
	// the reference tool's Active column locks the value, so a frozen Firstspark
	// record exports as an activated LastState.
	if e.Frozen && !e.Group {
		if out.LastState == nil {
			out.LastState = &ceOutLastState{}
		}
		if out.LastState.Value == "" {
			out.LastState.Value = e.Value
		}
		if out.LastState.RealAddress == "" {
			out.LastState.RealAddress = strings.TrimPrefix(e.Address, "0x")
		}
		out.LastState.Activated = true
	}
	if e.Comments != "" {
		out.Comments = e.Comments
	}
	if e.DontSaveValue {
		out.DontSaveValue = 1
	}
	out.Hotkeys = ceHotkeysFromEntry(e)
	out.Extras = e.ExtraElements
	out.Address, out.Offsets = ceAddress(e)
	if len(e.Children) > 0 {
		for i := range e.Children {
			out.Children = append(out.Children, entryToCE(&e.Children[i]))
		}
	}
	return out
}

// ceHotkeysFromEntry returns the hotkeys to emit: the preserved ones, or a
// single Toggle Activation binding for a Firstspark hotkey.
func ceHotkeysFromEntry(e *Entry) []ceOutHotkey {
	var out []ceOutHotkey
	for _, hk := range e.CEHotkeys {
		out = append(out, ceOutHotkey{
			Action:        hk.Action,
			Active:        hk.Active,
			OnlyWhileDown: hk.OnlyWhileDown,
			Keys:          parseKeyList(hk.Keys),
			Value:         hk.Value,
			Description:   hk.Description,
			ID:            hk.ID,
		})
	}
	if len(out) == 0 && e.Hotkey != "" {
		if vk, ok := vkCode(e.Hotkey); ok {
			out = append(out, ceOutHotkey{Action: "Toggle Activation", Active: true, Keys: []int{vk}})
		}
	}
	return out
}

func parseKeyList(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// MarshalCE renders the table as a .CT document.
func (t *Table) MarshalCE() ([]byte, error) {
	doc := ceOutTable{
		CheatEngineTableVersion: ceSchemaVersion,
		LuaScript:               t.LuaScript,
		Comments:                t.Comments,
		Extras:                  t.ExtraElements,
	}
	for i := range t.Entries {
		doc.CheatEntries = append(doc.CheatEntries, entryToCE(&t.Entries[i]))
	}
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cheattable: marshal CE xml: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

// ExportCE writes the table as a .CT file.
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

// ceVariableType maps a firstspark Entry to the VariableType (and CustomType).
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

// ceAddress renders an entry's address and offset chain in format.
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
