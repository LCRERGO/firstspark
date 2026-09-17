package config

import (
	"path/filepath"
	"testing"
	"testing/quick"
)

// TestPropertyConfigRoundTrip checks that a configuration survives a save and
// load unchanged for the fields that drive the GUI.
func TestPropertyConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	families := []string{"cyberpunk", "nord", "dracula", "tokyo-night"}
	variants := []string{"light", "dark", "system"}
	prop := func(align, limit uint16, fs, sc, fam, vr uint8) bool {
		cfg := Default()
		cfg.Scan.Alignment = int(align % 4096)
		cfg.UI.ResultLimit = int(limit)
		cfg.UI.FontSize = float64(8 + fs%32)
		cfg.UI.Scale = float64(1 + sc%4)
		cfg.UI.Theme = families[int(fam)%len(families)]
		cfg.UI.ThemeVariant = variants[int(vr)%len(variants)]
		if err := cfg.Save(path); err != nil {
			return false
		}
		got, err := Load(path)
		if err != nil {
			return false
		}
		return got.Scan.Alignment == cfg.Scan.Alignment &&
			got.UI.ResultLimit == cfg.UI.ResultLimit &&
			got.UI.FontSize == cfg.UI.FontSize &&
			got.UI.Scale == cfg.UI.Scale &&
			got.UI.Theme == cfg.UI.Theme &&
			got.UI.ThemeVariant == cfg.UI.ThemeVariant
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}
