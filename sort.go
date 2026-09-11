package main

import (
	"bufio"
	"errors"
	"os"
	"sort"

	"github.com/jsoubrier/mytools/internal/bed"
	"github.com/jsoubrier/mytools/internal/cli"
)

const sortUsage = `usage: mytools sort [-i <bed>] [-header]

Sorts a BED file by chromosome, then start, then end. Chromosome order is
byte-lexicographic, so chr10 sorts before chr2. Ties keep input order.

Options:
  -i <bed>   Input file. - means stdin. Defaults to stdin.
  -header    Print the leading #/track/browser lines ahead of the records.
  -h         Print this help and exit.

A single bare filename is accepted in place of -i, as a convenience over
bedtools: mytools sort foo.bed
`

// runSort implements the default ordering of SPEC.md §2.1 only. The rest of
// bedtools sort's flag set (-sizeA/-sizeD, -chrThenSizeA/D, -chrThenScoreA/D,
// -g, -faidx) belongs to issue #6 and slots in alongside -header here; this
// much exists so that issue #5's golden cases have a subcommand to run.
func runSort(args []string) int {
	fs := cli.NewFlagSet("sort", sortUsage)
	in := fs.String("i", "")
	header := fs.Bool("header")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, cli.ErrHelp) {
			return fs.Help()
		}
		return fs.Fail(err)
	}

	name := *in
	switch {
	case name == "" && len(fs.Args()) == 1:
		name = fs.Args()[0]
	case name == "" && len(fs.Args()) == 0:
		// bedtools sort with no -i reads stdin (probed).
		name = "-"
	case len(fs.Args()) > 0:
		return fs.Fail(errors.New("\n*****ERROR: sort takes at most one input file *****\n"))
	}

	if err := bed.CheckSingleStdin(name); err != nil {
		return fs.Fail(err)
	}

	r, err := bed.Open(name)
	if err != nil {
		return cli.Fatal(err)
	}
	defer r.Close()

	// sort is the one command allowed to hold its input in memory; the 2 MiB
	// cap is what makes that safe (SPEC.md §6).
	recs, err := bed.ReadAll(r)
	if err != nil {
		return cli.Fatal(err)
	}

	sort.SliceStable(recs, func(i, j int) bool {
		a, b := recs[i], recs[j]
		if c := bed.CompareChrom(a.Chrom, b.Chrom); c != 0 {
			return c < 0
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.End < b.End
	})

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	if *header {
		for _, h := range r.Header() {
			out.WriteString(h)
			out.WriteByte('\n')
		}
	}
	for _, rec := range recs {
		out.WriteString(rec.String())
		out.WriteByte('\n')
	}
	return cli.ExitOK
}
