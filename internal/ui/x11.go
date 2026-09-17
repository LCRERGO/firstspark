//go:build gui

package ui

import (
	"image"
	"os"
	"sync"

	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgbutil"
	"github.com/jezek/xgbutil/ewmh"
	"github.com/jezek/xgbutil/xprop"
)

// x11Client is a lazily connected X11 client used to read window icons and
// WM_CLASS. It disables itself when there is no X display (pure Wayland or
// headless), so its methods are always safe to call.
type x11Client struct {
	once sync.Once
	xu   *xgbutil.XUtil
}

func (c *x11Client) conn() *xgbutil.XUtil {
	c.once.Do(func() {
		if os.Getenv("DISPLAY") == "" {
			return
		}
		if xu, err := xgbutil.NewConn(); err == nil {
			c.xu = xu
		}
	})
	return c.xu
}

// activePID returns the PID of the window that currently has input focus, or
// an error when there is no X display or no active window.
func (c *x11Client) activePID() (int, error) {
	xu := c.conn()
	if xu == nil {
		return 0, os.ErrNotExist
	}
	win, err := ewmh.ActiveWindowGet(xu)
	if err != nil {
		return 0, err
	}
	pid, err := ewmh.WmPidGet(xu, win)
	if err != nil {
		return 0, err
	}
	return int(pid), nil
}

// pidWindows maps each PID to one of its X11 windows.
func (c *x11Client) pidWindows() map[int]xproto.Window {
	xu := c.conn()
	if xu == nil {
		return nil
	}
	wins, err := ewmh.ClientListGet(xu)
	if err != nil {
		return nil
	}
	m := make(map[int]xproto.Window, len(wins))
	for _, w := range wins {
		pid, err := ewmh.WmPidGet(xu, w)
		if err != nil {
			continue
		}
		if _, exists := m[int(pid)]; !exists {
			m[int(pid)] = w
		}
	}
	return m
}

// icon returns the most appropriate _NET_WM_ICON for a window.
func (c *x11Client) icon(win xproto.Window) image.Image {
	xu := c.conn()
	if xu == nil || win == 0 {
		return nil
	}
	icons, err := ewmh.WmIconGet(xu, win)
	if err != nil || len(icons) == 0 {
		return nil
	}
	best := icons[0]
	bestScore := iconScore(best)
	for _, ic := range icons[1:] {
		if s := iconScore(ic); s < bestScore {
			best, bestScore = ic, s
		}
	}
	if best.Width == 0 || best.Height == 0 {
		return nil
	}
	return wmIconImage(best)
}

// wmClass returns the WM_CLASS class (the second element) for a window.
func (c *x11Client) wmClass(win xproto.Window) string {
	xu := c.conn()
	if xu == nil || win == 0 {
		return ""
	}
	prop, err := xprop.GetProperty(xu, win, "WM_CLASS")
	if err != nil {
		return ""
	}
	strs, err := xprop.PropValStrs(prop, nil)
	if err != nil || len(strs) == 0 {
		return ""
	}
	return strs[len(strs)-1]
}

// iconScore prefers icons close to 32px, penalising anything under 16px.
func iconScore(ic ewmh.WmIcon) int {
	if ic.Width == 0 {
		return 1 << 20
	}
	diff := int(ic.Width) - 32
	if diff < 0 {
		diff = -diff
	}
	if ic.Width < 16 {
		return 1 << 10
	}
	return diff
}

func wmIconImage(ic ewmh.WmIcon) image.Image {
	w, h := int(ic.Width), int(ic.Height)
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h && i < len(ic.Data); i++ {
		px := ic.Data[i]
		img.Pix[i*4+0] = uint8(px >> 16)
		img.Pix[i*4+1] = uint8(px >> 8)
		img.Pix[i*4+2] = uint8(px)
		img.Pix[i*4+3] = uint8(px >> 24)
	}
	return img
}
