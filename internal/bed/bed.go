// Package bed implements the plumbing every mytools subcommand sits on: BED
// record parsing, header-line handling, the oracle's error messages, and the
// 2 MiB input cap (SPEC.md §4, §6, §7).
//
// bedtools v2.31.1 is the oracle. Every message and every quirk reproduced in
// this package was probed against it rather than assumed; where the behaviour
// is surprising, the comment says so and the tests encode it as-is.
package bed

import (
	"fmt"
	"strconv"
	"strings"
)

// Record is one BED feature. Fields holds every column of the source line, so
// that a record echoed whole round-trips byte for byte (SPEC.md §4.1).
//
// Columns 2 and 3 are the one exception: they are re-rendered from the parsed
// integers, because bedtools does the same. `chr1\t007\t20` comes back out of
// `bedtools sort` as `chr1\t7\t20`, so storing the raw text would diverge.
type Record struct {
	Chrom  string
	Start  int64
	End    int64
	Fields []string
	Line   int // physical 1-based line number the record was read from
}

// String renders the record as bedtools would write it: tab-delimited, every
// column preserved, no trailing tab.
func (r *Record) String() string { return strings.Join(r.Fields, "\t") }

// Field returns column i (0-based) or "" when the record is too short. Column
// meanings mytools interprets: 0 chrom, 1 start, 2 end, 4 score, 5 strand,
// 9-11 blocks. Everything else is carried opaquely.
func (r *Record) Field(i int) string {
	if i < 0 || i >= len(r.Fields) {
		return ""
	}
	return r.Fields[i]
}

// Strand returns column 6, or "" on BED3/BED4/BED5 input.
func (r *Record) Strand() string { return r.Field(5) }

// Score returns column 5, or "" when absent.
func (r *Record) Score() string { return r.Field(4) }

// IsHeaderLine reports whether a line is a header/comment line: one beginning
// with "#", "track" or "browser".
//
// The match is a bare prefix test, which is bedtools' own rule and is wider
// than it looks: `trackx<TAB>1<TAB>2` and `browserfoo<TAB>1<TAB>2` are both
// swallowed as header lines rather than parsed as features (probed). Do not
// tighten this to require a word boundary — the oracle does not.
func IsHeaderLine(line string) bool {
	return strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "track") ||
		strings.HasPrefix(line, "browser")
}

// SplitFields splits a BED line into columns on tabs, dropping the single
// empty column that a trailing tab would otherwise produce.
//
// This mirrors bedtools' getline-based tokeniser: `chr1\t100\t200\t` yields
// three columns, while `chr1\t100\t200\t\t` yields four, the last one empty
// (probed). Only the final delimiter is absorbed.
func SplitFields(line string) []string {
	f := strings.Split(line, "\t")
	if len(f) > 1 && f[len(f)-1] == "" {
		f = f[:len(f)-1]
	}
	return f
}

// isUnsigned reports whether s is a non-empty run of ASCII digits. bedtools'
// first-record format sniffer accepts exactly this — "007" passes, while "+5",
// "1e3", " 5" and "-5" are all rejected as "non-integer" (probed).
func isUnsigned(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ParseRecord turns one non-empty, non-header line into a Record, applying the
// oracle's validation in the oracle's order (SPEC.md §7).
//
// first must be true for the first feature line of a file. bedtools validates
// that line with its file-format sniffer and every later line with a different,
// stricter code path, and the two disagree about what a bad coordinate is:
//
//	line 1:  chr1 -5 200  ->  "Unexpected file format. ... non-integer starts or ends at line 1?"
//	line 2:  chr1 -5 200  ->  "Error: malformed BED entry at line 2. Start Coordinate detected that is < 0. Exiting."
//
// Both exit 1. This is bedtools' behaviour, not a design of ours; it is encoded
// because the oracle is the specification.
func ParseRecord(line string, lineno int, name string, first bool) (*Record, error) {
	fields := SplitFields(line)
	if len(fields) < 3 {
		return nil, errFewColumns(lineno, name)
	}

	var start, end int64
	if first {
		// The format sniffer only accepts unsigned decimal, so a negative
		// coordinate is reported as "non-integer" here.
		if !isUnsigned(fields[1]) || !isUnsigned(fields[2]) {
			return nil, errUnexpectedFormat(lineno)
		}
		start, _ = strconv.ParseInt(fields[1], 10, 64)
		end, _ = strconv.ParseInt(fields[2], 10, 64)
	} else {
		var err1, err2 error
		start, err1 = strconv.ParseInt(fields[1], 10, 64)
		end, err2 = strconv.ParseInt(fields[2], 10, 64)
		if err1 != nil || err2 != nil {
			// Deliberate divergence, and the only one in this package.
			// Real bedtools calls stoll() here with no guard, so a
			// non-integer coordinate on any line after the first raises
			// std::invalid_argument and the process dies on SIGABRT with a
			// core dump (exit 134), printing nothing useful. We refuse to
			// reproduce a crash: we report the same message the oracle gives
			// for line 1 and exit 1. Noted in the golden suite, which keeps
			// its non-integer case on line 1 so it still matches the oracle.
			return nil, errUnexpectedFormat(lineno)
		}
		// Negative start is checked before negative end, and both before
		// start > end (probed with chr1 -5 -20, which reports the start).
		if start < 0 {
			return nil, errNegativeCoord(lineno, "Start")
		}
		if end < 0 {
			return nil, errNegativeCoord(lineno, "End")
		}
	}
	if start > end {
		return nil, errStartAfterEnd(lineno)
	}

	// Re-render the coordinates so that "007" normalises to "7" as bedtools
	// does; every other column stays exactly as it was read.
	fields[1] = strconv.FormatInt(start, 10)
	fields[2] = strconv.FormatInt(end, 10)

	return &Record{
		Chrom:  fields[0],
		Start:  start,
		End:    end,
		Fields: fields,
		Line:   lineno,
	}, nil
}

// CompareChrom orders chromosome names the way bedtools does: byte-lexicographic,
// case-sensitive, with no natural or numeric ordering. "chr10" sorts before
// "chr2" and "chr17" before "chr7" (SPEC.md §2.1).
func CompareChrom(a, b string) int { return strings.Compare(a, b) }

func errFewColumns(lineno int, name string) error {
	// Two spaces after the full stop are bedtools' own; reproduced verbatim.
	return fmt.Errorf("It looks as though you have less than 3 columns at line %d in file %s.  Are you sure your files are tab-delimited?", lineno, name)
}

func errUnexpectedFormat(lineno int) error {
	return fmt.Errorf("Unexpected file format.  Please use tab-delimited BED, GFF, or VCF. Perhaps you have non-integer starts or ends at line %d?", lineno)
}

func errStartAfterEnd(lineno int) error {
	return fmt.Errorf("Error: malformed BED entry at line %d. Start was greater than end. Exiting.", lineno)
}

func errNegativeCoord(lineno int, which string) error {
	return fmt.Errorf("Error: malformed BED entry at line %d. %s Coordinate detected that is < 0. Exiting.", lineno, which)
}
