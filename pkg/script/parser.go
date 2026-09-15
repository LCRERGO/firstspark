package script

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/LCRERGO/firstspark/pkg/combinator"
)

var keywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true,
	"end": true, "false": true, "for": true, "function": true, "if": true,
	"in": true, "local": true, "nil": true, "not": true, "or": true,
	"repeat": true, "return": true, "then": true, "true": true,
	"until": true, "while": true,
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\v' || r == '\f'
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isHexDigit(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isAlpha(r rune) bool { return r == '_' || unicode.IsLetter(r) }

func isAlnum(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func parse(src string) ([]Stmt, error) {
	p := combinator.Map(
		combinator.Seq2(ws(), block()),
		func(pr combinator.Pair[struct{}, []Stmt]) []Stmt { return pr.B },
	)
	return combinator.Run(p, src)
}

func ws() combinator.Parser[struct{}] {
	return combinator.Skip(combinator.Many(combinator.Choice(space(), comment())))
}

func space() combinator.Parser[struct{}] {
	return combinator.Skip(combinator.Rune(isSpace))
}

func comment() combinator.Parser[struct{}] {
	short := combinator.Skip(combinator.Seq2(
		combinator.Str("--"),
		combinator.Many(combinator.Rune(func(r rune) bool { return r != '\n' })),
	))
	long := combinator.Skip(combinator.Seq2(combinator.Str("--"), longBracket()))
	return combinator.Choice(long, short)
}

func longBracket() combinator.Parser[string] {
	return combinator.Bind(
		combinator.Between(
			combinator.RuneLit('['),
			combinator.TakeWhile(func(r rune) bool { return r == '=' }),
			combinator.RuneLit('['),
		),
		func(eq string) combinator.Parser[string] {
			closer := "]" + eq + "]"
			return combinator.Between(
				combinator.Pure(struct{}{}),
				combinator.TakeUntil(combinator.Str(closer)),
				combinator.Str(closer),
			)
		},
	)
}

func lex[T any](p combinator.Parser[T]) combinator.Parser[T] {
	return combinator.Bind(ws(), func(struct{}) combinator.Parser[T] {
		return combinator.Bind(p, func(v T) combinator.Parser[T] {
			return combinator.Map(ws(), func(struct{}) T { return v })
		})
	})
}

func keyword(s string) combinator.Parser[string] {
	return lex(combinator.Bind(combinator.Str(s), func(string) combinator.Parser[string] {
		return combinator.Map(combinator.Not(combinator.Rune(isAlnum)), func(struct{}) string { return s })
	}))
}

func sym(s string) combinator.Parser[string] { return lex(combinator.Str(s)) }

func symDotDot() combinator.Parser[string] {
	return lex(combinator.Bind(combinator.Str(".."), func(string) combinator.Parser[string] {
		return combinator.Map(combinator.Not(combinator.RuneLit('.')), func(struct{}) string { return ".." })
	}))
}

func ident() combinator.Parser[string] {
	return lex(combinator.Bind(
		combinator.Seq2(combinator.Rune(isAlpha), combinator.TakeWhile(isAlnum)),
		func(p combinator.Pair[rune, string]) combinator.Parser[string] {
			name := string(p.A) + p.B
			if keywords[name] {
				return combinator.Fail[string]("unexpected keyword " + name)
			}
			return combinator.Pure(name)
		},
	))
}

func number() combinator.Parser[Expr] {
	return lex(combinator.Choice(hexNumber(), decNumber()))
}

func hexNumber() combinator.Parser[Expr] {
	return combinator.Map(
		combinator.Seq2(combinator.Choice(combinator.Str("0x"), combinator.Str("0X")), combinator.TakeWhile1(isHexDigit)),
		func(p combinator.Pair[string, string]) Expr {
			u, err := strconv.ParseUint(p.B, 16, 64)
			if err != nil {
				return &FloatLit{Value: 0}
			}
			return &IntLit{Value: int64(u)}
		},
	)
}

func decNumber() combinator.Parser[Expr] {
	const none = "\x00"
	frac := combinator.Optional(combinator.Bind(combinator.RuneLit('.'), func(rune) combinator.Parser[string] {
		return combinator.TakeWhile(isDigit)
	}), none)
	exp := combinator.Optional(combinator.Bind(
		combinator.Rune(func(r rune) bool { return r == 'e' || r == 'E' }),
		func(rune) combinator.Parser[string] {
			sign := combinator.Optional(combinator.Choice(combinator.Str("+"), combinator.Str("-")), "")
			return combinator.Map(
				combinator.Seq2(sign, combinator.TakeWhile1(isDigit)),
				func(p combinator.Pair[string, string]) string { return "e" + p.A + p.B },
			)
		},
	), none)
	return combinator.Map(
		combinator.Seq3(combinator.TakeWhile1(isDigit), frac, exp),
		func(t combinator.Triple[string, string, string]) Expr {
			text := t.A
			isFloat := false
			if t.B != none {
				text += "." + t.B
				isFloat = true
			}
			if t.C != none {
				text += t.C
				isFloat = true
			}
			if !isFloat {
				if n, err := strconv.ParseInt(text, 10, 64); err == nil {
					return &IntLit{Value: n}
				}
			}
			f, _ := strconv.ParseFloat(text, 64)
			return &FloatLit{Value: f}
		},
	)
}

func stringLit() combinator.Parser[Expr] {
	short := combinator.Choice(
		combinator.Map(combinator.Between(combinator.RuneLit('"'), quotedBody('"'), combinator.RuneLit('"')),
			func(s string) Expr { return &StrLit{Value: s} }),
		combinator.Map(combinator.Between(combinator.RuneLit('\''), quotedBody('\''), combinator.RuneLit('\'')),
			func(s string) Expr { return &StrLit{Value: s} }),
	)
	long := combinator.Map(longBracket(), func(s string) Expr { return &StrLit{Value: s} })
	return lex(combinator.Choice(long, short))
}

func quotedBody(quote rune) combinator.Parser[string] {
	esc := combinator.Bind(combinator.RuneLit('\\'), func(rune) combinator.Parser[string] {
		return combinator.Choice(
			combinator.Map(combinator.RuneLit('n'), func(rune) string { return "\n" }),
			combinator.Map(combinator.RuneLit('t'), func(rune) string { return "\t" }),
			combinator.Map(combinator.RuneLit('r'), func(rune) string { return "\r" }),
			combinator.Map(combinator.RuneLit('a'), func(rune) string { return "\a" }),
			combinator.Map(combinator.RuneLit('b'), func(rune) string { return "\b" }),
			combinator.Map(combinator.RuneLit('f'), func(rune) string { return "\f" }),
			combinator.Map(combinator.RuneLit('v'), func(rune) string { return "\v" }),
			combinator.Map(combinator.RuneLit('"'), func(rune) string { return `"` }),
			combinator.Map(combinator.RuneLit('\''), func(rune) string { return "'" }),
			combinator.Map(combinator.RuneLit('\\'), func(rune) string { return `\` }),
			combinator.Map(combinator.TakeWhileN(isDigit, 3), func(s string) string {
				n, _ := strconv.Atoi(s)
				return string(rune(byte(n)))
			}),
		)
	})
	plain := combinator.Map(
		combinator.Rune(func(r rune) bool { return r != quote && r != '\\' && r != '\n' }),
		func(r rune) string { return string(r) },
	)
	return combinator.Map(combinator.Many(combinator.Choice(esc, plain)),
		func(parts []string) string { return strings.Join(parts, "") })
}

func block() combinator.Parser[[]Stmt] {
	return combinator.Bind(combinator.Many(stat()), func(stmts []Stmt) combinator.Parser[[]Stmt] {
		return combinator.Bind(combinator.Optional(returnStat(), nil), func(ret []Stmt) combinator.Parser[[]Stmt] {
			return combinator.Pure(append(stmts, ret...))
		})
	})
}

func stat() combinator.Parser[Stmt] {
	return combinator.Lazy(func() combinator.Parser[Stmt] { return statParser() })
}

func statParser() combinator.Parser[Stmt] {
	return combinator.Choice(
		combinator.Map(sym(";"), func(string) Stmt { return &DoStmt{} }),
		doStat(),
		whileStat(),
		repeatStat(),
		ifStat(),
		forStat(),
		functionStat(),
		localStat(),
		combinator.Map(sym("break"), func(string) Stmt { return &BreakStmt{} }),
		assignOrCallStat(),
	)
}

func doStat() combinator.Parser[Stmt] {
	return combinator.Map(combinator.Seq3(sym("do"), block(), sym("end")),
		func(t combinator.Triple[string, []Stmt, string]) Stmt { return &DoStmt{Body: t.B} })
}

func whileStat() combinator.Parser[Stmt] {
	return combinator.Bind(combinator.Seq3(sym("while"), exp(), sym("do")),
		func(t combinator.Triple[string, Expr, string]) combinator.Parser[Stmt] {
			return combinator.Bind(block(), func(body []Stmt) combinator.Parser[Stmt] {
				return combinator.Map(sym("end"), func(string) Stmt { return &WhileStmt{Cond: t.B, Body: body} })
			})
		})
}

func repeatStat() combinator.Parser[Stmt] {
	return combinator.Bind(combinator.Seq2(sym("repeat"), block()),
		func(p combinator.Pair[string, []Stmt]) combinator.Parser[Stmt] {
			return combinator.Map(combinator.Seq2(sym("until"), exp()),
				func(q combinator.Pair[string, Expr]) Stmt { return &RepeatStmt{Body: p.B, Cond: q.B} })
		})
}

func ifStat() combinator.Parser[Stmt] {
	return combinator.Bind(combinator.Seq3(sym("if"), exp(), sym("then")),
		func(t combinator.Triple[string, Expr, string]) combinator.Parser[Stmt] {
			return combinator.Bind(block(), func(then []Stmt) combinator.Parser[Stmt] {
				return combinator.Bind(elseIfs(), func(eis []ElseIf) combinator.Parser[Stmt] {
					return combinator.Bind(combinator.Optional(elseBlock(), nil), func(els []Stmt) combinator.Parser[Stmt] {
						return combinator.Map(sym("end"), func(string) Stmt {
							return &IfStmt{Cond: t.B, Then: then, ElseIfs: eis, Else: els}
						})
					})
				})
			})
		})
}

func elseIfs() combinator.Parser[[]ElseIf] {
	return combinator.Many(combinator.Bind(combinator.Seq3(sym("elseif"), exp(), sym("then")),
		func(t combinator.Triple[string, Expr, string]) combinator.Parser[ElseIf] {
			return combinator.Map(block(), func(body []Stmt) ElseIf { return ElseIf{Cond: t.B, Body: body} })
		}))
}

func elseBlock() combinator.Parser[[]Stmt] {
	return combinator.Map(combinator.Seq2(sym("else"), block()),
		func(p combinator.Pair[string, []Stmt]) []Stmt { return p.B })
}

func forStat() combinator.Parser[Stmt] {
	numeric := combinator.Bind(combinator.Seq3(sym("for"), ident(), sym("=")),
		func(t combinator.Triple[string, string, string]) combinator.Parser[Stmt] {
			return combinator.Bind(combinator.Seq2(exp(), sym(",")), func(p combinator.Pair[Expr, string]) combinator.Parser[Stmt] {
				return combinator.Bind(exp(), func(limit Expr) combinator.Parser[Stmt] {
					return combinator.Bind(combinator.Optional(combinator.Seq2(sym(","), exp()), combinator.Pair[string, Expr]{}),
						func(step combinator.Pair[string, Expr]) combinator.Parser[Stmt] {
							return combinator.Bind(sym("do"), func(string) combinator.Parser[Stmt] {
								return combinator.Bind(block(), func(body []Stmt) combinator.Parser[Stmt] {
									return combinator.Map(sym("end"), func(string) Stmt {
										return &NumForStmt{Name: t.B, Start: p.A, Limit: limit, Step: step.B, Body: body}
									})
								})
							})
						})
				})
			})
		})
	generic := combinator.Bind(combinator.Seq2(sym("for"), combinator.SepBy1(ident(), sym(","))),
		func(p combinator.Pair[string, []string]) combinator.Parser[Stmt] {
			return combinator.Bind(combinator.Seq2(sym("in"), explist()), func(q combinator.Pair[string, []Expr]) combinator.Parser[Stmt] {
				return combinator.Bind(sym("do"), func(string) combinator.Parser[Stmt] {
					return combinator.Bind(block(), func(body []Stmt) combinator.Parser[Stmt] {
						return combinator.Map(sym("end"), func(string) Stmt {
							return &GenForStmt{Names: p.B, Exprs: q.B, Body: body}
						})
					})
				})
			})
		})
	return combinator.Choice(numeric, generic)
}

func functionStat() combinator.Parser[Stmt] {
	return combinator.Map(combinator.Seq2(sym("function"), funcName()),
		func(p combinator.Pair[string, funcNameResult]) Stmt {
			return &FuncStmt{Target: p.B.target, Method: p.B.method, Fn: p.B.body}
		})
}

type funcNameResult struct {
	target Expr
	method string
	body   *FuncLit
}

func funcName() combinator.Parser[funcNameResult] {
	return combinator.Bind(ident(), func(first string) combinator.Parser[funcNameResult] {
		return combinator.Bind(combinator.Many(combinator.Bind(sym("."), func(string) combinator.Parser[string] {
			return ident()
		})), func(path []string) combinator.Parser[funcNameResult] {
			target := Expr(&Name{Name: first})
			for _, n := range path {
				target = &Index{X: target, Key: &StrLit{Value: n}}
			}
			return combinator.Bind(combinator.Optional(combinator.Seq2(sym(":"), ident()), combinator.Pair[string, string]{}),
				func(m combinator.Pair[string, string]) combinator.Parser[funcNameResult] {
					method := m.B
					if method != "" {
						target = &Index{X: target, Key: &StrLit{Value: method}}
					}
					return combinator.Map(funcBody(), func(fn *FuncLit) funcNameResult {
						return funcNameResult{target: target, method: method, body: fn}
					})
				})
		})
	})
}

func localStat() combinator.Parser[Stmt] {
	localFunc := combinator.Bind(combinator.Seq3(sym("local"), sym("function"), ident()),
		func(t combinator.Triple[string, string, string]) combinator.Parser[Stmt] {
			return combinator.Map(funcBody(), func(fn *FuncLit) Stmt { return &LocalFuncStmt{Name: t.C, Fn: fn} })
		})
	localVars := combinator.Bind(combinator.Seq2(sym("local"), combinator.SepBy1(ident(), sym(","))),
		func(p combinator.Pair[string, []string]) combinator.Parser[Stmt] {
			withValues := combinator.Map(combinator.Seq2(sym("="), explist()),
				func(q combinator.Pair[string, []Expr]) []Expr { return q.B })
			return combinator.Map(combinator.Optional(withValues, nil),
				func(e []Expr) Stmt { return &LocalStmt{Names: p.B, Exprs: e} })
		})
	return combinator.Choice(localFunc, localVars)
}

func returnStat() combinator.Parser[[]Stmt] {
	return combinator.Map(
		combinator.Seq3(sym("return"), combinator.Optional(explist(), nil), combinator.Optional(sym(";"), "")),
		func(t combinator.Triple[string, []Expr, string]) []Stmt {
			return []Stmt{&ReturnStmt{Exprs: t.B}}
		})
}

func assignOrCallStat() combinator.Parser[Stmt] {
	return combinator.Bind(prefixExp(), func(first Expr) combinator.Parser[Stmt] {
		callStmt := func(in combinator.Input) (Stmt, combinator.Input, bool) {
			c, ok := first.(*Call)
			if !ok {
				return nil, in, false
			}
			return &CallStmt{Call: c}, in, true
		}
		assign := combinator.Bind(combinator.Many(combinator.Seq2(sym(","), prefixExp())),
			func(rest []combinator.Pair[string, Expr]) combinator.Parser[Stmt] {
				return combinator.Bind(sym("="), func(string) combinator.Parser[Stmt] {
					return combinator.Map(explist(), func(rhs []Expr) Stmt {
						targets := []Expr{first}
						for _, p := range rest {
							targets = append(targets, p.B)
						}
						return &AssignStmt{Targets: targets, Exprs: rhs}
					})
				})
			})
		return combinator.Choice(assign, callStmt)
	})
}

func explist() combinator.Parser[[]Expr] { return combinator.SepBy1(exp(), sym(",")) }

func exp() combinator.Parser[Expr] {
	return combinator.Lazy(func() combinator.Parser[Expr] { return parseBin(1) })
}

type opInfo struct {
	prec       int
	rightAssoc bool
}

var binops = map[string]opInfo{
	"or": {1, false}, "and": {2, false},
	"<": {3, false}, ">": {3, false}, "<=": {3, false}, ">=": {3, false}, "~=": {3, false}, "==": {3, false},
	"..": {4, true},
	"+":  {5, false}, "-": {5, false},
	"*": {6, false}, "/": {6, false}, "%": {6, false},
	"^": {8, true},
}

func parseBin(minPrec int) combinator.Parser[Expr] {
	return combinator.Bind(unaryExp(), func(lhs Expr) combinator.Parser[Expr] {
		return binRest(lhs, minPrec)
	})
}

func binRest(lhs Expr, minPrec int) combinator.Parser[Expr] {
	return combinator.Choice(
		combinator.Bind(binOpMin(minPrec), func(op string) combinator.Parser[Expr] {
			info := binops[op]
			nextMin := info.prec + 1
			if info.rightAssoc {
				nextMin = info.prec
			}
			return combinator.Bind(parseBin(nextMin), func(rhs Expr) combinator.Parser[Expr] {
				return binRest(&Binop{Op: op, L: lhs, R: rhs}, minPrec)
			})
		}),
		combinator.Pure(lhs),
	)
}

func binOpMin(minPrec int) combinator.Parser[string] {
	return func(in combinator.Input) (string, combinator.Input, bool) {
		op, rest, ok := binOp()(in)
		if !ok || binops[op].prec < minPrec {
			return "", in, false
		}
		return op, rest, true
	}
}

func binOp() combinator.Parser[string] {
	ps := []combinator.Parser[string]{keyword("and"), keyword("or"), symDotDot()}
	for _, s := range []string{"==", "~=", "<=", ">=", "<", ">", "+", "-", "*", "/", "%", "^"} {
		ps = append(ps, sym(s))
	}
	return combinator.Choice(ps...)
}

func unaryExp() combinator.Parser[Expr] {
	return combinator.Choice(
		combinator.Bind(unaryOp(), func(op string) combinator.Parser[Expr] {
			return combinator.Map(parseBin(7), func(x Expr) Expr { return &Unop{Op: op, X: x} })
		}),
		simpleExp(),
	)
}

func unaryOp() combinator.Parser[string] {
	return combinator.Choice(keyword("not"), sym("#"), sym("-"))
}

func simpleExp() combinator.Parser[Expr] {
	return combinator.Choice(
		combinator.Map(keyword("nil"), func(string) Expr { return &NilLit{} }),
		combinator.Map(keyword("true"), func(string) Expr { return &BoolLit{Value: true} }),
		combinator.Map(keyword("false"), func(string) Expr { return &BoolLit{Value: false} }),
		number(),
		stringLit(),
		combinator.Map(sym("..."), func(string) Expr { return &Vararg{} }),
		functionExp(),
		tableConstructor(),
		prefixExp(),
	)
}

func functionExp() combinator.Parser[Expr] {
	return combinator.Map(combinator.Seq2(sym("function"), funcBody()),
		func(p combinator.Pair[string, *FuncLit]) Expr { return p.B })
}

func funcBody() combinator.Parser[*FuncLit] {
	return combinator.Bind(combinator.Between(sym("("), params(), sym(")")),
		func(pr paramsResult) combinator.Parser[*FuncLit] {
			return combinator.Bind(block(), func(body []Stmt) combinator.Parser[*FuncLit] {
				return combinator.Map(sym("end"), func(string) *FuncLit {
					return &FuncLit{Params: pr.names, Vararg: pr.vararg, Body: body}
				})
			})
		})
}

type paramsResult struct {
	names  []string
	vararg bool
}

func params() combinator.Parser[paramsResult] {
	withNames := combinator.Bind(combinator.SepBy1(ident(), sym(",")), func(names []string) combinator.Parser[paramsResult] {
		return combinator.Choice(
			combinator.Map(combinator.Seq2(sym(","), sym("...")), func(combinator.Pair[string, string]) paramsResult {
				return paramsResult{names: names, vararg: true}
			}),
			combinator.Pure(paramsResult{names: names}),
		)
	})
	varargOnly := combinator.Map(sym("..."), func(string) paramsResult { return paramsResult{vararg: true} })
	return combinator.Choice(withNames, varargOnly, combinator.Pure(paramsResult{}))
}

func tableConstructor() combinator.Parser[Expr] {
	return combinator.Map(combinator.Between(sym("{"), fields(), sym("}")),
		func(f []TableField) Expr { return &TableLit{Fields: f} })
}

func fields() combinator.Parser[[]TableField] {
	return combinator.Optional(combinator.SepBy(field(), combinator.Choice(sym(","), sym(";"))), nil)
}

func field() combinator.Parser[TableField] {
	keyed := combinator.Choice(
		combinator.Map(combinator.Seq3(sym("["), exp(), sym("]")),
			func(t combinator.Triple[string, Expr, string]) Expr { return t.B }),
		combinator.Map(combinator.Seq2(ident(), sym("=")),
			func(p combinator.Pair[string, string]) Expr { return &StrLit{Value: p.A} }),
	)
	withKey := combinator.Bind(keyed, func(k Expr) combinator.Parser[TableField] {
		return combinator.Map(combinator.Seq2(sym("="), exp()), func(p combinator.Pair[string, Expr]) TableField {
			return TableField{Key: k, Value: p.B}
		})
	})
	array := combinator.Map(exp(), func(e Expr) TableField { return TableField{Value: e} })
	return combinator.Choice(withKey, array)
}

func prefixExp() combinator.Parser[Expr] {
	return combinator.Bind(primaryExp(), func(base Expr) combinator.Parser[Expr] {
		return suffixed(base)
	})
}

func primaryExp() combinator.Parser[Expr] {
	return combinator.Choice(
		combinator.Map(combinator.Between(sym("("), exp(), sym(")")), func(e Expr) Expr { return e }),
		combinator.Map(ident(), func(s string) Expr { return &Name{Name: s} }),
	)
}

func suffixed(base Expr) combinator.Parser[Expr] {
	return combinator.Choice(
		combinator.Bind(suffix(base), func(next Expr) combinator.Parser[Expr] { return suffixed(next) }),
		combinator.Pure(base),
	)
}

func suffix(base Expr) combinator.Parser[Expr] {
	return combinator.Choice(
		combinator.Map(combinator.Between(sym("["), exp(), sym("]")),
			func(k Expr) Expr { return &Index{X: base, Key: k} }),
		combinator.Map(combinator.Seq2(sym("."), ident()),
			func(p combinator.Pair[string, string]) Expr { return &Index{X: base, Key: &StrLit{Value: p.B}} }),
		combinator.Map(combinator.Seq3(sym(":"), ident(), args()),
			func(t combinator.Triple[string, string, []Expr]) Expr {
				return &Call{Fn: &Index{X: base, Key: &StrLit{Value: t.B}}, Method: t.B, Args: t.C}
			}),
		combinator.Map(args(), func(a []Expr) Expr { return &Call{Fn: base, Args: a} }),
	)
}

func args() combinator.Parser[[]Expr] {
	return combinator.Choice(
		combinator.Between(sym("("), combinator.Optional(explist(), nil), sym(")")),
		combinator.Map(tableConstructor(), func(e Expr) []Expr { return []Expr{e} }),
		combinator.Map(stringLit(), func(e Expr) []Expr { return []Expr{e} }),
	)
}
