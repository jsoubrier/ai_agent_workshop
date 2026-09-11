#!/usr/bin/env bash
# Golden tests: diff mytools against real bedtools.
# Usage: ./tests/run_golden.sh
#
# Every case runs mytools and bedtools on identical arguments and diffs stdout
# and the exit code. stderr is not part of the contract (tests/README.md):
# bedtools' wording is its own.
set -uo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
DATA=$root/data

if [[ -z ${MYTOOLS:-} ]]; then
  go build -o "$root/mytools" "$root" || { echo "build failed"; exit 1; }
  MYTOOLS=$root/mytools
fi
command -v bedtools >/dev/null || { echo "bedtools not on PATH; golden tests need the oracle"; exit 1; }

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
pass=0; fail=0

# check <name> -- <args...>
#   runs "$MYTOOLS <args>" and "bedtools <args>", diffs stdout and exit codes.
check() {
  local name=$1; shift; shift        # drop the literal --
  "$MYTOOLS" "$@" > "$tmp/got"  2>"$tmp/got.err"
  local got_rc=$?
  bedtools   "$@" > "$tmp/want" 2>/dev/null
  local want_rc=$?

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

# check_stdin <name> <file> -- <args...>
#   same, with <file> piped in rather than named.
check_stdin() {
  local name=$1 file=$2; shift 3
  "$MYTOOLS" "$@" < "$file" > "$tmp/got"  2>/dev/null; local got_rc=$?
  bedtools   "$@" < "$file" > "$tmp/want" 2>/dev/null; local want_rc=$?
  if [[ $got_rc -ne $want_rc ]] || ! diff -q "$tmp/want" "$tmp/got" >/dev/null; then
    echo "FAIL $name (exit $got_rc vs $want_rc)"
    diff -u "$tmp/want" "$tmp/got" | sed 's/^/      /' | head -20
    (( fail++ )); return
  fi
  echo "ok   $name"; (( pass++ ))
}

# check_args <name> -- <mytools args...> -- <bedtools args...>
#   for the one place the two command lines legitimately differ: mytools takes
#   a bare positional argument in place of -i as a convenience over bedtools
#   (SPEC.md §5), which bedtools itself rejects. The output must still be
#   byte-identical to the -i form.
check_args() {
  local name=$1; shift 2
  local mine=() theirs=() seen=0
  for a in "$@"; do
    if [[ $a == "--" ]]; then seen=1; continue; fi
    if [[ $seen -eq 0 ]]; then mine+=("$a"); else theirs+=("$a"); fi
  done
  "$MYTOOLS" "${mine[@]}"   > "$tmp/got"  2>/dev/null; local got_rc=$?
  bedtools   "${theirs[@]}" > "$tmp/want" 2>/dev/null; local want_rc=$?
  if [[ $got_rc -ne $want_rc ]] || ! diff -q "$tmp/want" "$tmp/got" >/dev/null; then
    echo "FAIL $name (exit $got_rc vs $want_rc)"
    diff -u "$tmp/want" "$tmp/got" | sed 's/^/      /' | head -20
    (( fail++ )); return
  fi
  echo "ok   $name"; (( pass++ ))
}

# ---------------------------------------------------------------------------
# Fixtures built here rather than under data/: a.bed, b.bed and genes.bed are
# deliberate and must not be edited or added to (CLAUDE.md).
# ---------------------------------------------------------------------------
printf 'chr10\t1\t2\tc10\nchr2\t1\t2\tc2\nchr1\t1\t2\tc1\nchr17\t1\t2\tc17\nchr7\t1\t2\tc7\n' > "$tmp/lexico.bed"
printf '# a comment\ntrack name=x\nbrowser position chr1\nchr2\t10\t20\tr1\t5\t+\nchr1\t100\t200\tr2\t6\t-\n' > "$tmp/header.bed"
printf 'chrX\t1000\nchr2\t2000\nchr1\t3000\nchr3\t4000\n' > "$tmp/genome.txt"
printf 'chrX\nchr2\nchr1\nchr3\n' > "$tmp/names.txt"
printf 'chr1\t1000\n' > "$tmp/partial_genome.txt"          # missing chr2/chrX
printf 'chr1\t100\t200\nchr1\t300\n' > "$tmp/short.bed"    # fewer than 3 columns
printf 'chr1\t100\t200\nchr1\t500\t400\n' > "$tmp/reversed.bed"
printf 'chr1\t100\t200\nchr1\t-5\t400\n' > "$tmp/negative.bed"
printf 'chr1\t10\t20\nchr1\t30\t40\t50\t60\t70\t80\n' > "$tmp/ragged.bed"
printf 'chr1\t10\t20\nchr1\t30\t40\n' > "$tmp/bed3.bed"
: > "$tmp/empty.bed"
# A larger deterministic file: >16 records per chromosome is what drags
# std::sort out of its stable insertion-sort path, so this is the case that
# catches a tie broken differently from the oracle.
awk 'BEGIN{srand(11);OFS="\t";for(i=1;i<=600;i++){
       ch=sprintf("chr%d",int(rand()*6)); s=int(rand()*120);
       r=rand(); l=(r<0.3?0:(r<0.45?1:int(rand()*8)));
       print ch,s,s+l,"r" i,int(rand()*8),(rand()<0.5?"+":"-")}}' > "$tmp/many.bed"

