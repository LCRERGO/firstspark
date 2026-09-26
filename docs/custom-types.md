# Custom value types

A custom type teaches the scanner how to interpret a fixed number of bytes as a
value. It is a name, a byte size, a result kind, and a small script that
converts bytes to a value and back. Custom types appear in the **Value Type**
dropdown next to the built-ins.

Types are stored in `customtypes.yaml` under the Firstspark config directory
(`$XDG_CONFIG_HOME/firstspark/customtypes.yaml`). Edit them from the manager
(the **…** button beside the Value Type dropdown, or **Table → Custom Types**),
or by hand.

## File format

```yaml
types:
  - name: Money
    size: 4
    kind: float
    alignment: 4
    description: A 32-bit integer scaled by 1/100.
    script: |
      function bytes_to_value(bytes, address)
        local raw = bytes[1] + bytes[2]*256 + bytes[3]*65536 + bytes[4]*16777216
        return raw / 100
      end
      function value_to_bytes(value, address)
        local raw = math.floor(value * 100)
        return { raw % 256, math.floor(raw/256) % 256, math.floor(raw/65536) % 256, math.floor(raw/16777216) % 256 }
      end
```

| Field | Required | Meaning |
| --- | --- | --- |
| `name` | yes | Display name; must be unique. |
| `size` | yes | Number of bytes read per candidate. Must be positive. |
| `kind` | no | `int` (default), `float` or `string`; selects how values are compared. |
| `script` | yes | The Lua script defining the conversions. |
| `alignment` | no | Preferred scan alignment in bytes; defaults to the type size. |
| `max_string_size` | no | Conversion buffer for a string Auto Assembler type (default 64). |
| `description` | no | Free text shown in the manager. |

## The script

The script is a **Lua 5.1.4-compatible subset** (see ADR 0012). It must define:

- `bytes_to_value(bytes, address)` — **required**. Receives the raw bytes and
  the address the value was read from, and returns a number (or a string when
  `kind` is `string`).
- `value_to_bytes(value, address)` — **optional**. Receives the value the user
  typed and the address, and returns a 1-indexed table of byte integers. A type
  without this function is **read-only**: it can be scanned but not edited or
  frozen.

`bytes` is a 1-indexed table of integers, so the first byte is `bytes[1]`, the
second `bytes[2]`, and so on.

The language supports: `nil`, booleans, 64-bit integers and floats, strings,
tables, functions and closures, `local`/global variables, `if`/`elseif`/`else`,
`while`, `repeat`, numeric and generic `for`, `break`, `return`, varargs,
arithmetic (`+ - * / % ^`), comparisons, `and`/`or`/`not`, concatenation, and
the `string`, `math` and `table` libraries (including `math.floor`,
`math.hugeint` and `string.format`).

It is sandboxed: there is no `io`, `os`, `debug`, `require`, metatables,
coroutines or `pcall`. Integer arithmetic stays exact; `/` and `^` produce
floats.

### Comparison semantics

Comparisons (`==`, `!=`, `>`, `>=`, `<`, `<=`, increased/decreased, value
between) run on the **converted** value:

- `kind: int` compares the returned number as a signed integer,
- `kind: float` compares it as a float with the configured epsilon,
- `kind: string` compares the returned string lexicographically.

This is why a type such as `Money` behaves like a number rather than raw bytes.

## Examples

**16-bit big-endian integer**

```lua
function bytes_to_value(bytes, address)
  return bytes[1]*256 + bytes[2]
end
function value_to_bytes(value, address)
  return { math.floor(value/256) % 256, value % 256 }
end
```

**24-bit little-endian integer**

```lua
function bytes_to_value(bytes, address)
  return bytes[1] + bytes[2]*256 + bytes[3]*65536
end
function value_to_bytes(value, address)
  return { value % 256, math.floor(value/256) % 256, math.floor(value/65536) % 256 }
end
```

**Bit flags (read-only)**

```lua
function bytes_to_value(bytes, address)
  local v = bytes[1]
  if v % 2 == 1 then return 1 end
  return 0
end
```

## Testing

The manager's **Test** panel converts without scanning:

- **Bytes → value**: type a hex byte string such as `39 30 00 00` and see the
  converted value.
- **Read & convert**: when a process is selected, read the type's byte size at
  an address and convert it.
- **Round-trip**: convert a value back to bytes with `value_to_bytes` and show
  the result, so a broken write-back is caught immediately.

A **Check** action compiles the script and reports parse errors.

## Auto Assembler types

A type can instead be defined by an **Auto Assembler** script (`mode: aa`). The
script's `[ENABLE]` section must define a `ConvertRoutine` label and may define
a `ConvertBackRoutine` label; the code is assembled and loaded into Firstspark
itself (a small local JIT), so conversions run at native speed and no target
process is required.

```yaml
  - name: Big Endian 2
    size: 2
    kind: int
    mode: aa
    script: |
      [ENABLE]
      ConvertRoutine:
        ; rdi = pointer to the bytes; return the value in rax
        movzx eax, byte ptr [rdi]
        shl eax, 8
        movzx ecx, byte ptr [rdi+1]
        or eax, ecx
        ret

      ConvertBackRoutine:
        ; rdi = value; rsi = pointer to the output bytes
        mov eax, edi
        mov [rsi+1], al
        shr eax, 8
        mov [rsi], al
        ret
```

Conventions:

- `ConvertRoutine` receives a pointer to the value's bytes in **RDI** and
  returns the value in **RAX**.
- `ConvertBackRoutine` receives the value in **RDI** and a pointer to the
  output bytes in **RSI**; it writes `size` bytes.
- `kind: int` and `kind: float` are supported. A float type's routine returns the
  IEEE-754 single bit pattern as an integer, and the value is interpreted as a
  32-bit float (Cheat Engine's `USESFLOAT` convention).
- `kind: string` takes three arguments instead: the value pointer in **RDI**,
  the address (always 0 here) in **RSI**, and the output pointer in **RDX**.
  `ConvertRoutine` writes a NUL-terminated string to the output buffer;
  `ConvertBackRoutine` writes `size` bytes to the output pointer. Set
  `max_string_size` to bound the conversion buffer (default 64). This matches
  Cheat Engine's `USESSTRING` convention.
- The script must be self-contained (no external symbols); `alloc` directives
  are ignored.

Because the routine runs in Firstspark's process, a faulty script can crash the
application — the same trust model as Cheat Engine's Auto Assembler. The CGO
build (`make`) executes the routine natively through `pkg/jit`; the headless
build interprets the same common instruction subset in pure Go (`pkg/aaexec`)
and rejects routines outside it, so a headless build still supports Auto
Assembler types without CGO.

## Differences from Cheat Engine

- The script dialect is Lua 5.1.4 with a 64-bit integer extension, matching
  Cheat Engine, but the sandbox omits `io`/`os`/`require`/metatables/`pcall`
  (ADR 0012).
- Auto-Assembler-defined types are not supported; only Lua scripts (ADR 0020).
- Cheat Engine keeps types in the Windows registry; Firstspark keeps them in
  `customtypes.yaml`.
