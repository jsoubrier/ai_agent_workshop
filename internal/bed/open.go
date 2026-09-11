package bed

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

// MaxInputBytes is the per-file input cap (SPEC.md §6). It is applied
// independently to every input — sort's -i, intersect's -a, and each -b —
// with no combined total. It is what guarantees the process fits in memory,
// and it is why sort may hold a whole input and intersect a whole -b index.
const MaxInputBytes = 2 * 1024 * 1024 // 2 MiB

// StdinName is the name the cap error uses for standard input. Parse errors
// still name stdin "-", because that is what bedtools prints.
const StdinName = "stdin"

// Open opens a BED input by name. "-" means stdin. The returned Reader must be
// closed. Errors carry bedtools' wording and mean exit 1.
func Open(name string) (*Reader, error) {
	if name == "-" {
		r := NewReader(newCapReader(os.Stdin, StdinName), "-")
		return r, nil
	}

	fi, err := os.Stat(name)
	if err != nil {
		return nil, openError(name, err)
	}
	if fi.IsDir() {
		// os.Open succeeds on a directory in Go and only fails on read, so
		// check here to produce bedtools' "(Is a directory)" message.
		return nil, openError(name, syscall.EISDIR)
	}
	if fi.Mode().IsRegular() && fi.Size() > MaxInputBytes {
		// A regular file's size is known up front, so check it before reading
		// a single byte.
		return nil, ErrTooLarge(fmt.Sprintf("file (%s)", name))
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, openError(name, err)
	}

	var src io.Reader = f
	if !fi.Mode().IsRegular() {
		// FIFOs and character devices report no meaningful size, so cap them
		// by counting as they stream, the same way stdin is capped.
		src = newCapReader(f, fmt.Sprintf("file (%s)", name))
	}
	r := NewReader(src, name)
	r.closer = f
	return r, nil
}

// CheckSingleStdin enforces the one-stdin-per-invocation rule (SPEC.md §5):
// "-" may appear at most once across all of an invocation's inputs, because
// two readers cannot share one stream.
func CheckSingleStdin(names ...string) error {
	n := 0
	for _, name := range names {
		if name == "-" {
			n++
		}
	}
	if n > 1 {
		return errors.New("Error: only one input may be stdin (-) per invocation. Exiting!")
	}
	return nil
}

// ErrTooLarge builds the input-cap error. what is already phrased for the
// message: either `file (a.bed)` or `stdin`.
func ErrTooLarge(what string) error {
	return fmt.Errorf("Error: %s exceeds the maximum input size of %d bytes (2 MiB). Exiting!", what, MaxInputBytes)
}

// openError reproduces bedtools' unopenable-file message. The parenthesised
// text is the errno string with its first letter capitalised, which is how
// "(No such file or directory)", "(Permission denied)" and "(Is a directory)"
// all come out of the oracle.
func openError(name string, err error) error {
	return fmt.Errorf("Error: The requested file (%s) could not be opened. Error message: (%s). Exiting!", name, errnoText(err))
}

func errnoText(err error) string {
	var errno syscall.Errno
	msg := ""
	if errors.As(err, &errno) {
		msg = errno.Error()
	} else {
		var pe *os.PathError
		if errors.As(err, &pe) {
			msg = pe.Err.Error()
		} else {
			msg = err.Error()
		}
	}
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:]
}

// NewCappedReader wraps a stream whose size cannot be stat'ed so that it
// aborts once MaxInputBytes is exceeded. what is already phrased for the error
// message: either `stdin` or `file (a.bed)`.
func NewCappedReader(r io.Reader, what string) io.Reader { return newCapReader(r, what) }

// capReader enforces MaxInputBytes on a stream whose size cannot be stat'ed.
type capReader struct {
	r    io.Reader
	what string
	n    int64
	err  error
}

func newCapReader(r io.Reader, what string) *capReader {
	return &capReader{r: r, what: what}
}

// Read aborts the moment the running count passes the cap. It deliberately
// does not buffer the stream to measure it (SPEC.md §6): the bytes from the
// offending read are discarded because the run is over, and the caller's
// non-zero exit — not a truncated but plausible-looking result — is the signal.
func (c *capReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > MaxInputBytes {
		c.err = ErrTooLarge(c.what)
		return 0, c.err
	}
	return n, err
}

// streamErr lets Reader surface the cap ahead of the truncated final line that
// bufio.Scanner would otherwise hand it. Without this the user would see a
// spurious "less than 3 columns" complaint about a line we cut in half.
func (c *capReader) streamErr() error { return c.err }
