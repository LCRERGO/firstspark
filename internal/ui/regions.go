//go:build gui

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/mem"
)

// showRegionManager lets the user pick the exact regions a scan covers. The
// selection is kept in memory for the session and cleared on process change.
func (a *scanTab) showRegionManager() {
	if a.proc == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.no_process")))
		return
	}
	regions, err := mem.Regions(a.proc.PID)
	if err != nil {
		a.fail(err)
		return
	}
	var readable []mem.Region
	for _, r := range regions {
		if r.Readable() && r.Size() > 0 {
			readable = append(readable, r)
		}
	}
	selected := map[uint64]bool{}
	if len(a.regionSel) > 0 {
		for _, r := range a.regionSel {
			selected[r.Start] = true
		}
	} else {
		for _, r := range readable {
			selected[r.Start] = true
		}
	}
	boxes := make([]*widget.Check, len(readable))
	rows := make([]fyne.CanvasObject, len(readable))
	for i, r := range readable {
		label := fmt.Sprintf("%x-%x %s %s", r.Start, r.End, r.Perms, r.Path)
		c := widget.NewCheck(label, nil)
		c.SetChecked(selected[r.Start])
		boxes[i] = c
		rows[i] = c
	}
	content := container.NewVScroll(container.NewVBox(rows...))
	d := dialog.NewCustomConfirm(i18n.T("regions.title"), i18n.T("action.apply"), i18n.T("action.cancel"), content,
		func(ok bool) {
			if !ok {
				return
			}
			var out []mem.Region
			for i, r := range readable {
				if boxes[i].Checked {
					out = append(out, r)
				}
			}
			if len(out) == len(readable) {
				a.regionSel = nil
			} else {
				a.regionSel = out
			}
			a.setStatusText(i18n.Tf("status.regions_selected", map[string]any{"Count": len(out)}))
		}, a.win)
	d.Resize(fyne.NewSize(680, 520))
	d.Show()
}
