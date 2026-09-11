package bed

// This file reimplements libstdc++'s std::sort (introsort) element for element.
//
// Why: bedtools is the oracle (CLAUDE.md, SPEC.md §1) and `bedtools sort` calls
// std::sort, which is *not* stable. On inputs longer than 16 records the order of
// records that compare equal is whatever introsort's partitioning happens to
// produce, and that permutation is part of the output we have to match byte for
// byte. `bedtools sort -i data/a.bed -sizeA` is such a case: a03/a04 and a09/a10
// come out reversed relative to their input order. A stable Go sort cannot
// reproduce that, so we reproduce the algorithm instead.
//
// The code below follows libstdc++ bits/stl_algo.h and bits/stl_heap.h:
//   __sort -> __introsort_loop + __final_insertion_sort
//   __introsort_loop -> __unguarded_partition_pivot / __partial_sort (heapsort)
// Names are kept close to the originals so the two can be diffed by eye.

// Less reports whether a sorts before b, mirroring a C++ strict-weak-ordering
// comparator.
type Less func(a, b Record) bool

// insertionThreshold is libstdc++'s _S_threshold.
const insertionThreshold = 16

// StdSort sorts v exactly as C++ `std::sort(v.begin(), v.end(), less)` would.
func StdSort(v []Record, less Less) {
	if len(v) == 0 {
		return
	}
	introsortLoop(v, 0, len(v), lg(len(v))*2, less)
	finalInsertionSort(v, 0, len(v), less)
}

// lg is libstdc++'s __lg: floor(log2(n)).
func lg(n int) int {
	k := 0
	for n > 1 {
		n >>= 1
		k++
	}
	return k
}

func introsortLoop(v []Record, first, last, depthLimit int, less Less) {
	for last-first > insertionThreshold {
		if depthLimit == 0 {
			// __partial_sort(first, last, last) == heapsort of the range.
			partialSortAll(v, first, last, less)
			return
		}
		depthLimit--
		cut := unguardedPartitionPivot(v, first, last, less)
		introsortLoop(v, cut, last, depthLimit, less)
		last = cut
	}
}

func unguardedPartitionPivot(v []Record, first, last int, less Less) int {
	mid := first + (last-first)/2
	moveMedianToFirst(v, first, first+1, mid, last-1, less)
	return unguardedPartition(v, first+1, last, first, less)
}

func moveMedianToFirst(v []Record, result, a, b, c int, less Less) {
	switch {
	case less(v[a], v[b]):
		switch {
		case less(v[b], v[c]):
			v[result], v[b] = v[b], v[result]
		case less(v[a], v[c]):
			v[result], v[c] = v[c], v[result]
		default:
			v[result], v[a] = v[a], v[result]
		}
	case less(v[a], v[c]):
		v[result], v[a] = v[a], v[result]
	case less(v[b], v[c]):
		v[result], v[c] = v[c], v[result]
	default:
		v[result], v[b] = v[b], v[result]
	}
}

// unguardedPartition partitions [first,last) around the value at index pivot,
// which sits outside the range and is never moved.
func unguardedPartition(v []Record, first, last, pivot int, less Less) int {
	for {
		for less(v[first], v[pivot]) {
			first++
		}
		last--
		for less(v[pivot], v[last]) {
			last--
		}
		if first >= last {
			return first
		}
		v[first], v[last] = v[last], v[first]
		first++
	}
}

func finalInsertionSort(v []Record, first, last int, less Less) {
	if last-first > insertionThreshold {
		insertionSort(v, first, first+insertionThreshold, less)
		unguardedInsertionSort(v, first+insertionThreshold, last, less)
	} else {
		insertionSort(v, first, last, less)
	}
}

func insertionSort(v []Record, first, last int, less Less) {
	if first == last {
		return
	}
	for i := first + 1; i != last; i++ {
		if less(v[i], v[first]) {
			val := v[i]
			copy(v[first+1:i+1], v[first:i])
			v[first] = val
		} else {
			unguardedLinearInsert(v, i, less)
		}
	}
}

func unguardedInsertionSort(v []Record, first, last int, less Less) {
	for i := first; i != last; i++ {
		unguardedLinearInsert(v, i, less)
	}
}

// unguardedLinearInsert assumes a smaller element exists somewhere to the left,
// so it needs no bounds check — as in libstdc++.
func unguardedLinearInsert(v []Record, last int, less Less) {
	val := v[last]
	next := last - 1
	for less(val, v[next]) {
		v[last] = v[next]
		last = next
		next--
	}
	v[last] = val
}

// partialSortAll is __partial_sort(first, last, last): make_heap then sort_heap.
func partialSortAll(v []Record, first, last int, less Less) {
	makeHeap(v, first, last, less)
	sortHeap(v, first, last, less)
}

func makeHeap(v []Record, first, last int, less Less) {
	length := last - first
	if length < 2 {
		return
	}
	parent := (length - 2) / 2
	for {
		value := v[first+parent]
		adjustHeap(v, first, parent, length, value, less)
		if parent == 0 {
			return
		}
		parent--
	}
}

func adjustHeap(v []Record, first, holeIndex, length int, value Record, less Less) {
	topIndex := holeIndex
	secondChild := holeIndex
	for secondChild < (length-1)/2 {
		secondChild = 2 * (secondChild + 1)
		if less(v[first+secondChild], v[first+secondChild-1]) {
			secondChild--
		}
		v[first+holeIndex] = v[first+secondChild]
		holeIndex = secondChild
	}
	if length&1 == 0 && secondChild == (length-2)/2 {
		secondChild = 2 * (secondChild + 1)
		v[first+holeIndex] = v[first+secondChild-1]
		holeIndex = secondChild - 1
	}
	pushHeap(v, first, holeIndex, topIndex, value, less)
}

func pushHeap(v []Record, first, holeIndex, topIndex int, value Record, less Less) {
	parent := (holeIndex - 1) / 2
	for holeIndex > topIndex && less(v[first+parent], value) {
		v[first+holeIndex] = v[first+parent]
		holeIndex = parent
		parent = (holeIndex - 1) / 2
	}
	v[first+holeIndex] = value
}

func sortHeap(v []Record, first, last int, less Less) {
	for last-first > 1 {
		last--
		popHeap(v, first, last, last, less)
	}
}

func popHeap(v []Record, first, last, result int, less Less) {
	value := v[result]
	v[result] = v[first]
	adjustHeap(v, first, 0, last-first, value, less)
}
