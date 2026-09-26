//go:build gui

package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/LCRERGO/firstspark/internal/i18n"
)

// luaMaxLines caps the console buffer.
const luaMaxLines = 2000

// luaInput is the console entry; Up/Down walk the command history.
type luaInput struct {
	*toolTipEntry
	onPrev func()
	onNext func()
}

func newLuaInput() *luaInput {
	return &luaInput{toolTipEntry: newToolTipEntry()}
}

func (e *luaInput) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyUp:
		if e.onPrev != nil {
			e.onPrev()
			return
		}
	case fyne.KeyDown:
		if e.onNext != nil {
			e.onNext()
			return
		}
	}
	e.toolTipEntry.TypedKey(ev)
}

// openLuaConsole shows the Lua Engine window, creating it lazily.
func (a *App) openLuaConsole() {
	if a.luaWin == nil {
		a.luaWin = a.fapp.NewWindow(i18n.T("lua.title"))
		a.luaWin.Resize(fyne.NewSize(640, 460))
		a.buildLuaConsole()
	}
	a.luaWin.Show()
	a.luaWin.Canvas().Focus(a.luaInput)
}

func (a *App) buildLuaConsole() {
	a.luaList = widget.NewList(
		func() int { return len(a.luaLines) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.luaLines) {
				t.Text = ""
				t.Refresh()
				return
			}
			t.Text = a.luaLines[id]
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	a.luaInput = newLuaInput()
	a.luaInput.SetPlaceHolder(i18n.T("lua.placeholder"))
	a.luaInput.OnSubmitted = func(text string) { a.runLuaInput(text) }
	a.luaInput.onPrev = a.luaHistoryPrev
	a.luaInput.onNext = a.luaHistoryNext
	bar := container.NewHBox(
		newHintButton(i18n.T("lua.run"), "lua.hint.run", func() { a.runLuaInput(a.luaInput.Text) }),
		newHintButton(i18n.T("lua.clear"), "lua.hint.clear", func() {
			a.luaLines = nil
			if a.luaList != nil {
				a.luaList.Refresh()
			}
		}),
	)
	bottom := container.NewBorder(nil, nil, nil, bar, a.luaInput)
	content := container.NewBorder(nil, bottom, nil, nil, a.luaList)
	a.luaWin.SetContent(fynetooltip.AddWindowToolTipLayer(content, a.luaWin.Canvas()))
}

// runLuaInput evaluates one console command against the shared runtime.
func (a *App) runLuaInput(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.luaHistory = append(a.luaHistory, text)
	a.luaHistPos = len(a.luaHistory)
	if a.luaInput != nil {
		a.luaInput.SetText("")
	}
	a.appendLuaOutput("> " + text)
	if err := a.luaRuntime().Eval(text); err != nil {
		a.appendLuaOutput(i18n.T("lua.error") + ": " + err.Error())
	}
}

// appendLuaOutput adds text to the console buffer, one line at a time.
func (a *App) appendLuaOutput(text string) {
	for _, line := range strings.Split(text, "\n") {
		a.luaLines = append(a.luaLines, line)
	}
	if len(a.luaLines) > luaMaxLines {
		a.luaLines = a.luaLines[len(a.luaLines)-luaMaxLines:]
	}
	if a.luaList != nil {
		a.luaList.Refresh()
		a.luaList.ScrollToBottom()
	}
}

func (a *App) luaHistoryPrev() {
	if len(a.luaHistory) == 0 {
		return
	}
	if a.luaHistPos > 0 {
		a.luaHistPos--
	}
	a.luaInput.SetText(a.luaHistory[a.luaHistPos])
}

func (a *App) luaHistoryNext() {
	if len(a.luaHistory) == 0 {
		return
	}
	if a.luaHistPos < len(a.luaHistory)-1 {
		a.luaHistPos++
		a.luaInput.SetText(a.luaHistory[a.luaHistPos])
		return
	}
	a.luaHistPos = len(a.luaHistory)
	a.luaInput.SetText("")
}
