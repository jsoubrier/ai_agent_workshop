// Package cli is the mytools command-line skeleton: a bedtools-shaped flag
// parser, the usage/help convention, and the exit-code discipline of
// SPEC.md §5.
//
// Exit codes are bedtools': 0 on success, 1 on every error, data and usage
// alike. 2 is never returned. CLAUDE.md still documents a 0/1/2 scheme and is
// out of date on this point; SPEC.md §9 explains why the oracle wins.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Exit codes. There are only two.
const (
	ExitOK    = 0
	ExitError = 1
)

// ErrHelp is returned by Parse when the user asked for help. The caller prints
// usage to stdout and exits 0.
var ErrHelp = errors.New("help requested")

// FlagSet is a small bedtools-style parser. bedtools writes flags as `-i FILE`
// and `-header`, accepts them in any order including after positionals, and
// tolerates a double dash. Go's flag package stops at the first non-flag
// argument and owns its own error text and exit status, so mytools parses for
// itself.
type FlagSet struct {
	name    string
	usage   string
	strs    map[string]*string
	bools   map[string]*bool
	order   []string
	args    []string
	stdouts io.Writer
	stderrs io.Writer
}

// NewFlagSet returns a parser for subcommand name, printing usage when asked.
func NewFlagSet(name, usage string) *FlagSet {
	return &FlagSet{
		name:    name,
		usage:   usage,
		strs:    map[string]*string{},
		bools:   map[string]*bool{},
		stdouts: os.Stdout,
		stderrs: os.Stderr,
	}
}

// SetOutput redirects usage and error text, for tests.
func (fs *FlagSet) SetOutput(stdout, stderr io.Writer) {
	fs.stdouts, fs.stderrs = stdout, stderr
}

// String declares a flag that takes the following argument as its value.
func (fs *FlagSet) String(name, def string) *string {
	v := def
	fs.strs[name] = &v
	fs.order = append(fs.order, name)
	return &v
}

// Bool declares a flag that takes no value.
func (fs *FlagSet) Bool(name string) *bool {
	v := false
	fs.bools[name] = &v
	fs.order = append(fs.order, name)
	return &v
}

// Args returns the positional arguments left after parsing.
func (fs *FlagSet) Args() []string { return fs.args }

// Parse consumes args. It returns ErrHelp for -h/--help, and otherwise an
// error whose text is ready for stderr.
func (fs *FlagSet) Parse(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			fs.args = append(fs.args, arg)
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
		if name == "h" || name == "help" {
			return ErrHelp
		}
		if p, ok := fs.bools[name]; ok {
			*p = true
			continue
		}
		if p, ok := fs.strs[name]; ok {
			if i+1 >= len(args) {
				return fmt.Errorf("\n*****ERROR: %s requires a value *****\n", arg)
			}
			i++
			*p = args[i]
			continue
		}
		// bedtools' own wording, blank lines and all, followed by usage.
		return fmt.Errorf("\n*****ERROR: Unrecognized parameter: %s *****\n", arg)
	}
	return nil
}

// Help prints usage to stdout and returns ExitOK. Usage asked for is data;
// usage printed because of an error is a diagnostic and goes to stderr.
func (fs *FlagSet) Help() int {
	fmt.Fprint(fs.stdouts, fs.usage)
	return ExitOK
}

// Fail prints err, then usage, to stderr and returns ExitError.
func (fs *FlagSet) Fail(err error) int {
	fmt.Fprintln(fs.stderrs, err)
	fmt.Fprint(fs.stderrs, fs.usage)
	return ExitError
}

// Fatal prints a runtime error to stderr and returns ExitError. Unlike Fail it
// prints no usage: a malformed record is not a usage problem.
func Fatal(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return ExitError
}
