# ADR 0005: Unprivileged by default, `sudo` re-exec as a fallback

## Status

Accepted. Amended by ADR 0025 (GUI startup prompt).

## Context

Attaching to another process requires either a permissive
`kernel.yama.ptrace_scope` or `CAP_SYS_PTRACE`. Many distributions default to
`ptrace_scope=1`, so an unprivileged attach fails for other users' processes.
Shipping a setuid binary or a `pkexec` helper would be invasive and harder to
audit.

## Decision

Run entirely as the invoking user. Detect permission failures and report an
actionable error that names `ptrace_scope` and `CAP_SYS_PTRACE`. Allow the CLI
to re-exec under `sudo` when the user chooses to. Do not ship setuid binaries or
privileged helpers.

## Consequences

- No privileged code to audit or package.
- Users on restrictive kernels must opt in to `sudo` or adjust `ptrace_scope`.
