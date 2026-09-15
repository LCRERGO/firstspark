//go:build gui

package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/autoasm"
	"github.com/LCRERGO/firstspark/pkg/script"
)

// language selects the tokenizer used for highlighting.
type language int

const (
	langLua language = iota
	langAutoasm
)

type rowCol struct{ row, col int }

// codeEditor is a multi-line code editor with syntax highlighting, selection,
// clipboard support, find/replace and a line:column status bar (ADR 0021).
type codeEditor struct {
	widget.BaseWidget

	lines []string
	row   int
	col   int

	anchor rowCol
	hasSel bool
	shift  bool

	undo []editorState
	redo []editorState

	lang      language
	onChange  func(string)
	errorLine int
	errorMsg  string

	gutter *widget.TextGrid
	code   *widget.TextGrid
	box    *fyne.Container
	scroll *container.Scroll
	status *widget.Label

	findEntry *widget.Entry
	replEntry *widget.Entry
	content   *fyne.Container
}

type editorState struct {
	lines []string
	row   int
	col   int
}

func newCodeEditor(onChange func(string)) *codeEditor {
	e := &codeEditor{lines: []string{""}, onChange: onChange, errorLine: -1}
	e.gutter = widget.NewTextGrid()
	e.code = widget.NewTextGrid()
	e.box = container.NewHBox(e.gutter, e.code)
	e.scroll = container.NewScroll(e.box)
	e.status = widget.NewLabel("Ln 1, Col 1")

	e.findEntry = widget.NewEntry()
	e.findEntry.SetPlaceHolder("find")
	e.replEntry = widget.NewEntry()
	e.replEntry.SetPlaceHolder("replace")
	findRow := container.NewHBox(
		e.findEntry,
		widget.NewButton("Prev", e.findPrev),
		widget.NewButton("Next", e.findNext),
		e.replEntry,
		widget.NewButton("Replace", e.replaceOne),
		widget.NewButton("All", e.replaceAll),
		widget.NewButton("Go to...", e.gotoDialog),
		widget.NewButton("Comment", e.ToggleComment),
	)
	e.content = container.NewBorder(findRow, e.status, nil, nil, e.scroll)
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
	e.anchor = rowCol{}
	e.hasSel = false
	e.undo, e.redo = nil, nil
	e.refresh()
}

// SetLanguage selects the highlighter.
func (e *codeEditor) SetLanguage(l language) {
	e.lang = l
	e.refresh()
}

// SetError marks a line as erroneous and shows the message in the status bar.
func (e *codeEditor) SetError(line int, msg string) {
	e.errorLine = line
	e.errorMsg = msg
	e.refresh()
}

// ClearError removes any error marker.
func (e *codeEditor) ClearError() {
	e.errorLine = -1
	e.errorMsg = ""
	e.refresh()
}

// CreateRenderer returns the editor's renderer.
func (e *codeEditor) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(e.content)
}

// MinSize gives the editor a sensible default size inside dialogs.
func (e *codeEditor) MinSize() fyne.Size { return fyne.NewSize(560, 260) }

// FocusGained is part of fyne.Focusable.
func (e *codeEditor) FocusGained() {}

// FocusLost is part of fyne.Focusable.
func (e *codeEditor) FocusLost() { e.shift = false }

// KeyDown tracks the shift modifier for keyboard selection.
func (e *codeEditor) KeyDown(ev *fyne.KeyEvent) {
	if ev.Name == desktop.KeyShiftLeft || ev.Name == desktop.KeyShiftRight {
		e.shift = true
	}
}

// KeyUp clears the shift modifier.
func (e *codeEditor) KeyUp(ev *fyne.KeyEvent) {
	if ev.Name == desktop.KeyShiftLeft || ev.Name == desktop.KeyShiftRight {
		e.shift = false
	}
}

// TypedShortcut handles clipboard, undo/redo and select-all.
func (e *codeEditor) TypedShortcut(s fyne.Shortcut) {
	switch s.(type) {
	case *fyne.ShortcutCopy:
		e.copy()
	case *fyne.ShortcutCut:
		e.cut()
	case *fyne.ShortcutPaste:
		e.paste()
	case *fyne.ShortcutSelectAll:
		e.selectAll()
	case *fyne.ShortcutUndo:
		e.Undo()
	case *fyne.ShortcutRedo:
		e.Redo()
	}
}

