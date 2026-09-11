package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/jsoubrier/mytools/internal/bed"
)

const sortUsage = `Tool:    mytools sort
Summary: Sorts a feature file in various and useful ways.

Usage:   mytools sort [OPTIONS] -i <bed>

Options: 
	-sizeA			Sort by feature size in ascending order.
	-sizeD			Sort by feature size in descending order.
	-chrThenSizeA		Sort by chrom (asc), then feature size (asc).
	-chrThenSizeD		Sort by chrom (asc), then feature size (desc).
	-chrThenScoreA		Sort by chrom (asc), then score (asc).
	-chrThenScoreD		Sort by chrom (asc), then score (desc).
	-g (genome.txt)	Sort according to the chromosomes declared in "genome.txt"
	-faidx (names.txt)	Sort according to the chromosomes declared in "names.txt"
	-header	Print the header from the input file prior to results.
`

// runSort implements `mytools sort`. It returns the process exit status: 0 or
// 1, never 2 (SPEC.md §5).
func runSort(args []string) int {
	var (
		input      string
		haveInput  bool
		header     bool
		opt        bed.Options
		orderFile  string
		orderIsSet bool
	)

	setMode := func(m bed.Mode) bool {
		if orderIsSet {
			usageError("\n*****\n*****ERROR: Sorting options are mutually exclusive.  Please choose just one. \n*****\n")
			return false
		}
		orderIsSet, opt.Mode = true, m
		return true
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		needValue := func() (string, bool) {
			if i+1 >= len(args) {
				usageError("\n*****ERROR: %s requires a value. *****\n", arg)
				return "", false
			}
			i++
			return args[i], true
		}

		switch arg {
		case "-h", "--help":
			fmt.Print(sortUsage)
			return 0
		case "-i":
			v, ok := needValue()
			if !ok {
				return 1
			}
			input, haveInput = v, true
		case "-header":
			header = true
		case "-sizeA":
			if !setMode(bed.SizeAsc) {
				return 1
			}
		case "-sizeD":
			if !setMode(bed.SizeDesc) {
				return 1
			}
		case "-chrThenSizeA":
			if !setMode(bed.ChromThenSizeAsc) {
				return 1
			}
		case "-chrThenSizeD":
			if !setMode(bed.ChromThenSizeDesc) {
				return 1
			}
		case "-chrThenScoreA":
			if !setMode(bed.ChromThenScoreAsc) {
				return 1
			}
		case "-chrThenScoreD":
			if !setMode(bed.ChromThenScoreDesc) {
				return 1
			}
		case "-g", "-faidx":
			// -g and -faidx are part of the same mutually exclusive group as
			// the ordering flags: bedtools rejects `-g x -sizeA` and even
			// `-g x -faidx x`.
			if !setMode(bed.ChromThenStart) {
				return 1
			}
			v, ok := needValue()
			if !ok {
				return 1
			}
			orderFile = v
		default:
			// A single bare positional argument stands in for -i, a mytools
			// convenience over bedtools (SPEC.md §5); bedtools itself rejects
			// it as an unrecognized parameter.
			if (strings.HasPrefix(arg, "-") && arg != "-") || haveInput {
				usageError("\n*****ERROR: Unrecognized parameter: %s *****\n\n", arg)
				return 1
			}
			input, haveInput = arg, true
		}
	}

	if orderFile != "" {
		order, err := bed.ReadChromOrder(orderFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		opt.ChromOrder, opt.ChromOrderFile = order, orderFile
	}

	// With no -i and no positional argument, read stdin — that is what
	// bedtools does, and the oracle decides (SPEC.md §1).
	f, err := bed.Read(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	records, err := bed.Sort(f, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	if header {
		for _, h := range f.Header {
			fmt.Fprintln(out, h)
		}
	}
	for _, r := range records {
		fmt.Fprintln(out, r.Line())
	}
	return 0
}

// usageError prints a bedtools-shaped error plus the usage block to stderr.
// stdout stays clean: it is data (CLAUDE.md).
func usageError(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprint(os.Stderr, sortUsage)
}
