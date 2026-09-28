# ADR 0029: Wire up existing engines to the GUI

## Status

Accepted.

## Context

Several engines were implemented and tested but unreachable from the UI:
`speedhack` (the checkbox only flipped a flag), `debugger.RemoteCaller`, the
pointermap (built in memory only, never cached or loaded), and parts of the
ptrace backend (`SetRegisters` unused, no step-over, no breakpoint list). The
result was a visible feature gap against the reference tool even though the hard parts
existed.

## Decision

Expose the engines and fill the small gaps they needed:

- **Speedhack**: the checkbox and Tools menu install `speedhack.Hook` on the
  selected process (attaching a ptrace backend, hooking `clock_gettime` and
  `gettimeofday`, then detaching) and remove the hooks on toggle-off or process
  change. A scale field in the scan panel overrides `speedhack.scale`.
- **Remote call**: the Debugger window gains a *Call Function* dialog. The
  ptrace backend now implements a typed `FloatCaller` (integer, float and double
  arguments following the System V AMD64 ABI, separate GP and SSE register
  counters, result in `RAX`/`XMM0`). SSE registers are read and written through
  `PTRACE_GETREGSET`/`SETREGSET` with `NT_PRFPREG` because `PTRACE_POKEUSER`
  cannot touch the x87/SSE area reliably.
- **Pointer scan**: `pointerscan.BuildOrLoad` caches the pointermap under
  `$XDG_CACHE_HOME/firstspark`, keyed by the target's region map and build
  options; the results dialog can load a saved `.ptr` file.
- **Debugger**: true step-over (a temporary breakpoint at the return address
  read from `[rsp]` for `call`, otherwise a single step), register editing via a
  `NAME=value` field, and a breakpoint/watchpoint list.

## Consequences

- The features are usable from the GUI with no new UI framework.
- Speedhack attaches and detaches around installation; the target is briefly
  stopped. It hooks libc symbols, so calls through the vDSO still bypass it.
- Typed remote calls clobber and restore the SSE registers they use.
- Multi-thread debugging remains deferred (ADR 0016).
