package bed

import (
	"bufio"
	"io"
)

// Reader streams BED records from an input, tracking physical line numbers and
// capturing the leading header block.
type Reader struct {
	name       string // the input's name as the user spelled it; "-" for stdin
	sc         *bufio.Scanner
	closer     io.Closer
	lineno     int
	header     []string
	headerOver bool
	seenRecord bool
	stream     streamErrer
}

// streamErrer is implemented by inputs that can fail mid-stream — today, the
// input-size cap.
type streamErrer interface{ streamErr() error }

// NewReader reads BED records from r. name is used verbatim in error messages,
// exactly as bedtools does — a parse error on stdin names the file as "-".
func NewReader(r io.Reader, name string) *Reader {
	sc := bufio.NewScanner(r)
	// One record must fit in one token. The input cap bounds how large that
	// can get, so size the buffer to match rather than to bufio's 64 KiB.
	sc.Buffer(make([]byte, 0, 64*1024), MaxInputBytes+1)
	rd := &Reader{name: name, sc: sc}
	if se, ok := r.(streamErrer); ok {
		rd.stream = se
	}
	return rd
}

// Name returns the input's name as it will appear in error messages.
func (r *Reader) Name() string { return r.name }

// Next returns the next record, or io.EOF when the input is exhausted. Any
// other error is fatal and already carries the oracle's wording: print it to
// stderr and exit 1.
func (r *Reader) Next() (*Record, error) {
	for r.sc.Scan() {
		if r.stream != nil {
			if err := r.stream.streamErr(); err != nil {
				// The cap tripped part-way through this line. Abort on it
				// rather than parsing the fragment the scanner just handed us.
				return nil, err
			}
		}
		r.lineno++
		// bufio.ScanLines has already stripped the LF and a trailing CR, which
		// is what bedtools does too: a CRLF file parses, and a lone CR in the
		// middle of a line is kept as data (probed).
		line := r.sc.Text()

		if line == "" {
			// Empty lines are skipped silently — but they close the header
			// block. `# a`, blank, `# b` reproduces only `# a` under -header
			// (probed). A line of spaces is *not* empty and falls through to
			// the parser, where it fails the 3-column check, as in bedtools.
			r.headerOver = true
			continue
		}

		if IsHeaderLine(line) {
			if !r.headerOver {
				r.header = append(r.header, line)
			}
			// #, track and browser lines are dropped wherever they appear;
			// only the leading run is ever re-emitted, and only under -header.
			continue
		}

		r.headerOver = true
		rec, err := ParseRecord(line, r.lineno, r.name, !r.seenRecord)
		if err != nil {
			return nil, err
		}
		r.seenRecord = true
		return rec, nil
	}
	if err := r.sc.Err(); err != nil {
		// Includes the input-size cap, which surfaces here mid-stream.
		return nil, err
	}
	return nil, io.EOF
}

// Header returns the leading block of #/track/browser lines, in input order,
// unsorted and undeduplicated (SPEC.md §4.3).
//
// The block is only known to be complete once the first record has been read
// or Next has returned io.EOF, so call this after reading, not before.
func (r *Reader) Header() []string { return r.header }

// Lines returns the number of physical lines consumed so far.
func (r *Reader) Lines() int { return r.lineno }

// Close releases the underlying file, if any. Closing stdin is a no-op.
func (r *Reader) Close() error {
	if r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

// ReadAll drains the reader into a slice. Callers that must hold a whole input
// in memory rely on the input cap to bound it (SPEC.md §6).
func ReadAll(r *Reader) ([]*Record, error) {
	var out []*Record
	for {
		rec, err := r.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
}