# --- default ordering ------------------------------------------------------
check "sort a.bed"                    -- sort -i "$DATA/a.bed"
check "sort b.bed"                    -- sort -i "$DATA/b.bed"
check "sort genes.bed"                -- sort -i "$DATA/genes.bed"
check "sort many.bed (>16 per chrom)" -- sort -i "$tmp/many.bed"
# chr10 sorts before chr2 and chr17 before chr7: the comparison is
# byte-lexicographic, with no numeric reading of the digits (SPEC.md §2.1).
check "sort lexicographic chrom order" -- sort -i "$tmp/lexico.bed"

# --- ordering flags --------------------------------------------------------
for flag in -sizeA -sizeD -chrThenSizeA -chrThenSizeD -chrThenScoreA -chrThenScoreD; do
  check "sort a.bed $flag"    -- sort -i "$DATA/a.bed" "$flag"
  check "sort b.bed $flag"    -- sort -i "$DATA/b.bed" "$flag"
  check "sort many.bed $flag" -- sort -i "$tmp/many.bed" "$flag"
done

# --- chromosome order from a file ------------------------------------------
check "sort a.bed -g"           -- sort -i "$DATA/a.bed" -g "$tmp/genome.txt"
check "sort b.bed -g"           -- sort -i "$DATA/b.bed" -g "$tmp/genome.txt"
check "sort a.bed -faidx"       -- sort -i "$DATA/a.bed" -faidx "$tmp/names.txt"
check "sort b.bed -faidx"       -- sort -i "$DATA/b.bed" -faidx "$tmp/names.txt"

# --- header ----------------------------------------------------------------
check "sort header default (dropped)" -- sort -i "$tmp/header.bed"
check "sort header -header"           -- sort -i "$tmp/header.bed" -header
check "sort header -header -sizeA"    -- sort -i "$tmp/header.bed" -header -sizeA

# --- stdin and the positional form -----------------------------------------
# Each command needs its own redirection: sharing one stdin would let the
# first of the two drain it.
check_stdin "sort a.bed via -i -"      "$DATA/a.bed" -- sort -i -
check_stdin "sort a.bed piped, no -i"  "$DATA/a.bed" -- sort
check_stdin "sort b.bed via -i - -sizeA" "$DATA/b.bed" -- sort -i - -sizeA
# mytools accepts a bare filename where bedtools insists on -i; the output has
# to match bedtools' -i form all the same.
check_args "sort a.bed positional" -- sort "$DATA/a.bed" -- sort -i "$DATA/a.bed"
check_args "sort b.bed positional -sizeD" -- sort "$DATA/b.bed" -sizeD -- sort -i "$DATA/b.bed" -sizeD

# --- error paths (exit codes are compared like any other case) -------------
check "two ordering flags"        -- sort -i "$DATA/a.bed" -sizeA -sizeD
check "ordering flag plus -g"     -- sort -i "$DATA/a.bed" -sizeA -g "$tmp/genome.txt"
check "-g missing a chromosome"   -- sort -i "$DATA/a.bed" -g "$tmp/partial_genome.txt"
check "-faidx missing a chromosome" -- sort -i "$DATA/a.bed" -faidx "$tmp/partial_genome.txt"
check "unrecognized flag"         -- sort -i "$DATA/a.bed" --bogus-flag
check "fewer than 3 columns"      -- sort -i "$tmp/short.bed"
check "start greater than end"    -- sort -i "$tmp/reversed.bed"
check "negative coordinate"       -- sort -i "$tmp/negative.bed"
check "differing field counts"    -- sort -i "$tmp/ragged.bed"
check "score sort on BED3"        -- sort -i "$tmp/bed3.bed" -chrThenScoreA
check "missing file"              -- sort -i "$tmp/does-not-exist.bed"
check "empty file"                -- sort -i "$tmp/empty.bed"

# Deliberately not a golden case: a non-integer coordinate *after* the first
# record. bedtools v2.31.1 does not handle it — std::stoll throws and the
# process dies on SIGABRT (exit 134) with nothing but a C++ terminate message.
# mytools exits 1 with the message SPEC.md §7 gives. That divergence is
# covered by TestNonIntegerCoordinate in tests/unit_test.go.

echo "---"
echo "$pass passed, $fail failed"
[[ $fail -eq 0 ]]
