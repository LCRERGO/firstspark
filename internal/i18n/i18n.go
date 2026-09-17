// Package i18n provides the GUI and CLI translation layer (ADR 0028). Message
// catalogs are JSON files embedded from locales/; English is the source and
// fallback language. The selected language is applied once at startup, since
// Fyne cannot retranslate a built widget tree.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var locales embed.FS

// DefaultLanguage is used when the configured language has no catalog.
const DefaultLanguage = "en"

var (
	mu        sync.RWMutex
	bundle    *i18n.Bundle
	localizer *i18n.Localizer
	current   = DefaultLanguage
)

// Init loads the embedded catalogs and selects lang, falling back to English.
func Init(lang string) error {
	b := i18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("json", json.Unmarshal)

	entries, err := locales.ReadDir("locales")
	if err != nil {
		return fmt.Errorf("i18n: read locales: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := locales.ReadFile(path.Join("locales", e.Name()))
		if err != nil {
			return fmt.Errorf("i18n: read %s: %w", e.Name(), err)
		}
		if _, err := b.ParseMessageFileBytes(data, e.Name()); err != nil {
			return fmt.Errorf("i18n: parse %s: %w", e.Name(), err)
		}
	}

	lang = normalize(lang)
	tag, err := language.Parse(lang)
	if err != nil {
		lang = DefaultLanguage
		tag = language.English
	}

	mu.Lock()
	bundle = b
	localizer = i18n.NewLocalizer(b, tag.String(), DefaultLanguage)
	current = lang
	mu.Unlock()
	return nil
}

// Language returns the active language code.
func Language() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Supported lists the language codes with an embedded catalog, English first.
func Supported() []string {
	entries, err := locales.ReadDir("locales")
	if err != nil {
		return []string{DefaultLanguage}
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i] == DefaultLanguage {
			return true
		}
		if out[j] == DefaultLanguage {
			return false
		}
		return out[i] < out[j]
	})
	return out
}

// T returns the translation of key in the active language, falling back to the
// English catalog. An unknown key is returned unchanged so gaps stay visible.
func T(key string) string { return Tf(key, nil) }

// Tf returns the translation of key with template data, falling back to
// English and finally to the key itself.
func Tf(key string, data any) string {
	mu.RLock()
	loc := localizer
	mu.RUnlock()
	if loc == nil {
		return key
	}
	cfg := &i18n.LocalizeConfig{MessageID: key}
	if data != nil {
		cfg.TemplateData = data
	}
	s, err := loc.Localize(cfg)
	if err != nil {
		return key
	}
	return s
}

func normalize(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(lang))
	if lang == "" {
		return DefaultLanguage
	}
	if i := strings.IndexAny(lang, "._-"); i > 0 {
		lang = lang[:i]
	}
	return lang
}
