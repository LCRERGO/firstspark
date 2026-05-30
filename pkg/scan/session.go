package scan

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

// ErrNoInitialScan is returned by Next before a successful First.
var ErrNoInitialScan = errors.New("scan: no initial scan performed")

// scanChunk bounds the working set used while scanning a single region.
const scanChunk = 1 * 1024 * 1024

// Result is a single scan hit together with the value observed during the
// previous pass.
type Result struct {
	Addr uint64
	Prev Value
}

// Session holds the state of a scan against one process.
type Session struct {
	proc          *mem.Process
	opts          Options
	regions       []mem.Region
	results       []Result
	started       bool
	snapshotBytes int64
}

// NewSession creates a scan session for proc.
func NewSession(proc *mem.Process, opts Options) *Session {
	if opts.SnapshotLimit <= 0 {
		opts.SnapshotLimit = 2 << 30
	}
	if opts.Epsilon == 0 {
		opts.Epsilon = 1e-6
	}
	return &Session{proc: proc, opts: opts}
}

// Options returns the session options.
func (s *Session) Options() Options { return s.opts }

// Regions returns the regions selected by the last scan.
func (s *Session) Regions() []mem.Region { return s.regions }

// Results returns the current result set.
func (s *Session) Results() []Result { return s.results }

// Count returns the number of results.
func (s *Session) Count() int { return len(s.results) }

// Started reports whether an initial scan has completed.
func (s *Session) Started() bool { return s.started }

// SetMode changes the scan mode used by subsequent Next calls.
func (s *Session) SetMode(m ScanMode) { s.opts.Mode = m }

// SetValue changes the target value used by subsequent scans.
func (s *Session) SetValue(v Value) { s.opts.Value = v }

// SetCompare changes the comparison operator used by exact scans.
func (s *Session) SetCompare(op CompareOp) { s.opts.Compare = op }

// First performs the initial scan, discarding any previous results.
func (s *Session) First() error {
	if s.opts.Mode != ModeExact && s.opts.Mode != ModeUnknown {
		return fmt.Errorf("scan: %s is not a valid initial scan mode", s.opts.Mode)
	}
	if s.opts.Type.Variable() && s.opts.Mode != ModeExact {
		return fmt.Errorf("scan: %s scans require a fixed-width type", s.opts.Mode)
	}
	if s.opts.width() <= 0 {
		return fmt.Errorf("scan: exact scan requires a non-empty value")
	}

	regions, err := s.selectRegions()
	if err != nil {
		return err
	}
	s.regions = regions
	s.results = s.results[:0]
	s.snapshotBytes = 0

	for _, r := range regions {
		if err := s.scanRegion(r); err != nil {
			return err
		}
	}
	s.started = true
	return nil
}

// Next filters the current results according to the session mode.
func (s *Session) Next() error {
	if !s.started {
		return ErrNoInitialScan
	}
	if s.opts.Type.Variable() {
		switch s.opts.Mode {
		case ModeExact, ModeChanged, ModeUnchanged:
		default:
			return fmt.Errorf("scan: %s scans require a fixed-width type", s.opts.Mode)
		}
	}

	kept := s.results[:0]
	for _, res := range s.results {
		w := len(res.Prev.Raw)
		if w == 0 {
			w = s.opts.width()
		}
		raw, err := s.proc.Read(res.Addr, w)
		if err != nil || len(raw) != w {
			continue
		}
		cur := NewValue(s.opts.Type, raw)
		if s.keep(cur, res.Prev) {
			kept = append(kept, Result{Addr: res.Addr, Prev: cur})
		}
	}
	s.results = kept
	return nil
}

// Read reads the value at addr using the session's value type.
func (s *Session) Read(addr uint64) (Value, error) {
	w := s.opts.width()
	if w <= 0 {
		return Value{}, fmt.Errorf("scan: no width for type %s", s.opts.Type)
	}
	raw, err := s.proc.Read(addr, w)
	if err != nil {
		return Value{}, err
	}
	return NewValue(s.opts.Type, raw), nil
}

// Write writes v to addr.
func (s *Session) Write(addr uint64, v Value) error {
	return s.proc.Write(addr, v.Raw)
}

// WriteBytes writes raw bytes to addr.
func (s *Session) WriteBytes(addr uint64, raw []byte) error {
	return s.proc.Write(addr, raw)
}

