# ADR 0048: Applying Cheat Engine custom-type conversions

## Status

Accepted. P0–P4 are implemented.

## Context

A Cheat Engine `.CT` can define custom value types with
`registerCustomTypeAutoAssembler(script)`. The script allocates a set of
symbols and defines two conversion routines in x86-64 assembly:

| Symbol | Meaning |
| --- | --- |
| `TypeName` | the display name (string data) |
| `ByteSize` | the value width in bytes |
| `PREFEREDALIGNMENT` | scan alignment override |
| `ConvertRoutine` | bytes → integer |
| `ConvertBackRoutine` | integer → bytes |
| `USESFLOAT` | the integer result is a float bit pattern |
| `USESSTRING`, `MAXSTRINGSIZE` | string conversion via an output buffer |
| `CALLMETHOD` | the routines take an extra `address` argument (cdecl) |

CE calls the routines with the Windows x64 convention; the routine shapes it
uses are (`CustomTypeHandler.pas`):

```
TConversionRoutine        = function(data: pointer): integer; stdcall;
TReverseConversionRoutine = procedure(i: integer; output: pointer); stdcall;
TConversionRoutine2       = function(data: pointer; address: ptruint): integer; cdecl;
TReverseConversionRoutine2= procedure(i: integer; address: ptruint; output: pointer); cdecl;
TConversionRoutineString  = procedure(data: pointer; address: ptruint; output: pchar); cdecl;
```

Firstspark already has an almost identical mechanism: `pkg/customtype.RegisterAA`
(ADR 0023) assembles an Auto Assembler `[ENABLE]` section, loads it with
`pkg/jit` and registers a type backed by labels `ConvertRoutine` and
`ConvertBackRoutine`. Its ABI differs:

- read: `RDI = bytes pointer`, result in `RAX`;
- write: `RDI = value`, `RSI = output pointer`.

Today, `pkg/cheattable` extracts `TypeName`/`ByteSize` (and now the routine
bodies) into `CustomTypeDef`, and `internal/ui/files.go` registers every CE
custom type with `customtype.RegisterRaw` — the name and width survive but the
conversion does not run.

## Proposed decision

Bridge the two ABIs and reuse the existing JIT path, in phases. A new
`customtype.RegisterCE(def)` reconstructs a Firstspark Auto Assembler
definition from the CE script:

1. synthesise a `[ENABLE]` section containing `TypeName`, `ByteSize`, alignment
   and feature flags, the extracted routine bodies, and a SysV shim in front of
   each CE routine that moves the arguments from `RDI`/`RSI` to `RCX`/`RDX`/`R8`
   (and, for `CALLMETHOD`, supplies `address = 0`), keeping the stack 16-byte
   aligned;
2. hand it to `RegisterAA` on CGO builds;
3. fall back to `RegisterRaw` and record an import reason whenever assembly,
   registration or a runtime call fails.

Routine ABIs to support, in order:

- integer, `CALLMETHOD=0`: read `RCX=data → EAX`; write `RCX=value, RDX=out`.
- integer, `CALLMETHOD=1`: read `RCX=data, RDX=0 → EAX`; write `RCX=value,
  RDX=0, R8=out`.
- float (`USESFLOAT`): the routine still returns an integer, reinterpreted as an
  IEEE-754 bit pattern; needs a `kind: float` in `RegisterAA`.
- string (`USESSTRING`): a third `R8=out` buffer; the largest change.

## Phases

- **P0 — metadata (small).** Extract every feature symbol (`PREFEREDALIGNMENT`,
  `USESFLOAT`, `USESSTRING`, `MAXSTRINGSIZE`, `CALLMETHOD`) alongside the
  routines into `CustomTypeDef`. Implemented.
- **P1 — integer types (medium).** `RegisterCE` with the SysV shim for the two
  integer ABIs, wired into the GUI import with a `RegisterRaw` fallback.
  Implemented and unit-tested with `customtype.RegisterCE`.
- **P2 — float (medium).** Extend `RegisterAA` with `kind: float`
  (bit-pattern reinterpretation) and accept `USESFLOAT`. Implemented and
  unit-tested.
- **P3 — string (large).** `kind: string`, the three-argument ABI and the output
  buffer. Implemented: `jit.Program` gained offset buffers, `RegisterAA` gained a
  string path, and `RegisterCE` accepts `USESSTRING` with `MAXSTRINGSIZE`.
- **P4 — pure-Go fallback (optional, large).** A translator for the common
  integer instruction subset so the headless (`!cgo`) build can apply
  conversions without `pkg/jit`. Implemented as `pkg/aaexec`, a bounded x86-64
  interpreter; `buildAA` uses it whenever the JIT is unavailable, and rejects
  routines outside the subset.

## Risks

- **Assembler coverage.** A CE routine may use instructions `pkg/asm` does not
  encode; P1 rejects and falls back per type rather than failing the import.
- **ABI subtleties.** Windows x64 reserves 32 bytes of shadow space and keeps
  16-byte stack alignment; the shim realigns the stack but cannot guarantee that
  a routine which writes shadow space behaves. Documented per type.
- **Data references.** Routines that read lookup tables or other `alloc`ed data
  need those symbols reproduced; P1 supports only self-contained routines and
  falls back otherwise.
- **`address` argument.** Display/parse conversions have no live address; P1
  passes 0 and documents that routines depending on it fall back to raw.
- **Executing third-party assembly.** Unchanged from ADR 0023: the routines run
  in-process on CGO builds; headless stays raw.

## Consequences

- Imported CE custom types format and parse values like CE does for the common
  integer case, instead of showing raw bytes.
- Conversions run on both builds: the CGO build uses `pkg/jit`, and the headless
  build falls back to the `pkg/aaexec` interpreter, which covers the common
  instruction subset and rejects the rest.
- Import reports per-type fallback reasons, so an unsupported routine is visible
  rather than silently wrong.
- This closes the ADR 0037 S5 follow-up for integer types and narrows it to
  float/string and data-dependent routines.

## Test strategy

- Extraction unit tests for every feature flag over a realistic CE script.
- A synthetic integer CE script is registered and `customtype.AATest` asserts the
  bytes↔value round trip (skipped without CGO).
- A routine using an unsupported mnemonic registers raw and reports a reason.
- Golden conversion values copied from a real CE table for the integer and float
  phases.
