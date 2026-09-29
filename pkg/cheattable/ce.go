package cheattable

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// ceTable mirrors the element-based schema of a .CT file.
type ceTable struct {
	XMLName   xml.Name  `xml:"CheatTable"`
	Version   int       `xml:"CheatEngineTableVersion,attr"`
	Entries   []ceEntry `xml:"CheatEntries>CheatEntry"`
	LuaScript string    `xml:"LuaScript"`
	Comments  string    `xml:"Comments"`
	Extra     []ceExtra `xml:",any"`
}

// ceEntry is one record. the format nests children under parents and
// stores values as child elements rather than attributes.
type ceEntry struct {
	ID            int          `xml:"ID"`
	Description   string       `xml:"Description"`
	VariableType  string       `xml:"VariableType"`
	Address       string       `xml:"Address"`
	Offsets       []string     `xml:"Offsets>Offset"`
	GroupHeader   bool         `xml:"GroupHeader"`
	ShowAsHex     bool         `xml:"ShowAsHex"`
	ShowAsBinary  bool         `xml:"ShowAsBinary"`
	ShowAsSigned  bool         `xml:"ShowAsSigned"`
	Length        int          `xml:"Length"`
	Unicode       bool         `xml:"Unicode"`
	CodePage      int          `xml:"CodePage"`
	ZeroTerminate bool         `xml:"ZeroTerminate"`
	ByteLength    int          `xml:"ByteLength"`
	CustomType    string       `xml:"CustomType"`
	Script        string       `xml:"AssemblerScript"`
	Color         string       `xml:"Color"`
	Comments      string       `xml:"Comments"`
	DontSaveValue bool         `xml:"DontSaveValue"`
	LastState     *ceLastState `xml:"LastState"`
	Hotkeys       []ceHotkey   `xml:"Hotkeys>Hotkey"`
	Extra         []ceExtra    `xml:",any"`
	Entries       []ceEntry    `xml:"CheatEntries>CheatEntry"`
}

// ceLastState is the reference tool's cached value/address for a record.
type ceLastState struct {
	RealAddress string `xml:"RealAddress,attr"`
	Value       string `xml:"Value,attr"`
	Activated   string `xml:"Activated,attr"`
}

// ceHotkey is one hotkey binding.
type ceHotkey struct {
	Action        string `xml:"Action"`
	Active        string `xml:"Active,attr"`
	OnlyWhileDown string `xml:"OnlyWhileDown,attr"`
	Keys          []int  `xml:"Keys>Key"`
	Value         string `xml:"Value"`
	Description   string `xml:"Description"`
	ID            string `xml:"ID"`
}

// ceExtra captures an unmodelled child element, keeping its direct text and
// its inner XML so nested markup survives a round-trip.
type ceExtra struct {
	Name  string
	Text  string
	Inner string
}

// UnmarshalXML records the element's name, direct text and re-serialized inner
// XML.
func (e *ceExtra) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	e.Name = start.Name.Local
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	depth := 0
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		if end, ok := tok.(xml.EndElement); ok && depth == 0 && end.Name == start.Name {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 {
				e.Text += string(t)
			}
		}
		if err := enc.EncodeToken(tok); err != nil {
			return err
		}
	}
	if err := enc.Flush(); err != nil {
		return err
	}
	e.Inner = buf.String()
	return nil
}

// parseCE converts a .CT document into Firstspark's model, preserving
// the group tree. Entries whose type or address cannot be represented are
// skipped and counted in Table.Stats; symbolic and parent-relative addresses are
// kept as Expr for the resolver.
func parseCE(data []byte) (*Table, error) {
	var raw ceTable
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("cheattable: parse Cheat Engine xml: %w", err)
	}
	t := &Table{Version: SchemaVersion, Stats: ImportStats{Reasons: map[string]int{}}}
	t.Entries = convertCE(raw.Entries, &t.Stats)
	t.CustomTypes = extractCustomTypes(raw.Entries)
	t.LuaScript = raw.LuaScript
	t.Comments = strings.TrimSpace(raw.Comments)
	t.ExtraElements = convertExtras(raw.Extra)
	return t, nil
}

// extractCustomTypes walks the the entries' scripts for
// registerCustomTypeAutoAssembler definitions.
func extractCustomTypes(entries []ceEntry) []CustomTypeDef {
	var out []CustomTypeDef
	var walk func([]ceEntry)
	walk = func(es []ceEntry) {
		for i := range es {
			if es[i].Script != "" {
				out = append(out, parseCustomTypeDefs(es[i].Script)...)
			}
			walk(es[i].Entries)
		}
	}
	walk(entries)
	return out
}

