//go:build linux

// Package hotkey registers system-wide keyboard shortcuts on X11 with
// XGrabKey. A combo is grabbed on the root window for the CapsLock/NumLock
// variants, so it fires while another application has focus. Grabs are
// exclusive: the focused application does not receive the key, unlike Cheat
// Engine's non-consuming poll. When no X display is available the manager is
// disabled and every method is a no-op.
package hotkey

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgbutil"
	"github.com/jezek/xgbutil/keybind"
	"github.com/jezek/xgbutil/xevent"

	"github.com/LCRERGO/firstspark/pkg/log"
)

// debounce ignores repeated presses (X autorepeat) of the same combo.
const debounce = 300 * time.Millisecond

type binding struct {
	mods uint16
	kc   xproto.Keycode
	cb   func()
	last time.Time
}

// Manager owns the X connection and the registered grabs.
type Manager struct {
	xu       *xgbutil.XUtil
	root     xproto.Window
	mu       sync.Mutex
	bindings map[string]*binding
	closed   bool
}

// New opens a dedicated X connection and starts its event loop. It returns
// (nil, nil) when no X display is available, in which case the manager is
// disabled and Available reports false.
func New() (*Manager, error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, nil
	}
	xu, err := xgbutil.NewConn()
	if err != nil {
		return nil, fmt.Errorf("hotkey: connect X: %w", err)
	}
	keybind.Initialize(xu)
	m := &Manager{xu: xu, root: xu.RootWin(), bindings: map[string]*binding{}}
	xevent.KeyPressFun(m.onKeyPress).Connect(xu, m.root)
	go xevent.Main(xu)
	log.Info("global hotkeys available", "display", os.Getenv("DISPLAY"))
	return m, nil
}

// Available reports whether global hotkeys can be registered.
func (m *Manager) Available() bool { return m != nil }

func (m *Manager) onKeyPress(_ *xgbutil.XUtil, ev xevent.KeyPressEvent) {
	mods, kc := keybind.DeduceKeyInfo(ev.State, ev.Detail)
	m.mu.Lock()
	var cb func()
	for _, b := range m.bindings {
		if b.mods == mods && b.kc == kc {
			if time.Since(b.last) < debounce {
				m.mu.Unlock()
				return
			}
			b.last = time.Now()
			cb = b.cb
			break
		}
	}
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// Register grabs combo (for example "Ctrl+Alt+S" or "F5") and calls cb when it
// is pressed anywhere. It returns an error when the combo cannot be parsed or
// is already grabbed by another client.
func (m *Manager) Register(id, combo string, cb func()) error {
	if m == nil {
		return nil
	}
	mods, kc, err := parseCombo(m.xu, combo)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("hotkey: manager closed")
	}
	if old, ok := m.bindings[id]; ok {
		keybind.Ungrab(m.xu, m.root, old.mods, old.kc)
		delete(m.bindings, id)
	}
	if err := keybind.GrabChecked(m.xu, m.root, mods, kc); err != nil {
		return fmt.Errorf("hotkey: %s: %w", combo, err)
	}
	m.bindings[id] = &binding{mods: mods, kc: kc, cb: cb}
	log.Debug("hotkey registered", "id", id, "combo", combo)
	return nil
}

// Unregister releases the grab for id, if any.
func (m *Manager) Unregister(id string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.bindings[id]; ok {
		keybind.Ungrab(m.xu, m.root, b.mods, b.kc)
		delete(m.bindings, id)
	}
}

// Close releases every grab and closes the X connection.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	for _, b := range m.bindings {
		keybind.Ungrab(m.xu, m.root, b.mods, b.kc)
	}
	m.bindings = map[string]*binding{}
	m.closed = true
	m.mu.Unlock()
	xevent.Quit(m.xu)
	m.xu.Conn().Close()
}

// parseCombo converts a canonical combo to X modifiers and a keycode.
func parseCombo(xu *xgbutil.XUtil, combo string) (uint16, xproto.Keycode, error) {
	s, err := xString(combo)
	if err != nil {
		return 0, 0, err
	}
	mods, kcs, err := keybind.ParseString(xu, s)
	if err != nil {
		return 0, 0, fmt.Errorf("hotkey: %s: %w", combo, err)
	}
	if len(kcs) == 0 {
		return 0, 0, fmt.Errorf("hotkey: unknown key in %q", combo)
	}
	return mods, kcs[0], nil
}

// xString converts a canonical combo ("Ctrl+Alt+S", "F5") to the
// "Control-Mod1-s" form keybind parses.
func xString(combo string) (string, error) {
	parts := strings.Split(combo, "+")
	if len(parts) == 0 || strings.TrimSpace(parts[len(parts)-1]) == "" {
		return "", fmt.Errorf("hotkey: empty combo")
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	if strings.HasPrefix(strings.ToUpper(key), "F") && len(key) > 1 {
		key = strings.ToUpper(key)
	} else {
		key = strings.ToLower(key)
	}
	mods := make([]string, 0, len(parts)-1)
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "ctrl", "control":
			mods = append(mods, "Control")
		case "alt":
			mods = append(mods, "Mod1")
		case "shift":
			mods = append(mods, "Shift")
		case "super", "meta", "cmd":
			mods = append(mods, "Mod4")
		default:
			return "", fmt.Errorf("hotkey: unknown modifier %q", p)
		}
	}
	return strings.Join(append(mods, key), "-"), nil
}
