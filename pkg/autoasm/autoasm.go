// Package autoasm implements a subset of Cheat Engine's Auto Assembler on top
// of pkg/combinator and pkg/asm (ADR 0018). It parses [ENABLE]/[DISABLE]
// sections containing directives, labels and instructions, and assembles them
// into code that can be written into a target process.
package autoasm

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/combinator"
)

// Kind classifies an item in a section.
type Kind int

const (
	// KindInstruction is an assembly line.
	KindInstruction Kind = iota
	// KindLabel is a `name:` label.
	KindLabel
	// KindData is a db/dw/dd/dq directive.
	KindData
	// KindAlloc is an alloc(name, size) directive.
	KindAlloc
	// KindDealloc is a dealloc(name) directive.
	KindDealloc
	// KindDefine is a define(name, value) directive.
	KindDefine
	// KindRegisterSymbol is a registersymbol(name) directive.
	KindRegisterSymbol
	// KindAOBScan is an aobscan(name, pattern) directive.
	KindAOBScan
)

// Item is one parsed line.
type Item struct {
	Kind Kind
	Name string
	Args []string
	Text string
	Data []byte
	Line int
}

// Section is an [ENABLE] or [DISABLE] block.
type Section struct {
	Enable bool
	Items  []Item
}

// Script is a parsed Auto Assembler script.
type Script struct {
	Sections []Section
}

// Parse reads an Auto Assembler script.
func Parse(src string) (*Script, error) {
	sections := []Section{}
	var cur *Section
	add := func(it Item) {
		if cur == nil {
			sections = append(sections, Section{Enable: true})
			cur = &sections[len(sections)-1]
		}
		cur.Items = append(cur.Items, it)
	}
	defines := map[string]string{}

	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		switch strings.ToLower(line) {
		case "[enable]":
			sections = append(sections, Section{Enable: true})
			cur = &sections[len(sections)-1]
			continue
		case "[disable]":
			sections = append(sections, Section{Enable: false})
			cur = &sections[len(sections)-1]
			continue
		}
		line = substituteWords(line, defines)

		if name, args, ok := parseDirective(line); ok {
			switch name {
			case "define":
				if len(args) >= 2 {
					defines[args[0]] = args[1]
				}
				add(Item{Kind: KindDefine, Name: args[0], Args: args, Line: i + 1})
				continue
			case "alloc":
				add(Item{Kind: KindAlloc, Name: arg(args, 0), Args: args, Line: i + 1})
				continue
			case "dealloc":
				add(Item{Kind: KindDealloc, Name: arg(args, 0), Args: args, Line: i + 1})
				continue
			case "label":
				add(Item{Kind: KindLabel, Name: arg(args, 0), Args: args, Line: i + 1})
				continue
			case "registersymbol":
				add(Item{Kind: KindRegisterSymbol, Name: arg(args, 0), Args: args, Line: i + 1})
				continue
			case "aobscan", "aobscanmodule":
				add(Item{Kind: KindAOBScan, Name: arg(args, 0), Args: args, Line: i + 1})
				continue
			}
		}

		fields := strings.Fields(line)
		if len(fields) > 0 {
			head := strings.ToLower(fields[0])
			if head == "db" || head == "dw" || head == "dd" || head == "dq" {
				data, err := parseData(head, fields[1:])
				if err != nil {
					return nil, fmt.Errorf("autoasm: line %d: %w", i+1, err)
				}
				add(Item{Kind: KindData, Data: data, Line: i + 1})
				continue
			}
		}
		if strings.HasSuffix(line, ":") && !strings.ContainsAny(line, " []()") {
			add(Item{Kind: KindLabel, Name: strings.TrimSuffix(line, ":"), Line: i + 1})
			continue
		}
		add(Item{Kind: KindInstruction, Text: line, Line: i + 1})
	}
	return &Script{Sections: sections}, nil
}

func arg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func stripComment(line string) string {
	for i := 0; i+1 < len(line); i++ {
		if line[i] == '/' && line[i+1] == '/' {
			return line[:i]
		}
		if line[i] == ';' {
			return line[:i]
		}
	}
	return line
}