// parseCustomTypeDefs finds each registerCustomTypeAutoAssembler block and
// reads its TypeName and ByteSize.
func parseCustomTypeDefs(script string) []CustomTypeDef {
	var out []CustomTypeDef
	const marker = "registerCustomTypeAutoAssembler"
	for {
		i := strings.Index(script, marker)
		if i < 0 {
			return out
		}
		script = script[i+len(marker):]
		if def, ok := customTypeFromAA(script); ok {
			out = append(out, def)
		}
	}
}

func customTypeFromAA(s string) (CustomTypeDef, bool) {
	name := ""
	size := 0
	lines := strings.Split(s, "\n")
	for i := 0; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "TypeName:") {
			for j := i; j < len(lines) && j < i+4; j++ {
				if p := strings.Index(lines[j], "db '"); p >= 0 {
					rest := lines[j][p+4:]
					if q := strings.IndexByte(rest, '\''); q >= 0 {
						name = rest[:q]
					}
					break
				}
			}
		}
		if strings.HasPrefix(l, "ByteSize:") {
			for j := i; j < len(lines) && j < i+4; j++ {
				f := strings.Fields(lines[j])
				if len(f) >= 2 && f[0] == "dd" {
					if n, err := strconv.Atoi(f[1]); err == nil {
						size = n
					}
					break
				}
			}
		}
		if name != "" && size > 0 {
			break
		}
	}
	if name == "" || size <= 0 {
		return CustomTypeDef{}, false
	}
	return CustomTypeDef{
		Name:               name,
		Size:               size,
		Alignment:          symbolValue(lines, "PREFEREDALIGNMENT"),
		CallMethod:         symbolValue(lines, "CALLMETHOD") != 0,
		UsesFloat:          symbolValue(lines, "USESFLOAT") != 0,
		UsesString:         symbolValue(lines, "USESSTRING") != 0,
		MaxStringSize:      symbolValue(lines, "MAXSTRINGSIZE"),
		ConvertRoutine:     extractAARoutine(lines, "ConvertRoutine"),
		ConvertBackRoutine: extractAARoutine(lines, "ConvertBackRoutine"),
	}, true
}

// symbolValue finds a reference-tool custom-type flag symbol and reads the db/dd
// value that follows it (a label or an alloc). It returns 0 when absent.
func symbolValue(lines []string, symbol string) int {
	for i, l := range lines {
		if !containsSymbol(l, symbol) {
			continue
		}
		for j := i; j < len(lines) && j < i+5; j++ {
			fields := strings.Fields(lines[j])
			for k := 0; k+1 < len(fields); k++ {
				op := strings.ToLower(strings.TrimSuffix(fields[k], ","))
				if op != "db" && op != "dw" && op != "dd" && op != "dq" {
					continue
				}
				v := strings.TrimSuffix(fields[k+1], ",")
				if n, err := strconv.ParseInt(v, 0, 64); err == nil {
					return int(n)
				}
			}
		}
	}
	return 0
}

// containsSymbol reports whether line contains symbol as a whole identifier.
func containsSymbol(line, symbol string) bool {
	up := strings.ToUpper(line)
	sym := strings.ToUpper(symbol)
	start := 0
	for {
		i := strings.Index(up[start:], sym)
		if i < 0 {
			return false
		}
		i += start
		before := byte(0)
		if i > 0 {
			before = up[i-1]
		}
		after := byte(0)
		if i+len(sym) < len(up) {
			after = up[i+len(sym)]
		}
		if !isIdentByte(before) && !isIdentByte(after) {
			return true
		}
		start = i + 1
	}
}

func isIdentByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// extractAARoutine returns the lines after a `label:` up to the next label.
func extractAARoutine(lines []string, label string) string {
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == label+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	var out []string
	for i := start + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		if strings.HasSuffix(t, ":") && !strings.HasPrefix(t, "//") && !strings.HasPrefix(t, ";") {
			break
		}
		out = append(out, strings.TrimSpace(lines[i]))
	}
	return strings.Join(out, "\n")
}

func (s *ImportStats) skip(reason string) {
	s.Skipped++
	s.Reasons[reason]++
}

// addressResult classifies how a address was converted.
type addressResult int

