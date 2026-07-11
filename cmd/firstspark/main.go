// Command firstspark is a Cheat Engine style memory scanner and debugger for
// Linux.
package main

import (
	"fmt"
	"os"

	"github.com/LCRERGO/firstspark/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "firstspark:", err)
		os.Exit(1)
	}
}
