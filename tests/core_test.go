// Package tests holds mytools' unit tests. Everything here runs with bedtools
// absent from PATH: these pin the behaviours we had to stop and reason about,
// so that a later refactor cannot quietly undo them. The golden suite
// (run_golden.sh) is what proves agreement with the oracle.
package tests

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsoubrier/mytools/internal/bed"
)

// ------------------------------------------------------------ record parsing

// Every column survives a round trip byte for byte, including the ones mytools
// never interprets (SPEC.md §4.1).
func TestRecordRoundTripsVerbatim(t *testing.T) {
	cases := []struct{ name, line string }{
		{"BED3", "chr1\t100\t200"},
		{"BED4", "chr1\t100\t200\tname"},
		{"BED6", "chr1\t100\t200\tname\t60\t+"},
		{"BED12", "chr1\t100\t200\tn\t0\t+\t100\t200\t0,0,0\t2\t10,10\t0,90"},
		{"BED6+n", "chr1\t100\t200\tn\t0\t+\tkept\tverbatim\t{\"json\":1}"},
		{"trailing empty column", "chr1\t100\t200\t\tx"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, err := bed.ParseRecord(c.line, 1, "t.bed", true)
			if err != nil {
				t.Fatalf("ParseRecord: %v", err)
			}
			if got := rec.String(); got != c.line {
				t.Errorf("round trip\n got %q\nwant %q", got, c.line)
			}
		})
	}
}

// A single trailing tab is absorbed rather than producing an empty column,
// matching bedtools' getline tokeniser.
func TestTrailingTabAbsorbedOnce(t *testing.T) {
	for line, want := range map[string]string{
		"chr1\t100\t200\t":     "chr1\t100\t200",
		"chr1\t100\t200\t\t":   "chr1\t100\t200\t",
		"chr1\t100\t200\t\t\t": "chr1\t100\t200\t\t",
	} {
		rec, err := bed.ParseRecord(line, 1, "t.bed", true)
		if err != nil {
			t.Fatalf("ParseRecord(%q): %v", line, err)
		}
		if got := rec.String(); got != want {
			t.Errorf("ParseRecord(%q)\n got %q\nwant %q", line, got, want)
		}
	}
}

// Position 0 is a real coordinate, not a missing value. data/a.bed's a01 and
// data/b.bed's b01/b16 are there to make sure nobody treats it as falsy.
func TestIntervalAtPositionZero(t *testing.T) {
	rec, err := bed.ParseRecord("chr1\t0\t100\ta01\t10\t+", 1, "a.bed", true)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec.Start != 0 || rec.End != 100 {
		t.Errorf("got %d-%d, want 0-100", rec.Start, rec.End)
	}
}

// start == end is legal input, not an error: data/a.bed carries a07, a12 and
// a16, and bedtools accepts all three (SPEC.md §7).
func TestZeroLengthIntervalIsLegal(t *testing.T) {
	for _, line := range []string{"chr1\t500\t500\ta07\t0\t+", "chr2\t0\t0\ta12\t0\t+"} {
		if _, err := bed.ParseRecord(line, 1, "a.bed", true); err != nil {
			t.Errorf("ParseRecord(%q): %v", line, err)
		}
	}
}

