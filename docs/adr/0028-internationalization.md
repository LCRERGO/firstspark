# ADR 0028: Internationalization with go-i18n

## Status

Accepted.

## Context

All user-facing text was hard-coded English: roughly 150 string literals inline
in Fyne constructors across `internal/ui`, plus the CLI messages in
`internal/app`. There was no catalog, no language setting and no extraction
pipeline, so translating the UI meant editing every call site.

Fyne ships `fyne.io/fyne/v2/lang`, which is backed by `go-i18n`, but it only
selects a locale from the system environment and exposes **no runtime setter**
(Fyne's `Settings` has no language method), so it cannot honour a config-driven
language choice.

## Decision

- Add `internal/i18n`, a small front-end translation layer over
  `github.com/nicksnyder/go-i18n/v2` (already an indirect dependency).
- Catalogs are JSON files under `internal/i18n/locales/`, embedded with
  `go:embed`; message IDs are flat dotted keys (`menu.file.open_process`).
- English is the source and fallback language. `T`/`Tf` resolve through a
  localizer whose preference chain is `<selected>, en`, so a missing key falls
  back to English and finally to the key itself.
- `ui.language` (default `en`) selects the language; the Settings dialog lists
  the embedded catalogs. Because Fyne cannot retranslate a built widget tree,
  the change is applied **on restart** (like `ui.scale`) and the dialog says so.
- GUI chrome and CLI chrome are translated; engine `error` sentinels stay
  English. Label-keyed `Select`s were decoupled into `{key, value}` tables so
  the display label can be translated without breaking parsing.
- A test parses `internal/ui` and `internal/app` with `go/parser`, collects
  every `i18n.T`/`Tf` key and asserts it exists in `en.json`, and that every
  catalog key is referenced in the sources.

## Consequences

- Adding a language is dropping a new `locales/<code>.json` file; the settings
  dropdown discovers it automatically.
- The completeness test keeps the catalog honest in the default (non-GUI) build
  because it only reads source text.
- No live language switching; users restart, as with `ui.scale`.
- Untranslated areas (for example the code editor and some engine-adjacent
  dialogs) can be migrated incrementally; the test only enforces keys that are
  actually referenced.
