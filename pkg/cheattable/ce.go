package cheattable

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// ceTable mirrors the element-based schema of a Cheat Engine .CT file.
type ceTable struct {
	XMLName xml.Name  `xml:"CheatTable"`
	Entries []ceEntry `xml:"CheatEntries>CheatEntry"`
}

// ceEntry is one Cheat Engine record. CE nests children under parents and
// stores values as child elements rather than attributes.
type ceEntry struct {
	ID            int       `xml:"ID"`
	Description   string    `xml:"Description"`
	VariableType  string    `xml:"VariableType"`
	Address       string    `xml:"Address"`
	Offsets       []string  `xml:"Offsets>Offset"`
	GroupHeader   bool      `xml:"GroupHeader"`
	ShowAsHex     bool      `xml:"ShowAsHex"`
	ShowAsSigned  bool      `xml:"ShowAsSigned"`
	Length        int       `xml:"Length"`
	Unicode       bool      `xml:"Unicode"`
	CodePage      int       `xml:"CodePage"`
	ZeroTerminate bool      `xml:"ZeroTerminate"`
	ByteLength    int       `xml:"ByteLength"`
	CustomType    string    `xml:"CustomType"`
	Script        string    `xml:"AssemblerScript"`
	Entries       []ceEntry `xml:"CheatEntries>CheatEntry"`
}

// parseCE converts a Cheat Engine document into Firstspark's model, preserving
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
	return t, nil
}

// extractCustomTypes walks the CE entries' scripts for
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
		if name, size := customTypeFromAA(script); name != "" && size > 0 {
			out = append(out, CustomTypeDef{Name: name, Size: size})
		}
	}
}

func customTypeFromAA(s string) (string, int) {
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
	return name, size
}

func (s *ImportStats) skip(reason string) {
	s.Skipped++
	s.Reasons[reason]++
}

// addressResult classifies how a CE address was converted.
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
			out = append(out, Entry{
				Description: ceDescription(e.Description),
				Group:       true,
				Script:      e.Script,
				Children:    children,
			})
			stats.Imported++
			continue
		}
		if e.GroupHeader {
			g := Entry{Description: ceDescription(e.Description), Group: true, Children: children}
			applyCEAddress(&g, e)
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
		if e.ShowAsHex {
			leaf.Display = "hex"
		}
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

// ceTypeName maps a Cheat Engine VariableType to a Firstspark type name.
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

// ceDescription strips the quotes Cheat Engine wraps descriptions in.
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

// parseCEOffsets parses CE's offset list, which may be signed hex or, when it
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
