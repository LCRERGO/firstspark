package scan

import "testing"

func res(addrs ...uint64) []Result {
	out := make([]Result, len(addrs))
	for i, a := range addrs {
		out[i] = Result{Addr: a}
	}
	return out
}

func addrs(r []Result) []uint64 {
	out := make([]uint64, len(r))
	for i, x := range r {
		out[i] = x.Addr
	}
	return out
}

func equalAddrs(a, b []uint64) bool {
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

func TestCompareResultSets(t *testing.T) {
	a := res(1, 2, 3)
	b := res(3, 4, 5)

	tests := []struct {
		name string
		op   SetOp
		want []uint64
	}{
		{"only a", SetOnlyA, []uint64{1, 2}},
		{"only b", SetOnlyB, []uint64{4, 5}},
		{"both", SetBoth, []uint64{3}},
		{"either", SetEither, []uint64{1, 2, 4, 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := addrs(CompareResultSets(a, b, tc.op))
			if !equalAddrs(got, tc.want) {
				t.Fatalf("CompareResultSets = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCompareResultSetsEmpty(t *testing.T) {
	got := CompareResultSets(nil, res(7), SetBoth)
	if len(got) != 0 {
		t.Fatalf("expected no matches, got %v", addrs(got))
	}
	got = CompareResultSets(nil, res(7), SetOnlyB)
	if !equalAddrs(addrs(got), []uint64{7}) {
		t.Fatalf("SetOnlyB = %v, want [7]", addrs(got))
	}
}
