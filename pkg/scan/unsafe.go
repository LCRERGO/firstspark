package scan

import "unsafe"

// unsafePointer is isolated in its own file so the unsafe import does not
// leak into the scanning logic.
func unsafePointer(p *byte) unsafe.Pointer { return unsafe.Pointer(p) }
