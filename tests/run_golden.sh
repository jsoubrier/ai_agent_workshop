#!/usr/bin/env bash
# Golden tests: diff mytools against real bedtools.
#
#   ./tests/run_golden.sh          builds ./mytools and tests it
#   MYTOOLS=/path/to/bin ./tests/run_golden.sh    tests an existing binary
#
# bedtools is the oracle. If mytools and bedtools differ on the same input,
# mytools is wrong. We compare stdout bytes and exit codes, never stderr
# (bedtools' wording is its own), and never sort the output before diffing
# (row order is part of the answer).
set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)
DATA=$root/data

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

if [[ -z "${MYTOOLS:-}" ]]; then
  MYTOOLS=$tmp/mytools
  (cd "$root" && go build -o "$MYTOOLS" .) || { echo "build failed"; exit 1; }
fi

pass=0; fail=0

# check <name> -- <args...>
#   runs "$MYTOOLS <args>" and "bedtools <args>", diffs stdout, compares exit
check() {
  local name=$1; shift; shift        # drop the literal --
  "$MYTOOLS" "$@" > "$tmp/got"  2>"$tmp/got.err"; local got_rc=$?
  bedtools   "$@" > "$tmp/want" 2>"$tmp/want.err"; local want_rc=$?
  report "$name" "$got_rc" "$want_rc"
}

# check_stdin <name> <file> -- <args...>
#   same, but <file> is fed to both processes on stdin
check_stdin() {
  local name=$1 input=$2; shift 2; shift
  "$MYTOOLS" "$@" < "$input" > "$tmp/got"  2>"$tmp/got.err"; local got_rc=$?
  bedtools   "$@" < "$input" > "$tmp/want" 2>"$tmp/want.err"; local want_rc=$?
  report "$name" "$got_rc" "$want_rc"
}

report() {
  local name=$1 got_rc=$2 want_rc=$3
  if [[ $got_rc -ne $want_rc ]]; then
    echo "FAIL $name (exit $got_rc, bedtools gave $want_rc)"
    sed 's/^/      /' "$tmp/got.err" | head -3
    (( fail++ )); return
  fi
  if diff -q "$tmp/want" "$tmp/got" >/dev/null; then
    echo "ok   $name"; (( pass++ ))
  else
    echo "FAIL $name"
    diff -u "$tmp/want" "$tmp/got" | sed 's/^/      /' | head -20
    (( fail++ ))
  fi
}

# ---------------------------------------------------------------- fixtures --
# Built here, never in data/: the committed fixtures are deliberate and are
# not to be edited or added to.
printf 'chr1\t100\n'                                    > "$tmp/few-columns.bed"
printf 'chr1\tfoo\t200\n'                               > "$tmp/non-integer.bed"
printf 'chr1\t-5\t200\n'                                > "$tmp/negative.bed"
printf 'chr1\t300\t200\n'                               > "$tmp/start-after-end.bed"
printf 'chr1\t500\t500\tz1\t0\t+\n'                     > "$tmp/zero-length.bed"
printf 'chr1\t0\t100\tp0\t0\t+\n'                       > "$tmp/position-zero.bed"
: > "$tmp/empty.bed"
printf '# comment\ntrack name=x\nbrowser pos\nchr2\t50\t60\nchr1\t100\t200\n' > "$tmp/header.bed"
printf 'chr1\t1\t2\tn\t0\t+\t100\t200\t0,0,0\t2\t10,10\t0,90\n' > "$tmp/bed12.bed"
printf 'chr1\t1\t2\tn\t0\t+\tx\ty\tz\n'                 > "$tmp/bed6plus.bed"

# -------------------------------------------------------------------- sort --
check "sort a.bed"                 -- sort -i "$DATA/a.bed"
check "sort b.bed"                 -- sort -i "$DATA/b.bed"
check "sort genes.bed"             -- sort -i "$DATA/genes.bed"

# ------------------------------------------------------------ core: BED I/O --
# Every column survives a round trip, including ones mytools never interprets.
check "roundtrip BED12"            -- sort -i "$tmp/bed12.bed"
check "roundtrip BED6+n"           -- sort -i "$tmp/bed6plus.bed"
check "interval at position 0"     -- sort -i "$tmp/position-zero.bed"
check "zero-length interval"       -- sort -i "$tmp/zero-length.bed"

# ------------------------------------------------------------- core: stdin --
# -i - must be byte-identical to the file form.
check_stdin "sort a.bed via stdin" "$DATA/a.bed" -- sort -i -
check_stdin "header via stdin"     "$tmp/header.bed" -- sort -i - -header

# ------------------------------------------------------------ core: header --
# Leading #/track/browser lines are dropped by default and reproduced verbatim,
# in order, ahead of the records under -header. They are not sorted.
check "header dropped by default"  -- sort -i "$tmp/header.bed"
check "header kept with -header"   -- sort -i "$tmp/header.bed" -header

# ------------------------------------------------------------- core: empty --
check "empty file"                 -- sort -i "$tmp/empty.bed"
check "empty file -header"         -- sort -i "$tmp/empty.bed" -header

# ------------------------------------------------------------ core: errors --
# stdout is empty for every one of these, so what each case actually proves is
# that the exit code matches bedtools. stderr is not part of the contract.
check "err: fewer than 3 columns"  -- sort -i "$tmp/few-columns.bed"
check "err: non-integer coord"     -- sort -i "$tmp/non-integer.bed"
check "err: negative coord"        -- sort -i "$tmp/negative.bed"
check "err: start > end"           -- sort -i "$tmp/start-after-end.bed"
check "err: missing file"          -- sort -i "$tmp/no-such-file.bed"
check "err: unrecognized flag"     -- sort --bogus-flag -i "$DATA/a.bed"

# The non-integer and negative cases above deliberately put the bad record on
# the *first* line. bedtools validates the first record with its file-format
# sniffer and every later record with a different code path, and on a later
# record a non-integer coordinate makes bedtools abort on SIGABRT (exit 134,
# core dumped) rather than report anything. mytools reports the error and exits
# 1 there instead; that divergence is covered by a unit test, not here, because
# there is no sane way to match a crash.

echo "---"
echo "$pass passed, $fail failed"
[[ $fail -eq 0 ]]
