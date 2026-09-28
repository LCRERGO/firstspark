//go:build gui

package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/LCRERGO/firstspark/pkg/combinator"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/customtype"
	"github.com/LCRERGO/firstspark/pkg/scan"
	"github.com/LCRERGO/firstspark/pkg/script"
)

const defaultTypeScript = `function bytes_to_value(bytes, address)
  return bytes[1]
end

function value_to_bytes(value, address)
  return { value % 256 }
end`

const defaultAATypeScript = `[ENABLE]
ConvertRoutine:
  ; rdi = pointer to the bytes; return the value in rax
  mov eax, [rdi]
  ret

ConvertBackRoutine:
  ; rdi = value; rsi = pointer to the output bytes
  mov eax, edi
  mov [rsi], eax
  ret`

// showCustomTypes opens the custom-type manager, creating it lazily.
func (a *App) showCustomTypes() {
	if a.ctWin == nil {
		a.ctWin = a.fapp.NewWindow("Custom Types")
		a.ctWin.Resize(fyne.NewSize(820, 620))
		a.buildCustomTypes()
	}
	a.ctReload()
	a.ctWin.Show()
}

func (a *App) buildCustomTypes() {
	a.ctList = widget.NewList(
		func() int { return len(a.ctDefs) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			l := o.(*widget.Label)
			if id < 0 || id >= len(a.ctDefs) {
				l.SetText("")
				return
			}
			d := a.ctDefs[id]
			l.SetText(fmt.Sprintf("%s  (%d bytes, %s)", d.Name, d.Size, kindOr(d.Kind)))
		},
	)
	a.ctList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(a.ctDefs) {
			a.ctSel = int(id)
			a.ctLoad(a.ctDefs[id])
		}
	}

	a.ctName = newHintEntry("customtypes.hint.name")
	a.ctSize = newHintEntry("customtypes.hint.size")
	a.ctSize.SetText("4")
	a.ctMode = newHintSelect([]string{"Lua", "Auto Assembler"}, "customtypes.hint.mode", func(s string) {
		if s == "Auto Assembler" {
			a.ctEditor.SetLanguage(langAutoasm)
		} else {
			a.ctEditor.SetLanguage(langLua)
		}
	})
	a.ctMode.SetSelected("Lua")
	a.ctKind = newHintSelect([]string{"int", "float", "string"}, "customtypes.hint.kind", nil)
	a.ctKind.SetSelected("int")
	a.ctAlign = newHintEntry("customtypes.hint.align")
	a.ctAlign.SetPlaceHolder("0 = size")
	a.ctDesc = newHintEntry("customtypes.hint.desc")
	a.ctEditor = newCodeEditor(nil)
	a.ctEditor.SetText(defaultTypeScript)
	a.ctStatus = widget.NewLabel("ready")

	a.ctTestBytes = newHintEntry("customtypes.hint.test_bytes")
	a.ctTestBytes.SetPlaceHolder("hex bytes, e.g. 39 30 00 00")
	a.ctTestAddr = newHintEntry("customtypes.hint.test_addr")
	a.ctTestAddr.SetPlaceHolder("or read from address")
	a.ctTestOut = widget.NewLabel("")
	a.ctTestOut.Wrapping = fyne.TextWrapWord

	form := widget.NewForm(
		widget.NewFormItem("Name", a.ctName),
		widget.NewFormItem("Mode", a.ctMode),
		widget.NewFormItem("Size (bytes)", a.ctSize),
		widget.NewFormItem("Kind", a.ctKind),
		widget.NewFormItem("Alignment", a.ctAlign),
		widget.NewFormItem("Description", a.ctDesc),
	)
	test := container.NewVBox(
		widget.NewLabel("Test"),
		container.NewHBox(a.ctTestBytes, a.ctTestAddr, newHintButton("Test", "customtypes.hint.test", a.ctTest)),
		a.ctTestOut,
	)
	buttons := container.NewHBox(
		newHintButton("Add", "customtypes.hint.add", a.ctAdd),
		newHintButton("Save", "customtypes.hint.save", a.ctSave),
		newHintButton("Delete", "customtypes.hint.delete", a.ctDelete),
		newHintButton("Check", "customtypes.hint.check", a.ctCheck),
	)
	right := container.NewVScroll(container.NewVBox(form, a.ctEditor, test, buttons, a.ctStatus))

	left := container.NewBorder(nil, nil, nil, nil, a.ctList)
	split := container.NewHSplit(left, right)
	split.SetOffset(0.3)
	a.ctWin.SetContent(fynetooltip.AddWindowToolTipLayer(split, a.ctWin.Canvas()))
}