// bedtools re-renders the coordinate columns, so a zero-padded start does not
// survive as typed. Encoded because it is the one exception to "verbatim".
func TestCoordinatesAreNormalised(t *testing.T) {
	rec, err := bed.ParseRecord("chr1\t007\t20", 1, "t.bed", true)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if got, want := rec.String(), "chr1\t7\t20"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Chromosome order is byte-lexicographic with no natural ordering anywhere:
// chr10 before chr2, and chr17 before chr7 (SPEC.md §2.1).
func TestChromComparatorIsLexicographic(t *testing.T) {
	for _, c := range []struct{ a, b string }{
		{"chr1", "chr10"}, {"chr10", "chr2"}, {"chr17", "chr7"}, {"chr2", "chrX"},
	} {
		if bed.CompareChrom(c.a, c.b) >= 0 {
			t.Errorf("CompareChrom(%q, %q) should be < 0", c.a, c.b)
		}
	}
}

// ------------------------------------------------------------ error messages

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		lineno int
		first  bool
		want   string
	}{
		{
			name: "fewer than 3 columns", line: "chr1\t100", lineno: 1, first: true,
			want: "It looks as though you have less than 3 columns at line 1 in file t.bed.  Are you sure your files are tab-delimited?",
		},
		{
			// A line of spaces is not an empty line: it reaches the parser and
			// fails the column check, as it does in bedtools.
			name: "line of spaces", line: "   ", lineno: 2, first: true,
			want: "It looks as though you have less than 3 columns at line 2 in file t.bed.  Are you sure your files are tab-delimited?",
		},
		{
			name: "non-integer on first record", line: "chr1\tfoo\t200", lineno: 1, first: true,
			want: "Unexpected file format.  Please use tab-delimited BED, GFF, or VCF. Perhaps you have non-integer starts or ends at line 1?",
		},
		{
			// Surprising, and bedtools': the first record goes through the
			// file-format sniffer, which only accepts unsigned decimal, so a
			// negative coordinate is reported as "non-integer" here...
			name: "negative on first record", line: "chr1\t-5\t200", lineno: 1, first: true,
			want: "Unexpected file format.  Please use tab-delimited BED, GFF, or VCF. Perhaps you have non-integer starts or ends at line 1?",
		},
		{
			// ...and as an out-of-range coordinate on every later record.
			name: "negative start on later record", line: "chr1\t-5\t200", lineno: 2, first: false,
			want: "Error: malformed BED entry at line 2. Start Coordinate detected that is < 0. Exiting.",
		},
		{
			// Start is checked before end, even when both are negative.
			name: "negative end on later record", line: "chr1\t5\t-200", lineno: 2, first: false,
			want: "Error: malformed BED entry at line 2. End Coordinate detected that is < 0. Exiting.",
		},
		{
			name: "start after end", line: "chr1\t300\t200", lineno: 1, first: true,
			want: "Error: malformed BED entry at line 1. Start was greater than end. Exiting.",
		},
		{
			// Deliberate divergence: real bedtools calls stoll() unguarded on
			// every record after the first, so this input kills it with
			// SIGABRT (exit 134, core dumped) and no message at all. We report
			// the line-1 wording and exit 1 rather than reproduce a crash.
			// The golden suite keeps its non-integer case on line 1 so that it
			// still matches the oracle exactly.
			name: "non-integer on later record", line: "chr1\tfoo\t200", lineno: 4, first: false,
			want: "Unexpected file format.  Please use tab-delimited BED, GFF, or VCF. Perhaps you have non-integer starts or ends at line 4?",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := bed.ParseRecord(c.line, c.lineno, "t.bed", c.first)
			if err == nil {
				t.Fatal("expected an error")
			}
			if err.Error() != c.want {
				t.Errorf("\n got %q\nwant %q", err.Error(), c.want)
			}
		})
	}
}

func TestOpenErrorNamesTheFileAndTheReason(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-file.bed")
	_, err := bed.Open(missing)
	if err == nil {
		t.Fatal("expected an error")
	}
	want := "Error: The requested file (" + missing + ") could not be opened. Error message: (No such file or directory). Exiting!"
	if err.Error() != want {
		t.Errorf("\n got %q\nwant %q", err.Error(), want)
	}
}

// ------------------------------------------------------------ header handling

