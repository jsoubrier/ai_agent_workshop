// Unit tests for mytools. These run without bedtools installed — they pin the
// behaviour the golden tests discovered, one edge case at a time, so that a
// later refactor cannot quietly undo it (tests/README.md).
//
// Everything asserted here was probed against bedtools v2.31.1. Where the
// oracle is surprising, the test says so and encodes the oracle anyway; it
// never encodes what the behaviour ought to be.
package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsoubrier/mytools/internal/bed"
)

// parse is a helper: BED text in, records out, fatal on error.
func parse(t *testing.T, text string) *bed.File {
	t.Helper()
	f, err := bed.ReadFrom(strings.NewReader(text), "test.bed")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func sorted(t *testing.T, text string, mode bed.Mode) []string {
	t.Helper()
	recs, err := bed.Sort(parse(t, text), bed.Options{Mode: mode})
	if err != nil {
		t.Fatalf("sort: %v", err)
	}
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.Line()
	}
	return out
}

func wantOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d records %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d:\n got  %q\n want %q", i, got[i], want[i])
		}
	}
}

// Chromosomes are compared byte-lexicographically, with no numeric reading of
// the digits: chr1 < chr10 < chr2 < chrX, and chr17 before chr7. Natural
// ordering was considered for mytools and explicitly scrapped (SPEC.md §9).
func TestChromosomeOrderIsLexicographic(t *testing.T) {
	in := "chrX\t1\t2\nchr7\t1\t2\nchr2\t1\t2\nchr17\t1\t2\nchr10\t1\t2\nchr1\t1\t2\n"
	wantOrder(t, sorted(t, in, bed.ChromThenStart),
		"chr1\t1\t2",
		"chr10\t1\t2",
		"chr17\t1\t2",
		"chr2\t1\t2",
		"chr7\t1\t2",
		"chrX\t1\t2",
	)
}

// An interval at position 0 is ordinary input and sorts first, and its
// coordinates survive the round trip.
func TestIntervalAtPositionZero(t *testing.T) {
	in := "chr1\t100\t200\tb\nchr1\t0\t100\ta\nchr2\t0\t0\tz\n"
	wantOrder(t, sorted(t, in, bed.ChromThenStart),
		"chr1\t0\t100\ta",
		"chr1\t100\t200\tb",
		"chr2\t0\t0\tz",
	)
}

// Records that are identical on (chrom, start, end) keep their input order.
// bedtools sorts with std::sort, which is stable only below its 16-element
// insertion-sort threshold — so this holds for short runs, which is what the
// fixtures contain (a09/a10 in data/a.bed are fully identical, a03/a04 differ
// only by strand). Longer runs are the golden tests' business.
func TestTiesKeepInputOrder(t *testing.T) {
	in := "chr1\t150\t250\tfirst\t30\t+\nchr1\t150\t250\tsecond\t30\t-\nchr1\t150\t250\tthird\t30\t+\n"
	wantOrder(t, sorted(t, in, bed.ChromThenStart),
		"chr1\t150\t250\tfirst\t30\t+",
		"chr1\t150\t250\tsecond\t30\t-",
		"chr1\t150\t250\tthird\t30\t+",
	)
}

// The default ordering compares the start and *not* the end: bedtools sorts
// each chromosome with `a.start < b.start` alone, so records sharing a start
// stay in input order however their ends compare. Surprising, and the oracle.
func TestDefaultOrderIgnoresEnd(t *testing.T) {
	in := "chr1\t100\t500\tlong\nchr1\t100\t200\tshort\nchr1\t100\t300\tmid\n"
	wantOrder(t, sorted(t, in, bed.ChromThenStart),
		"chr1\t100\t500\tlong",
		"chr1\t100\t200\tshort",
		"chr1\t100\t300\tmid",
	)
}

