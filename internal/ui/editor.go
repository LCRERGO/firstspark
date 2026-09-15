//go:build gui

package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/script"
)

// codeEditor is a small multi-line editor with Lua syntax highlighting, a
// line-number gutter and undo/redo. Highlighting uses the pkg/script
// tokenizer so it always matches the language the engine accepts (ADR 0014).
type codeEditor struct {
	widget.BaseWidget

	lines    []string
	row, col int
	undo     []editorState
	redo     []editorState
	onChange func(string)

	gutter *widget.TextGrid
	code   *widget.TextGrid
	box    *fyne.Container
}

type editorState struct {
	lines    []string
	row, col int
}

func newCodeEditor(onChange func(string)) *codeEditor {
	e := &codeEditor{lines: []string{""}, onChange: onChange}
	e.gutter = widget.NewTextGrid()
	e.code = widget.NewTextGrid()
	e.box = container.NewHBox(e.gutter, e.code)
	e.ExtendBaseWidget(e)
	e.refresh()
	return e
}

// Text returns the editor contents.
func (e *codeEditor) Text() string { return strings.Join(e.lines, "\n") }

// SetText replaces the contents and resets the cursor and history.
func (e *codeEditor) SetText(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	e.lines = strings.Split(s, "\n")
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	e.row, e.col = 0, 0
	e.undo, e.redo = nil, nil
	e.refresh()
}

// CreateRenderer returns the editor's renderer.
func (e *codeEditor) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(e.box)
}

// FocusGained is part of fyne.Focusable.
func (e *codeEditor) FocusGained() {}

// FocusLost is part of fyne.Focusable.
func (e *codeEditor) FocusLost() {}

// TypedRune inserts a typed character.
func (e *codeEditor) TypedRune(r rune) {
	if r == '\n' || r == '\r' {
		return
	}
	e.snapshot()
	e.insertRune(r)
	e.refresh()
}

// TypedKey handles navigation and editing keys.
func (e *codeEditor) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyBackspace:
		e.snapshot()
		e.backspace()
	case fyne.KeyDelete:
		e.snapshot()
		e.deleteForward()
	case fyne.KeyLeft:
		e.moveLeft()
	case fyne.KeyRight:
		e.moveRight()
	case fyne.KeyUp:
		e.moveUp()
	case fyne.KeyDown:
		e.moveDown()
	case fyne.KeyHome:
		e.col = 0
	case fyne.KeyEnd:
		e.col = len([]rune(e.lines[e.row]))
	case fyne.KeyReturn, fyne.KeyEnter:
		e.snapshot()
		e.newline()
	case fyne.KeyTab:
		e.snapshot()
		e.insertString("  ")
	default:
		return
	}
	e.refresh()
}

// Tapped moves the cursor to the tapped cell.
func (e *codeEditor) Tapped(ev *fyne.PointEvent) {
	row, col := e.code.CursorLocationForPosition(ev.Position.Subtract(e.code.Position()))
	e.setCursor(row, col)
	e.refresh()
}

// Undo restores the previous edit.
func (e *codeEditor) Undo() {
	if len(e.undo) == 0 {
		return
	}
	st := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.redo = append(e.redo, e.state())
	e.lines, e.row, e.col = st.lines, st.row, st.col
	e.refresh()
}

// Redo reapplies an undone edit.
func (e *codeEditor) Redo() {
	if len(e.redo) == 0 {
		return
	}
	st := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.undo = append(e.undo, e.state())
	e.lines, e.row, e.col = st.lines, st.row, st.col
	e.refresh()
}

func (e *codeEditor) state() editorState {
	return editorState{lines: append([]string(nil), e.lines...), row: e.row, col: e.col}
}

func (e *codeEditor) snapshot() {
	e.undo = append(e.undo, e.state())
	if len(e.undo) > 200 {
		e.undo = e.undo[1:]
	}
	e.redo = nil
}

func (e *codeEditor) setCursor(row, col int) {
	if row < 0 {
		row = 0
	}
	if row >= len(e.lines) {
		row = len(e.lines) - 1
	}
	e.row = row
	n := len([]rune(e.lines[row]))
	if col < 0 {
		col = 0
	}
	if col > n {
		col = n
	}
	e.col = col
}

