//go:build !linux || !amd64

package debugger

import "fmt"

// NewPtrace is only available on linux/amd64.
func NewPtrace(pid int) (Backend, error) {
	return nil, fmt.Errorf("debugger: ptrace backend requires linux/amd64")
}
