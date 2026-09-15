//go:build gui

package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/customtype"
)

const defaultTypeScript = `function bytes_to_value(bytes, address)
  return bytes[1]
end

function value_to_bytes(value, address)
  return { value % 256 }
end`

// showCustomTypes opens the user-defined type editor.
func (a *App) showCustomTypes() {
	name := widget.NewEntry()
	name.SetPlaceHolder("e.g. Money")
	size := widget.NewEntry()
	size.SetText("4")
	kind := widget.NewSelect([]string{"int", "float", "string"}, nil)
	kind.SetSelected("int")
	desc := widget.NewEntry()
	ed := newCodeEditor(nil)
	ed.SetText(defaultTypeScript)

	form := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Name", name),
			widget.NewFormItem("Size (bytes)", size),
			widget.NewFormItem("Kind", kind),
			widget.NewFormItem("Description", desc),
		),
		widget.NewLabel("bytes_to_value(bytes[, address]) is required; value_to_bytes(value[, address]) is optional"),
		ed,
	)
	d := dialog.NewCustomConfirm("Custom Types", "Save", "Cancel", container.NewVScroll(form), func(ok bool) {
		if !ok {
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(size.Text))
		if err != nil {
			a.fail(err)
			return
		}
		def := customtype.Definition{
			Name:        strings.TrimSpace(name.Text),
			Size:        n,
			Kind:        kind.Selected,
			Description: strings.TrimSpace(desc.Text),
			Script:      ed.Text(),
		}
		if _, err := customtype.Register(def); err != nil {
			a.fail(err)
			return
		}
		defs, err := customtype.Load(config.CustomTypesPath())
		if err != nil {
			a.fail(err)
			return
		}
		defs = replaceDefinition(defs, def)
		if err := customtype.Save(config.CustomTypesPath(), defs); err != nil {
			a.fail(err)
			return
		}
		a.refreshValueTypes()
		a.setStatus("registered custom type %s", def.Name)
	}, a.win)
	d.Resize(fyne.NewSize(680, 620))
	d.Show()
}

func replaceDefinition(defs []customtype.Definition, def customtype.Definition) []customtype.Definition {
	for i := range defs {
		if strings.EqualFold(defs[i].Name, def.Name) {
			defs[i] = def
			return defs
		}
	}
	return append(defs, def)
}

// refreshValueTypes rebuilds the Value Type dropdown after a type is added.
func (a *App) refreshValueTypes() {
	if a.valueType == nil {
		return
	}
	a.valueType.Options = valueTypeOptions()
	a.valueType.Refresh()
}
