//go:build gui

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestCodeEditorEditing(t *testing.T) {
	e := newCodeEditor(nil)
	e.SetText("ab")
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	e.TypedRune('c')
	if e.Text() != "abc" {
		t.Fatalf("after typing: %q", e.Text())
	}
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if e.Text() != "ab" {
		t.Fatalf("after backspace: %q", e.Text())
	}
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	e.TypedRune('d')
	if e.Text() != "ab\nd" {
		t.Fatalf("after newline: %q", e.Text())
	}
	e.Undo()
	if e.Text() == "ab\nd" {
		t.Fatal("undo did not change the text")
	}
}

func TestCodeEditorHighlighting(t *testing.T) {
	e := newCodeEditor(nil)
	e.SetText("local x = 1")
	e.applyStyles()
}

func TestCodeEditorSelectionAndComment(t *testing.T) {
	e := newCodeEditor(nil)
	e.SetText("local x = 1\nlocal y = 2")
	e.selectAll()
	if got := e.selectedText(); got != "local x = 1\nlocal y = 2" {
		t.Fatalf("selection = %q", got)
	}
	e.ToggleComment()
	if e.lines[0] != "--local x = 1" || e.lines[1] != "--local y = 2" {
		t.Fatalf("commented = %q", e.lines)
	}
	e.ToggleComment()
	if e.lines[0] != "local x = 1" {
		t.Fatalf("uncommented = %q", e.lines[0])
	}
}

func TestCodeEditorFindAndReplace(t *testing.T) {
	e := newCodeEditor(nil)
	e.SetText("alpha beta gamma")
	e.findEntry.SetText("beta")
	e.findNext()
	if got := e.selectedText(); got != "beta" {
		t.Fatalf("found %q", got)
	}
	e.replEntry.SetText("BETA")
	e.replaceOne()
	if e.Text() != "alpha BETA gamma" {
		t.Fatalf("text = %q", e.Text())
	}
	e.findEntry.SetText("a")
	e.replEntry.SetText("A")
	e.replaceAll()
	if e.Text() != "AlphA BETA gAmmA" {
		t.Fatalf("replace all = %q", e.Text())
	}
}
