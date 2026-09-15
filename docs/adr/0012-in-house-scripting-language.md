# ADR 0012: In-house Lua 5.1.4-compatible scripting language

## Status

Accepted.

## Context

User-defined value types need per-type conversion logic (bytes to a comparable
value and back). Cheat Engine implements these as Lua scripts executed by an
embedded Lua. We want the same capability without depending on a third-party
Lua VM, and we want scripts that are portable with Cheat Engine's custom types.

Cheat Engine 7.5 embeds **patched Lua 5.1.4** (`LUA_VERSION_NUM 501`) with the
"lnum" number-model patch (`LNUM_INT64`): numbers are C doubles plus a separate
signed 64-bit integer subtype, and `math.hugeint` is
`0x7fffffffffffffff`. It has no 5.2/5.3 features (no bitwise operators, no
`//`, no `goto`, no `math.type`/`math.maxinteger`), and it opens the full stock
5.1 library set (base + coroutine, package/require, table, io, os, string,
math, debug) with metatables and `pcall`.

## Decision

Implement a small scripting language in-tree:

- `pkg/combinator` — hand-rolled, dependency-free parser combinators.
- `pkg/script` — a **Lua 5.1.4 + int64** subset: tokenizer/parser (combinators)
  producing an AST, and a **closure compiler** that lowers the AST to Go
  closures, called once per scan candidate without an interpreter loop.

Supported: nil/boolean/integer+float/string/table/function, closures,
`local`/global, `if`/`elseif`/`else`, `while`, `repeat`, numeric and generic
`for`, `break`, `return`, varargs, arithmetic `+ - * / % ^`, unary `-`, `#`,
comparisons, `and`/`or`/`not`, concatenation, `string.*`, `math.*` (including
`math.hugeint`), `table.*`.

Excluded for safety (sandboxed): bitwise operators, `//`, `goto`, coroutines,
metatables, `pcall`/`error`, `require`/modules, `io`/`os`/`debug`,
`string.pack`, `utf8`, `string.dump`.

## Gap versus Cheat Engine

- CE opens the full 5.1 stdlib, including metatables, `pcall`, `require`,
  `io` and `os`; we sandbox those out, so a CE script using `class()`,
  metatables, `pcall`, `require`, `io`/`os` or coroutines will not run here.
- CE's lnum integer/float promotion rules are approximated (integer `+ - *`
  stay integer unless they overflow, `/` and `^` produce floats); exact parity
  is refined as cases are found.
- Typical custom-type scripts (`%`, `math.floor`, `string.format`, plain
  arithmetic) run in both.

## Consequences

- No third-party Lua dependency, no CGO, and the engine stays headless-safe.
- The language and the highlighter share one grammar, so they cannot drift.
- We own the runtime: numeric edge cases and builtins are ours to maintain.
