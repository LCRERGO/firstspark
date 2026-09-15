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

// maxHistory bounds the number of undoable scan steps.
const maxHistory = 16

// Session holds the state of a scan against one process.
type Session struct {
	proc          *mem.Process
	opts          Options
	regions       []mem.Region
	results       []Result
	history       [][]Result
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

// SetValue2 changes the upper bound used by between scans.
func (s *Session) SetValue2(v Value) { s.opts.Value2 = v }

// SetCompare changes the comparison operator used by exact scans.
func (s *Session) SetCompare(op CompareOp) { s.opts.Compare = op }

// First performs the initial scan, discarding any previous results.
func (s *Session) First() error {
	if s.opts.Mode != ModeExact && s.opts.Mode != ModeUnknown && s.opts.Mode != ModeBetween {
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
	s.pushHistory()
	s.regions = regions
	s.results = nil
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

	s.pushHistory()
	kept := make([]Result, 0, len(s.results))
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
	case ModeBetween:
		if s.matchBetween(raw) {
			s.results = append(s.results, Result{Addr: addr, Prev: NewValue(s.opts.Type, raw)})
		}
	}
	return nil
}

func (s *Session) matchExact(raw []byte) bool {
	t := TypeByID(s.opts.Type)
	if t == nil {
		return false
	}
	switch t.Kind {
	case KindString, KindBytes:
		if s.opts.Type == TypeAOB {
			p := &AOBPattern{Bytes: s.opts.Value.Raw, Mask: s.opts.Value.Mask}
			return p.Match(raw)
		}
		return bytes.Equal(raw, s.opts.Value.Raw)
	default:
		return t.Compare(NewValue(s.opts.Type, raw), s.opts.Value, s.opts.Compare, s.opts.Epsilon)
	}
}

func (s *Session) keep(cur, prev Value) bool {
	t := TypeByID(s.opts.Type)
	switch s.opts.Mode {
	case ModeExact:
		return s.matchExact(cur.Raw)
	case ModeChanged:
		return !s.valueEqual(t, cur, prev)
	case ModeUnchanged:
		return s.valueEqual(t, cur, prev)
	case ModeIncreased:
		return s.valueGreater(t, cur, prev)
	case ModeDecreased:
		return s.valueLess(t, cur, prev)
	case ModeIncreasedBy:
		return s.valueDelta(t, cur, prev, s.opts.Value, true)
	case ModeDecreasedBy:
		return s.valueDelta(t, cur, prev, s.opts.Value, false)
	case ModeBetween:
		return s.matchBetween(cur.Raw)
	default:
		return false
	}
}

// matchBetween reports whether raw falls inside the configured range.
func (s *Session) matchBetween(raw []byte) bool {
	t := TypeByID(s.opts.Type)
	if t == nil {
		return false
	}
	switch t.Kind {
	case KindString, KindBytes, KindBinary:
		return false
	}
	cur := NewValue(s.opts.Type, raw)
	if t.Kind == KindFloat {
		lo, hi := t.Numeric(s.opts.Value), t.Numeric(s.opts.Value2)
		if lo > hi {
			lo, hi = hi, lo
		}
		x := t.Numeric(cur)
		return x >= lo-s.opts.Epsilon && x <= hi+s.opts.Epsilon
	}
	lo, hi := t.Int64(s.opts.Value), t.Int64(s.opts.Value2)
	if lo > hi {
		lo, hi = hi, lo
	}
	x := t.Int64(cur)
	return x >= lo && x <= hi
}

// pushHistory snapshots the current results so the last step can be undone.
func (s *Session) pushHistory() {
	snapshot := make([]Result, len(s.results))
	copy(snapshot, s.results)
	s.history = append(s.history, snapshot)
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
}

// Undo restores the result set from before the last scan step.
func (s *Session) Undo() bool {
	if len(s.history) == 0 {
		return false
	}
	last := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	s.results = last
	return true
}

// CanUndo reports whether an undo step is available.
func (s *Session) CanUndo() bool { return len(s.history) > 0 }

func (s *Session) valueEqual(t *Type, a, b Value) bool {
	switch t.Kind {
	case KindString, KindBytes, KindBinary:
		return bytes.Equal(a.Raw, b.Raw)
	case KindFloat:
		return math.Abs(t.Numeric(a)-t.Numeric(b)) <= s.opts.Epsilon
	default:
		return t.Int64(a) == t.Int64(b)
	}
}

func (s *Session) valueGreater(t *Type, a, b Value) bool {
	if t.Kind == KindFloat {
		return t.Numeric(a) > t.Numeric(b)+s.opts.Epsilon
	}
	return t.Int64(a) > t.Int64(b)
}

func (s *Session) valueLess(t *Type, a, b Value) bool {
	if t.Kind == KindFloat {
		return t.Numeric(a) < t.Numeric(b)-s.opts.Epsilon
	}
	return t.Int64(a) < t.Int64(b)
}

func (s *Session) valueDelta(t *Type, cur, prev, delta Value, increased bool) bool {
	return valueDelta(t, cur, prev, delta, increased, s.opts.Epsilon)
}

func valueDelta(t *Type, cur, prev, delta Value, increased bool, eps float64) bool {
	if t.Kind == KindFloat {
		d := t.Numeric(cur) - t.Numeric(prev)
		if !increased {
			d = -d
		}
		return math.Abs(d-t.Numeric(delta)) <= eps
	}
	d := t.Int64(cur) - t.Int64(prev)
	if !increased {
		d = -d
	}
	return d == t.Int64(delta)
}
