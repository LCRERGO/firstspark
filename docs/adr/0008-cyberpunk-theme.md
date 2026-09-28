# ADR 0008: Cyberpunk flat theme, light by default, configurable scheme

## Status

Accepted.

## Context

The GUI should look intentional rather than like a default toolkit demo, while
keeping text readable. The reference tool's classic grey palette is dated; a flat,
modern look was preferred, with a cyberpunk flavour (cyan/magenta neon on
near-black). A light scheme is also required, and the existing `ui.theme`
config field was declared but never read.

## Decision

- Ship a single custom `fyne.Theme` implementation, **flat** with a
  **cyan-primary / magenta-secondary** palette. Dark uses a near-black
  background (`#0A0B12`) and cyan `#00E5FF`; light uses a cool near-white
  background (`#F4F6FB`) and cyan `#00A6C4`.
- Neon is applied to **accents, borders, separators and headings**; body text
  keeps a high-contrast neutral colour for readability.
- `ui.theme` accepts `light`, `dark` or `system`; **the default is `light`**.
  `system` follows Fyne's FreeDesktop dark-style detection. The scheme is
  switchable from a top-level **View ▸ Theme** menu (radio items) as well as
  Edit ▸ Settings; the menu's checked state tracks the active scheme.
- Typography: an embedded OFL display font (Rajdhani) for headings and titles,
  Noto Sans for body text, and DejaVu Sans Mono for data (addresses, hex,
  disassembly). Base text size 14, monospace size 12.
- Fyne uses `ColorNameOverlayBackground` as the **dialog surface** (not the
  scrim) and `ColorNameShadow` as the scrim. The theme sets the overlay to an
  opaque dialog surface, adds an `InnerWindowBorder`, and raises
  `ColorNameInputBorder` in both variants so checkboxes and entries stay
  visible. All colour names are mapped explicitly; unknown names fall back to
  Fyne's default theme rather than returning transparent.

## Consequences

- The previously dead `ui.theme` field becomes meaningful and is persisted
  when changed in Settings.
- The theme is one file (`internal/ui/theme.go`) plus one embedded font asset,
  so restyling stays contained.
- Fyne does not expose a per-widget font for native menus, so the display font
  is applied to in-window headings and titles, not the OS menu bar.
- Adding a View menu is a deliberate deviation from the reference tool (which has
  none) to make the theme control discoverable; ADR 0007's menu list is
  superseded on this point.
