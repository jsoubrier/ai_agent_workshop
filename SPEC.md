# SPEC.md — `mytools` v1

Specification for `mytools`, a partial reimplementation of `bedtools` in Go.
Derived from an interview; oracle behaviour below was probed against the installed
`bedtools v2.31.1` on 2026-09-11, not assumed.

Read alongside `CLAUDE.md`. Where the two disagree, the conflicts are called out
explicitly in [§9](#9-known-conflicts-and-deliberate-divergences) rather than silently
resolved.

---

## 1. Scope

v1 implements exactly two subcommands:

- `mytools sort`
- `mytools intersect`

`merge`, `subtract` and `closest` are **out of scope for v1**. They are named in
`CLAUDE.md` as the eventual target set; nothing in v1 should preclude them, but no
work goes into them now.

Input is **BED only**. BAM, VCF, GFF and GTF inputs are out of scope, including the
`-abam`, `-ubam`, `-bed` and `-split` machinery that exists to serve them. (`-split`
is retained as a flag — see §3.2 — but its BAM/GFF-specific paths are not.)

The oracle rule from `CLAUDE.md` governs everything not pinned down here, without
exception: if `mytools` and real `bedtools` differ on the same input, `mytools` is
wrong. That includes exit codes (§5) and error messages (§7), not just interval
semantics.

---

## 2. `mytools sort`

### 2.1 Default ordering

With no flags, `sort` orders records by **chromosome ascending, then start ascending,
then end ascending**, where chromosome comparison is **byte-lexicographic**, exactly
as `bedtools sort` does. So:

```
$ printf 'chr10\t1\t2\nchr2\t1\t2\nchr1\t1\t2\n' | bedtools sort -i -
chr1	1	2
chr10	1	2
chr2	1	2
```

`chr10` sorts before `chr2`. There is **no natural/version ordering** and no flag to
request one. Comparison is case-sensitive and byte-oriented — no locale, no Unicode
collation, no numeric interpretation of embedded digits.

`mytools sort` must be byte-identical to `bedtools sort` on every input. There is no
divergence from the oracle anywhere in this subcommand.

Ties on `(chrom, start, end)` preserve input order — the sort is **stable**.

### 2.2 Flags

All flags of `bedtools sort v2.31.1` are supported.

| Flag | Behaviour |
| --- | --- |
| `-i <file>` | Input file. `-` means stdin. Required (see §5 for the positional-argument allowance). |
| `-sizeA` | Sort by feature size (`end - start`) ascending, across all chromosomes. |
| `-sizeD` | Sort by feature size descending. |
| `-chrThenSizeA` | Chromosome ascending, then size ascending. |
| `-chrThenSizeD` | Chromosome ascending, then size descending. |
| `-chrThenScoreA` | Chromosome ascending, then BED column 5 (score) ascending. |
| `-chrThenScoreD` | Chromosome ascending, then score descending. |
| `-g <genome.txt>` | Order chromosomes by their order of declaration in the genome file. |
| `-faidx <names.txt>` | Order chromosomes by their order in the `.fai`/names file. |
| `-header` | Emit leading header lines before the sorted records (see §4.3). |

There are no `mytools`-only flags. The flag set is exactly bedtools'.

Notes binding these down:

- The ordering flags are mutually exclusive; passing two of them is an error. Match
  the oracle's message and exit status.
- Under `-g` and `-faidx`, a record whose chromosome does not appear in the supplied
  file is an error — match the oracle's message and exit status.
- `-sizeA`/`-sizeD` sort globally by size and do not group by chromosome; this is
  bedtools' behaviour and is not to be "improved".
- Within any of these orderings, ties break by input order (stable).

---

## 3. `mytools intersect`

### 3.1 Overlap predicate

BED is 0-based, half-open. Intervals `a` and `b` overlap iff

```
a.start < b.end  &&  b.start < a.end
```

Strict `<` on both sides. Bookended intervals (`a.end == b.start`) do **not** overlap.
This predicate gets its own unit test; per `CLAUDE.md` every off-by-one in this
project lives here.

**Zero-length intervals are the exception and must not be run through the predicate
above.** `data/a.bed` and `data/b.bed` deliberately contain them (`a07`, `a12`, `a16`,
`b02`, `b07`), and bedtools special-cases them in ways the formula does not predict.
Probed behaviour, to be encoded verbatim in golden tests:

```
$ cat za.bed            $ cat zb.bed
chr1 100 200 z1         chr1 200 300 y1
chr1 200 200 z2         chr1 200 200 y2
                        chr1 150 250 y3

$ bedtools intersect -a za.bed -b zb.bed -wa -wb
chr1 100 200 z1 0 +   chr1 200 200 y2 0 +     <- zero-length B *does* hit at a.end
chr1 100 200 z1 0 +   chr1 150 250 y3 0 +
chr1 200 200 z2 0 +   chr1 200 300 y1 0 -     <- zero-length A *does* hit at b.start
chr1 200 200 z2 0 +   chr1 200 200 y2 0 +
chr1 200 200 z2 0 +   chr1 150 250 y3 0 +
```

That is, a zero-length interval overlaps anything touching its point, including
bookended neighbours that a non-degenerate interval would not match.

`-wao` reports overlap lengths that are **negative** for these cases:

```
chr1 100 200 z1 0 + chr1 200 200 y2 0 +  -1
chr1 100 200 z1 0 + chr1 150 250 y3 0 +  50
chr1 200 200 z2 0 + chr1 200 300 y1 0 -   0
chr1 200 200 z2 0 + chr1 200 200 y2 0 +  -2
chr1 200 200 z2 0 + chr1 150 250 y3 0 +   0
```

Do not normalise `-1`, `-2` or `0` to something more sensible. Reproduce them, and
comment the code and the test with the reason: bedtools is the oracle.

### 3.2 Flags

All `bedtools intersect` flags are supported except the BAM/VCF-specific ones, which
are out of scope with BED-only input.

**Inputs**

| Flag | Behaviour |
| --- | --- |
| `-a <file>` | Query file. `-` means stdin. Required. |
| `-b <file>[,<file>…]` | One or more subject files. Repeatable, and accepts multiple space- or comma-separated paths in one `-b`. Required. |

**Output modes** (mutually exclusive except where bedtools itself allows combination)

| Flag | Behaviour |
| --- | --- |
| *(default)* | Write the intersected region only — the overlapping portion of A, carrying A's trailing columns. |
| `-wa` | Write the original A record for each overlap. |
| `-wb` | Write the original B record for each overlap. Combines with `-wa`. |
| `-wo` | Write A and B and the number of overlapping bases; suppress zero-overlap pairs. |
| `-wao` | As `-wo` but report A records with no overlap, padded with `.` fields and `0`. |
| `-loj` | Left outer join: every A record appears at least once, padded with `.` when unmatched. |
| `-u` | Write each A record once if it has any overlap. |
| `-c` | Append the count of overlapping B features to each A record. |
| `-C` | As `-c` but report a separate count per `-b` file. |
| `-v` | Write only A records with **no** overlap. |

**Overlap thresholds**

| Flag | Behaviour |
| --- | --- |
| `-f <frac>` | Minimum fraction of **A** that must be overlapped. Default `1E-9` (any overlap). |
| `-F <frac>` | Minimum fraction of **B** that must be overlapped. |
| `-r` | Require that the fraction is reciprocal — `-f` applies to B as well. |
| `-e` | With both `-f` and `-F`, require **either** threshold rather than both. |

**Strand**

| Flag | Behaviour |
| --- | --- |
| `-s` | Require same strand (BED column 6). |
| `-S` | Require opposite strand. |

`-s`/`-S` require a strand column; behaviour on BED3/BED4/BED5 input matches the
oracle.

**Other**

| Flag | Behaviour |
| --- | --- |
| `-split` | Treat BED12 blocks as separate intervals. BED12 only; BAM/GFF paths out of scope. |
| `-sorted` | Use the low-memory sweep algorithm, requiring position-sorted input (see §3.3). |
| `-g <genome.txt>` | With `-sorted`, the chromosome order the inputs are expected to follow. |
| `-header` | Emit A's header lines before results. |
| `-names <name>[,…]` | Label each `-b` file in the output with the given names. |
| `-filenames` | Label each `-b` file in the output with its file path. |
| `-sortout` | Sort the output of multiple `-b` files by file. |

### 3.3 `-sorted` and input order

`-sorted` is the streaming path: A and B are swept in parallel and neither is fully
resident. It requires both inputs to be in position-sorted order — the order
`mytools sort` produces with no flags (§2.1) — unless `-g` supplies an explicit
chromosome order.

If input violates that order, bedtools aborts with exit `1` and names the offending
record:

```
$ bedtools intersect -a data/a.bed -b data/b.bed -sorted
Error: Sorted input specified, but the file data/b.bed has the following out of order record
chr1	0	50	b01	11	+
```

`mytools` matches this: same exit code, same shape of message, aborting at the first
out-of-order record. Note the consequence for the golden harness — `data/a.bed` and
`data/b.bed` are *unsorted on purpose*, so `-sorted` golden cases must run against
sorted derivatives produced by `mytools sort`, not against the raw fixtures.

Without `-sorted`, the `-b` file(s) are loaded into an in-memory interval index and A
is streamed against it, as bedtools does. This is acceptable — see §6.

---

## 4. Input and output format

### 4.1 BED records

- Tab-delimited. BED3 through BED12 plus arbitrary trailing columns (`BED6+n`) are
  all accepted; the column count is not validated beyond the 3-field minimum.
- Column meanings used by `mytools`: 1 chrom, 2 start, 3 end, 5 score
  (`-chrThenScore*`), 6 strand (`-s`/`-S`), 10–12 blocks (`-split`).
- Coordinates are 0-based, half-open, and the output is likewise 0-based. There is no
  1-based mode and no coordinate translation anywhere in `mytools`.
- When a record is echoed whole — `sort`, `intersect -wa`, `-wb`, `-u`, `-v`, and the
  A-side of `-loj`/`-wo`/`-wao` — **every original column is preserved verbatim**,
  including columns `mytools` does not interpret. Only the default (no-flag)
  `intersect` output rewrites coordinates, and it rewrites only columns 2 and 3.

### 4.2 Output

- stdout is data: tab-delimited BED, LF line endings, one record per line, trailing
  newline on the final record.
- stderr is diagnostics only. Nothing that is not data ever reaches stdout.

### 4.3 Header, `track`, `browser` and `#` lines — oracle-defined

Probed:

```
$ cat hdr.bed
# comment
track name=x
browser pos
chr1	100	200

$ bedtools sort -i hdr.bed            $ bedtools sort -i hdr.bed -header
chr1	100	200                         # comment
                                      track name=x
                                      browser pos
                                      chr1	100	200
```

So: leading `#`, `track` and `browser` lines are **silently dropped by default** and
**reproduced verbatim, in order, ahead of the records** under `-header`. They are not
sorted, not deduplicated, and not re-emitted per chromosome. Only the leading block
counts as header; behaviour for such lines appearing mid-file follows the oracle.

An empty input file is not an error — exit `0`, empty output.

---

## 5. CLI conventions

- Subcommand form: `mytools <subcommand> [flags]`.
- `-` as a filename means stdin, for `-i`, `-a` and `-b`. At most one input may be
  stdin per invocation; a second `-` is a usage error.
- As a convenience over bedtools, a single bare positional argument is accepted in
  place of `-i` for `sort` (`mytools sort foo.bed`). `intersect` requires explicit
  `-a`/`-b`.
- `mytools`, `mytools -h`, `mytools <subcommand> -h` print usage to stdout and exit
  `0`. Usage printed *because of an error* goes to stderr with a non-zero exit.

### Exit codes

Exit codes are **bedtools'**, not the `0`/`1`/`2` scheme in `CLAUDE.md` — see §9.

| Code | Meaning |
| --- | --- |
| `0` | Success, including empty output and empty input. |
| `1` | Every error. Bad input data (malformed record, unopenable file, out-of-order input under `-sorted`, unknown chromosome under `-g`/`-faidx`, input exceeding the size cap) **and** usage errors (unknown flag, missing required flag, mutually exclusive flags, malformed flag value). |

Verified for the usage case:

```
$ bedtools sort --bogus-flag -i chrord.bed ; echo "exit=$?"
*****ERROR: Unrecognized parameter: --bogus-flag *****
[usage block]
exit=1
```

`2` is never returned. If a future caller needs to distinguish usage errors from data
errors, it must parse stderr, as it would with bedtools.

---

## 6. Performance and resource limits

**Correctness is the only performance bar.** There is no runtime target relative to
bedtools; a `mytools` invocation may be arbitrarily slower than the oracle provided
the output matches.

Memory is bounded instead, by a hard input-size cap:

- **No single input file may exceed 2 MiB (2,097,152 bytes).** The cap is **per
  file**, applied independently to `sort`'s `-i`, `intersect`'s `-a`, and each `-b`.
  There is no combined-total limit.
- For a regular file, the size is checked before reading. For stdin (`-`), bytes are
  counted as they stream and the process **aborts the moment the count passes
  2 MiB** — it does not buffer the whole stream to measure it.
- Exceeding the cap is a bad-input error: message to stderr naming the file (or
  `stdin`) and the limit, exit `1`. Not an OOM kill, not a truncated result, and
  never a partial-but-plausible-looking output — if a run aborts mid-stream, anything
  already written to stdout is invalid and the non-zero exit is the signal.

With that cap in place, `sort` may hold an entire input in memory and `intersect`
without `-sorted` may hold all `-b` files in an in-memory index. Streaming remains
the preferred style per `CLAUDE.md` — `intersect -sorted` in particular is a true
streaming sweep — but the cap, not discipline, is what guarantees the process fits in
this VM (3.8 GiB total, ~3.1 GiB available, 2 cores).

---

## 7. Error handling

Behaviour on malformed data is **oracle-defined**. Probed against `bedtools sort`;
the messages below are the contract, and the golden tests encode them.

| Input condition | Exit | stderr |
| --- | --- | --- |
| Fewer than 3 columns | `1` | `It looks as though you have less than 3 columns at line N in file F.  Are you sure your files are tab-delimited?` |
| Non-integer coordinate | `1` | `Unexpected file format.  Please use tab-delimited BED, GFF, or VCF. Perhaps you have non-integer starts or ends at line N?` |
| Negative coordinate | `1` | Same message as non-integer — bedtools does not distinguish the two. |
| `start > end` | `1` | `Error: malformed BED entry at line N. Start was greater than end. Exiting.` |
| `start == end` | `0` | Legal. Zero-length intervals are valid input; see §3.1. |
| Unopenable / missing file | `1` | `Error: The requested file (F) could not be opened. Error message: (No such file or directory). Exiting!` |
| Empty file | `0` | — (empty output) |
| Out of order under `-sorted` | `1` | See §3.3. |
| Unrecognised flag | `1` | `*****ERROR: Unrecognized parameter: F *****` followed by the subcommand usage block. |

Every one of these aborts on the **first** offending line. Nothing is skipped with a
warning, and nothing malformed is passed through to stdout.

---

## 8. Testing

Per `CLAUDE.md`, and restated here as spec obligations:

- `./tests/run_golden.sh` diffs every subcommand and flag against real `bedtools` on
  `data/`. It does not exist yet; `tests/README.md` holds the worked example to build
  it from. It runs before every commit.
- Golden coverage required for v1: every flag in §2.2 and §3.2, and every row of the
  §7 error table.
- Every golden case diffs directly against real `bedtools`; there are no cases
  needing a checked-in expected file, because `mytools` never diverges from the
  oracle. No new fixtures are required, and `data/a.bed`, `data/b.bed` and
  `data/genes.bed` must not be edited or regenerated.
- One golden case needs special construction: **`-sorted` cases** run against
  `mytools sort` output, since the raw fixtures are deliberately unsorted (§3.3).
- Unit tests live in `tests/`, run without bedtools installed, and cover at minimum:
  the overlap predicate itself, bookended intervals, zero-length intervals (including
  the negative `-wao` lengths from §3.1), nested intervals, intervals at position 0,
  the lexicographic chromosome comparator (`chr10` before `chr2`), and the 2 MiB cap
  on both a regular file and stdin.
- A fixed golden failure gets the unit test that would have caught it, in the same
  commit.
- Where a test encodes surprising bedtools behaviour, it carries a comment saying so.
  Tests encode the oracle, never what the behaviour ought to be.

---

## 9. Known conflicts and deliberate divergences

**`mytools` does not diverge from bedtools on behaviour.** Any observable difference
between `mytools` and the oracle on the same input is a bug in `mytools`, without
exception. Two divergences considered during the interview — natural chromosome
ordering for `sort`, and a `-lexico` flag to escape it — were **explicitly scrapped**;
do not reintroduce either, and do not add `mytools`-only flags of any kind.

The remaining items:

1. **Exit codes follow bedtools, overriding `CLAUDE.md`.** `CLAUDE.md` mandates
   `0` success / `1` bad input / `2` usage error. Real bedtools exits `1` for usage
   errors too, and by explicit instruction the oracle wins: `mytools` returns only
   `0` and `1` (§5). **`CLAUDE.md` is therefore out of date on this point** and its
   exit-code convention should be amended to match, so that a future session reading
   it does not reinstate `2`.

2. **`-split` is in scope only for BED12.** The flag exists to handle spliced BAM
   alignments and GFF/GTF features as much as BED12 blocks, and with BED-only input
   most of what it means in bedtools is unreachable. `mytools` implements the BED12
   block semantics and nothing else. This is a scope limit, not a behavioural
   difference: within BED12 input, `-split` matches the oracle.

---

## 10. Out of scope for v1

- `merge`, `subtract`, `closest`, and every other bedtools subcommand.
- BAM, VCF, GFF, GTF input or output; `-abam`, `-ubam`, `-bed`.
- Compressed input (`.gz`, `.bgz`) and tabix indexes. `data/hg002.vcf.gz` and its
  index are not v1 inputs.
- Third-party Go dependencies. Standard library only, per `CLAUDE.md`.
- Any language other than Go, per `CLAUDE.md`.
- Parallelism. Two cores, correctness-only bar; single-threaded throughout.