// A zero-length feature does not have size 0 for ordering purposes: bedtools
// widens it by one base on each side, so (10,10) counts as 2 bp. It therefore
// sorts *after* a 1 bp feature and ties with a 2 bp one — exactly the
// behaviour data/a.bed's a07, a12 and a16 exercise.
func TestZeroLengthCountsAsTwoBases(t *testing.T) {
	z := bed.Record{Start: 10, End: 10}
	if got := z.Size(); got != 2 {
		t.Errorf("zero-length size = %d, want 2", got)
	}
	if got := z.SizeStart(); got != 9 {
		t.Errorf("zero-length size start = %d, want 9", got)
	}
	if got := z.SizeEnd(); got != 11 {
		t.Errorf("zero-length size end = %d, want 11", got)
	}

	in := "chr1\t10\t10\tzero\nchr1\t20\t21\tone\nchr1\t30\t32\ttwo\nchr1\t40\t43\tthree\n"
	wantOrder(t, sorted(t, in, bed.ChromThenSizeAsc),
		"chr1\t20\t21\tone",   // 1 bp
		"chr1\t10\t10\tzero",  // counts as 2 bp, and starts earliest of the two
		"chr1\t30\t32\ttwo",   // 2 bp
		"chr1\t40\t43\tthree", // 3 bp
	)
	wantOrder(t, sorted(t, in, bed.ChromThenSizeDesc),
		"chr1\t40\t43\tthree",
		"chr1\t10\t10\tzero",
		"chr1\t30\t32\ttwo",
		"chr1\t20\t21\tone",
	)
}

// When two features tie on size, the widened coordinates decide: (100,100)
// sorts ahead of (100,102) because its widened start is 99. The descending
// orderings have no such tie-break at all — they compare the size and nothing
// else. The asymmetry is bedtools'.
func TestSizeTieBreakUsesWidenedStart(t *testing.T) {
	in := "chr1\t100\t102\twide\nchr1\t100\t100\tzero\n"
	wantOrder(t, sorted(t, in, bed.ChromThenSizeAsc),
		"chr1\t100\t100\tzero",
		"chr1\t100\t102\twide",
	)
	// Same two records, default ordering: the widening does not apply there,
	// so they tie on start 100 and keep input order.
	wantOrder(t, sorted(t, in, bed.ChromThenStart),
		"chr1\t100\t102\twide",
		"chr1\t100\t100\tzero",
	)
}

// -sizeA sorts globally and does not group by chromosome; -sizeD likewise.
func TestSizeSortIsGlobal(t *testing.T) {
	in := "chr1\t0\t100\tbig1\nchr2\t0\t10\tsmall2\nchr1\t0\t10\tsmall1\n"
	wantOrder(t, sorted(t, in, bed.SizeAsc),
		"chr1\t0\t10\tsmall1",
		"chr2\t0\t10\tsmall2",
		"chr1\t0\t100\tbig1",
	)
}

// Scores are compared as strings, not numbers: "-5" < "100" < "20" < "3".
// That is bedtools comparing BED column 5 without converting it.
func TestScoreComparedAsString(t *testing.T) {
	in := "chr1\t10\t20\ta\t100\t+\nchr1\t30\t40\tb\t20\t+\nchr1\t50\t60\tc\t3\t+\nchr1\t70\t80\td\t-5\t+\n"
	wantOrder(t, sorted(t, in, bed.ChromThenScoreAsc),
		"chr1\t70\t80\td\t-5\t+",
		"chr1\t10\t20\ta\t100\t+",
		"chr1\t30\t40\tb\t20\t+",
		"chr1\t50\t60\tc\t3\t+",
	)
}

func TestScoreSortNeedsFiveColumns(t *testing.T) {
	_, err := bed.Sort(parse(t, "chr1\t10\t20\n"), bed.Options{Mode: bed.ChromThenScoreAsc})
	if err == nil || !strings.Contains(err.Error(), "BED 5 format or greater") {
		t.Fatalf("BED3 score sort error = %v, want the BED-5 complaint", err)
	}
	// bedtools makes the same complaint about a file with no records at all.
	if _, err := bed.Sort(parse(t, ""), bed.Options{Mode: bed.ChromThenScoreDesc}); err == nil {
		t.Fatal("empty file score sort: want an error, got nil")
	}
}

