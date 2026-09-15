# ADR 0022: Dedicated ptrace worker thread and remote function calls

## Status

Accepted.

## Context

ptrace operations must be issued by the **same OS thread** that attached to the
tracee: Linux ties the tracer/tracee relationship to the tracer thread. Go
multiplexes goroutines across OS threads, so a backend that called
`PTRACE_ATTACH` from one goroutine and `PTRACE_GETREGS` from another would get
`ESRCH` as soon as the goroutine migrated (which happens on any blocking call,
such as waiting for a debug event). This also affected `PTRACE_SETREGS`.

Calling a function inside the target (needed for Auto-Assembler-defined custom
types) requires attaching, setting registers, resuming, and reading the result
— all of which must run on that one thread.

## Decision

- Funnel every ptrace operation for a target through a single worker goroutine
  that calls `runtime.LockOSThread` and processes commands from a channel. The
  public `Backend` methods marshal to the worker and wait for the result;
  internal helpers are only ever called on the worker.
- Read and write general-purpose registers through `PTRACE_PEEKUSER` /
  `PTRACE_POKEUSER` (the same mechanism used for the debug registers), which is
  more reliable across stop states than `PTRACE_GETREGS`/`PTRACE_SETREGS`.
- Add a `RemoteCaller` interface: `Call(fn, args)` invokes a function in the
  target using a scratch page for the return stub (`int3`) and a private stack,
  clearing RAX so the kernel does not restart an interrupted syscall, restoring
  the target's registers afterwards.

## Consequences

- The debugger is reliable under Go's scheduler; the remote call is verified
  against a live process by `TestRemoteCall`.
- All ptrace work for a target is serialised on one thread.
- A remote call is still a stop-resume cycle and costs on the order of
  microseconds, so it is unsuitable for the inner loop of a memory scan.