func TestHeaderBlock(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantHeader []string
		wantRecs   int
	}{
		{
			name:       "leading block captured in order",
			in:         "# comment\ntrack name=x\nbrowser pos\nchr1\t100\t200\n",
			wantHeader: []string{"# comment", "track name=x", "browser pos"},
			wantRecs:   1,
		},
		{
			// A blank line closes the header block, so "# b" is dropped and
			// never reappears under -header (probed against bedtools).
			name:       "blank line closes the block",
			in:         "# a\n\n# b\nchr1\t1\t2\n",
			wantHeader: []string{"# a"},
			wantRecs:   1,
		},
		{
			// #/track/browser lines after the records are dropped entirely.
			name:       "mid-file header lines are not header",
			in:         "chr1\t1\t2\n# late\ntrack late\nchr1\t5\t6\n",
			wantHeader: nil,
			wantRecs:   2,
		},
		{
			// bedtools matches "track" as a bare prefix, so a feature whose
			// chromosome starts with "track" is swallowed as a header line.
			// Surprising, and deliberately preserved.
			name:       "trackx is a header line, not a feature",
			in:         "trackx\t1\t2\n",
			wantHeader: []string{"trackx\t1\t2"},
			wantRecs:   0,
		},
		{
			name:       "header-only input is not an error",
			in:         "# only\ntrack t\n",
			wantHeader: []string{"# only", "track t"},
			wantRecs:   0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := bed.NewReader(strings.NewReader(c.in), "-")
			recs, err := bed.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if len(recs) != c.wantRecs {
				t.Errorf("got %d records, want %d", len(recs), c.wantRecs)
			}
			if got := r.Header(); !equal(got, c.wantHeader) {
				t.Errorf("header\n got %q\nwant %q", got, c.wantHeader)
			}
		})
	}
}

// Line numbers in error messages are physical: header and blank lines count.
func TestErrorLineNumbersArePhysical(t *testing.T) {
	in := "# c\n\nchr1\t1\t2\nchr1\t9\n"
	r := bed.NewReader(strings.NewReader(in), "t.bed")
	_, err := bed.ReadAll(r)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "at line 4 ") {
		t.Errorf("want the error to name physical line 4, got %q", err.Error())
	}
}

// A CRLF file parses, and a lone CR inside a line stays as data.
func TestLineEndings(t *testing.T) {
	r := bed.NewReader(strings.NewReader("chr1\t100\t200\tn\r\nchr1\t1\t2\tx\ry\n"), "-")
	recs, err := bed.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got, want := recs[0].String(), "chr1\t100\t200\tn"; got != want {
		t.Errorf("CRLF: got %q, want %q", got, want)
	}
	if got, want := recs[1].String(), "chr1\t1\t2\tx\ry"; got != want {
		t.Errorf("lone CR: got %q, want %q", got, want)
	}
}

// ------------------------------------------------------------ the 2 MiB cap

// A regular file's size is known before a byte is read, so the cap trips on
// Open (SPEC.md §6).
func TestSizeCapOnRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.bed")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), bed.MaxInputBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := bed.Open(path)
	if err == nil {
		t.Fatal("expected the cap to trip")
	}
	want := "Error: file (" + path + ") exceeds the maximum input size of 2097152 bytes (2 MiB). Exiting!"
	if err.Error() != want {
		t.Errorf("\n got %q\nwant %q", err.Error(), want)
	}

	// One byte under the cap is fine.
	ok := filepath.Join(t.TempDir(), "ok.bed")
	if err := os.WriteFile(ok, bytes.Repeat([]byte("chr1\t1\t2\n"), 10), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := bed.Open(ok)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	r.Close()
}

// countingReader hands out an unbounded stream of valid BED and records how
// much of it was actually consumed.
type countingReader struct {
	chunk []byte
	n     int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n := copy(p, c.chunk[:min(len(p), len(c.chunk))])
	c.n += int64(n)
	return n, nil
}

// A stream has no size to stat, so the cap is counted as it goes — and it must
// abort the moment the count passes, not buffer the stream to measure it.
func TestSizeCapOnStreamAbortsWithoutBuffering(t *testing.T) {
	src := &countingReader{chunk: bytes.Repeat([]byte("chr1\t100\t200\n"), 4096)}
	r := bed.NewReader(bed.NewCappedReader(src, bed.StdinName), "-")
	_, err := bed.ReadAll(r)
	if err == nil {
		t.Fatal("expected the cap to trip")
	}
	want := "Error: stdin exceeds the maximum input size of 2097152 bytes (2 MiB). Exiting!"
	if err.Error() != want {
		t.Errorf("\n got %q\nwant %q", err.Error(), want)
	}
	// The reader is infinite, so finishing at all proves we stopped early; the
	// bound proves we stopped promptly rather than after some larger buffer.
	if limit := int64(bed.MaxInputBytes) + int64(len(src.chunk)); src.n > limit {
		t.Errorf("consumed %d bytes, want no more than %d", src.n, limit)
	}
}

