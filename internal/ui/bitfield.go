//go:build gui

package ui

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// typeForSize maps a container width to the integer scan type of that width.
func typeForSize(size int) scan.ValueType {
	switch size {
	case 1:
		return scan.TypeByte
	case 2:
		return scan.TypeWord
	case 8:
		return scan.TypeQword
	default:
		return scan.TypeDword
	}
}

// bitSpec describes a bitfield inside a size-byte container. It is a
// table-only edit/display type: bitfields are not scannable.
type bitSpec struct {
	size   int
	offset int
	width  int
	signed bool
}

func (b bitSpec) mask() uint64 {
	if b.width >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << uint(b.width)) - 1
}

// extract reads the field from the container's raw bytes.
func (b bitSpec) extract(raw []byte) (uint64, error) {
	if len(raw) < b.size || b.size <= 0 {
		return 0, fmt.Errorf("%s", i18n.T("error.bitfield_range"))
	}
	var container uint64
	switch b.size {
	case 1:
		container = uint64(raw[0])
	case 2:
		container = uint64(binary.LittleEndian.Uint16(raw))
	case 4:
		container = uint64(binary.LittleEndian.Uint32(raw))
	case 8:
		container = binary.LittleEndian.Uint64(raw)
	default:
		return 0, fmt.Errorf("%s", i18n.T("error.bitfield_range"))
	}
	return (container >> uint(b.offset)) & b.mask(), nil
}

// format renders the field value, sign-extending when configured.
func (b bitSpec) format(raw []byte) string {
	v, err := b.extract(raw)
	if err != nil {
		return ""
	}
	if b.signed && b.width < 64 && v&(1<<uint(b.width-1)) != 0 {
		return strconv.FormatInt(int64(v)-int64(uint64(1)<<uint(b.width)), 10)
	}
	return strconv.FormatUint(v, 10)
}

// parse converts user input into the field value.
func (b bitSpec) parse(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	var v uint64
	var err error
	if strings.HasPrefix(s, "-") {
		n, perr := strconv.ParseInt(s, 0, 64)
		if perr != nil {
			return 0, perr
		}
		v = uint64(n)
	} else {
		v, err = strconv.ParseUint(s, 0, 64)
		if err != nil {
			return 0, err
		}
	}
	if b.width < 64 && v > b.mask() {
		return 0, fmt.Errorf("%s", i18n.T("error.bitfield_range"))
	}
	return v, nil
}

// set returns a copy of raw with the field replaced.
func (b bitSpec) set(raw []byte, value uint64) ([]byte, error) {
	if len(raw) < b.size || b.size <= 0 {
		return nil, fmt.Errorf("%s", i18n.T("error.bitfield_range"))
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	var container uint64
	switch b.size {
	case 1:
		container = uint64(out[0])
	case 2:
		container = uint64(binary.LittleEndian.Uint16(out))
	case 4:
		container = uint64(binary.LittleEndian.Uint32(out))
	case 8:
		container = binary.LittleEndian.Uint64(out)
	default:
		return nil, fmt.Errorf("%s", i18n.T("error.bitfield_range"))
	}
	container &^= b.mask() << uint(b.offset)
	container |= (value & b.mask()) << uint(b.offset)
	switch b.size {
	case 1:
		out[0] = byte(container)
	case 2:
		binary.LittleEndian.PutUint16(out, uint16(container))
	case 4:
		binary.LittleEndian.PutUint32(out, uint32(container))
	case 8:
		binary.LittleEndian.PutUint64(out, container)
	}
	return out, nil
}

// readBitfield reads the container and extracts the field.
func (a *App) readBitfield(addr uint64, b bitSpec) (uint64, error) {
	if a.proc == nil {
		return 0, fmt.Errorf("%s", i18n.T("error.no_process"))
	}
	raw, err := a.proc.Read(addr, b.size)
	if err != nil {
		return 0, err
	}
	return b.extract(raw)
}

// writeBitfield performs a read-modify-write so neighbouring bits are kept.
func (a *App) writeBitfield(addr uint64, b bitSpec, value uint64) ([]byte, error) {
	if a.proc == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.no_process"))
	}
	raw, err := a.proc.Read(addr, b.size)
	if err != nil {
		return nil, err
	}
	updated, err := b.set(raw, value)
	if err != nil {
		return nil, err
	}
	if err := a.proc.Write(addr, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// defaultBitSpec builds a bitfield over a container of the given size.
func defaultBitSpec(size int) bitSpec {
	if size != 1 && size != 2 && size != 4 && size != 8 {
		size = 4
	}
	return bitSpec{size: size, width: size * 8}
}