func (s *Session) selectRegions() ([]mem.Region, error) {
	src := s.opts.Regions
	if len(src) == 0 {
		r, err := mem.Regions(s.proc.PID)
		if err != nil {
			return nil, err
		}
		src = r
	}
	var out []mem.Region
	for _, r := range src {
		if !r.Readable() || r.Size() == 0 {
			continue
		}
		if s.opts.WritableOnly && !r.Writable() {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Session) scanRegion(r mem.Region) error {
	w := s.opts.width()
	if w <= 0 {
		return nil
	}
	overlap := w - 1
	if overlap < 0 {
		overlap = 0
	}
	size := r.Size()
	const page = uint64(4096)

	// Read large chunks while the region is contiguous; fall back to
	// page-sized reads after the first fault so holes do not hide the pages
	// that follow them.
	off := uint64(0)
	chunked := true
	for off < size {
		want, stride := scanChunk, uint64(scanChunk)
		if !chunked {
			want, stride = int(page), page
		}
		want += overlap
		if uint64(want) > size-off {
			want = int(size - off)
		}
		limit := min(want, int(stride))

		data, err := s.proc.Read(r.Start+off, want)
		if len(data) > 0 {
			if err := s.scanBytes(r.Start+off, data, limit); err != nil {
				return err
			}
		}
		if err != nil {
			if chunked {
				// Retry the same offset page by page; the chunk may have
				// failed only because it crossed a hole.
				chunked = false
				continue
			}
			off += page
			continue
		}
		off += stride
	}
	return nil
}

// scanBytes considers every candidate in data whose start is below limit.
func (s *Session) scanBytes(addr uint64, data []byte, limit int) error {
	w := s.opts.width()
	step := s.opts.step()
	if limit > len(data) {
		limit = len(data)
	}
	for i := 0; i < limit && i+w <= len(data); i += step {
		if err := s.consider(addr+uint64(i), data[i:i+w]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) consider(addr uint64, raw []byte) error {
	switch s.opts.Mode {
	case ModeUnknown:
		s.results = append(s.results, Result{Addr: addr, Prev: NewValue(s.opts.Type, raw)})
		s.snapshotBytes += int64(len(raw))
		if s.snapshotBytes > s.opts.SnapshotLimit {
			return fmt.Errorf("scan: snapshot exceeded limit of %d bytes", s.opts.SnapshotLimit)
		}
	case ModeExact:
		if s.matchExact(raw) {
			s.results = append(s.results, Result{Addr: addr, Prev: NewValue(s.opts.Type, raw)})
		}
	}
	return nil
}

func (s *Session) matchExact(raw []byte) bool {
	switch s.opts.Type {
	case TypeString:
		return bytes.Equal(raw, s.opts.Value.Raw)
	case TypeAOB:
		p := &AOBPattern{Bytes: s.opts.Value.Raw, Mask: s.opts.Value.Mask}
		return p.Match(raw)
	default:
		return compareExact(NewValue(s.opts.Type, raw), s.opts.Value, s.opts.Compare, s.opts.Epsilon)
	}
}

func (s *Session) keep(cur, prev Value) bool {
	switch s.opts.Mode {
	case ModeExact:
		return s.matchExact(cur.Raw)
	case ModeChanged:
		return !valueEqual(cur, prev, s.opts.Epsilon)
	case ModeUnchanged:
		return valueEqual(cur, prev, s.opts.Epsilon)
	case ModeIncreased:
		return valueGreater(cur, prev, s.opts.Epsilon)
	case ModeDecreased:
		return valueLess(cur, prev, s.opts.Epsilon)
	case ModeIncreasedBy:
		return valueDelta(cur, prev, s.opts.Value, true, s.opts.Epsilon)
	case ModeDecreasedBy:
		return valueDelta(cur, prev, s.opts.Value, false, s.opts.Epsilon)
	default:
		return false
	}
}

func compareExact(cur, target Value, op CompareOp, eps float64) bool {
	if cur.Type == TypeFloat || cur.Type == TypeDouble {
		return compareFloat(cur.Float64(), target.Float64(), op, eps)
	}
	return compareInt(cur.Int64(), target.Int64(), op)
}

func compareInt(a, b int64, op CompareOp) bool {
	switch op {
	case OpEqual:
		return a == b
	case OpNotEqual:
		return a != b
	case OpGreater:
		return a > b
	case OpGreaterEqual:
		return a >= b
	case OpLess:
		return a < b
	case OpLessEqual:
		return a <= b
	default:
		return false
	}
}

func compareFloat(a, b float64, op CompareOp, eps float64) bool {
	switch op {
	case OpEqual:
		return math.Abs(a-b) <= eps
	case OpNotEqual:
		return math.Abs(a-b) > eps
	case OpGreater:
		return a > b+eps
	case OpGreaterEqual:
		return a >= b-eps
	case OpLess:
		return a < b-eps
	case OpLessEqual:
		return a <= b+eps
	default:
		return false
	}
}

func valueEqual(a, b Value, eps float64) bool {
	switch {
	case a.Type == TypeString || a.Type == TypeAOB:
		return bytes.Equal(a.Raw, b.Raw)
	case a.Type == TypeFloat || a.Type == TypeDouble:
		return math.Abs(a.Float64()-b.Float64()) <= eps
	default:
		return a.Int64() == b.Int64()
	}
}

func valueGreater(a, b Value, eps float64) bool {
	if a.Type == TypeFloat || a.Type == TypeDouble {
		return a.Float64() > b.Float64()+eps
	}
	return a.Int64() > b.Int64()
}

func valueLess(a, b Value, eps float64) bool {
	if a.Type == TypeFloat || a.Type == TypeDouble {
		return a.Float64() < b.Float64()-eps
	}
	return a.Int64() < b.Int64()
}

func valueDelta(cur, prev, delta Value, increased bool, eps float64) bool {
	if cur.Type == TypeFloat || cur.Type == TypeDouble {
		d := cur.Float64() - prev.Float64()
		if !increased {
			d = -d
		}
		return math.Abs(d-delta.Float64()) <= eps
	}
	d := cur.Int64() - prev.Int64()
	if !increased {
		d = -d
	}
	return d == delta.Int64()
}
