// Package dissect compares a memory region across several instances and
// guesses a field layout (ADR 0017). It is pure memory access: no debugger.
package dissect

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// Kind is a guessed field type.
type Kind uint8

const (
	// KindByte is a single byte.
	KindByte Kind = iota
	// KindWord is a 16-bit integer.
	KindWord
	// KindDword is a 32-bit integer.
	KindDword
	// KindQword is a 64-bit integer.
	KindQword
	// KindFloat is a 32-bit float.
	KindFloat
	// KindDouble is a 64-bit float.
	KindDouble
	// KindPointer is a pointer-sized value that points into the process.
	KindPointer
	// KindBytes is an unclassified run of bytes.
	KindBytes
)

func (k Kind) String() string {
	switch k {
	case KindByte:
		return "byte"
	case KindWord:
		return "2 bytes"
	case KindDword:
		return "4 bytes"
	case KindQword:
		return "8 bytes"
	case KindFloat:
		return "float"
	case KindDouble:
		return "double"
	case KindPointer:
		return "pointer"
	default:
		return "bytes"
	}
}

// ScanType maps a field kind to the scan value type used for editing.
func (k Kind) ScanType() scan.ValueType {
	switch k {
	case KindByte:
		return scan.TypeByte
	case KindWord:
		return scan.TypeWord
	case KindDword:
		return scan.TypeDword
	case KindQword, KindPointer:
		return scan.TypeQword
	case KindFloat:
		return scan.TypeFloat
	case KindDouble:
		return scan.TypeDouble
	default:
		return scan.TypeAOB
	}
}

// Field is one guessed field of the layout.
type Field struct {
	Offset int
	Size   int
	Kind   Kind
	// Values holds Size bytes for each instance.
	Values [][]byte
	// Targets holds the resolved pointer target per instance for KindPointer.
	Targets []uint64
}

// Format renders instance i's value.
func (f Field) Format(i int) string {
	if i < 0 || i >= len(f.Values) {
		return ""
	}
	b := f.Values[i]
	if len(b) < f.Size {
		return ""
	}
	switch f.Kind {
	case KindPointer:
		v := binary.LittleEndian.Uint64(b)
		if v == 0 {
			return "0"
		}
		return fmt.Sprintf("0x%x", v)
	case KindFloat:
		return strconv.FormatFloat(float64(math.Float32frombits(binary.LittleEndian.Uint32(b))), 'g', -1, 32)
	case KindDouble:
		return strconv.FormatFloat(math.Float64frombits(binary.LittleEndian.Uint64(b)), 'g', -1, 64)
	case KindQword:
		return strconv.FormatUint(binary.LittleEndian.Uint64(b), 10)
	case KindDword:
		return strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b)), 10)
	case KindWord:
		return strconv.FormatUint(uint64(binary.LittleEndian.Uint16(b)), 10)
	case KindBytes:
		return fmt.Sprintf("% x", b)
	default:
		return strconv.FormatUint(uint64(b[0]), 10)
	}
}

// Range is the addressable range used to recognise pointers.
type Range struct {
	Min, Max uint64
}

// AddressRange returns the addressable range of pid from its memory map.
func AddressRange(pid int) (Range, error) {
	regions, err := mem.Regions(pid)
	if err != nil {
		return Range{}, err
	}
	var r Range
	for _, reg := range regions {
		if reg.Size() == 0 {
			continue
		}
		if r.Min == 0 || reg.Start < r.Min {
			r.Min = reg.Start
		}
		if reg.End > r.Max {
			r.Max = reg.End
		}
	}
	return r, nil
}

// Dissect reads size bytes at each instance and guesses the field layout. The
// first instance is the primary; inst may be empty to dissect just base.
func Dissect(p *mem.Process, base uint64, size int, inst []uint64, r Range) ([]Field, error) {
	if size <= 0 {
		return nil, fmt.Errorf("dissect: size must be positive")
	}
	if len(inst) == 0 {
		inst = []uint64{base}
	}
	bufs := make([][]byte, len(inst))
	for i, addr := range inst {
		data, err := p.Read(addr, size)
		if err != nil && len(data) == 0 {
			return nil, err
		}
		bufs[i] = data
	}
	var fields []Field
	for off := 0; off < size; {
		f := guess(off, bufs, r)
		fields = append(fields, f)
		off += f.Size
	}
	return fields, nil
}

func guess(off int, bufs [][]byte, r Range) Field {
	if isPointer(off, bufs, r) {
		return makeField(off, 8, KindPointer, bufs, true)
	}
	if isDouble(off, bufs) {
		return makeField(off, 8, KindDouble, bufs, false)
	}
	if isFloat(off, bufs) {
		return makeField(off, 4, KindFloat, bufs, false)
	}
	if isDword(off, bufs) {
		return makeField(off, 4, KindDword, bufs, false)
	}
	return makeField(off, 1, KindByte, bufs, false)
}

func makeField(off, size int, kind Kind, bufs [][]byte, pointers bool) Field {
	f := Field{Offset: off, Size: size, Kind: kind}
	for _, b := range bufs {
		if off+size > len(b) {
			f.Values = append(f.Values, nil)
			continue
		}
		chunk := make([]byte, size)
		copy(chunk, b[off:off+size])
		f.Values = append(f.Values, chunk)
		if pointers {
			f.Targets = append(f.Targets, binary.LittleEndian.Uint64(chunk))
		}
	}
	return f
}

func isPointer(off int, bufs [][]byte, r Range) bool {
	if r.Max == 0 {
		return false
	}
	for _, b := range bufs {
		if off+8 > len(b) {
			return false
		}
		v := binary.LittleEndian.Uint64(b[off:])
		if v < r.Min || v > r.Max {
			return false
		}
	}
	return true
}

func isDouble(off int, bufs [][]byte) bool {
	for _, b := range bufs {
		if off+8 > len(b) {
			return false
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(b[off:]))
		if !plausibleFloat(v) {
			return false
		}
	}
	return true
}

func isFloat(off int, bufs [][]byte) bool {
	for _, b := range bufs {
		if off+4 > len(b) {
			return false
		}
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[off:])))
		if !plausibleFloat(v) {
			return false
		}
	}
	return true
}

func plausibleFloat(v float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return false
	}
	if v == 0 {
		return true
	}
	a := math.Abs(v)
	return a >= 1e-4 && a <= 1e12
}

func isDword(off int, bufs [][]byte) bool {
	for _, b := range bufs {
		if off+4 > len(b) {
			return false
		}
		if binary.LittleEndian.Uint32(b[off:]) >= 0x10000000 {
			return false
		}
	}
	return true
}