const (
	addrResolved    addressResult = iota // absolute or module+offset
	addrExpr                             // kept as an unresolved expression
	addrUnsupported                      // nothing usable
)

func convertCE(entries []ceEntry, stats *ImportStats) []Entry {
	var out []Entry
	for i := range entries {
		e := &entries[i]
		children := convertCE(e.Entries, stats)
		if strings.EqualFold(strings.TrimSpace(e.VariableType), "Auto Assembler Script") {
			g := Entry{
				Description: ceDescription(e.Description),
				Group:       true,
				Script:      e.Script,
				Children:    children,
			}
			applyCEExtras(&g, e)
			out = append(out, g)
			stats.Imported++
			continue
		}
		if e.GroupHeader {
			g := Entry{Description: ceDescription(e.Description), Group: true, Children: children}
			applyCEAddress(&g, e)
			applyCEExtras(&g, e)
			out = append(out, g)
			stats.Imported++
			continue
		}
		typeName, ok := ceTypeName(e)
		if !ok {
			stats.skip("type")
			out = append(out, children...)
			continue
		}
		leaf := Entry{Description: ceDescription(e.Description), Type: typeName, Children: children}
		leaf.ShowAsSigned = e.ShowAsSigned
		switch {
		case e.ShowAsHex:
			leaf.Display = "hex"
		case e.ShowAsBinary:
			leaf.Display = "binary"
		}
		applyCEExtras(&leaf, e)
		switch applyCEAddress(&leaf, e) {
		case addrResolved, addrExpr:
			out = append(out, leaf)
			stats.Imported++
		default:
			stats.skip("address")
			out = append(out, children...)
		}
	}
	return out
}

// applyCEExtras copies the fields Firstspark models loosely: row
// colour, cached last state, hotkeys and unmodelled elements.
func applyCEExtras(entry *Entry, e *ceEntry) {
	entry.Color = strings.TrimSpace(e.Color)
	entry.Comments = strings.TrimSpace(e.Comments)
	entry.DontSaveValue = e.DontSaveValue
	if e.LastState != nil {
		entry.LastAddress = strings.TrimSpace(e.LastState.RealAddress)
		entry.LastValue = e.LastState.Value
		entry.Activated = e.LastState.Activated == "1"
	}
	entry.CEHotkeys = convertHotkeys(e.Hotkeys)
	if len(e.Extra) > 0 {
		entry.ExtraElements = convertExtras(e.Extra)
	}
	if entry.Hotkey == "" {
		entry.Hotkey = simpleToggleHotkey(e.Hotkeys)
	}
}

func convertHotkeys(hks []ceHotkey) []CEHotkey {
	var out []CEHotkey
	for _, h := range hks {
		keys := make([]string, len(h.Keys))
		for i, k := range h.Keys {
			keys[i] = strconv.Itoa(k)
		}
		id, _ := strconv.Atoi(strings.TrimSpace(h.ID))
		out = append(out, CEHotkey{
			Action:        strings.TrimSpace(h.Action),
			Keys:          strings.Join(keys, ","),
			Value:         h.Value,
			Description:   h.Description,
			ID:            id,
			Active:        h.Active != "0",
			OnlyWhileDown: h.OnlyWhileDown == "1",
		})
	}
	return out
}

func convertExtras(extras []ceExtra) []RawElement {
	var out []RawElement
	for _, e := range extras {
		out = append(out, RawElement{
			Name:  e.Name,
			Text:  strings.TrimSpace(e.Text),
			Inner: strings.TrimSpace(e.Inner),
		})
	}
	return out
}

// simpleToggleHotkey maps a single-key "Toggle Activation" hotkey to a
// Firstspark hotkey name (F1..F12 or a letter/digit).
func simpleToggleHotkey(hks []ceHotkey) string {
	for _, h := range hks {
		if !strings.EqualFold(strings.TrimSpace(h.Action), "Toggle Activation") {
			continue
		}
		if len(h.Keys) != 1 {
			continue
		}
		if name := vkName(h.Keys[0]); name != "" {
			return name
		}
	}
	return ""
}

// vkName maps a Windows virtual-key code to a Firstspark hotkey name.
func vkName(vk int) string {
	switch {
	case vk >= 0x70 && vk <= 0x7B:
		return fmt.Sprintf("F%d", vk-0x70+1)
	case vk >= 'A' && vk <= 'Z':
		return string(rune(vk))
	case vk >= '0' && vk <= '9':
		return string(rune(vk))
	default:
		return ""
	}
}