func kindOr(k string) string {
	if strings.TrimSpace(k) == "" {
		return "int"
	}
	return k
}

func (a *App) ctReload() {
	defs, err := customtype.Load(config.CustomTypesPath())
	if err != nil {
		a.fail(err)
		return
	}
	a.ctDefs = defs
	a.ctSel = -1
	a.ctID = 0
	if a.ctList != nil {
		a.ctList.Refresh()
	}
}

func (a *App) ctLoad(d customtype.Definition) {
	a.ctName.SetText(d.Name)
	a.ctSize.SetText(strconv.Itoa(d.Size))
	a.ctKind.SetSelected(kindOr(d.Kind))
	if d.Alignment > 0 {
		a.ctAlign.SetText(strconv.Itoa(d.Alignment))
	} else {
		a.ctAlign.SetText("")
	}
	a.ctDesc.SetText(d.Description)
	if strings.EqualFold(d.Mode, "aa") {
		a.ctMode.SetSelected("Auto Assembler")
	} else {
		a.ctMode.SetSelected("Lua")
	}
	a.ctEditor.SetText(d.Script)
	a.ctEditor.ClearError()
	if t, ok := scan.LookupType(d.Name); ok {
		a.ctID = t.ID
	}
}

func (a *App) ctAdd() {
	a.ctName.SetText("")
	a.ctSize.SetText("4")
	a.ctMode.SetSelected("Lua")
	a.ctKind.SetSelected("int")
	a.ctAlign.SetText("")
	a.ctDesc.SetText("")
	a.ctEditor.SetText(defaultTypeScript)
	a.ctEditor.ClearError()
	a.ctID = 0
	a.ctList.UnselectAll()
}

func (a *App) ctDefinition() (customtype.Definition, error) {
	size, err := strconv.Atoi(strings.TrimSpace(a.ctSize.Text))
	if err != nil || size <= 0 {
		return customtype.Definition{}, fmt.Errorf("size must be a positive integer")
	}
	align := 0
	if s := strings.TrimSpace(a.ctAlign.Text); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			align = n
		}
	}
	mode := "lua"
	if a.ctMode.Selected == "Auto Assembler" {
		mode = "aa"
	}
	return customtype.Definition{
		Name:        strings.TrimSpace(a.ctName.Text),
		Size:        size,
		Kind:        a.ctKind.Selected,
		Mode:        mode,
		Alignment:   align,
		Description: strings.TrimSpace(a.ctDesc.Text),
		Script:      a.ctEditor.Text(),
	}, nil
}

func (a *App) ctSave() {
	def, err := a.ctDefinition()
	if err != nil {
		a.fail(err)
		return
	}
	if a.ctID != 0 {
		scan.UnregisterType(a.ctID)
	}
	t, err := customtype.Register(def)
	if err != nil {
		a.ctEditor.SetError(0, err.Error())
		a.ctStatus.SetText("error: " + err.Error())
		return
	}
	a.ctID = t.ID
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
	a.ctReload()
	a.ctStatus.SetText("saved " + def.Name)
}

func (a *App) ctDelete() {
	if a.ctSel < 0 || a.ctSel >= len(a.ctDefs) {
		a.ctStatus.SetText("select a type to delete")
		return
	}
	name := a.ctDefs[a.ctSel].Name
	if t, ok := scan.LookupType(name); ok {
		scan.UnregisterType(t.ID)
	}
	defs, err := customtype.Load(config.CustomTypesPath())
	if err != nil {
		a.fail(err)
		return
	}
	var kept []customtype.Definition
	for _, d := range defs {
		if !strings.EqualFold(d.Name, name) {
			kept = append(kept, d)
		}
	}
	if err := customtype.Save(config.CustomTypesPath(), kept); err != nil {
		a.fail(err)
		return
	}
	a.refreshValueTypes()
	a.ctReload()
	a.ctAdd()
	a.ctStatus.SetText("deleted " + name)
}