// parseDirective parses `name(arg, arg, ...)`.
func parseDirective(line string) (string, []string, bool) {
	ident := combinator.TakeWhile1(func(r rune) bool {
		return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
	})
	argP := combinator.Map(
		combinator.TakeWhile(func(r rune) bool { return r != ',' && r != ')' }),
		func(s string) string { return strings.TrimSpace(s) },
	)
	argsP := combinator.SepBy(argP, combinator.RuneLit(','))
	p := combinator.Bind(ident, func(name string) combinator.Parser[combinator.Pair[string, []string]] {
		return combinator.Map(
			combinator.Between(combinator.RuneLit('('), argsP, combinator.RuneLit(')')),
			func(args []string) combinator.Pair[string, []string] {
				var out []string
				for _, a := range args {
					if a != "" {
						out = append(out, a)
					}
				}
				return combinator.Pair[string, []string]{A: name, B: out}
			},
		)
	})
	got, err := combinator.Run(p, line)
	if err != nil {
		return "", nil, false
	}
	return strings.ToLower(got.A), got.B, true
}

func parseData(size string, fields []string) ([]byte, error) {
	var out []byte
	for _, f := range fields {
		f = strings.TrimSuffix(strings.TrimSpace(f), ",")
		if f == "" {
			continue
		}
		n, err := strconv.ParseUint(f, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value %q", size, f)
		}
		switch size {
		case "db":
			out = append(out, byte(n))
		case "dw":
			out = append(out, byte(n), byte(n>>8))
		case "dd":
			out = append(out, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
		default:
			for i := 0; i < 8; i++ {
				out = append(out, byte(n>>(8*i)))
			}
		}
	}
	return out, nil
}

// substituteWords replaces whole-word occurrences of defines.
func substituteWords(line string, defs map[string]string) string {
	if len(defs) == 0 {
		return line
	}
	for name, value := range defs {
		line = replaceWord(line, name, value)
	}
	return line
}

func replaceWord(s, word, repl string) string {
	if word == "" {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], word)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		beforeOK := j == 0 || !isIdentByte(s[j-1])
		after := j + len(word)
		afterOK := after >= len(s) || !isIdentByte(s[after])
		if beforeOK && afterOK {
			b.WriteString(s[i:j])
			b.WriteString(repl)
			i = after
			continue
		}
		b.WriteString(s[i : j+len(word)])
		i = j + len(word)
	}
	return b.String()
}

func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// Assemble encodes a section's instructions and data at base, resolving labels
// and external symbols. It returns the bytes and the section's label addresses.
func Assemble(s *Section, base uint64, symbols map[string]uint64) ([]byte, map[string]uint64, error) {
	labels := map[string]uint64{}
	for _, it := range s.Items {
		if it.Kind == KindLabel {
			labels[it.Name] = 0
		}
	}
	type emit struct {
		it   Item
		addr uint64
		n    int
	}
	var emits []emit
	cur := base
	for _, it := range s.Items {
		switch it.Kind {
		case KindLabel:
			labels[it.Name] = cur
		case KindData:
			emits = append(emits, emit{it: it, addr: cur, n: len(it.Data)})
			cur += uint64(len(it.Data))
		case KindInstruction:
			line := resolve(it.Text, labels, symbols)
			b, err := asm.Assemble(line, cur)
			if err != nil {
				return nil, nil, fmt.Errorf("autoasm: line %d: %w", it.Line, err)
			}
			emits = append(emits, emit{it: it, addr: cur, n: len(b)})
			cur += uint64(len(b))
		}
	}
	var out []byte
	for _, e := range emits {
		switch e.it.Kind {
		case KindData:
			out = append(out, e.it.Data...)
		case KindInstruction:
			b, err := asm.Assemble(resolve(e.it.Text, labels, symbols), e.addr)
			if err != nil {
				return nil, nil, fmt.Errorf("autoasm: line %d: %w", e.it.Line, err)
			}
			out = append(out, b...)
		}
	}
	return out, labels, nil
}

func resolve(line string, labels, symbols map[string]uint64) string {
	for name, addr := range labels {
		line = replaceWord(line, name, fmt.Sprintf("0x%x", addr))
	}
	for name, addr := range symbols {
		line = replaceWord(line, name, fmt.Sprintf("0x%x", addr))
	}
	return line
}
