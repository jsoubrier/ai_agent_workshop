// Package bed parses BED records the way bedtools does.
//
// bedtools is the oracle (CLAUDE.md): where its parser accepts, rejects or
// rewrites something, so does this one, including the details that look like
// bugs. Each of those carries a comment saying so.
package bed

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// MaxInputBytes is the per-file input cap from SPEC.md §6: 2 MiB, applied
// independently to every input file, so that `sort` can hold a whole file in
// memory without the process outgrowing the VM.
const MaxInputBytes = 2 * 1024 * 1024

// Record is one BED feature. Fields holds every column verbatim, so records are
// echoed with their trailing columns intact (SPEC.md §4.1). Fields[1] and
// Fields[2] are the *normalised* coordinates: bedtools reparses and reprints
// start and end, so "0100" comes back out as "100".
type Record struct {
	Chrom  string
	Start  int64
	End    int64
	Fields []string
}

// bedtools widens a zero-length feature by one base on each side before it
// compares feature sizes: (100,100) behaves as (99,101). That is why a
// zero-length feature ties with a 2 bp one under -sizeA/-chrThenSizeA and
// sorts ahead of it when their starts tie. The widening applies to the size
// orderings only — the default ordering still sorts (500,500) at 500, not 499.
// Probed against bedtools v2.31.1; reproduced, not rationalised (CLAUDE.md).
func (r Record) zeroLength() bool { return r.Start == r.End }

// SizeStart and SizeEnd are the widened coordinates used by the size
// orderings.
func (r Record) SizeStart() int64 {
	if r.zeroLength() {
		return r.Start - 1
	}
	return r.Start
}

func (r Record) SizeEnd() int64 {
	if r.zeroLength() {
		return r.End + 1
	}
	return r.End
}

// Size is the feature length the -size* orderings compare: 2 for a
// zero-length feature, end - start for every other.
func (r Record) Size() int64 { return r.SizeEnd() - r.SizeStart() }

// Score is BED column 5, compared as a string because bedtools compares it as a
// string: "-5" < "100" < "20" < "3" < "9.5" are in sorted order for
// -chrThenScoreA. Do not "fix" this into a numeric comparison.
func (r Record) Score() string {
	if len(r.Fields) >= 5 {
		return r.Fields[4]
	}
	return ""
}

// Line renders the record for output.
func (r Record) Line() string { return strings.Join(r.Fields, "\t") }

// File is a parsed BED file.
type File struct {
	Records []Record
	// Header holds the leading #/track/browser lines, verbatim and in order.
	// Only the leading block counts: such lines appearing after the first
	// record are dropped and never reprinted, which is what bedtools does.
	Header []string
	// BedType is the column count fixed by the first record, 0 if the file
	// held no records.
	BedType int
}

// Error is a bad-input error. Every one of them exits 1 (SPEC.md §5).
type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

func errf(format string, a ...any) error { return &Error{msg: fmt.Sprintf(format, a...)} }