func (a *App) ctCheck() {
	def, err := a.ctDefinition()
	if err != nil {
		a.ctEditor.SetError(0, err.Error())
		a.ctStatus.SetText("error: " + err.Error())
		return
	}
	if err := customtype.Validate(def); err != nil {
		a.markScriptError(err)
		a.ctStatus.SetText("error: " + err.Error())
		return
	}
	a.ctEditor.ClearError()
	a.ctStatus.SetText("OK")
}

func (a *App) markScriptError(err error) {
	line := 0
	if pe, ok := err.(*combinator.ParseError); ok {
		line = pe.Line - 1
	}
	a.ctEditor.SetError(line, err.Error())
}

func (a *App) ctTest() {
	size, _ := strconv.Atoi(strings.TrimSpace(a.ctSize.Text))
	var data []byte
	if addrText := strings.TrimSpace(a.ctTestAddr.Text); addrText != "" && a.proc != nil && size > 0 {
		addr, err := parseAddress(addrText)
		if err != nil {
			a.fail(err)
			return
		}
		d, err := a.proc.Read(addr, size)
		if err != nil && len(d) == 0 {
			a.fail(err)
			return
		}
		data = d
	} else {
		d, err := parseHexBytes(a.ctTestBytes.Text)
		if err != nil {
			a.ctTestOut.SetText("error: " + err.Error())
			return
		}
		data = d
	}
	def, err := a.ctDefinition()
	if err != nil {
		a.ctTestOut.SetText("error: " + err.Error())
		return
	}
	if def.Mode == "aa" {
		value, back, err := customtype.AATest(def, data)
		if err != nil {
			a.ctTestOut.SetText("error: " + err.Error())
			return
		}
		a.ctTestOut.SetText(fmt.Sprintf("bytes: % x\nvalue: %d\nback:  % x", data, value, back))
		return
	}
	prog, err := script.Compile(a.ctEditor.Text())
	if err != nil {
		a.markScriptError(err)
		a.ctTestOut.SetText("error: " + err.Error())
		return
	}
	fn, err := prog.Func("bytes_to_value")
	if err != nil {
		a.ctTestOut.SetText("error: " + err.Error())
		return
	}
	out := fn.Call(prog.Env(), script.BytesValue(data), script.Int(0))
	if len(out) == 0 {
		a.ctTestOut.SetText("error: bytes_to_value returned nothing")
		return
	}
	value := out[0]
	back := "(no value_to_bytes)"
	if wfn, err := prog.Func("value_to_bytes"); err == nil {
		if wout := wfn.Call(prog.Env(), value, script.Int(0)); len(wout) > 0 {
			if b, err := script.ValueBytes(wout[0]); err == nil {
				back = fmt.Sprintf("% x", b)
			}
		}
	}
	a.ctTestOut.SetText(fmt.Sprintf("bytes: % x\nvalue: %s\nback:  %s", data, value.String(), back))
}

func parseHexBytes(s string) ([]byte, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf("no bytes")
	}
	out := make([]byte, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimPrefix(f, "0x"), "0X"), 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid byte %q", f)
		}
		out = append(out, byte(n))
	}
	return out, nil
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

// refreshValueTypes rebuilds every tab's Value Type dropdown after a type
// changes.
func (a *App) refreshValueTypes() {
	for _, t := range a.tabs {
		if t.valueType == nil {
			continue
		}
		t.valueType.Options = valueTypeOptions()
		t.valueType.Refresh()
	}
}

// customTypeAlignment returns a custom type's preferred alignment, or 0.
func customTypeAlignment(label string) int {
	t, ok := scan.LookupType(label)
	if !ok || t.Alignment <= 0 {
		return 0
	}
	return t.Alignment
}
