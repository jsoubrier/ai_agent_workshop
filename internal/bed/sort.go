package bed

import "sort"

// Mode is one of the mutually exclusive orderings `bedtools sort` offers.
type Mode int

const (
	// ChromThenStart is the default: chromosome ascending, then start
	// ascending. Note that the *end* is not part of the comparison — bedtools
	// sorts each chromosome with `a.start < b.start` and nothing else, so
	// records sharing a start come out in whatever order std::sort leaves
	// them, not in end order.
	ChromThenStart Mode = iota
	SizeAsc
	SizeDesc
	ChromThenSizeAsc
	ChromThenSizeDesc
	ChromThenScoreAsc
	ChromThenScoreDesc
)

// Options selects the ordering. ChromOrder, when non-nil, is the chromosome
// order declared by a -g genome file or a -faidx names file; ChromOrderFile is
// its path, for the error message.
type Options struct {
	Mode           Mode
	ChromOrder     []string
	ChromOrderFile string
}

func byStart(a, b Record) bool { return a.Start < b.Start }

// sizeAsc and sizeDesc are not mirror images, and the asymmetry is bedtools',
// not a slip here: ascending breaks ties on chromosome and then start, while
// descending compares nothing but the size and leaves equal-sized features
// wherever std::sort happens to drop them. Probed against bedtools v2.31.1 —
// a symmetric pair reproduces neither. Both use the widened coordinates that
// Record.Size documents.
//
// The start tie-break makes the end one redundant: two features of equal size
// starting at the same base also end at the same base.
func sizeAsc(a, b Record) bool {
	if a.Size() != b.Size() {
		return a.Size() < b.Size()
	}
	if a.Chrom != b.Chrom {
		return a.Chrom < b.Chrom
	}
	return a.SizeStart() < b.SizeStart()
}

func sizeDesc(a, b Record) bool { return a.Size() > b.Size() }

// chromSizeAsc is applied within one chromosome, so only the size and start
// halves of the comparator above can bite. Its descending counterpart is,
// again, size and nothing else.
func chromSizeAsc(a, b Record) bool {
	if a.Size() != b.Size() {
		return a.Size() < b.Size()
	}
	return a.SizeStart() < b.SizeStart()
}

func scoreAsc(a, b Record) bool  { return a.Score() < b.Score() }
func scoreDesc(a, b Record) bool { return a.Score() > b.Score() }

// Sort returns f's records in the requested order.
//
// The shape of this function mirrors bedtools' own: records are bucketed by
// chromosome, each bucket is sorted by start, and the requested ordering is
// then applied on top of that already-start-sorted input. Every sort goes
// through StdSort, because which of two equal records comes first is decided by
// std::sort's partitioning and is part of the output.
func Sort(f *File, opt Options) ([]Record, error) {
	switch opt.Mode {
	case ChromThenScoreAsc, ChromThenScoreDesc:
		// bedtools checks the column count, not the individual record, and it
		// makes this check even when the file held no records at all.
		if f.BedType < 5 {
			return nil, errf("Error: Requested a sort by score, but your BED file does not appear to be in BED 5 format or greater.  Exiting.")
		}
	}

	byChrom := map[string][]Record{}
	for _, r := range f.Records {
		byChrom[r.Chrom] = append(byChrom[r.Chrom], r)
	}
	for chrom := range byChrom {
		StdSort(byChrom[chrom], byStart)
	}

	// Default chromosome order is byte-lexicographic, the order a C++
	// std::map<string, ...> iterates in: chr1, chr10, chr2, chrX.
	chroms := make([]string, 0, len(byChrom))
	for chrom := range byChrom {
		chroms = append(chroms, chrom)
	}
	sort.Strings(chroms)

	if opt.ChromOrder != nil {
		declared := map[string]bool{}
		for _, c := range opt.ChromOrder {
			declared[c] = true
		}
		for _, c := range chroms {
			if !declared[c] {
				return nil, errf("Chromosome %q undefined in %s", c, opt.ChromOrderFile)
			}
		}
		ordered := make([]string, 0, len(chroms))
		for _, c := range opt.ChromOrder {
			if _, ok := byChrom[c]; ok {
				ordered = append(ordered, c)
			}
		}
		chroms = ordered
	}

	switch opt.Mode {
	case ChromThenSizeAsc:
		sortEach(byChrom, chroms, chromSizeAsc)
	case ChromThenSizeDesc:
		sortEach(byChrom, chroms, sizeDesc)
	case ChromThenScoreAsc:
		sortEach(byChrom, chroms, scoreAsc)
	case ChromThenScoreDesc:
		sortEach(byChrom, chroms, scoreDesc)
	}

	out := make([]Record, 0, len(f.Records))
	for _, c := range chroms {
		out = append(out, byChrom[c]...)
	}

	// -sizeA/-sizeD sort globally and do not group by chromosome. bedtools
	// gathers every chromosome's (start-sorted) records into one list in
	// chromosome order and sorts that by size; the chromosome order therefore
	// still shows through in the ties.
	switch opt.Mode {
	case SizeAsc:
		StdSort(out, sizeAsc)
	case SizeDesc:
		StdSort(out, sizeDesc)
	}
	return out, nil
}

func sortEach(byChrom map[string][]Record, chroms []string, less Less) {
	for _, c := range chroms {
		StdSort(byChrom[c], less)
	}
}