// Leading #/track/browser lines are header. They are dropped by default and
// reprinted verbatim ahead of the records under -header. Only the leading
// block counts: the same line after a record is dropped and never comes back.
func TestHeaderIsLeadingBlockOnly(t *testing.T) {
	f := parse(t, "# one\ntrack name=x\nbrowser pos\nchr1\t10\t20\n# not a header any more\nchr1\t5\t6\n")
	want := []string{"# one", "track name=x", "browser pos"}
	if strings.Join(f.Header, "|") != strings.Join(want, "|") {
		t.Errorf("header = %v, want %v", f.Header, want)
	}
	if len(f.Records) != 2 {
		t.Errorf("records = %d, want 2", len(f.Records))
	}
}

// "chrtrack" and "ch#r1" are chromosome names, not headers: the test is a
// prefix match on the line, not a substring one.
func TestHeaderPrefixNotSubstring(t *testing.T) {
	f := parse(t, "chrtrack\t10\t20\nch#r1\t10\t20\n")
	if len(f.Header) != 0 || len(f.Records) != 2 {
		t.Fatalf("header = %v, records = %d; want 0 header lines and 2 records", f.Header, len(f.Records))
	}
}

func TestMalformedRecords(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"fewer than 3 columns", "chr1\t100\t200\nchr1\t300\n", "less than 3 columns at line 2"},
		{"start greater than end", "chr1\t100\t200\nchr1\t500\t400\n", "Start was greater than end"},
		{"negative start", "chr1\t100\t200\nchr1\t-5\t400\n", "Start Coordinate detected that is < 0"},
		{"differing field counts", "chr1\t10\t20\nchr1\t30\t40\t50\t60\t70\t80\n", "Differing number of BED fields"},
		// A negative or non-numeric coordinate on the *first* record is not
		// even recognised as BED: bedtools decides the file type from that
		// line and complains about the format instead.
		{"negative end on first record", "chr1\t100\t-200\n", "non-integer starts or ends at line 1"},
		{"leading tab on first record", "\tchr1\t100\t200\n", "non-integer starts or ends at line 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := bed.ReadFrom(strings.NewReader(c.in), "test.bed")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want one containing %q", err, c.want)
			}
		})
	}
}

// Line numbers in those messages count every physical line, headers and blank
// lines included.
func TestErrorLineNumbersCountEveryLine(t *testing.T) {
	_, err := bed.ReadFrom(strings.NewReader("# h\n\nchr1\t10\t20\nchr1\t30\n"), "test.bed")
	if err == nil || !strings.Contains(err.Error(), "at line 4") {
		t.Fatalf("error = %v, want it to name line 4", err)
	}
}

// A non-integer coordinate *after* the first record is the one place mytools
// does not match the oracle: bedtools v2.31.1 lets std::stoll throw and dies
// on SIGABRT (exit 134). mytools exits 1 with the SPEC.md §7 message, so
// tests/run_golden.sh deliberately carries no golden case for it.
func TestNonIntegerCoordinate(t *testing.T) {
	_, err := bed.ReadFrom(strings.NewReader("chr1\t10\t20\nchr1\tfoo\t400\n"), "test.bed")
	if err == nil || !strings.Contains(err.Error(), "non-integer starts or ends at line 2") {
		t.Fatalf("error = %v, want the non-integer complaint", err)
	}
}

func TestZeroLengthAndBlankLinesAreLegal(t *testing.T) {
	f := parse(t, "chr1\t500\t500\tz\n\nchr1\t0\t0\tz0\n")
	if len(f.Records) != 2 {
		t.Fatalf("records = %d, want 2 (start == end is valid input)", len(f.Records))
	}
}

// Echoed records keep every original column, but the coordinates are the
// reparsed ones: bedtools prints "100", not "0100". One trailing tab is
// stripped before the line is split, a second one leaves an empty name field,
// and a trailing CR goes too.
func TestRecordRoundTrip(t *testing.T) {
	cases := []struct{ in, want string }{
		{"chr1\t0100\t0200\tx\t5\t+\textra\n", "chr1\t100\t200\tx\t5\t+\textra"},
		{"chr1\t100\t200\t\n", "chr1\t100\t200"},
		{"chr1\t100\t200\t\t\n", "chr1\t100\t200\t"},
		{"chr1\t100\t200\tx\r\n", "chr1\t100\t200\tx"},
		{"chr1\t100\t200", "chr1\t100\t200"}, // no trailing newline
	}
	for _, c := range cases {
		f := parse(t, c.in)
		if len(f.Records) != 1 {
			t.Fatalf("%q: records = %d, want 1", c.in, len(f.Records))
		}
		if got := f.Records[0].Line(); got != c.want {
			t.Errorf("%q:\n got  %q\n want %q", c.in, got, c.want)
		}
	}
}

