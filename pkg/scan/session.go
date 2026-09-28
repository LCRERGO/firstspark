package scan

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sort"
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
// the value the address held in the pass before that, and First is the value
// captured by the initial scan. Previous is empty on a first scan; First is
// only empty for results that predate the first scan.
type Result struct {
	Addr     uint64
	Value    Value
	Previous Value
	First    Value
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

	// Derived scan state, refreshed when the type or target changes, so the
	// hot per-candidate path avoids the type-registry lock and allocations.
	typ    *Type
	width  int
	aob    *AOBPattern
	binary *BinaryPattern
}

// NewSession creates a scan session for proc.
func NewSession(proc *mem.Process, opts Options) *Session {
	if opts.Epsilon == 0 {
		opts.Epsilon = 1e-6
	}
	if opts.Scope != ScopeAllWritable && opts.Scope != ScopeHeapStackExecBSS && opts.Scope != ScopeAllReadable {
		opts.Scope = ScopeAllWritable
	}
	s := &Session{proc: proc, opts: opts}
	s.refresh()
	return s
}

// refresh recomputes the derived scan state from the current options.
func (s *Session) refresh() {
	s.typ = TypeByID(s.opts.Type)
	s.width = s.opts.width()
	s.aob, s.binary = nil, nil
	switch s.opts.Type {
	case TypeAOB:
		s.aob = &AOBPattern{Bytes: s.opts.Value.Raw, Mask: s.opts.Value.Mask}
	case TypeBinary:
		s.binary = &BinaryPattern{Bytes: s.opts.Value.Raw, Mask: s.opts.Value.Mask, Bits: s.opts.Value.Bits}
	}
}

// typeOf returns the resolved value type, falling back to a registry lookup for
// sessions built without NewSession (tests).
func (s *Session) typeOf() *Type {
	if s.typ != nil {
		return s.typ
	}
	return TypeByID(s.opts.Type)
}

