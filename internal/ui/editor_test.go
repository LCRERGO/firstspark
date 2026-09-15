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
