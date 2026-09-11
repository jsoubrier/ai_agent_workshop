// mytools is a small reimplementation of a subset of bedtools.
package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

const usage = `usage: mytools <subcommand> [options]

mytools is a small reimplementation of a subset of bedtools.

Subcommands:
	sort	Sort a BED file in various and useful ways.

Other:
	--version	Print the version and exit.
	-h		Print this help and exit.
`

func main() {
	args := os.Args[1:]
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Printf("mytools %s\n", version)
		os.Exit(0)
	}
	// Usage asked for is not an error: it goes to stdout and exits 0
	// (SPEC.md §5). Usage printed *because* of an error goes to stderr.
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(usage)
		os.Exit(0)
	}

	switch args[0] {
	case "sort":
		os.Exit(runSort(args[1:]))
	default:
		fmt.Fprintf(os.Stderr, "\n*****ERROR: Unrecognized subcommand: %s *****\n\n", args[0])
		fmt.Fprint(os.Stderr, usage)
		// Exit 1, never 2: mytools returns bedtools' exit codes, and bedtools
		// exits 1 for usage errors too (SPEC.md §5 and §9.1).
		os.Exit(1)
	}
}