// ------------------------------------------------------------ stdin, for real

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mytools-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	binary = filepath.Join(dir, "mytools")
	build := exec.Command("go", "build", "-o", binary, "github.com/jsoubrier/mytools")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// The end-to-end version of the stream cap: feed the process far more than
// 2 MiB on stdin and it must exit 1 having read only a little past the cap,
// leaving our writes to fail on a closed pipe.
func TestStdinCapAbortsMidStream(t *testing.T) {
	const feed = 64 << 20 // 32x the cap: finishing the write means we failed

	cmd := exec.Command(binary, "sort", "-i", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	chunk := bytes.Repeat([]byte("chr1\t100\t200\n"), 8192) // ~104 KiB
	written := 0
	var writeErr error
	for written < feed {
		n, err := stdin.Write(chunk)
		written += n
		if err != nil {
			writeErr = err
			break
		}
	}
	stdin.Close()

	if got := exitCode(cmd.Wait()); got != 1 {
		t.Errorf("exit %d, want 1 (stderr: %s)", got, stderr.String())
	}
	if writeErr == nil {
		t.Errorf("wrote all %d bytes without the process hanging up; it did not abort mid-stream", written)
	}
	// Allow a few chunks of pipe buffer and in-flight reads over the cap.
	if limit := bed.MaxInputBytes + 4*len(chunk); written > limit {
		t.Errorf("process consumed %d bytes before aborting, want no more than %d", written, limit)
	}
	if !strings.Contains(stderr.String(), "exceeds the maximum input size") {
		t.Errorf("stderr did not name the cap: %s", stderr.String())
	}
}

// ------------------------------------------------------------ CLI conventions

func TestCLIExitCodes(t *testing.T) {
	good := filepath.Join(t.TempDir(), "g.bed")
	if err := os.WriteFile(good, []byte("chr1\t1\t2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		args   []string
		want   int
		stdout bool // usage is data when asked for, a diagnostic when forced
	}{
		{name: "no arguments prints usage", args: nil, want: 0, stdout: true},
		{name: "-h prints usage", args: []string{"-h"}, want: 0, stdout: true},
		{name: "sort -h prints usage", args: []string{"sort", "-h"}, want: 0, stdout: true},
		{name: "--version", args: []string{"--version"}, want: 0, stdout: true},
		{name: "unknown subcommand", args: []string{"bogus"}, want: 1},
		{name: "unknown flag", args: []string{"sort", "--bogus", "-i", good}, want: 1},
		{name: "missing file", args: []string{"sort", "-i", "no-such-file.bed"}, want: 1},
		{name: "bare positional stands in for -i", args: []string{"sort", good}, want: 0, stdout: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(binary, c.args...)
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			cmd.Stdin = strings.NewReader("")
			got := exitCode(cmd.Run())
			if got != c.want {
				t.Errorf("exit %d, want %d (stderr: %s)", got, c.want, errb.String())
			}
			if c.stdout && out.Len() == 0 {
				t.Error("expected output on stdout")
			}
			// Nothing that is not data ever reaches stdout.
			if !c.stdout && out.Len() != 0 {
				t.Errorf("error path wrote to stdout: %q", out.String())
			}
		})
	}
}

// Two stdin inputs cannot share one stream (SPEC.md §5).
func TestCheckSingleStdin(t *testing.T) {
	if err := bed.CheckSingleStdin("-", "a.bed"); err != nil {
		t.Errorf("one stdin should be fine: %v", err)
	}
	if err := bed.CheckSingleStdin("-", "a.bed", "-"); err == nil {
		t.Error("two stdin inputs should be an error")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