func TestChromOrderFromGenomeFile(t *testing.T) {
	dir := t.TempDir()
	genome := filepath.Join(dir, "genome.txt")
	if err := os.WriteFile(genome, []byte("chrX\t100\nchr2\t200\nchr1\t300\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	order, err := bed.ReadChromOrder(genome)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "chrX,chr2,chr1" {
		t.Fatalf("order = %v, want chrX,chr2,chr1", order)
	}

	in := "chr1\t10\t20\ta\nchr2\t10\t20\tb\nchrX\t10\t20\tc\n"
	recs, err := bed.Sort(parse(t, in), bed.Options{ChromOrder: order, ChromOrderFile: genome})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range recs {
		got = append(got, r.Chrom)
	}
	if strings.Join(got, ",") != "chrX,chr2,chr1" {
		t.Fatalf("sorted chroms = %v, want chrX,chr2,chr1", got)
	}

	// A chromosome missing from the genome file is an error, and nothing is
	// printed before it: the whole file is checked up front.
	_, err = bed.Sort(parse(t, in), bed.Options{ChromOrder: []string{"chr1"}, ChromOrderFile: genome})
	if err == nil || !strings.Contains(err.Error(), `Chromosome "chr2" undefined`) {
		t.Fatalf("error = %v, want the undefined-chromosome complaint", err)
	}
}

// SPEC.md §6: no single input file may exceed 2 MiB. A regular file is
// measured before it is read; stdin is counted as it streams and aborts the
// moment the count passes the cap.
func TestInputSizeCap(t *testing.T) {
	record := "chr1\t100\t200\tpadding-to-make-this-line-longer\n"
	big := strings.Repeat(record, bed.MaxInputBytes/len(record)+100)
	if len(big) <= bed.MaxInputBytes {
		t.Fatalf("test fixture is only %d bytes, cap is %d", len(big), bed.MaxInputBytes)
	}

	path := filepath.Join(t.TempDir(), "big.bed")
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := bed.Read(path); err == nil || !strings.Contains(err.Error(), fmt.Sprint(bed.MaxInputBytes)) {
		t.Fatalf("file cap: error = %v, want it to name the %d byte limit", err, bed.MaxInputBytes)
	}

	if _, err := bed.ReadFrom(strings.NewReader(big), "stdin"); err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("stdin cap: error = %v, want it to name stdin", err)
	}
	// A file just under the cap is fine.
	small := strings.Repeat(record, 10)
	if _, err := bed.ReadFrom(strings.NewReader(small), "stdin"); err != nil {
		t.Fatalf("small input: %v", err)
	}
}

// StdSort reproduces libstdc++'s std::sort, unstable tail and all. Twenty
// records that all compare equal come out in this order under bedtools; a
// stable sort would leave them in input order, and mytools would then disagree
// with the oracle on any file with more than 16 records in a chromosome.
func TestStdSortReproducesIntrosortPermutation(t *testing.T) {
	var recs []bed.Record
	for i := 1; i <= 20; i++ {
		recs = append(recs, bed.Record{Chrom: "chr1", Start: 100, End: 200,
			Fields: []string{"chr1", "100", "200", fmt.Sprintf("E%02d", i)}})
	}
	bed.StdSort(recs, func(a, b bed.Record) bool { return a.Start < b.Start })
	var got []string
	for _, r := range recs {
		got = append(got, r.Fields[3])
	}
	want := "E11 E20 E19 E18 E17 E16 E15 E14 E13 E12 E01 E10 E09 E08 E07 E06 E05 E04 E03 E02"
	if strings.Join(got, " ") != want {
		t.Errorf("permutation:\n got  %s\n want %s", strings.Join(got, " "), want)
	}
}
