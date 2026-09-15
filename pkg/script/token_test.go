package script

import "testing"

func TestTokenize(t *testing.T) {
	toks := Tokenize("local x = 1 -- note\nreturn \"a\" .. x")
	kinds := map[string]TokenKind{}
	for _, tok := range toks {
		kinds[tok.Text] = tok.Kind
	}
	if kinds["local"] != TokenKeyword || kinds["return"] != TokenKeyword {
		t.Fatalf("keywords: %+v", kinds)
	}
	if kinds["x"] != TokenIdentifier {
		t.Fatalf("identifier: %+v", kinds)
	}
	if kinds["1"] != TokenNumber {
		t.Fatalf("number: %+v", kinds)
	}
	if kinds[`"a"`] != TokenString {
		t.Fatalf("string: %+v", kinds)
	}
	found := false
	for _, tok := range toks {
		if tok.Kind == TokenComment {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a comment token")
	}
}

func TestTokenizeLongComment(t *testing.T) {
	toks := Tokenize("--[[ multi\nline ]] x")
	if len(toks) == 0 || toks[0].Kind != TokenComment {
		t.Fatalf("expected a leading comment token: %+v", toks)
	}
}
