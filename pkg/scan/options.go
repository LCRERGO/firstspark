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
	ModeBigger
	ModeSmaller
	ModeSameAsFirst
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
	case ModeBigger:
		return "bigger"
	case ModeSmaller:
		return "smaller"
	case ModeSameAsFirst:
		return "same as first"
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
	case "bigger", "bigger than", "greater", "greater than", "gt":
		return ModeBigger, nil
	case "smaller", "smaller than", "less", "less than", "lt":
		return ModeSmaller, nil
	case "same as first", "same as first scan", "first":
		return ModeSameAsFirst, nil
	default:
		return ModeExact, fmt.Errorf("scan: unknown scan mode %q", s)
	}
}

// ExecutableMode selects which regions the executable permission filter keeps.
type ExecutableMode int

const (
	// ExecAny keeps executable and non-executable regions.
	ExecAny ExecutableMode = iota
	// ExecOnly keeps executable regions only.
	ExecOnly
	// ExecNonExec keeps non-executable regions only.
	ExecNonExec
)

func (m ExecutableMode) String() string {
	switch m {
	case ExecOnly:
		return "only"
	case ExecNonExec:
		return "non-executable"
	default:
		return "any"
	}
}

// ParseExecutableMode maps a textual filter to an ExecutableMode.
func ParseExecutableMode(s string) (ExecutableMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "any":
		return ExecAny, nil
	case "only", "executable", "yes":
		return ExecOnly, nil
	case "non", "non-executable", "nonexec", "no":
		return ExecNonExec, nil
	default:
		return ExecAny, fmt.Errorf("scan: unknown executable filter %q", s)
	}
}

// RegionScope selects which memory regions a scan covers.
type RegionScope int

const (
	// ScopeAllWritable scans every readable writable region (the reference tool's
	// default).
	ScopeAllWritable RegionScope = iota
	// ScopeHeapStackExecBSS scans the heap, the stack, anonymous mappings and
	// the main executable (scanmem's default).
	ScopeHeapStackExecBSS
	// ScopeAllReadable scans every readable region.
	ScopeAllReadable
)

// Options controls a scan session.
type Options struct {
	Type          ValueType
	Mode          ScanMode
	Value         Value // target for exact/bigger/smaller scans, delta for *By modes, lower bound for between
	Value2        Value // upper bound for ModeBetween
	Grouped       *GroupedPattern
	Compare       CompareOp
	WritableOnly  bool
	Alignment     int
	SnapshotLimit int64 // maximum bytes captured by an unknown-value snapshot
	MaxResults    int   // stop after this many matches (0 = unlimited)
	Scope         RegionScope
	Epsilon       float64
	Regions       []mem.Region // optional explicit region set
	Executable    ExecutableMode
	CopyOnWrite   bool
	Start         uint64 // optional inclusive lower bound override (0 = none)
	Stop          uint64 // optional exclusive upper bound override (0 = none)
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
		Scope:         ScopeAllWritable,
		Epsilon:       1e-6,
	}
}

// width returns the number of bytes a single candidate occupies.
func (o Options) width() int {
	if o.Type == TypeGrouped {
		if o.Grouped == nil {
			return 0
		}
		return o.Grouped.Size()
	}
	switch o.Type {
	case TypeString, TypeAOB, TypeBinary, TypeUTF16LE, TypeUTF16BE, TypeUTF32LE, TypeUTF32BE:
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