// NewSessionFromResults builds a started session seeded with results, so a
// combined or compared result set can be refined by further next scans.
func NewSessionFromResults(proc *mem.Process, opts Options, results []Result) *Session {
	s := NewSession(proc, opts)
	s.results = append([]Result(nil), results...)
	s.started = true
	return s
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
func (s *Session) SetValue(v Value) {
	s.opts.Value = v
	s.refresh()
}

// SetValue2 changes the upper bound used by between scans.
func (s *Session) SetValue2(v Value) { s.opts.Value2 = v }

// SetCompare changes the comparison operator used by exact scans.
func (s *Session) SetCompare(op CompareOp) { s.opts.Compare = op }

// First performs the initial scan. It honours ctx cancellation and reports
// progress through onProgress. On error or cancellation the previous results
// are left untouched.
func (s *Session) First(ctx context.Context, onProgress func(Progress)) error {
	if s.opts.Mode != ModeExact && s.opts.Mode != ModeUnknown && s.opts.Mode != ModeBetween &&
		s.opts.Mode != ModeBigger && s.opts.Mode != ModeSmaller {
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
		if s.opts.Start != 0 || s.opts.Stop != 0 {
			if s.opts.Start > r.Start {
				r.Start = s.opts.Start
			}
			if s.opts.Stop != 0 && s.opts.Stop < r.End {
				r.End = s.opts.Stop
			}
			if r.End <= r.Start {
				continue
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Session) regionInScope(r mem.Region, exe string) bool {
	switch s.opts.Executable {
	case ExecOnly:
		if !r.Executable() {
			return false
		}
	case ExecNonExec:
		if r.Executable() {
			return false
		}
	}
	if s.opts.CopyOnWrite && !r.Private() {
		return false
	}
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
	work := splitRegions(regions)
	var total uint64
	for _, r := range work {
		total += r.Size()
	}
	var scanned, matches int64
	stop := s.startReporter(ctx, onProgress, total, &scanned, &matches)
	defer stop()

	workers := runtime.NumCPU()
	if workers > len(work) {
		workers = len(work)
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
			for i := w; i < len(work); i += workers {
				if ctx.Err() != nil {
					errs[w] = ctx.Err()
					return
				}
				if err := s.scanRegion(ctx, work[i], &local, &scanned, &matches); err != nil {
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

// filterChunk bounds a batched read while filtering a next scan.
const filterChunk = 64 * 1024

// filterResults applies the next-scan predicate to the current results in
// parallel, preserving their order. Results are visited in address order and
// read in batches so a next scan issues one read per window instead of one per
// address.
func (s *Session) filterResults(ctx context.Context, onProgress func(Progress)) ([]Result, error) {
	n := len(s.results)
	if n == 0 {
		return nil, nil
	}
	width := func(r Result) int {
		if w := len(r.Value.Raw); w > 0 {
			return w
		}
		if s.width > 0 {
			return s.width
		}
		return s.opts.width()
	}
	var total uint64
	for _, r := range s.results {
		total += uint64(width(r))
	}
	var scanned, matches int64
	stop := s.startReporter(ctx, onProgress, total, &scanned, &matches)
	defer stop()

	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return s.results[order[a]].Addr < s.results[order[b]].Addr })

	keep := make([]bool, n)
	workers := runtime.NumCPU()
	if workers > n {
		workers = n
	}
	if workers < 1 {
		workers = 1
	}
	step := (n + workers - 1) / workers
	errs := make([]error, 0, workers)
	var wg sync.WaitGroup
	for lo := 0; lo < n; lo += step {
		hi := min(lo+step, n)
		errs = append(errs, nil)
		wg.Add(1)
		go func(idx, lo, hi int) {
			defer wg.Done()
			if err := s.filterWindow(ctx, order[lo:hi], width, keep, &scanned, &matches); err != nil {
				errs[idx] = err
			}
		}(len(errs)-1, lo, hi)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	var out []Result
	for i := range s.results {
		if keep[i] {
			out = append(out, s.results[i])
		}
	}
	if max := s.opts.MaxResults; max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// filterWindow re-reads and filters a run of result indices sorted by address,
// reading a shared window per contiguous batch.
func (s *Session) filterWindow(ctx context.Context, idx []int, width func(Result) int, keep []bool, scanned, matches *int64) error {
	max := int64(s.opts.MaxResults)
	var buf []byte
	var bufStart uint64
	for _, i := range idx {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if max > 0 && atomic.LoadInt64(matches) >= max {
			return nil
		}
		res := s.results[i]
		w := width(res)
		if buf == nil || res.Addr+uint64(w) > bufStart+uint64(len(buf)) {
			data, _ := s.proc.Read(res.Addr, filterChunk)
			atomic.AddInt64(scanned, int64(len(data)))
			if len(data) == 0 {
				buf = nil
				continue
			}
			buf, bufStart = data, res.Addr
		}
		off := res.Addr - bufStart
		if off+uint64(w) > uint64(len(buf)) {
			continue
		}
		cur := Value{Type: s.opts.Type, Raw: buf[off : off+uint64(w)]}
		if s.keep(cur, res) {
			s.results[i].Previous = res.Value
			s.results[i].Value = NewValue(s.opts.Type, cur.Raw)
			keep[i] = true
			atomic.AddInt64(matches, 1)
		}
	}
	return nil
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

// regionSplit bounds the size of a scan work item so a single huge region is
// still scanned by several workers. It is a variable so tests can shrink it.
var regionSplit = 32 << 20

// splitRegions tiles regions larger than regionSplit into sub-regions, so the
// scheduler can parallelise within them. The sub-regions keep the original
// permission/path fields.
func splitRegions(regions []mem.Region) []mem.Region {
	split := false
	for _, r := range regions {
		if r.Size() > uint64(regionSplit) {
			split = true
			break
		}
	}
	if !split {
		return regions
	}
	out := make([]mem.Region, 0, len(regions)*2)
	for _, r := range regions {
		if r.Size() <= uint64(regionSplit) {
			out = append(out, r)
			continue
		}
		for start := r.Start; start < r.End; start += uint64(regionSplit) {
			end := start + uint64(regionSplit)
			if end > r.End {
				end = r.End
			}
			sub := r
			sub.Start, sub.End = start, end
			out = append(out, sub)
		}
	}
	return out
}

func (s *Session) scanRegion(ctx context.Context, r mem.Region, out *[]Result, scanned, matches *int64) error {
	w := s.width
	if w == 0 {
		w = s.opts.width()
	}
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
	// that follow them. Each read asks for `overlap` extra bytes so a value
	// spanning the chunk (or region) boundary is still seen.
	off := uint64(0)
	chunked := true
	for off < size {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.capped(matches) {
			return nil
		}
		chunk := uint64(scanChunk)
		if !chunked {
			chunk = page
		}
		if chunk > size-off {
			chunk = size - off
		}
		data, err := s.proc.Read(r.Start+off, int(chunk)+overlap)
		if len(data) > 0 {
			atomic.AddInt64(scanned, int64(len(data)))
			if serr := s.scanBytes(ctx, r.Start+off, data, int(chunk), out, matches); serr != nil {
				return serr
			}
		}
		if err != nil {
			// A fault ends the contiguous run. Advance past what was read
			// before falling back to page-sized reads, otherwise the prefix
			// would be read and scanned twice.
			if chunked {
				chunked = false
				adv := uint64(len(data))
				if adv == 0 {
					adv = page
				}
				off += adv
				continue
			}
			off += page
			continue
		}
		off += chunk
	}
	return nil
}

// scanBytes considers every candidate in data whose start is below limit. It
// batches the match counter per chunk so the hot loop does no atomic work, and
// uses a specialized loop for exact integer scans.
func (s *Session) scanBytes(ctx context.Context, addr uint64, data []byte, limit int, out *[]Result, matches *int64) error {
	if p, ok := s.exactIntProbe(); ok {
		return s.scanBytesInt(addr, data, limit, out, matches, p)
	}
	w := s.width
	if w == 0 {
		w = s.opts.width()
	}
	if w <= 0 {
		return nil
	}
	step := s.opts.step()
	if limit > len(data) {
		limit = len(data)
	}
	max := int64(s.opts.MaxResults)
	var base int64
	if max > 0 {
		base = atomic.LoadInt64(matches)
	}
	var local int64
	for i := 0; i < limit && i+w <= len(data); i += step {
		if max > 0 && base+local >= max {
			break
		}
		local += int64(s.consider(addr+uint64(i), data[i:i+w], out))
	}
	if local > 0 {
		atomic.AddInt64(matches, local)
	}
	return nil
}

// intProbe is a prepared exact/comparison integer scan.
type intProbe struct {
	width  int
	target int64
	op     CompareOp
}

// exactIntProbe reports whether the scan is an exact, bigger or smaller scan
// over a builtin integer type, which the specialized loop can run.
func (s *Session) exactIntProbe() (intProbe, bool) {
	switch s.opts.Mode {
	case ModeExact, ModeBigger, ModeSmaller:
	default:
		return intProbe{}, false
	}
	switch s.opts.Type {
	case TypeByte, TypeWord, TypeDword, TypeQword:
	default:
		return intProbe{}, false
	}
	op := s.opts.Compare
	switch s.opts.Mode {
	case ModeBigger:
		op = OpGreater
	case ModeSmaller:
		op = OpLess
	}
	w := s.width
	if w == 0 {
		w = s.opts.width()
	}
	return intProbe{width: w, target: s.opts.Value.Int64(), op: op}, true
}

// scanBytesInt is the specialized exact-integer loop: it decodes each candidate
// into a register and compares against a predecoded target, without allocating
// on non-matches.
func (s *Session) scanBytesInt(addr uint64, data []byte, limit int, out *[]Result, matches *int64, p intProbe) error {
	w, step := p.width, s.opts.step()
	if w <= 0 {
		return nil
	}
	if limit > len(data) {
		limit = len(data)
	}
	max := int64(s.opts.MaxResults)
	var base int64
	if max > 0 {
		base = atomic.LoadInt64(matches)
	}
	var local int64
	for i := 0; i < limit && i+w <= len(data); i += step {
		if max > 0 && base+local >= max {
			break
		}
		if compareInt(decodeIntRaw(data[i:i+w]), p.target, p.op) {
			v := NewValue(s.opts.Type, data[i:i+w])
			*out = append(*out, Result{Addr: addr + uint64(i), Value: v, First: v})
			local++
		}
	}
	if local > 0 {
		atomic.AddInt64(matches, local)
	}
	return nil
}

// decodeIntRaw decodes a little-endian signed integer of the given width.
func decodeIntRaw(raw []byte) int64 {
	switch len(raw) {
	case 1:
		return int64(int8(raw[0]))
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(raw)))
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(raw)))
	case 8:
		return int64(binary.LittleEndian.Uint64(raw))
	default:
		return 0
	}
}

// consider appends the matches at one candidate and returns how many.
func (s *Session) consider(addr uint64, raw []byte, out *[]Result) int {
	switch s.opts.Mode {
	case ModeUnknown:
		v := NewValue(s.opts.Type, raw)
		*out = append(*out, Result{Addr: addr, Value: v, First: v})
		return 1
	case ModeExact, ModeBigger, ModeSmaller:
		if s.opts.Mode == ModeExact && s.opts.Type == TypeAll {
			vs := s.matchAll(raw)
			for _, v := range vs {
				*out = append(*out, Result{Addr: addr, Value: v, First: v})
			}
			return len(vs)
		}
		if s.matchExact(raw) {
			v := NewValue(s.opts.Type, raw)
			*out = append(*out, Result{Addr: addr, Value: v, First: v})
			return 1
		}
	case ModeBetween:
		if s.matchBetween(raw) {
			v := NewValue(s.opts.Type, raw)
			*out = append(*out, Result{Addr: addr, Value: v, First: v})
			return 1
		}
	}
	return 0
}

// capped reports whether the result cap has been reached.
func (s *Session) capped(matches *int64) bool {
	return s.opts.MaxResults > 0 && atomic.LoadInt64(matches) >= int64(s.opts.MaxResults)
}

func (s *Session) matchExact(raw []byte) bool {
	if s.opts.Type == TypeGrouped {
		return s.opts.Grouped != nil && s.opts.Grouped.Match(raw)
	}
	t := s.typeOf()
	if t == nil {
		return false
	}
	op := s.opts.Compare
	switch s.opts.Mode {
	case ModeBigger:
		op = OpGreater
	case ModeSmaller:
		op = OpLess
	}
	switch t.Kind {
	case KindString, KindBytes, KindBinary:
		switch {
		case s.aob != nil:
			return s.aob.Match(raw)
		case s.binary != nil:
			return s.binary.Match(raw)
		default:
			return bytes.Equal(raw, s.opts.Value.Raw)
		}
	default:
		return t.Compare(Value{Type: s.opts.Type, Raw: raw}, s.opts.Value, op, s.opts.Epsilon)
	}
}

func (s *Session) keep(cur Value, res Result) bool {
	t := s.typeOf()
	prev := res.Value
	switch s.opts.Mode {
	case ModeExact, ModeBigger, ModeSmaller:
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
	case ModeSameAsFirst:
		return s.valueEqual(t, cur, res.First)
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
	t := s.typeOf()
	if t == nil {
		return false
	}
	switch t.Kind {
	case KindString, KindBytes, KindBinary:
		return false
	}
	cur := Value{Type: s.opts.Type, Raw: raw}
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
