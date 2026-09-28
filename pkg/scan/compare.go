package scan

// SetOp selects how two result sets are combined by CompareResultSets.
type SetOp int

const (
	// SetOnlyA keeps addresses found in a but not in b.
	SetOnlyA SetOp = iota
	// SetOnlyB keeps addresses found in b but not in a.
	SetOnlyB
	// SetBoth keeps addresses found in both sets.
	SetBoth
	// SetEither keeps addresses found in exactly one of the two sets.
	SetEither
)

// CompareResultSets combines two result sets by address, preserving the scan
// order of each source. Each output row is copied from the set it came from,
// so the caller keeps the source values and types.
func CompareResultSets(a, b []Result, op SetOp) []Result {
	inA := make(map[uint64]bool, len(a))
	for _, r := range a {
		inA[r.Addr] = true
	}
	inB := make(map[uint64]bool, len(b))
	for _, r := range b {
		inB[r.Addr] = true
	}

	var out []Result
	switch op {
	case SetOnlyA:
		for _, r := range a {
			if !inB[r.Addr] {
				out = append(out, r)
			}
		}
	case SetOnlyB:
		for _, r := range b {
			if !inA[r.Addr] {
				out = append(out, r)
			}
		}
	case SetBoth:
		for _, r := range a {
			if inB[r.Addr] {
				out = append(out, r)
			}
		}
	case SetEither:
		for _, r := range a {
			if !inB[r.Addr] {
				out = append(out, r)
			}
		}
		for _, r := range b {
			if !inA[r.Addr] {
				out = append(out, r)
			}
		}
	}
	return out
}
