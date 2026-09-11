// mytools is a small reimplementation of a subset of bedtools.
// Right now it only knows its own version; subcommands come later.
package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

const usage = `usage: mytools --version

mytools is a small reimplementation of a subset of bedtools.
No subcommands are implemented yet.
`

func main() {
	args := os.Args[1:]
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Printf("mytools %s\n", version)
		os.Exit(0)
	}
	// Usage goes to stderr and exits 2: stdout is data, and anything we do not
	// recognise is a usage error, not bad input data.
	fmt.Fprint(os.Stderr, usage)
	os.Exit(2)
}
