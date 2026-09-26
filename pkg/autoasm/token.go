package autoasm

import "strings"

// TokenKind classifies a token for syntax highlighting.
type TokenKind uint8

const (
	// TokenPlain is an instruction or unclassified text.
	TokenPlain TokenKind = iota
	// TokenDirective is an Auto Assembler directive.
	TokenDirective
	// TokenSection is an [ENABLE]/[DISABLE] header.
	TokenSection
	// TokenLabel is a `name:` label.
	TokenLabel
	// TokenNumber is a numeric literal.
	TokenNumber
	// TokenString is a string literal.
	TokenString
	// TokenComment is a comment.
	TokenComment
)

// Token is a lexeme with rune offsets into the source.
type Token struct {
	Kind       TokenKind
	Start, End int
	Text       string
}

var directives = map[string]bool{
	"alloc": true, "dealloc": true, "label": true, "define": true,
	"registersymbol": true, "unregistersymbol": true, "aobscan": true,
	"aobscanmodule": true, "createthread": true, "globalalloc": true,
	"db": true, "dw": true, "dd": true, "dq": true, "nop": true,
}

// Tokenize splits an Auto Assembler script into tokens for highlighting.
func Tokenize(src string) []Token {
	r := []rune(src)
	var out []Token
	for i := 0; i < len(r); {
		start := i
		switch {
		case (r[i] == '/' && i+1 < len(r) && r[i+1] == '/') || r[i] == ';':
			i = scanLineComment(r, i)
			out = append(out, Token{TokenComment, start, i, string(r[start:i])})
		case r[i] == '"' || r[i] == '\'':
			i = scanQuoted(r, i)
			out = append(out, Token{TokenString, start, i, string(r[start:i])})
		case r[i] == '[':
			i = scanBracket(r, i)
			text := string(r[start:i])
			kind := TokenPlain
			if isSection(text) {
				kind = TokenSection
			}
			out = append(out, Token{kind, start, i, text})
		case r[i] >= '0' && r[i] <= '9':
			i = scanWord(r, i)
			out = append(out, Token{TokenNumber, start, i, string(r[start:i])})
		case isWordStart(r[i]):
			i = scanWord(r, i)
			word := string(r[start:i])
			out = append(out, Token{wordKind(r, i, word), start, i, word})
		default:
			i++
		}
	}
	return out
}

// scanLineComment consumes a `//` or `;` comment up to the newline.
func scanLineComment(r []rune, i int) int {
	for i < len(r) && r[i] != '\n' {
		i++
	}
	return i
}

// scanBracket consumes a `[...]` run.
func scanBracket(r []rune, i int) int {
	for i < len(r) && r[i] != ']' && r[i] != '\n' {
		i++
	}
	if i < len(r) && r[i] == ']' {
		i++
	}
	return i
}

// scanWord consumes an identifier, directive or numeric run.
func scanWord(r []rune, i int) int {
	for i < len(r) && isWordByte(r[i]) {
		i++
	}
	return i
}

// wordKind classifies a word as a label, directive or plain text.
func wordKind(r []rune, i int, word string) TokenKind {
	if i < len(r) && r[i] == ':' {
		return TokenLabel
	}
	if directives[strings.ToLower(word)] {
		return TokenDirective
	}
	return TokenPlain
}

func isSection(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "[ENABLE]", "[DISABLE]":
		return true
	}
	return false
}

func isWordStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isWordByte(r rune) bool {
	return isWordStart(r) || (r >= '0' && r <= '9')
}

func scanQuoted(r []rune, i int) int {
	quote := r[i]
	i++
	for i < len(r) {
		if r[i] == '\\' {
			i += 2
			continue
		}
		if r[i] == quote {
			return i + 1
		}
		i++
	}
	return len(r)
}
