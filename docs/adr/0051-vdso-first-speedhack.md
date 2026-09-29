# ADR 0051: vDSO-first speedhack with a managed safe patcher

## Status

Accepted. Supersedes the speedhack interception strategy in ADR 0004.

## Context

The speedhack was marked experimental. It hooked the libc `clock_gettime` and
`gettimeofday` symbols, but modern glibc serves the time getters from the
per-process vDSO, `gettimeofday`/`time` are `STT_GNU_IFUNC` symbols whose
`.dynsym` value is a resolver rather than the implementation, and statically
linked or musl targets have no resolvable libc symbol at all. It also never
hooked sleep functions and scaled `tv_sec` and `tv_nsec` independently, so the
returned structs were not normalized. Hooks were installed from the UI without
freezing the target, could orphan on failure, and leaked a code cave on every
re-install.

The alternatives were to keep hooking libc `.dynsym` (resolving IFUNCs through
the GOT), to hook the vDSO, or to intercept at the syscall layer with `seccomp`.

## Decision

Intercept the time getters primarily in the target's `[vdso]` mapping:
`__vdso_clock_gettime`, `__vdso_gettimeofday` and `__vdso_time`. The vDSO is
present for dynamic, static, musl and stripped targets, so one mechanism covers
them all. Fall back per symbol to libc `.dynsym` with GOT/IFUNC resolution only
when the vDSO prologue is unrelocatable. Hook exactly one layer per getter to
avoid double-scaling, since libc routes through the vDSO internally.

Sleep functions are the inverse: `clock_nanosleep` has its duration (or
absolute deadline) divided by the scale before the original is called, with
`EINTR` remainders scaled back up. It is the only sleep hooked by default
because glibc's `nanosleep`, `usleep` and `sleep` all funnel through it
(`__nanosleep` aliases `nanosleep`), so hooking the wrappers as well would
double-scale a single request. A single toggle covers clock and sleep.
`poll`/`select`/`epoll_wait` timeouts and raw `syscall(SYS_clock_gettime)`
callers are out of scope.

All patching goes through a `pkg/speedhack` manager: it pauses every thread of
the target for the patch window (stop-the-world), installs or updates the hooks,
and releases. The manager is the only path used by the CLI, the GUI and plugins,
replacing the inline install/remove logic in `internal/ui`.

## Consequences

- Static, musl and stripped targets are covered as long as they use the vDSO.
- Raw-syscall and timeout-based waits remain unscaled; a documented limitation.
- Stop-the-world needs `/proc/<pid>/task` enumeration and a ptrace attach per
  thread, adding machinery but eliminating torn-prologue crashes.
- The `[vdso]` mapping rejects `mprotect` with `EINVAL`, so the patch is written
  with ptrace `POKEDATA` instead; `pkg/inject` gained a text-writer hook for it.
- `pkg/inject` gained cave free/`munmap` so re-installs stop leaking mappings.
- Scale changes remove and reinstall the hooks (releasing their caves); the
  code cave is never left behind.
- `pkg/inject` now relocates RIP-relative operands and relative branches into
  the trampoline, widening rel8 branches to rel32, so `gettimeofday`, `time`
  and `clock_nanosleep` (whose prologues need it) can be hooked.