// Read parses path, or stdin when path is "-" or empty. name is the spelling
// used in error messages.
func Read(path string) (*File, error) {
	if path == "-" || path == "" {
		return readStdin()
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		// bedtools' wording, verbatim.
		return nil, errf("Error: The requested file (%s) could not be opened. Error message: (No such file or directory). Exiting!", path)
	}
	// A regular file's size is known up front, so the cap is checked before a
	// single byte is read (SPEC.md §6).
	if info.Mode().IsRegular() && info.Size() > MaxInputBytes {
		return nil, errf("Error: input file %s is %d bytes, which exceeds the %d byte limit. Exiting.", path, info.Size(), MaxInputBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errf("Error: The requested file (%s) could not be opened. Error message: (No such file or directory). Exiting!", path)
	}
	defer f.Close()
	return parse(f, path, false)
}

func readStdin() (*File, error) { return ReadFrom(os.Stdin, "stdin") }

// ReadFrom parses BED records from r, applying the 2 MiB cap as the bytes
// stream past — the form used for stdin, where the size is not known up front.
func ReadFrom(r io.Reader, name string) (*File, error) { return parse(r, name, true) }

// parse reads BED records from r. When countBytes is set the 2 MiB cap is
// applied as the bytes stream past, aborting the moment it is passed rather
// than buffering the whole stream to measure it (SPEC.md §6).
func parse(r io.Reader, name string, countBytes bool) (*File, error) {
	out := &File{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxInputBytes+1)

	var total int64
	lineNum := 0
	for sc.Scan() {
		line := sc.Text()
		if countBytes {
			total += int64(len(line)) + 1
			if total > MaxInputBytes {
				return nil, errf("Error: input on stdin exceeds the %d byte limit. Exiting.", MaxInputBytes)
			}
		}
		// bedtools numbers every physical line, headers and blanks included.
		lineNum++
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		if isHeaderLine(line) {
			if len(out.Records) == 0 {
				out.Header = append(out.Header, line)
			}
			continue
		}
		rec, err := parseRecord(line, lineNum, name, &out.BedType)
		if err != nil {
			return nil, err
		}
		out.Records = append(out.Records, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, errf("Error: The requested file (%s) could not be opened. Error message: (%v). Exiting!", name, err)
	}
	return out, nil
}

// isHeaderLine matches bedtools: a leading "#", "track" or "browser" prefix.
// It is a prefix test, not a substring one — "chrtrack" and "ch#r1" are
// ordinary chromosome names and bedtools sorts them as records.
func isHeaderLine(line string) bool {
	return strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "track") ||
		strings.HasPrefix(line, "browser")
}

func parseRecord(line string, lineNum int, name string, bedType *int) (Record, error) {
	// bedtools strips one trailing tab before tokenising, so "chr1\t100\t200\t"
	// is a 3-field BED3 record while "chr1\t100\t200\t\t" is a BED4 whose name
	// is the empty string. Interior empty fields are kept either way.
	fields := strings.Split(strings.TrimSuffix(line, "\t"), "\t")

	if len(fields) < 3 {
		return Record{}, errf("It looks as though you have less than 3 columns at line %d in file %s.  Are you sure your files are tab-delimited?", lineNum, name)
	}
	if *bedType == 0 {
		// The first record fixes the column count for the whole file, and it
		// is BED only if columns 2 and 3 are runs of digits. bedtools' own
		// isInteger() rejects a leading "-", which is why a negative *end* is
		// reported as "non-integer" rather than as a negative coordinate.
		if !allDigits(fields[1]) || !allDigits(fields[2]) {
			return Record{}, errf("Unexpected file format.  Please use tab-delimited BED, GFF, or VCF. Perhaps you have non-integer starts or ends at line %d?", lineNum)
		}
		*bedType = len(fields)
	} else if len(fields) != *bedType {
		return Record{}, errf("Differing number of BED fields encountered at line: %d.  Exiting...", lineNum)
	}

	start, errS := strconv.ParseInt(fields[1], 10, 64)
	end, errE := strconv.ParseInt(fields[2], 10, 64)
	if errS != nil || errE != nil {
		// Divergence, deliberate and documented in tests/run_golden.sh: real
		// bedtools v2.31.1 does not handle this at all, it lets std::stoll
		// throw and dies on SIGABRT (exit 134). We exit 1 with the message
		// SPEC.md §7 gives for the case.
		return Record{}, errf("Unexpected file format.  Please use tab-delimited BED, GFF, or VCF. Perhaps you have non-integer starts or ends at line %d?", lineNum)
	}
	switch {
	case start > end:
		return Record{}, errf("Error: malformed BED entry at line %d. Start was greater than end. Exiting.", lineNum)
	case start < 0:
		return Record{}, errf("Error: malformed BED entry at line %d. Start Coordinate detected that is < 0. Exiting.", lineNum)
	case end < 0:
		return Record{}, errf("Error: malformed BED entry at line %d. Coordinate detected that is < 0. Exiting.", lineNum)
	}
	// start == end is legal: zero-length features are real input here.

	fields[1] = strconv.FormatInt(start, 10)
	fields[2] = strconv.FormatInt(end, 10)
	return Record{Chrom: fields[0], Start: start, End: end, Fields: fields}, nil
}

// allDigits is bedtools' isInteger(): every character a digit, and — note —
// true for the empty string.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ReadChromOrder reads the chromosome names declared by a -g genome file or a
// -faidx names/.fai file: first tab-separated column, in order of declaration.
func ReadChromOrder(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errf("Cannot open \"%s\"No such file or directory", path)
	}
	defer f.Close()

	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), MaxInputBytes+1)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if line == "" {
			continue
		}
		order = append(order, strings.Split(line, "\t")[0])
	}
	if err := sc.Err(); err != nil {
		return nil, errf("Cannot open \"%s\"%v", path, err)
	}
	return order, nil
}
