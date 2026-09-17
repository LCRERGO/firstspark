// Package pointerscan implements an N-level pointer scan over a pointermap
// (ADR 0015). A pointermap is a reverse index from pointer values to the
// addresses that contain them; addresses inside module-backed regions are
// tagged as static so a chain can start at a module base plus an offset.
package pointerscan

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/mem"
)

// StaticRegion marks a module-backed address range.
type StaticRegion struct {
	Start, End uint64
	Module     string
	Base       uint64
}

// Pointermap is a sorted reverse index of pointer values to their addresses.
type Pointermap struct {
	entries []entry
	statics []StaticRegion
}

type entry struct {
	value uint64
	addrs []uint64
}

// BuildOptions configures pointermap construction.
type BuildOptions struct {
	WritableOnly bool
	Aligned      bool
	MaxBytes     uint64
	Chunk        int
}

// BuildPointermap scans the target's memory for pointer-sized values that
// point inside its own address space.
func BuildPointermap(p *mem.Process, o BuildOptions) (*Pointermap, error) {
	regions, err := mem.Regions(p.PID)
	if err != nil {
		return nil, err
	}
	if o.Chunk <= 0 {
		o.Chunk = 1 << 20
	}
	pm := &Pointermap{}
	index := map[uint64][]uint64{}
	var minAddr, maxAddr uint64
	for _, r := range regions {
		if r.Size() == 0 {
			continue
		}
		if minAddr == 0 || r.Start < minAddr {
			minAddr = r.Start
		}
		if r.End > maxAddr {
			maxAddr = r.End
		}
	}
	var scanned uint64
	step := 1
	if o.Aligned {
		step = 8
	}
	for _, r := range regions {
		if !r.Readable() || r.Size() == 0 {
			continue
		}
		if o.WritableOnly && !r.Writable() {
			continue
		}
		if r.FileBacked() {
			pm.statics = append(pm.statics, StaticRegion{
				Start: r.Start, End: r.End,
				Module: filepath.Base(r.Path),
				Base:   r.Start - r.Offset,
			})
		}
		for off := uint64(0); off < r.Size(); off += uint64(o.Chunk) {
			if o.MaxBytes > 0 && scanned >= o.MaxBytes {
				break
			}
			want := uint64(o.Chunk)
			if want > r.Size()-off {
				want = r.Size() - off
			}
			data, rerr := p.Read(r.Start+off, int(want))
			if len(data) > 0 {
				scanned += uint64(len(data))
				for i := 0; i+8 <= len(data); i += step {
					v := binary.LittleEndian.Uint64(data[i:])
					if v == 0 || v < minAddr || v > maxAddr {
						continue
					}
					addr := r.Start + off + uint64(i)
					index[v] = append(index[v], addr)
				}
			}
			if rerr != nil {
				break
			}
		}
	}
	pm.entries = make([]entry, 0, len(index))
	for v, addrs := range index {
		pm.entries = append(pm.entries, entry{value: v, addrs: addrs})
	}
	sort.Slice(pm.entries, func(i, j int) bool { return pm.entries[i].value < pm.entries[j].value })
	return pm, nil
}

// wireEntry is the JSON form of an index entry.
type wireEntry struct {
	Value uint64   `json:"value"`
	Addrs []uint64 `json:"addrs"`
}

type pointermapJSON struct {
	Entries []wireEntry    `json:"entries"`
	Statics []StaticRegion `json:"statics"`
}

// BuildOrLoad returns a pointermap, reusing an on-disk cache keyed by the
// target's region map and the build options. A stale cache (different region
// map) is ignored.
func BuildOrLoad(p *mem.Process, o BuildOptions) (*Pointermap, error) {
	regions, err := mem.Regions(p.PID)
	if err != nil {
		return nil, err
	}
	path := CachePath(p.PID, regions, o)
	if pm, err := loadPointermap(path); err == nil {
		return pm, nil
	}
	pm, err := BuildPointermap(p, o)
	if err != nil {
		return nil, err
	}
	_ = savePointermap(path, pm)
	return pm, nil
}

// CachePath returns the on-disk cache path for a pointermap built from regions
// with the given options.
func CachePath(pid int, regions []mem.Region, o BuildOptions) string {
	h := sha256.New()
	for _, r := range regions {
		fmt.Fprintf(h, "%x-%x-%s\n", r.Start, r.End, r.Path)
	}
	fmt.Fprintf(h, "writable=%t aligned=%t", o.WritableOnly, o.Aligned)
	sum := hex.EncodeToString(h.Sum(nil))[:16]
	return filepath.Join(config.CacheDir(), fmt.Sprintf("pointermap-%d-%s.json", pid, sum))
}