// TypedRune inserts a typed character.
func (e *codeEditor) TypedRune(r rune) {
	if r == '\n' || r == '\r' {
		return
	}
	e.snapshot()
	e.deleteSelection()
	e.insertRune(r)
	e.clearSel()
	e.refresh()
}

// TypedKey handles navigation and editing keys.
func (e *codeEditor) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyBackspace:
		e.snapshot()
		if e.hasSel {
			e.deleteSelection()
		} else {
			e.backspace()
		}
		e.clearSel()
	case fyne.KeyDelete:
		e.snapshot()
		if e.hasSel {
			e.deleteSelection()
		} else {
			e.deleteForward()
		}
		e.clearSel()
	case fyne.KeyLeft:
		e.moveCursor(0, -1)
	case fyne.KeyRight:
		e.moveCursor(0, 1)
	case fyne.KeyUp:
		e.moveCursor(-1, 0)
	case fyne.KeyDown:
		e.moveCursor(1, 0)
	case fyne.KeyHome:
		e.moveCursor(0, -len([]rune(e.lines[e.row])))
	case fyne.KeyEnd:
		e.moveCursor(0, len([]rune(e.lines[e.row]))-e.col)
	case fyne.KeyReturn, fyne.KeyEnter:
		e.snapshot()
		e.deleteSelection()
		e.newline()
		e.clearSel()
	case fyne.KeyTab:
		e.snapshot()
		if e.shift {
			e.unindent()
		} else if e.hasSel {
			e.indent()
		} else {
			e.insertString("  ")
		}
		e.clearSel()
	default:
		return
	}
	e.refresh()
}

// Tapped moves the cursor to the tapped cell and clears the selection.
func (e *codeEditor) Tapped(ev *fyne.PointEvent) {
	rc := e.position(ev.Position)
	e.row, e.col = rc.row, rc.col
	e.clearSel()
	e.refresh()
}

// DoubleTapped selects the word under the cursor.
func (e *codeEditor) DoubleTapped(ev *fyne.PointEvent) {
	rc := e.position(ev.Position)
	e.row, e.col = rc.row, rc.col
	e.selectWord()
	e.refresh()
}

// Dragged extends the selection to the dragged cell.
func (e *codeEditor) Dragged(ev *fyne.DragEvent) {
	rc := e.position(ev.Position)
	if !e.hasSel {
		e.anchor = rowCol{e.row, e.col}
		e.hasSel = true
	}
	e.row, e.col = rc.row, rc.col
	e.refresh()
}

// DragEnd finishes a selection drag.
func (e *codeEditor) DragEnd() { e.refresh() }

// Undo restores the previous edit.
func (e *codeEditor) Undo() {
	if len(e.undo) == 0 {
		return
	}
	st := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.redo = append(e.redo, e.state())
	e.lines, e.row, e.col = st.lines, st.row, st.col
	e.clearSel()
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
	e.clearSel()
	e.refresh()
}

// ToggleComment comments or uncomments the selected lines.
func (e *codeEditor) ToggleComment() {
	e.snapshot()
	start, end := e.selRows()
	allCommented := true
	for r := start; r <= end; r++ {
		if !strings.HasPrefix(strings.TrimSpace(e.lines[r]), "--") {
			allCommented = false
			break
		}
	}
	for r := start; r <= end; r++ {
		if allCommented {
			idx := strings.Index(e.lines[r], "--")
			if idx >= 0 {
				e.lines[r] = strings.Replace(e.lines[r], "--", "", 1)
				_ = idx
			}
		} else {
			e.lines[r] = "--" + e.lines[r]
		}
	}
	e.refresh()
}

func (e *codeEditor) indent() {
	start, end := e.selRows()
	for r := start; r <= end; r++ {
		e.lines[r] = "  " + e.lines[r]
	}
}