func (e *codeEditor) insertRune(r rune) {
	line := []rune(e.lines[e.row])
	if e.col > len(line) {
		e.col = len(line)
	}
	line = append(line[:e.col], append([]rune{r}, line[e.col:]...)...)
	e.lines[e.row] = string(line)
	e.col++
}

func (e *codeEditor) insertString(s string) {
	for _, r := range s {
		e.insertRune(r)
	}
}

func (e *codeEditor) backspace() {
	if e.col > 0 {
		line := []rune(e.lines[e.row])
		line = append(line[:e.col-1], line[e.col:]...)
		e.lines[e.row] = string(line)
		e.col--
		return
	}
	if e.row > 0 {
		prev := []rune(e.lines[e.row-1])
		e.col = len(prev)
		e.lines[e.row-1] = e.lines[e.row-1] + e.lines[e.row]
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
	}
}

func (e *codeEditor) deleteForward() {
	line := []rune(e.lines[e.row])
	if e.col < len(line) {
		line = append(line[:e.col], line[e.col+1:]...)
		e.lines[e.row] = string(line)
		return
	}
	if e.row+1 < len(e.lines) {
		e.lines[e.row] = e.lines[e.row] + e.lines[e.row+1]
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
	}
}

func (e *codeEditor) newline() {
	line := []rune(e.lines[e.row])
	rest := string(line[e.col:])
	e.lines[e.row] = string(line[:e.col])
	e.lines = append(e.lines[:e.row+1], append([]string{rest}, e.lines[e.row+1:]...)...)
	e.row++
	e.col = 0
}

func (e *codeEditor) moveLeft() {
	if e.col > 0 {
		e.col--
		return
	}
	if e.row > 0 {
		e.row--
		e.col = len([]rune(e.lines[e.row]))
	}
}

func (e *codeEditor) moveRight() {
	if e.col < len([]rune(e.lines[e.row])) {
		e.col++
		return
	}
	if e.row+1 < len(e.lines) {
		e.row++
		e.col = 0
	}
}

func (e *codeEditor) moveUp() {
	if e.row > 0 {
		e.row--
		e.setCursor(e.row, e.col)
	}
}

func (e *codeEditor) moveDown() {
	if e.row+1 < len(e.lines) {
		e.row++
		e.setCursor(e.row, e.col)
	}
}

func (e *codeEditor) refresh() {
	e.gutter.SetText(gutterText(len(e.lines)))
	e.code.SetText(e.Text())
	e.applyStyles()
	e.code.Refresh()
	e.gutter.Refresh()
	if e.onChange != nil {
		e.onChange(e.Text())
	}
}

func gutterText(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString(strconv.Itoa(i))
		if i < n {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (e *codeEditor) applyStyles() {
	text := e.Text()
	runes := []rune(text)
	rows := make([]int, len(runes)+1)
	cols := make([]int, len(runes)+1)
	r, c := 0, 0
	for i := 0; i <= len(runes); i++ {
		rows[i], cols[i] = r, c
		if i < len(runes) {
			if runes[i] == '\n' {
				r++
				c = 0
			} else {
				c++
			}
		}
	}
	for _, tok := range script.Tokenize(text) {
		style := tokenStyle(tok.Kind)
		if style == nil {
			continue
		}
		s, en := tok.Start, tok.End
		if s < 0 {
			s = 0
		}
		if en > len(runes) {
			en = len(runes)
		}
		if s >= en {
			continue
		}
		e.code.SetStyleRange(rows[s], cols[s], rows[en], cols[en], style)
	}
	if e.row >= 0 && e.row < len(e.lines) {
		e.code.SetStyle(e.row, e.col, &widget.CustomTextGridStyle{
			FGColor: theme.Color(theme.ColorNameBackground),
			BGColor: theme.Color(theme.ColorNamePrimary),
		})
	}
}

func tokenStyle(kind script.TokenKind) *widget.CustomTextGridStyle {
	switch kind {
	case script.TokenKeyword:
		return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNamePrimary)}
	case script.TokenString:
		return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNameSuccess)}
	case script.TokenNumber:
		return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNameWarning)}
	case script.TokenComment:
		return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNameDisabled)}
	default:
		return nil
	}
}
