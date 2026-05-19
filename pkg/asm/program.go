package asm

import (
	"strconv"
	"strings"
)

// AssembleProgram encodes a multi-line program with support for labels. A
// label is written `name:` at the start of a line and may be used as the
// target of a jump or call. It returns the encoded bytes and the resolved
// label addresses.
//
// Instruction lengths are independent of the branch targets (relative jumps
// always use a 32-bit displacement), so a single sizing pass is sufficient.
func AssembleProgram(text string, addr uint64) ([]byte, map[string]uint64, error) {
	lines := splitLines(text)
	labels := map[string]uint64{}

	// Collect label names so forward references resolve during sizing.
	for _, line := range lines {
		_, names := splitLabel(line)
		for _, name := range names {
			labels[name] = 0
		}
	}

	// Sizing pass: assign each label its address.
	cur := addr
	program := make([]string, 0, len(lines))
	for _, line := range lines {
		rest, names := splitLabel(line)
		for _, name := range names {
			labels[name] = cur
		}
		if rest == "" {
			continue
		}
		program = append(program, rest)
		b, err := assembleResolved(rest, cur, labels)
		if err != nil {
			return nil, nil, err
		}
		cur += uint64(len(b))
	}

	// Encoding pass.
	var out []byte
	cur = addr
	for _, line := range program {
		b, err := assembleResolved(line, cur, labels)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, b...)
		cur += uint64(len(b))
	}
	return out, labels, nil
}

func splitLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		out = append(out, strings.TrimSpace(stripComment(line)))
	}
	return out
}

// splitLabel removes any leading `label:` prefixes and returns the remaining
// instruction text along with the label names found.
func splitLabel(line string) (rest string, names []string) {
	rest = strings.TrimSpace(line)
	for {
		i := strings.IndexByte(rest, ':')
		if i < 0 {
			return
		}
		name := strings.TrimSpace(rest[:i])
		if name == "" || strings.ContainsAny(name, "[] ,+*") {
			return
		}
		names = append(names, name)
		rest = strings.TrimSpace(rest[i+1:])
		if rest == "" {
			return
		}
	}
}

func assembleResolved(line string, addr uint64, labels map[string]uint64) ([]byte, error) {
	mnem, rest := splitMnemonic(line)
	if rest == "" {
		return Assemble(line, addr)
	}
	toks := splitOperands(rest)
	for i, tok := range toks {
		if v, ok := labels[tok]; ok {
			toks[i] = "0x" + strconv.FormatUint(v, 16)
		}
	}
	return Assemble(mnem+" "+strings.Join(toks, ", "), addr)
}