func (e *codeEditor) unindent() {
	start, end := e.selRows()
	for r := start; r <= end; r++ {
		e.lines[r] = strings.TrimPrefix(e.lines[r], "  ")
	}
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

func (e *codeEditor) clearSel() {
	e.hasSel = false
	e.anchor = rowCol{e.row, e.col}
}

func (e *codeEditor) selectAll() {
	e.anchor = rowCol{0, 0}
	e.hasSel = true
	e.row = len(e.lines) - 1
	e.col = len([]rune(e.lines[e.row]))
	e.refresh()
}

func (e *codeEditor) selectWord() {
	line := []rune(e.lines[e.row])
	if e.col >= len(line) {
		return
	}
	start, end := e.col, e.col
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	for end < len(line) && isWordRune(line[end]) {
		end++
	}
	e.anchor = rowCol{e.row, start}
	e.col = end
	e.hasSel = start != end
}

func isWordRune(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// selRows returns the inclusive first and last selected rows.
func (e *codeEditor) selRows() (int, int) {
	if !e.hasSel {
		return e.row, e.row
	}
	a, b := e.anchor, rowCol{e.row, e.col}
	if a.row > b.row || (a.row == b.row && a.col > b.col) {
		a, b = b, a
	}
	return a.row, b.row
}

func (e *codeEditor) selection() (rowCol, rowCol, bool) {
	if !e.hasSel {
		return rowCol{}, rowCol{}, false
	}
	a, b := e.anchor, rowCol{e.row, e.col}
	if a.row > b.row || (a.row == b.row && a.col > b.col) {
		a, b = b, a
	}
	return a, b, true
}

func (e *codeEditor) deleteSelection() {
	a, b, ok := e.selection()
	if !ok {
		return
	}
	head := []rune(e.lines[a.row])[:a.col]
	tail := []rune(e.lines[b.row])[b.col:]
	merged := string(head) + string(tail)
	e.lines = append(e.lines[:a.row], append([]string{merged}, e.lines[b.row+1:]...)...)
	e.row, e.col = a.row, a.col
	e.clearSel()
}

func (e *codeEditor) selectedText() string {
	a, b, ok := e.selection()
	if !ok {
		return ""
	}
	if a.row == b.row {
		return string([]rune(e.lines[a.row])[a.col:b.col])
	}
	var parts []string
	parts = append(parts, string([]rune(e.lines[a.row])[a.col:]))
	for r := a.row + 1; r < b.row; r++ {
		parts = append(parts, e.lines[r])
	}
	parts = append(parts, string([]rune(e.lines[b.row])[:b.col]))
	return strings.Join(parts, "\n")
}

func (e *codeEditor) copy() {
	if text := e.selectedText(); text != "" {
		if c := fyne.CurrentApp().Clipboard(); c != nil {
			c.SetContent(text)
		}
	}
}

func (e *codeEditor) cut() {
	if !e.hasSel {
		return
	}
	e.copy()
	e.snapshot()
	e.deleteSelection()
	e.refresh()
}

func (e *codeEditor) paste() {
	c := fyne.CurrentApp().Clipboard()
	if c == nil {
		return
	}
	text := c.Content()
	if text == "" {
		return
	}
	e.snapshot()
	e.deleteSelection()
	for i, part := range strings.Split(text, "\n") {
		if i > 0 {
			e.newline()
		}
		e.insertString(part)
	}
	e.clearSel()
	e.refresh()
}

func (e *codeEditor) moveCursor(dRow, dCol int) {
	if !e.shift {
		e.hasSel = false
	}
	if e.shift && !e.hasSel {
		e.anchor = rowCol{e.row, e.col}
		e.hasSel = true
	}
	if dRow != 0 {
		row := e.row + dRow
		if row < 0 {
			row = 0
		}
		if row >= len(e.lines) {
			row = len(e.lines) - 1
		}
		e.row = row
		e.setCol(e.col)
	} else if dCol != 0 {
		col := e.col + dCol
		if col < 0 && e.row > 0 {
			e.row--
			col = len([]rune(e.lines[e.row]))
		} else if col > len([]rune(e.lines[e.row])) && e.row+1 < len(e.lines) {
			e.row++
			col = 0
		}
		e.setCol(col)
	}
}

func (e *codeEditor) setCol(col int) {
	n := len([]rune(e.lines[e.row]))
	if col < 0 {
		col = 0
	}
	if col > n {
		col = n
	}
	e.col = col
}

func (e *codeEditor) position(pos fyne.Position) rowCol {
	row, col := e.code.CursorLocationForPosition(pos.Subtract(e.code.Position()))
	if row < 0 {
		row = 0
	}
	if row >= len(e.lines) {
		row = len(e.lines) - 1
	}
	n := len([]rune(e.lines[row]))
	if col < 0 {
		col = 0
	}
	if col > n {
		col = n
	}
	return rowCol{row, col}
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
		e.col = len([]rune(e.lines[e.row-1]))
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
	indent := leadingSpace(string(line))
	rest := string(line[e.col:])
	e.lines[e.row] = string(line[:e.col])
	e.lines = append(e.lines[:e.row+1], append([]string{indent + rest}, e.lines[e.row+1:]...)...)
	e.row++
	e.col = len([]rune(indent))
}

func leadingSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[:i]
}

func (e *codeEditor) findNext() { e.findFrom(1) }
func (e *codeEditor) findPrev() { e.findFrom(-1) }

func (e *codeEditor) findFrom(dir int) {
	q := e.findEntry.Text
	if q == "" {
		return
	}
	text := e.Text()
	off := e.offset()
	if dir > 0 {
		idx := strings.Index(text[off+1:], q)
		if idx >= 0 {
			e.selectOffsets(off+1+idx, off+1+idx+len(q))
			return
		}
	} else {
		idx := strings.LastIndex(text[:off], q)
		if idx >= 0 {
			e.selectOffsets(idx, idx+len(q))
			return
		}
	}
	e.status.SetText("not found: " + q)
}

func (e *codeEditor) replaceOne() {
	q := e.findEntry.Text
	if q == "" {
		return
	}
	if e.selectedText() != q {
		e.findNext()
		return
	}
	e.snapshot()
	e.deleteSelection()
	e.insertString(e.replEntry.Text)
	e.clearSel()
	e.refresh()
}

func (e *codeEditor) replaceAll() {
	q := e.findEntry.Text
	if q == "" {
		return
	}
	e.snapshot()
	text := strings.ReplaceAll(e.Text(), q, e.replEntry.Text)
	e.SetTextKeepHistory(text)
	e.refresh()
}

// SetTextKeepHistory replaces the contents without clearing the undo history.
func (e *codeEditor) SetTextKeepHistory(s string) {
	e.lines = strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	e.row, e.col = 0, 0
	e.clearSel()
}

func (e *codeEditor) gotoDialog() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("line number")
	d := dialog.NewForm("Go to Line", "Go", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Line", entry)},
		func(ok bool) {
			if !ok {
				return
			}
			n, err := strconv.Atoi(strings.TrimSpace(entry.Text))
			if err != nil || n < 1 || n > len(e.lines) {
				return
			}
			e.row, e.col = n-1, 0
			e.clearSel()
			e.refresh()
		}, nil)
	d.Resize(fyne.NewSize(300, 150))
	d.Show()
}