func savePointermap(path string, pm *Pointermap) error {
	entries := make([]wireEntry, len(pm.entries))
	for i, e := range pm.entries {
		entries[i] = wireEntry{Value: e.value, Addrs: e.addrs}
	}
	data, err := json.Marshal(pointermapJSON{Entries: entries, Statics: pm.statics})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func loadPointermap(path string) (*Pointermap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc pointermapJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	pm := &Pointermap{statics: doc.Statics}
	pm.entries = make([]entry, len(doc.Entries))
	for i, e := range doc.Entries {
		pm.entries[i] = entry{value: e.Value, addrs: e.Addrs}
	}
	return pm, nil
}

// Options configures a pointer scan.
type Options struct {
	MaxLevel   int
	MaxOffset  uint64
	Aligned    bool
	StaticOnly bool
	NoLoop     bool
	MaxResults int
}

// DefaultOptions returns Cheat Engine's pointer scan defaults.
func DefaultOptions() Options {
	return Options{MaxLevel: 5, MaxOffset: 2048, Aligned: true, NoLoop: true, MaxResults: 100000}
}

// Chain is one pointer path. When Module is set, Base is the module base and
// the offsets are relative to it; otherwise Base is an absolute address.
type Chain struct {
	Module  string   `json:"module,omitempty"`
	Base    uint64   `json:"base"`
	Offsets []uint64 `json:"offsets"`
}

// Scan finds pointer chains that resolve to target.
func (pm *Pointermap) Scan(target uint64, o Options) []Chain {
	if o.MaxLevel <= 0 {
		o.MaxLevel = 5
	}
	if o.MaxOffset == 0 {
		o.MaxOffset = 2048
	}
	if o.MaxResults <= 0 {
		o.MaxResults = 100000
	}
	s := &scanner{pm: pm, o: o, visited: map[uint64]bool{}}
	s.rscan(target, nil, 0)
	return s.out
}

type scanner struct {
	pm      *Pointermap
	o       Options
	visited map[uint64]bool
	out     []Chain
}

func (s *scanner) rscan(target uint64, offsets []uint64, level int) {
	if level >= s.o.MaxLevel || len(s.out) >= s.o.MaxResults {
		return
	}
	low := uint64(0)
	if target > s.o.MaxOffset {
		low = target - s.o.MaxOffset
	}
	lo := sort.Search(len(s.pm.entries), func(i int) bool { return s.pm.entries[i].value >= low })
	for i := lo; i < len(s.pm.entries); i++ {
		e := s.pm.entries[i]
		if e.value > target {
			break
		}
		off := target - e.value
		next := make([]uint64, 0, len(offsets)+1)
		next = append(next, off)
		next = append(next, offsets...)
		for _, addr := range e.addrs {
			if s.o.Aligned && addr%8 != 0 {
				continue
			}
			if len(s.out) >= s.o.MaxResults {
				return
			}
			if st, ok := s.pm.staticFor(addr); ok {
				chain := Chain{Module: st.Module, Base: st.Base, Offsets: append([]uint64{addr - st.Base}, next...)}
				s.out = append(s.out, chain)
				continue
			}
			if s.o.StaticOnly {
				continue
			}
			if level+1 >= s.o.MaxLevel {
				s.out = append(s.out, Chain{Base: addr, Offsets: next})
				continue
			}
			if s.o.NoLoop && s.visited[addr] {
				continue
			}
			s.visited[addr] = true
			s.rscan(addr, next, level+1)
			delete(s.visited, addr)
		}
	}
}

func (pm *Pointermap) staticFor(addr uint64) (StaticRegion, bool) {
	for _, st := range pm.statics {
		if addr >= st.Start && addr < st.End {
			return st, true
		}
	}
	return StaticRegion{}, false
}

// Resolve follows a chain in the target process.
func Resolve(p *mem.Process, c Chain) (uint64, error) {
	addr := c.Base
	for i, off := range c.Offsets {
		addr += off
		if i == len(c.Offsets)-1 {
			break
		}
		next, err := p.ReadUint64(addr)
		if err != nil {
			return 0, err
		}
		addr = next
	}
	return addr, nil
}

// Save writes chains as a .ptr JSON file.
func Save(path string, chains []Chain) error {
	data, err := json.MarshalIndent(chains, "", "  ")
	if err != nil {
		return fmt.Errorf("pointerscan: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("pointerscan: write %s: %w", path, err)
	}
	return nil
}

// Load reads chains from a .ptr JSON file.
func Load(path string) ([]Chain, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pointerscan: read %s: %w", path, err)
	}
	var chains []Chain
	if err := json.Unmarshal(data, &chains); err != nil {
		return nil, fmt.Errorf("pointerscan: parse %s: %w", path, err)
	}
	return chains, nil
}
