package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

// ErrNoInitialScan is returned by Next before a successful First.
var ErrNoInitialScan = errors.New("scan: no initial scan performed")

// scanChunk bounds the working set used while scanning a single region.
const scanChunk = 1 * 1024 * 1024

// Result is a single scan hit. Value is the value observed during the pass
// that produced the result (the baseline for the next comparison); Previous is
// the value the address held in the pass before that, empty on a first scan.
type Result struct {
	Addr     uint64
	Value    Value
	Previous Value
}

// Progress reports how far a scan has advanced.
type Progress struct {
	ScannedBytes uint64
	TotalBytes   uint64
	Matches      int
}

// maxHistory bounds the number of undoable scan steps.
const maxHistory = 16

// Session holds the state of a scan against one process.
type Session struct {
	proc    *mem.Process
	opts    Options
	regions []mem.Region
	results []Result
	history [][]Result
	started bool
}

// NewSession creates a scan session for proc.
func NewSession(proc *mem.Process, opts Options) *Session {
	if opts.Epsilon == 0 {
		opts.Epsilon = 1e-6
	}
	if opts.Scope != ScopeAllWritable && opts.Scope != ScopeHeapStackExecBSS && opts.Scope != ScopeAllReadable {
		opts.Scope = ScopeAllWritable
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

// Delete removes every result for which keep returns false and reports how many
// were removed. It is how the GUI drops results the user deleted.
func (s *Session) Delete(keep func(Result) bool) int {
	kept := s.results[:0]
	removed := 0
	for _, r := range s.results {
		if keep(r) {
			kept = append(kept, r)
			continue
		}
		removed++
	}
	for i := len(kept); i < len(s.results); i++ {
		s.results[i] = Result{}
	}
	s.results = kept
	return removed
}

// SetMode changes the scan mode used by subsequent Next calls.
func (s *Session) SetMode(m ScanMode) { s.opts.Mode = m }

// SetValue changes the target value used by subsequent scans.
func (s *Session) SetValue(v Value) { s.opts.Value = v }

// SetValue2 changes the upper bound used by between scans.
func (s *Session) SetValue2(v Value) { s.opts.Value2 = v }

// SetCompare changes the comparison operator used by exact scans.
func (s *Session) SetCompare(op CompareOp) { s.opts.Compare = op }

// First performs the initial scan. It honours ctx cancellation and reports
// progress through onProgress. On error or cancellation the previous results
// are left untouched.
func (s *Session) First(ctx context.Context, onProgress func(Progress)) error {
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
	results, err := s.scanRegions(ctx, regions, onProgress)
	if err != nil {
		return err
	}
	s.pushHistory()
	s.regions = regions
	s.results = results
	s.started = true
	return nil
}

// Next filters the current results. It honours ctx cancellation and reports
// progress; on error or cancellation the current results are left untouched.
func (s *Session) Next(ctx context.Context, onProgress func(Progress)) error {
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
	kept, err := s.filterResults(ctx, onProgress)
	if err != nil {
		return err
	}
	s.pushHistory()
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
	exe := s.proc.Exe
	var out []mem.Region
	for _, r := range src {
		if !r.Readable() || r.Size() == 0 {
			continue
		}
		if !s.regionInScope(r, exe) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Session) regionInScope(r mem.Region, exe string) bool {
	switch s.opts.Scope {
	case ScopeAllReadable:
		return true
	case ScopeHeapStackExecBSS:
		if r.Path == "[heap]" || r.Path == "[stack]" {
			return true
		}
		if r.Path == "" {
			return true
		}
		return exe != "" && r.Path == exe
	default:
		return r.Writable()
	}
}

// scanRegions scans the regions in parallel, merging the per-worker results.
func (s *Session) scanRegions(ctx context.Context, regions []mem.Region, onProgress func(Progress)) ([]Result, error) {
	if len(regions) == 0 {
		return nil, nil
	}
	var total uint64
	for _, r := range regions {
		total += r.Size()
	}
	var scanned, matches int64
	stop := s.startReporter(ctx, onProgress, total, &scanned, &matches)
	defer stop()

	workers := runtime.NumCPU()
	if workers > len(regions) {
		workers = len(regions)
	}
	if workers < 1 {
		workers = 1
	}
	chunks := make([][]Result, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			var local []Result
			for i := w; i < len(regions); i += workers {
				if ctx.Err() != nil {
					errs[w] = ctx.Err()
					return
				}
				if err := s.scanRegion(ctx, regions[i], &local, &scanned, &matches); err != nil {
					errs[w] = err
					return
				}
			}
			chunks[w] = local
		}(w)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	var out []Result
	for _, c := range chunks {
		out = append(out, c...)
	}
	if max := s.opts.MaxResults; max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// filterResults applies the next-scan predicate to the current results in
// parallel, preserving their order.
func (s *Session) filterResults(ctx context.Context, onProgress func(Progress)) ([]Result, error) {
	n := len(s.results)
	if n == 0 {
		return nil, nil
	}
	var total uint64
	for _, r := range s.results {
		w := len(r.Value.Raw)
		if w == 0 {
			w = s.opts.width()
		}
		total += uint64(w)
	}
	var scanned, matches int64
	stop := s.startReporter(ctx, onProgress, total, &scanned, &matches)
	defer stop()

	workers := runtime.NumCPU()
	if workers > n {
		workers = n
	}
	if workers < 1 {
		workers = 1
	}
	step := (n + workers - 1) / workers
	outs := make([][]Result, 0, workers)
	errs := make([]error, 0, workers)
	var wg sync.WaitGroup
	for lo := 0; lo < n; lo += step {
		hi := min(lo+step, n)
		outs = append(outs, nil)
		errs = append(errs, nil)
		wg.Add(1)
		go func(idx, lo, hi int) {
			defer wg.Done()
			var local []Result
			for i := lo; i < hi; i++ {
				if ctx.Err() != nil {
					errs[idx] = ctx.Err()
					return
				}
				if s.capped(&matches) {
					return
				}
				res := s.results[i]
				w := len(res.Value.Raw)
				if w == 0 {
					w = s.opts.width()
				}
				raw, err := s.proc.Read(res.Addr, w)
				atomic.AddInt64(&scanned, int64(w))
				if err != nil || len(raw) != w {
					continue
				}
				cur := NewValue(s.opts.Type, raw)
				if s.keep(cur, res.Value) {
					local = append(local, Result{Addr: res.Addr, Value: cur, Previous: res.Value})
					atomic.AddInt64(&matches, 1)
				}
			}
			outs[idx] = local
		}(len(outs)-1, lo, hi)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	var out []Result
	for _, c := range outs {
		out = append(out, c...)
	}
	if max := s.opts.MaxResults; max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// startReporter periodically reports progress until the returned stop function
// is called.
func (s *Session) startReporter(ctx context.Context, onProgress func(Progress), total uint64, scanned, matches *int64) func() {
	if onProgress == nil {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				onProgress(Progress{
					ScannedBytes: uint64(atomic.LoadInt64(scanned)),
					TotalBytes:   total,
					Matches:      int(atomic.LoadInt64(matches)),
				})
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func (s *Session) scanRegion(ctx context.Context, r mem.Region, out *[]Result, scanned, matches *int64) error {
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.capped(matches) {
			return nil
		}
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
			atomic.AddInt64(scanned, int64(len(data)))
			if serr := s.scanBytes(ctx, r.Start+off, data, limit, out, matches); serr != nil {
				return serr
			}
		}
		if err != nil {
			// A fault ends the contiguous run. Advance past what was read
			// before falling back to page-sized reads, otherwise the prefix
			// would be read and scanned twice.
			if chunked {
				chunked = false
				if len(data) > 0 {
					off += uint64(len(data))
				} else {
					off += page
				}
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
func (s *Session) scanBytes(ctx context.Context, addr uint64, data []byte, limit int, out *[]Result, matches *int64) error {
	w := s.opts.width()
	step := s.opts.step()
	if limit > len(data) {
		limit = len(data)
	}
	for i := 0; i < limit && i+w <= len(data); i += step {
		if s.capped(matches) {
			return nil
		}
		if err := s.consider(addr+uint64(i), data[i:i+w], out, matches); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) consider(addr uint64, raw []byte, out *[]Result, matches *int64) error {
	switch s.opts.Mode {
	case ModeUnknown:
		s.appendResult(out, matches, Result{Addr: addr, Value: NewValue(s.opts.Type, raw)})
	case ModeExact:
		if s.opts.Type == TypeAll {
			for _, v := range s.matchAll(raw) {
				s.appendResult(out, matches, Result{Addr: addr, Value: v})
			}
		} else if s.matchExact(raw) {
			s.appendResult(out, matches, Result{Addr: addr, Value: NewValue(s.opts.Type, raw)})
		}
	case ModeBetween:
		if s.matchBetween(raw) {
			s.appendResult(out, matches, Result{Addr: addr, Value: NewValue(s.opts.Type, raw)})
		}
	}
	return nil
}

func (s *Session) appendResult(out *[]Result, matches *int64, r Result) {
	*out = append(*out, r)
	atomic.AddInt64(matches, 1)
}

// capped reports whether the result cap has been reached.
func (s *Session) capped(matches *int64) bool {
	return s.opts.MaxResults > 0 && atomic.LoadInt64(matches) >= int64(s.opts.MaxResults)
}

func (s *Session) matchExact(raw []byte) bool {
	t := TypeByID(s.opts.Type)
	if t == nil {
		return false
	}
	switch t.Kind {
	case KindString, KindBytes, KindBinary:
		if s.opts.Type == TypeAOB {
			p := &AOBPattern{Bytes: s.opts.Value.Raw, Mask: s.opts.Value.Mask}
			return p.Match(raw)
		}
		if s.opts.Type == TypeBinary {
			p := &BinaryPattern{Bytes: s.opts.Value.Raw, Mask: s.opts.Value.Mask, Bits: s.opts.Value.Bits}
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

// matchAll tests the integer widths at one address. It records a match for
// every width whose decoded value equals the target (float and double are not
// covered by this first cut).
func (s *Session) matchAll(raw []byte) []Value {
	target := s.opts.Value.Uint64()
	widths := []struct {
		t ValueType
		n int
	}{{TypeByte, 1}, {TypeWord, 2}, {TypeDword, 4}, {TypeQword, 8}}
	var out []Value
	for _, w := range widths {
		if len(raw) < w.n {
			continue
		}
		v := NewValue(w.t, raw[:w.n])
		if v.Uint64() == target {
			out = append(out, v)
		}
	}
	return out
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