func (e *codeEditor) offset() int {
	off := 0
	for r := 0; r < e.row; r++ {
		off += len([]rune(e.lines[r])) + 1
	}
	return off + e.col
}

func (e *codeEditor) selectOffsets(start, end int) {
	e.anchor = e.rowColAt(start)
	e.row, e.col = e.rowColAt(end).row, e.rowColAt(end).col
	e.hasSel = true
	e.refresh()
}

func (e *codeEditor) rowColAt(off int) rowCol {
	rem := off
	for r, line := range e.lines {
		n := len([]rune(line))
		if rem <= n {
			return rowCol{r, rem}
		}
		rem -= n + 1
	}
	last := len(e.lines) - 1
	return rowCol{last, len([]rune(e.lines[last]))}
}

func (e *codeEditor) refresh() {
	e.gutter.SetText(gutterText(len(e.lines)))
	e.code.SetText(e.Text())
	e.applyStyles()
	e.code.Refresh()
	e.gutter.Refresh()
	e.status.SetText(fmt.Sprintf("Ln %d, Col %d", e.row+1, e.col+1))
	if e.errorLine >= 0 {
		e.status.SetText(fmt.Sprintf("Ln %d, Col %d  —  %s", e.row+1, e.col+1, e.errorMsg))
	}
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

	current := &widget.CustomTextGridStyle{BGColor: theme.Color(theme.ColorNameHover)}
	if e.row >= 0 && e.row < len(e.lines) {
		for col := 0; col <= len([]rune(e.lines[e.row])); col++ {
			e.code.SetStyle(e.row, col, current)
		}
	}
	if a, b, ok := e.selection(); ok {
		sel := &widget.CustomTextGridStyle{BGColor: theme.Color(theme.ColorNameSelection)}
		for row := a.row; row <= b.row; row++ {
			start, end := 0, len([]rune(e.lines[row]))
			if row == a.row {
				start = a.col
			}
			if row == b.row {
				end = b.col
			}
			for col := start; col < end; col++ {
				e.code.SetStyle(row, col, sel)
			}
		}
	}

	for _, tok := range e.tokens(text) {
		style := e.tokenStyle(tok.kind)
		if style == nil {
			continue
		}
		s, en := tok.start, tok.end
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

	e.matchBrackets(runes)

	if e.row >= 0 && e.row < len(e.lines) {
		e.code.SetStyle(e.row, e.col, &widget.CustomTextGridStyle{
			FGColor: theme.Color(theme.ColorNameBackground),
			BGColor: theme.Color(theme.ColorNamePrimary),
		})
	}
	if e.errorLine >= 0 && e.errorLine < len(e.lines) {
		e.gutter.SetRowStyle(e.errorLine, &widget.CustomTextGridStyle{
			FGColor: theme.Color(theme.ColorNameBackground),
			BGColor: theme.Color(theme.ColorNameError),
		})
	}
}

func (e *codeEditor) matchBrackets(runes []rune) {
	off := e.offset()
	for _, at := range []int{off - 1, off} {
		if at < 0 || at >= len(runes) {
			continue
		}
		open, close := bracketPair(runes[at])
		if open == 0 {
			continue
		}
		match := findMatch(runes, at, runes[at], open, close)
		if match < 0 {
			continue
		}
		style := &widget.CustomTextGridStyle{BGColor: theme.Color(theme.ColorNamePrimary)}
		e.setOffsetStyle(at, style)
		e.setOffsetStyle(match, style)
		return
	}
}

func (e *codeEditor) setOffsetStyle(off int, style *widget.CustomTextGridStyle) {
	rc := e.rowColAt(off)
	e.code.SetStyle(rc.row, rc.col, style)
}

func bracketPair(r rune) (rune, rune) {
	switch r {
	case '(':
		return '(', ')'
	case ')':
		return '(', ')'
	case '[':
		return '[', ']'
	case ']':
		return '[', ']'
	case '{':
		return '{', '}'
	case '}':
		return '{', '}'
	}
	return 0, 0
}

func findMatch(runes []rune, at int, r, open, close rune) int {
	dir := 1
	if r == close {
		dir = -1
	}
	depth := 0
	for i := at; i >= 0 && i < len(runes); i += dir {
		switch runes[i] {
		case open:
			depth += dir
		case close:
			depth -= dir
		}
		if i != at && depth == 0 {
			return i
		}
	}
	return -1
}

type editorToken struct {
	kind       int
	start, end int
}

func (e *codeEditor) tokens(text string) []editorToken {
	if e.lang == langAutoasm {
		var out []editorToken
		for _, t := range autoasm.Tokenize(text) {
			out = append(out, editorToken{int(t.Kind), t.Start, t.End})
		}
		return out
	}
	var out []editorToken
	for _, t := range script.Tokenize(text) {
		out = append(out, editorToken{int(t.Kind), t.Start, t.End})
	}
	return out
}

func (e *codeEditor) tokenStyle(kind int) *widget.CustomTextGridStyle {
	if e.lang == langAutoasm {
		switch autoasm.TokenKind(kind) {
		case autoasm.TokenDirective, autoasm.TokenSection:
			return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNamePrimary)}
		case autoasm.TokenLabel:
			return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNameSuccess)}
		case autoasm.TokenNumber:
			return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNameWarning)}
		case autoasm.TokenString:
			return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNameSuccess)}
		case autoasm.TokenComment:
			return &widget.CustomTextGridStyle{FGColor: theme.Color(theme.ColorNameDisabled)}
		}
		return nil
	}
	switch script.TokenKind(kind) {
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
