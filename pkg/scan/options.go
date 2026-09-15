package scan

import (
	"fmt"
	"strings"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

// CompareOp is the operator used by exact scans.
type CompareOp int

const (
	OpEqual CompareOp = iota
	OpNotEqual
	OpGreater
	OpGreaterEqual
	OpLess
	OpLessEqual
)

func (o CompareOp) String() string {
	switch o {
	case OpEqual:
		return "=="
	case OpNotEqual:
		return "!="
	case OpGreater:
		return ">"
	case OpGreaterEqual:
		return ">="
	case OpLess:
		return "<"
	case OpLessEqual:
		return "<="
	default:
		return "?"
	}
}

// ParseCompareOp maps a textual operator to a CompareOp.
func ParseCompareOp(s string) (CompareOp, error) {
	switch strings.TrimSpace(s) {
	case "==", "=", "eq":
		return OpEqual, nil
	case "!=", "<>", "ne":
		return OpNotEqual, nil
	case ">", "gt":
		return OpGreater, nil
	case ">=", "ge":
		return OpGreaterEqual, nil
	case "<", "lt":
		return OpLess, nil
	case "<=", "le":
		return OpLessEqual, nil
	default:
		return OpEqual, fmt.Errorf("scan: unknown comparison %q", s)
	}
}

// ScanMode selects the kind of scan performed.
type ScanMode int

const (
	ModeExact ScanMode = iota
	ModeUnknown
	ModeChanged
	ModeUnchanged
	ModeIncreased
	ModeDecreased
	ModeIncreasedBy
	ModeDecreasedBy
	ModeBetween
)

func (m ScanMode) String() string {
	switch m {
	case ModeExact:
		return "exact"
	case ModeUnknown:
		return "unknown"
	case ModeChanged:
		return "changed"
	case ModeUnchanged:
		return "unchanged"
	case ModeIncreased:
		return "increased"
	case ModeDecreased:
		return "decreased"
	case ModeIncreasedBy:
		return "increased by"
	case ModeDecreasedBy:
		return "decreased by"
	case ModeBetween:
		return "between"
	default:
		return "unknown"
	}
}

// ParseScanMode maps a textual mode to a ScanMode.
func ParseScanMode(s string) (ScanMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "exact", "value":
		return ModeExact, nil
	case "unknown":
		return ModeUnknown, nil
	case "changed":
		return ModeChanged, nil
	case "unchanged", "same":
		return ModeUnchanged, nil
	case "increased", "inc":
		return ModeIncreased, nil
	case "decreased", "dec":
		return ModeDecreased, nil
	case "increased by", "incby":
		return ModeIncreasedBy, nil
	case "decreased by", "decby":
		return ModeDecreasedBy, nil
	case "between", "value between", "range":
		return ModeBetween, nil
	default:
		return ModeExact, fmt.Errorf("scan: unknown scan mode %q", s)
	}
}

// Options controls a scan session.
type Options struct {
	Type          ValueType
	Mode          ScanMode
	Value         Value // target for exact scans, delta for *By modes, lower bound for between
	Value2        Value // upper bound for ModeBetween
	Compare       CompareOp
	WritableOnly  bool
	Alignment     int
	SnapshotLimit int64 // maximum bytes captured by an unknown-value snapshot
	Epsilon       float64
	Regions       []mem.Region // optional explicit region set
}

// DefaultOptions returns sensible defaults matching configs/config.yaml.
func DefaultOptions() Options {
	return Options{
		Type:          TypeDword,
		Mode:          ModeExact,
		Compare:       OpEqual,
		WritableOnly:  true,
		Alignment:     4,
		SnapshotLimit: 2 << 30,
		Epsilon:       1e-6,
	}
}

// width returns the number of bytes a single candidate occupies.
func (o Options) width() int {
	switch o.Type {
	case TypeString, TypeAOB, TypeBinary:
		return len(o.Value.Raw)
	default:
		return o.Type.Size()
	}
}

// step returns the alignment stride used while scanning.
func (o Options) step() int {
	if o.Alignment > 0 {
		return o.Alignment
	}
	if w := o.width(); w > 0 {
		return w
	}
	return 1
}
