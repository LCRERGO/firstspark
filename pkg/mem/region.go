package mem

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Region is a single entry from /proc/<pid>/maps.
type Region struct {
	Start  uint64
	End    uint64
	Perms  string
	Offset uint64
	Dev    string
	Inode  uint64
	Path   string
}

// Readable reports whether the region has the read permission.
func (r Region) Readable() bool { return strings.Contains(r.Perms, "r") }

// Writable reports whether the region has the write permission.
func (r Region) Writable() bool { return strings.Contains(r.Perms, "w") }

// Executable reports whether the region has the execute permission.
func (r Region) Executable() bool { return strings.Contains(r.Perms, "x") }

// Private reports whether the region is private (copy-on-write).
func (r Region) Private() bool { return strings.Contains(r.Perms, "p") }

// Shared reports whether the region is shared.
func (r Region) Shared() bool { return strings.Contains(r.Perms, "s") }

// FileBacked reports whether the region is backed by a file on disk.
func (r Region) FileBacked() bool { return r.Path != "" && !strings.HasPrefix(r.Path, "[") }

// Size returns the length of the region in bytes.
func (r Region) Size() uint64 {
	if r.End <= r.Start {
		return 0
	}
	return r.End - r.Start
}

// Contains reports whether addr falls inside the region.
func (r Region) Contains(addr uint64) bool { return addr >= r.Start && addr < r.End }

// String renders the region in /proc/maps form.
func (r Region) String() string {
	return fmt.Sprintf("%x-%x %s %08x %s %d %s", r.Start, r.End, r.Perms, r.Offset, r.Dev, r.Inode, r.Path)
}

// Regions returns the memory map of pid.
func Regions(pid int) ([]Region, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, fmt.Errorf("mem: open maps for pid %d: %w", pid, err)
	}
	defer f.Close()

	var out []Region
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		r, err := ParseRegion(sc.Text())
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("mem: read maps for pid %d: %w", pid, err)
	}
	return out, nil
}

// ParseRegion parses one line of /proc/<pid>/maps.
func ParseRegion(line string) (Region, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return Region{}, fmt.Errorf("mem: malformed map line %q", line)
	}
	start, end, err := parseRange(fields[0])
	if err != nil {
		return Region{}, err
	}
	offset, err := strconv.ParseUint(fields[2], 16, 64)
	if err != nil {
		return Region{}, fmt.Errorf("mem: bad offset %q: %w", fields[2], err)
	}
	inode, err := strconv.ParseUint(fields[4], 10, 64)
	if err != nil {
		return Region{}, fmt.Errorf("mem: bad inode %q: %w", fields[4], err)
	}
	r := Region{
		Start:  start,
		End:    end,
		Perms:  fields[1],
		Offset: offset,
		Dev:    fields[3],
		Inode:  inode,
	}
	if len(fields) > 5 {
		r.Path = strings.Join(fields[5:], " ")
	}
	return r, nil
}

func parseRange(s string) (uint64, uint64, error) {
	lo, hi, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("mem: bad range %q", s)
	}
	start, err := strconv.ParseUint(lo, 16, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("mem: bad range start %q: %w", lo, err)
	}
	end, err := strconv.ParseUint(hi, 16, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("mem: bad range end %q: %w", hi, err)
	}
	return start, end, nil
}

// ModuleBase returns the load base of the named module, matched by base name
// (e.g. "libc.so.6"). It uses the lowest file-backed region mapped at offset 0.
func ModuleBase(regions []Region, name string) (uint64, bool) {
	var base uint64
	found := false
	for _, r := range regions {
		if r.Offset != 0 {
			continue
		}
		if filepath.Base(strings.TrimSuffix(r.Path, " (deleted)")) != name {
			continue
		}
		if !found || r.Start < base {
			base = r.Start
			found = true
		}
	}
	return base, found
}

// RegionFor returns the region containing addr, if any.
func RegionFor(regions []Region, addr uint64) (Region, bool) {
	for _, r := range regions {
		if r.Contains(addr) {
			return r, true
		}
	}
	return Region{}, false
}
