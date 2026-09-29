//go:build gui

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// tabItem is one entry in the scan-tab view.
type tabItem struct {
	Text    string
	Content fyne.CanvasObject
}

// tabView is a horizontal, closable tab bar whose tabs are renamed by
// double-clicking them, like the reference tool. Fyne's DocTabs exposes no
// tab-button hook, so this small view provides the interaction.
type tabView struct {
	widget.BaseWidget

	items      []*tabItem
	selected   int
	onSelected func(*tabItem)
	onClosed   func(*tabItem)
	onRename   func(*tabItem)
	onCreate   func()

	buttons *fyne.Container
	body    *fyne.Container
}

func newTabView() *tabView {
	t := &tabView{selected: -1}
	t.ExtendBaseWidget(t)
	t.buttons = container.NewHBox()
	t.body = container.NewMax()
	return t
}

func (t *tabView) CreateRenderer() fyne.WidgetRenderer {
	bar := container.NewHScroll(t.buttons)
	content := container.NewBorder(bar, nil, nil, nil, t.body)
	return widget.NewSimpleRenderer(content)
}

// Append adds a tab at the end and selects it.
func (t *tabView) Append(item *tabItem) {
	t.items = append(t.items, item)
	t.rebuild()
	t.Select(item)
}

// SetItems replaces every tab.
func (t *tabView) SetItems(items []*tabItem) {
	t.items = items
	t.selected = -1
	t.rebuild()
	t.body.Objects = nil
	t.body.Refresh()
}

// RemoveIndex drops the tab at i and selects a neighbour.
func (t *tabView) RemoveIndex(i int) {
	if i < 0 || i >= len(t.items) {
		return
	}
	t.items = append(t.items[:i], t.items[i+1:]...)
	t.rebuild()
	if len(t.items) == 0 {
		t.selected = -1
		t.body.Objects = nil
		t.body.Refresh()
		return
	}
	if t.selected >= len(t.items) {
		t.selected = len(t.items) - 1
	}
	t.Select(t.items[t.selected])
}

// Selected returns the active tab, or nil.
func (t *tabView) Selected() *tabItem {
	if t.selected < 0 || t.selected >= len(t.items) {
		return nil
	}
	return t.items[t.selected]
}

// SelectedIndex returns the active tab index, or -1.
func (t *tabView) SelectedIndex() int { return t.selected }

// SelectIndex activates the tab at i.
func (t *tabView) SelectIndex(i int) {
	if i < 0 || i >= len(t.items) {
		return
	}
	t.Select(t.items[i])
}

// Select activates item.
func (t *tabView) Select(item *tabItem) {
	idx := t.indexOf(item)
	if idx < 0 {
		return
	}
	changed := idx != t.selected
	t.selected = idx
	t.body.Objects = []fyne.CanvasObject{item.Content}
	t.body.Refresh()
	t.refreshButtons()
	if changed && t.onSelected != nil {
		t.onSelected(item)
	}
}

func (t *tabView) indexOf(item *tabItem) int {
	for i, it := range t.items {
		if it == item {
			return i
		}
	}
	return -1
}

// rename opens the rename prompt for item.
func (t *tabView) rename(item *tabItem) {
	if t.onRename != nil {
		t.onRename(item)
	}
}

func (t *tabView) rebuild() {
	t.buttons.Objects = t.buttons.Objects[:0]
	for _, it := range t.items {
		t.buttons.Add(newTabButton(t, it))
	}
	if t.onCreate != nil {
		add := widget.NewButtonWithIcon("", theme.ContentAddIcon(), t.onCreate)
		add.Importance = widget.LowImportance
		t.buttons.Add(add)
	}
	t.buttons.Refresh()
}

func (t *tabView) refreshButtons() {
	for _, o := range t.buttons.Objects {
		if b, ok := o.(*tabButton); ok {
			b.setActive(b.item == t.Selected())
		}
	}
}

func (t *tabView) Refresh() {
	t.refreshButtons()
	t.BaseWidget.Refresh()
}

// tabButton is one tab: a label with a close button. Double-clicking it
// renames the tab.
type tabButton struct {
	widget.BaseWidget

	view     *tabView
	item     *tabItem
	label    *canvas.Text
	closeBtn *widget.Button
	active   bool
}

func newTabButton(view *tabView, item *tabItem) *tabButton {
	b := &tabButton{view: view, item: item}
	b.label = canvas.NewText(item.Text, theme.Color(theme.ColorNameForeground))
	b.label.TextSize = theme.Size(theme.SizeNameText)
	b.closeBtn = widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		if view.onClosed != nil {
			view.onClosed(item)
		}
	})
	b.closeBtn.Importance = widget.LowImportance
	b.closeBtn.Hide()
	b.ExtendBaseWidget(b)
	return b
}

func (b *tabButton) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewHBox(b.label, b.closeBtn))
}

func (b *tabButton) setActive(active bool) {
	b.active = active
	if active {
		b.label.Color = theme.Color(theme.ColorNamePrimary)
	} else {
		b.label.Color = theme.Color(theme.ColorNameForeground)
	}
	b.label.Text = b.item.Text
	b.label.Refresh()
}

func (b *tabButton) Tapped(*fyne.PointEvent) { b.view.Select(b.item) }

func (b *tabButton) DoubleTapped(*fyne.PointEvent) {
	b.view.Select(b.item)
	b.view.rename(b.item)
}

func (b *tabButton) MouseIn(*desktop.MouseEvent) { b.closeBtn.Show() }

func (b *tabButton) MouseOut() { b.closeBtn.Hide() }