// vkCode is the inverse of vkName.
func vkCode(name string) (int, bool) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if len(name) == 1 {
		if (name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= '0' && name[0] <= '9') {
			return int(name[0]), true
		}
	}
	if strings.HasPrefix(name, "F") {
		if n, err := strconv.Atoi(name[1:]); err == nil && n >= 1 && n <= 12 {
			return 0x70 + n - 1, true
		}
	}
	return 0, false
}

// applyCEAddress sets either a resolvable address (and pointer chain) or the
// raw Expr text plus offset list for the S3 resolver.
func applyCEAddress(entry *Entry, e *ceEntry) addressResult {
	module, value, ok := splitCEAddress(e.Address)
	offsets, offOK := parseCEOffsets(e.Offsets)
	if ok && offOK {
		switch {
		case module != "":
			entry.Address = "0x0"
			entry.Pointer = FormatPointerChain(PointerChain{Module: module, Offset: value, Offsets: offsets})
		case len(offsets) > 0:
			entry.Address = fmt.Sprintf("0x%x", value)
			entry.Pointer = FormatPointerChain(PointerChain{Base: value, Offsets: offsets})
		default:
			entry.Address = fmt.Sprintf("0x%x", value)
		}
		return addrResolved
	}
	raw := strings.TrimSpace(strings.Trim(e.Address, `"`))
	if raw == "" {
		return addrUnsupported
	}
	entry.Expr = raw
	if len(e.Offsets) > 0 {
		entry.Offsets = strings.Join(e.Offsets, ",")
	}
	return addrExpr
}

// ceTypeName maps a VariableType to a Firstspark type name.
func ceTypeName(e *ceEntry) (string, bool) {
	switch strings.TrimSpace(e.VariableType) {
	case "Byte":
		return "byte", true
	case "2 Bytes":
		return "word", true
	case "4 Bytes":
		return "dword", true
	case "8 Bytes":
		return "qword", true
	case "Float":
		return "float", true
	case "Double":
		return "double", true
	case "String":
		if e.Unicode {
			return "utf16le", true
		}
		return "string", true
	case "Array of byte", "Array of Bytes":
		return "aob", true
	case "Binary":
		return "binary", true
	case "Custom":
		name := strings.TrimSpace(e.CustomType)
		if name == "" {
			return "", false
		}
		return strings.ToLower(name), true
	default:
		return "", false
	}
}

// ceDescription strips the quotes the reference tool wraps descriptions in.
func ceDescription(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return s[1 : len(s)-1]
	}
	return s
}

// splitCEAddress parses an address into either a module plus offset or an
// absolute base. Parent-relative, symbolic and expression addresses are not
// supported yet and report ok=false.
func splitCEAddress(addr string) (module string, value uint64, ok bool) {
	a := strings.TrimSpace(addr)
	a = strings.Trim(a, `"`)
	if a == "" {
		return "", 0, false
	}
	if i := strings.LastIndex(a, "+"); i > 0 {
		left := strings.Trim(a[:i], `"`)
		if !isHexLiteral(left) {
			off, err := parseHexValue(a[i+1:])
			if err != nil {
				return "", 0, false
			}
			return left, off, true
		}
		return "", 0, false
	}
	if strings.HasPrefix(a, "+") || strings.HasPrefix(a, "-") || strings.ContainsAny(a, "$*()") {
		return "", 0, false
	}
	v, err := parseHexValue(a)
	if err != nil {
		return "", 0, false
	}
	return "", v, true
}

// parseCEOffsets parses the offset list, which may be signed hex or, when it
// uses arithmetic such as "+4*$1", an expression that is not supported yet.
func parseCEOffsets(raw []string) ([]int64, bool) {
	var out []int64
	for _, o := range raw {
		s := strings.TrimSpace(o)
		if s == "" {
			continue
		}
		neg := false
		switch {
		case strings.HasPrefix(s, "+"):
			s = s[1:]
		case strings.HasPrefix(s, "-"):
			neg = true
			s = s[1:]
		}
		if s == "" || strings.ContainsAny(s, "$*()") {
			return nil, false
		}
		v, err := parseHexValue(s)
		if err != nil {
			return nil, false
		}
		n := int64(v)
		if neg {
			n = -n
		}
		out = append(out, n)
	}
	return out, true
}

func parseHexValue(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	return strconv.ParseUint(s, 16, 64)
}

func isHexLiteral(s string) bool {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
