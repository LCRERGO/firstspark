# ADR 0034: Theme families and variants

## Status

Accepted. Extends ADR 0008.

## Context

ADR 0008 gave Firstspark a single flat theme with a light and a dark palette
and a `scheme` enum of `light`/`dark`/`system`. That enum conflated two
independent choices: which palette to use and whether it is light or dark, so
adding a second palette would have meant a flat, combinatorial list of named
themes. Users wanted more than the cyan/magenta cyberpunk look.

## Decision

Split the theme into a **family** (the palette pair) and a **variant**
(`light`, `dark`, `system`). `cyberTheme` carries both; `pal` resolves the
variant to light/dark and asks the family for that palette. The families are:

- **Cyberpunk** — the existing palettes, unchanged.
- **Nord** — Polar Night/Snow Storm/Frost/Aurora.
- **Dracula** — the Dracula colours, with Alucard as the light variant.
- **Tokyo Night** — the Night and Day colours.

Each family defines a complete `palette` for both variants. Fonts and sizes
stay global (`ui.font_size`); a family only changes colours. The unknown
`ColorName` fallback now passes the resolved variant to Fyne's default theme
instead of always `VariantDark`, the foreground-on-error/success/warning
colours are palette fields rather than hard-coded white/black, and the
previously dead `secondary` field now backs `ColorNameHyperlink` (with each
palette's `selection` kept as its own accent tint).

Config gains `ui.theme` (family slug, default `cyberpunk`) and
`ui.theme_variant` (`light`/`dark`/`system`, default `light`). An old
`ui.theme: light|dark|system` is migrated on load to `cyberpunk` plus that
variant. The View menu becomes **Theme ▸ \<Family\> ▸ Light/Dark/System**, and
Settings shows two dropdowns (family and variant).

## Consequences

- More looks share one code path; adding a family is a palette pair plus a
  label.
- `ui.theme` is no longer a variant; existing configurations are migrated
  transparently.
- Global hotkeys and the theme are independent, so switching theme does not
  affect bindings.
