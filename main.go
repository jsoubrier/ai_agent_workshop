// mytools is a small reimplementation of a subset of bedtools, in Go.
//
// Real bedtools v2.31.1 is the oracle: any observable difference on the same
// input — output, exit code or error text — is a bug in mytools. See SPEC.md.
package main

import (
	"fmt"
	"os"

	"github.com/jsoubrier/mytools/internal/cli"
)

const version = "0.1.0"

const usage = `usage: mytools <subcommand> [options]

mytools is a small reimplementation of a subset of bedtools.

Subcommands:
  sort        Sort a BED file by chromosome, then start, then end.

Options:
  --version   Print the version and exit.
  -h          Print this help and exit.

A filename of - means stdin, and at most one input per invocation may be stdin.
Errors go to stderr; stdout is data. Exit status is 0 on success and 1 on any
error, matching bedtools.
`

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		// Usage asked for, not usage forced by a mistake: stdout, exit 0.
		fmt.Print(usage)
		return cli.ExitOK
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return cli.ExitOK
	case "--version", "-version", "version":
		fmt.Printf("mytools %s\n", version)
		return cli.ExitOK
	case "sort":
		return runSort(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "error: unrecognized command: %s\n\n", args[0])
		return cli.ExitError
	}
}
