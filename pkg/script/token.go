package script

// TokenKind classifies a token for syntax highlighting.
type TokenKind uint8

const (
	// TokenPlain is unclassified text.
	TokenPlain TokenKind = iota
	// TokenKeyword is a reserved word.
	TokenKeyword
	// TokenIdentifier is a name.
	TokenIdentifier
	// TokenNumber is a numeric literal.
	TokenNumber
	// TokenString is a string literal.
	TokenString
	// TokenComment is a comment.
	TokenComment
	// TokenOperator is punctuation or an operator.
	TokenOperator
)

// Token is a lexeme with rune offsets into the source.
type Token struct {
	Kind       TokenKind
	Start, End int
	Text       string
}

// Tokenize splits src into tokens for highlighting. It never fails; unknown
// input is emitted as operator tokens.
func Tokenize(src string) []Token {
	r := []rune(src)
	var out []Token
	for i := 0; i < len(r); {
		start := i
		switch {
		case r[i] == '-' && i+1 < len(r) && r[i+1] == '-':
			i = scanComment(r, i)
			out = append(out, Token{TokenComment, start, i, string(r[start:i])})
		case r[i] == '"' || r[i] == '\'':
			i = scanQuoted(r, i)
			out = append(out, Token{TokenString, start, i, string(r[start:i])})
		case r[i] == '[' && i+1 < len(r) && (r[i+1] == '[' || r[i+1] == '='):
			if end, ok := longBracketEnd(r, i); ok {
				i = end
				out = append(out, Token{TokenString, start, i, string(r[start:i])})
			} else {
				i++
			}
		case isDigit(r[i]):
			i = scanNumber(r, i)
			out = append(out, Token{TokenNumber, start, i, string(r[start:i])})
		case isAlpha(r[i]):
			i = scanIdentifier(r, i)
			text := string(r[start:i])
			kind := TokenIdentifier
			if keywords[text] {
				kind = TokenKeyword
			}
			out = append(out, Token{kind, start, i, text})
		case isSpace(r[i]):
			i++
		default:
			i++
			out = append(out, Token{TokenOperator, start, i, string(r[start:i])})
		}
	}
	return out
}

// scanComment consumes a `--` comment, including a long-bracket body.
func scanComment(r []rune, i int) int {
	i += 2
	if i < len(r) && r[i] == '[' {
		if end, ok := longBracketEnd(r, i); ok {
			return end
		}
	}
	for i < len(r) && r[i] != '\n' {
		i++
	}
	return i
}

// scanNumber consumes a numeric literal.
func scanNumber(r []rune, i int) int {
	for i < len(r) && (isHexDigit(r[i]) || r[i] == '.' || r[i] == 'x' || r[i] == 'X') {
		i++
	}
	return i
}

// scanIdentifier consumes an identifier or keyword.
func scanIdentifier(r []rune, i int) int {
	for i < len(r) && isAlnum(r[i]) {
		i++
	}
	return i
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

func longBracketEnd(r []rune, i int) (int, bool) {
	if i >= len(r) || r[i] != '[' {
		return 0, false
	}
	j := i + 1
	eq := 0
	for j < len(r) && r[j] == '=' {
		eq++
		j++
	}
	if j >= len(r) || r[j] != '[' {
		return 0, false
	}
	j++
	for j < len(r) {
		if r[j] == ']' {
			k := j + 1
			n := 0
			for k < len(r) && r[k] == '=' {
				n++
				k++
			}
			if n == eq && k < len(r) && r[k] == ']' {
				return k + 1, true
			}
		}
		j++
	}
	return len(r), true
}
