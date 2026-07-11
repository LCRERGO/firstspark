//go:build !gui

package ui

import (
	"errors"

	"github.com/LCRERGO/firstspark/pkg/config"
)

// ErrNotBuilt is returned when the binary was compiled without GUI support.
var ErrNotBuilt = errors.New("firstspark: GUI support not built in; install the OpenGL/X11 development headers and rebuild with `-tags gui`")

// Run reports that GUI support was not compiled in.
func Run(config.Config) error { return ErrNotBuilt }
