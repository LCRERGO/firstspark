//go:build gui

package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/plugin"
)

// resolvePluginDir picks the configured plugin directory, then the default.
func resolvePluginDir(cfg config.Config) string {
	if cfg.Plugins.Dir != "" {
		return cfg.Plugins.Dir
	}
	return config.PluginsDir()
}

func (a *App) targetPID() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.proc == nil {
		return 0
	}
	return a.proc.PID
}

func (a *App) pluginRead(addr uint64, size int) ([]byte, error) {
	a.mu.Lock()
	p := a.proc
	a.mu.Unlock()
	if p == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.no_process"))
	}
	return p.Read(addr, size)
}

func (a *App) pluginWrite(addr uint64, data []byte) error {
	a.mu.Lock()
	p := a.proc
	a.mu.Unlock()
	if p == nil {
		return fmt.Errorf("%s", i18n.T("error.no_process"))
	}
	return p.Write(addr, data)
}

func (a *App) pluginNotify(msg string) {
	if a.win == nil {
		return
	}
	dialog.ShowInformation(i18n.T("plugins.title"), msg, a.win)
}

func (a *App) pluginMenuContribs() []plugin.MenuContribution {
	if a.plugins == nil {
		return nil
	}
	return a.plugins.Menu()
}

func (a *App) runPluginAction(id, action string) {
	if a.plugins == nil {
		return
	}
	if err := a.plugins.Run(id, action); err != nil {
		a.fail(err)
	}
}

// showPlugins opens the plugin manager: a list of discovered plugins with
// enable checkboxes. Changes apply on the next launch.
func (a *App) showPlugins() {
	if a.plugins == nil {
		return
	}
	infos := a.plugins.List()
	if len(infos) == 0 {
		dialog.ShowInformation(i18n.T("plugins.title"),
			i18n.Tf("plugins.none", map[string]any{"Dir": resolvePluginDir(a.cfg)}), a.win)
		return
	}
	box := container.NewVBox()
	checks := make([]*widget.Check, len(infos))
	for i, info := range infos {
		perms := make([]string, len(info.Manifest.Permissions))
		for j, p := range info.Manifest.Permissions {
			perms[j] = string(p)
		}
		label := info.Manifest.Name
		if label == "" {
			label = info.Manifest.ID
		}
		if len(perms) > 0 {
			label += " — " + strings.Join(perms, ", ")
		}
		c := widget.NewCheck(label, nil)
		c.SetChecked(info.Enabled)
		checks[i] = c
		box.Add(c)
	}
	content := container.NewVScroll(box)
	content.SetMinSize(fyne.NewSize(440, 240))
	dialog.ShowCustomConfirm(i18n.T("plugins.title"), i18n.T("plugins.apply"), i18n.T("action.cancel"),
		content, func(ok bool) {
			if !ok {
				return
			}
			var enabled []string
			for i, c := range checks {
				if c.Checked {
					enabled = append(enabled, infos[i].Manifest.ID)
				}
			}
			a.cfg.Plugins.Enabled = enabled
			if err := a.cfg.Save(config.DefaultPath()); err != nil {
				log.Warn("saving plugin config failed", "err", err)
			}
			dialog.ShowInformation(i18n.T("plugins.title"), i18n.T("plugins.restart"), a.win)
		}, a.win)
}
