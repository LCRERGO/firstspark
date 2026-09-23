// Package celua implements the core subset of Cheat Engine's Lua API used by
// Cheat Engine tables (ADR 0039): the AddressList/MemoryRecord object model,
// the scalar memory/process helpers and the globals scripts reference. It is
// UI-agnostic and bridges to the table through the Table and Record interfaces.
package celua

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
	"github.com/LCRERGO/firstspark/pkg/script"
)

// Record is a cheat-table record exposed to Lua.
type Record interface {
	Description() string
	ID() int
	Active() bool
	SetActive(bool) error
	Address() uint64
	SetAddress(uint64) error
	TypeName() string
	ValueString() string
	SetValueString(string) error
}

// Table exposes the cheat table to Lua.
type Table interface {
	Record(desc string) (Record, bool)
	Records() []Record
	// Add appends a new record and returns it.
	Add(desc string, addr uint64, typeName, value string) (Record, error)
}

// Config configures a Runtime.
type Config struct {
	// Proc is the process scripts read and write. It may be nil.
	Proc *mem.Process
	// Resolve resolves a symbol or module name for getAddressSafe.
	Resolve func(name string) (uint64, bool)
	// Table is the cheat table, or nil.
	Table Table
	// OpenProcess selects a process by name (for openProcess).
	OpenProcess func(name string) (*mem.Process, error)
	// Show displays a message (showMessage/ShowMessage).
	Show func(string)
	// Version is returned by getCEVersion.
	Version float64
	// Now overrides the clock used by timers (for tests).
	Now func() time.Time
	// Clipboard receives writeToClipboard text, if set.
	Clipboard func(string)
}

// Runtime evaluates Cheat Engine Lua chunks against a process and table. Its
// global scope persists across Eval calls, matching Cheat Engine's shared Lua
// environment.
type Runtime struct {
	cfg    Config
	proc   *mem.Process
	vars   map[string]script.Value
	timers []*timer
}

// New creates a runtime.
func New(cfg Config) *Runtime {
	if cfg.Version == 0 {
		cfg.Version = 7.5
	}
	return &Runtime{cfg: cfg, proc: cfg.Proc, vars: map[string]script.Value{}}
}

func (r *Runtime) now() time.Time {
	if r.cfg.Now != nil {
		return r.cfg.Now()
	}
	return time.Now()
}

// Proc returns the process the runtime is currently bound to.
func (r *Runtime) Proc() *mem.Process { return r.proc }

// SetProc rebinds the runtime to a process.
func (r *Runtime) SetProc(p *mem.Process) { r.proc = p }

// Eval compiles and runs a Lua chunk with the CE globals installed, then keeps
// any globals it defined for the next chunk.
func (r *Runtime) Eval(chunk string) error {
	api := r.apiGlobals()
	globals := make(map[string]script.Value, len(api)+len(r.vars))
	for k, v := range r.vars {
		globals[k] = v
	}
	for k, v := range api {
		globals[k] = v
	}
	p, err := script.CompileWithGlobals(chunk, globals)
	if err != nil {
		return fmt.Errorf("celua: %w", err)
	}
	for k, v := range p.Globals() {
		if _, ok := api[k]; ok {
			continue
		}
		r.vars[k] = v
	}
	return nil
}

// apiGlobals builds the built-in Cheat Engine globals.
func (r *Runtime) apiGlobals() map[string]script.Value {
	return map[string]script.Value{
		"process":           r.processValue(),
		"_G":                script.TableVal(script.NewTable()),
		"syntaxcheck":       script.Bool(false),
		"readInteger":       r.reader(4, true),
		"readQword":         r.reader(8, false),
		"readPointer":       r.reader(8, false),
		"readByte":          r.reader(1, false),
		"readSmallInteger":  r.reader(2, true),
		"readFloat":         r.readerFloat(4),
		"readDouble":        r.readerFloat(8),
		"readString":        script.GoFunc("readString", r.readString),
		"readBytes":         script.GoFunc("readBytes", r.readBytes),
		"readmem":           script.GoFunc("readmem", r.readMem),
		"writeInteger":      r.writer(4),
		"writeQword":        r.writer(8),
		"writeByte":         r.writer(1),
		"writeSmallInteger": r.writer(2),
		"writeFloat":        r.writerFloat(4),
		"writeDouble":       r.writerFloat(8),
		"writeBytes":        script.GoFunc("writeBytes", r.writeBytes),
		"getAddressSafe":    script.GoFunc("getAddressSafe", r.getAddressSafe),
		"getAddress":        script.GoFunc("getAddress", r.getAddressSafe),
		"AobScan":           script.GoFunc("AobScan", r.aobScan),
		"AobScanModule":     script.GoFunc("AobScanModule", r.aobScanModule),
		"getTickCount": script.GoFunc("getTickCount", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Int(time.Now().UnixMilli())}, nil
		}),
		"sleep": script.GoFunc("sleep", func(args []script.Value) ([]script.Value, error) {
			if len(args) > 0 {
				if ms := args[0].Int(); ms > 0 && ms <= 1000 {
					time.Sleep(time.Duration(ms) * time.Millisecond)
				}
			}
			return nil, nil
		}),
		"findAddressFromDatabase": script.GoFunc("findAddressFromDatabase", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Int(0)}, nil
		}),
		"writeToClipboard": script.GoFunc("writeToClipboard", func(args []script.Value) ([]script.Value, error) {
			if r.cfg.Clipboard != nil && len(args) > 0 {
				r.cfg.Clipboard(args[0].Str())
			}
			return nil, nil
		}),
		"getOpenedProcessID": script.GoFunc("getOpenedProcessID", func([]script.Value) ([]script.Value, error) {
			if r.proc == nil {
				return []script.Value{script.Int(0)}, nil
			}
			return []script.Value{script.Int(int64(r.proc.PID))}, nil
		}),
		"getProcessIDFromProcessName": script.GoFunc("getProcessIDFromProcessName", r.pidFromName),
		"getProcessNameFromProcessID": script.GoFunc("getProcessNameFromProcessID", r.nameFromPID),
		"registerCustomTypeAutoAssembler": script.GoFunc("registerCustomTypeAutoAssembler", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Bool(true)}, nil
		}),
		"getMainForm":   noopFunc("getMainForm"),
		"messageDialog": noopFunc("messageDialog"),
		"showMessage":   script.GoFunc("showMessage", r.showMessage),
		"ShowMessage":   script.GoFunc("ShowMessage", r.showMessage),
		"getCEVersion":  script.GoFunc("getCEVersion", func([]script.Value) ([]script.Value, error) { return []script.Value{script.Float(r.cfg.Version)}, nil }),
		"targetIs64Bit": script.GoFunc("targetIs64Bit", func([]script.Value) ([]script.Value, error) { return []script.Value{script.Bool(true)}, nil }),
		"openProcess":   script.GoFunc("openProcess", r.openProcess),
		"getAddressList": script.GoFunc("getAddressList", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.ObjectVal(addressList{r})}, nil
		}),
		"AddressList": script.ObjectVal(addressList{r}),
		"createMemoryRecord": script.GoFunc("createMemoryRecord", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.ObjectVal(&detached{table: r.cfg.Table})}, nil
		}),
		"appendToEntry": script.GoFunc("appendToEntry", r.appendToEntry),
		"setAddress":    script.GoFunc("setAddress", r.setAddress),
		"enableAutoDisable": script.GoFunc("enableAutoDisable", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Bool(true)}, nil
		}),
		// Timers run from Runtime.RunTimers on the UI tick; threads are
		// accepted and ignored.
		"createTimer":    script.GoFunc("createTimer", r.createTimer),
		"delayedExecute": script.GoFunc("delayedExecute", r.delayedExecute),
		"createthread":   noopFunc("createthread"),
	}
}

func (r *Runtime) processValue() script.Value {
	if r.proc == nil {
		return script.Nil()
	}
	return script.Int(int64(r.proc.PID))
}

func (r *Runtime) reader(size int, signed bool) script.Value {
	return script.GoFunc("read", func(args []script.Value) ([]script.Value, error) {
		if len(args) == 0 || r.proc == nil {
			return []script.Value{script.Nil()}, nil
		}
		addr := uint64(args[0].Int())
		data, err := r.proc.Read(addr, size)
		if err != nil || len(data) < size {
			return []script.Value{script.Nil()}, nil
		}
		var n int64
		switch size {
		case 1:
			n = int64(data[0])
			if signed {
				n = int64(int8(data[0]))
			}
		case 2:
			u := binary.LittleEndian.Uint16(data)
			n = int64(u)
			if signed {
				n = int64(int16(u))
			}
		case 4:
			u := binary.LittleEndian.Uint32(data)
			n = int64(u)
			if signed {
				n = int64(int32(u))
			}
		default:
			n = int64(binary.LittleEndian.Uint64(data))
		}
		return []script.Value{script.Int(n)}, nil
	})
}

func (r *Runtime) readerFloat(size int) script.Value {
	return script.GoFunc("read", func(args []script.Value) ([]script.Value, error) {
		if len(args) == 0 || r.proc == nil {
			return []script.Value{script.Nil()}, nil
		}
		addr := uint64(args[0].Int())
		data, err := r.proc.Read(addr, size)
		if err != nil || len(data) < size {
			return []script.Value{script.Nil()}, nil
		}
		if size == 4 {
			return []script.Value{script.Float(float64(math.Float32frombits(binary.LittleEndian.Uint32(data))))}, nil
		}
		return []script.Value{script.Float(math.Float64frombits(binary.LittleEndian.Uint64(data)))}, nil
	})
}

func (r *Runtime) readString(args []script.Value) ([]script.Value, error) {
	if len(args) == 0 || r.proc == nil {
		return []script.Value{script.Str("")}, nil
	}
	addr := uint64(args[0].Int())
	max := 64
	if len(args) > 1 && args[1].Int() > 0 {
		max = int(args[1].Int())
	}
	if max > 1<<16 {
		max = 1 << 16
	}
	data, err := r.proc.Read(addr, max)
	if err != nil && len(data) == 0 {
		return []script.Value{script.Str("")}, nil
	}
	if i := bytes.IndexByte(data, 0); i >= 0 {
		data = data[:i]
	}
	return []script.Value{script.Str(string(data))}, nil
}

func (r *Runtime) readBytes(args []script.Value) ([]script.Value, error) {
	if len(args) < 2 || r.proc == nil {
		return []script.Value{script.Nil()}, nil
	}
	addr := uint64(args[0].Int())
	n := int(args[1].Int())
	if n < 0 || n > 1<<20 {
		return nil, fmt.Errorf("readBytes: bad size %d", n)
	}
	data, err := r.proc.Read(addr, n)
	if err != nil {
		return []script.Value{script.Nil()}, nil
	}
	t := script.NewTable()
	for i, b := range data {
		t.Set(script.Int(int64(i+1)), script.Int(int64(b)))
	}
	return []script.Value{script.TableVal(t)}, nil
}

func (r *Runtime) readMem(args []script.Value) ([]script.Value, error) {
	if len(args) < 2 || r.proc == nil {
		return []script.Value{script.Nil()}, nil
	}
	addr := uint64(args[0].Int())
	n := int(args[1].Int())
	if n < 0 || n > 1<<20 {
		return nil, fmt.Errorf("readmem: bad size %d", n)
	}
	data, err := r.proc.Read(addr, n)
	if err != nil {
		return []script.Value{script.Nil()}, nil
	}
	return []script.Value{script.ObjectVal(&byteTable{data: data})}, nil
}

// byteTable is Cheat Engine's readmem result: a 0-based byte array with
// getSize/getByte.
type byteTable struct{ data []byte }

func (b *byteTable) Index(key script.Value) (script.Value, bool) {
	switch key.Kind() {
	case script.KindInt, script.KindFloat:
		i := int(key.Int())
		if i >= 0 && i < len(b.data) {
			return script.Int(int64(b.data[i])), true
		}
		return script.Int(0), true
	case script.KindString:
		switch key.Str() {
		case "getSize":
			return script.GoFunc("getSize", func([]script.Value) ([]script.Value, error) {
				return []script.Value{script.Int(int64(len(b.data)))}, nil
			}), true
		case "getByte":
			return script.GoFunc("getByte", func(args []script.Value) ([]script.Value, error) {
				if len(args) < 2 {
					return []script.Value{script.Int(0)}, nil
				}
				i := int(args[1].Int())
				if i >= 0 && i < len(b.data) {
					return []script.Value{script.Int(int64(b.data[i]))}, nil
				}
				return []script.Value{script.Int(0)}, nil
			}), true
		}
	}
	return script.Nil(), false
}

func (b *byteTable) SetIndex(script.Value, script.Value) error { return nil }

// timer is a Cheat Engine timer object.
type timer struct {
	interval  time.Duration
	next      time.Time
	callback  script.Value
	enabled   bool
	oneShot   bool
	destroyed bool
}

func (t *timer) Index(key script.Value) (script.Value, bool) {
	switch key.Str() {
	case "Enabled":
		return script.Bool(t.enabled), true
	case "Interval":
		return script.Int(t.interval.Milliseconds()), true
	case "Destroy":
		return script.GoFunc("Destroy", func([]script.Value) ([]script.Value, error) {
			t.destroyed = true
			return nil, nil
		}), true
	}
	return script.Nil(), false
}

func (t *timer) SetIndex(key, value script.Value) error {
	switch key.Str() {
	case "Enabled":
		t.enabled = truthy(value)
	case "Interval":
		t.interval = time.Duration(value.Int()) * time.Millisecond
	}
	return nil
}

func (r *Runtime) createTimer(args []script.Value) ([]script.Value, error) {
	interval, cb := r.timerArgs(args)
	if cb.IsNil() {
		return []script.Value{script.Nil()}, nil
	}
	t := &timer{interval: interval, next: r.now().Add(interval), callback: cb, enabled: true}
	r.timers = append(r.timers, t)
	return []script.Value{script.ObjectVal(t)}, nil
}

func (r *Runtime) delayedExecute(args []script.Value) ([]script.Value, error) {
	interval, cb := r.timerArgs(args)
	if cb.IsNil() {
		return []script.Value{script.Nil()}, nil
	}
	t := &timer{interval: interval, next: r.now().Add(interval), callback: cb, enabled: true, oneShot: true}
	r.timers = append(r.timers, t)
	return []script.Value{script.ObjectVal(t)}, nil
}

// timerArgs accepts CE's (owner, interval, callback) and the shorter
// (interval, callback) forms.
func (r *Runtime) timerArgs(args []script.Value) (time.Duration, script.Value) {
	if len(args) >= 3 {
		return time.Duration(args[1].Int()) * time.Millisecond, args[2]
	}
	if len(args) == 2 {
		return time.Duration(args[0].Int()) * time.Millisecond, args[1]
	}
	return 0, script.Nil()
}

// RunTimers fires the due timer callbacks. It must run on the UI goroutine.
func (r *Runtime) RunTimers() error {
	if len(r.timers) == 0 {
		return nil
	}
	now := r.now()
	keep := r.timers[:0]
	for _, t := range r.timers {
		if t.destroyed {
			continue
		}
		if t.enabled && !now.Before(t.next) {
			if _, err := script.CallValue(t.callback); err != nil {
				return err
			}
			if t.oneShot {
				continue
			}
			t.next = now.Add(t.interval)
		}
		keep = append(keep, t)
	}
	r.timers = keep
	return nil
}

func (r *Runtime) writer(size int) script.Value {
	return script.GoFunc("write", func(args []script.Value) ([]script.Value, error) {
		if len(args) < 2 || r.proc == nil {
			return []script.Value{script.Bool(false)}, nil
		}
		addr := uint64(args[0].Int())
		buf := make([]byte, size)
		switch size {
		case 1:
			buf[0] = byte(args[1].Int())
		case 2:
			binary.LittleEndian.PutUint16(buf, uint16(args[1].Int()))
		case 4:
			binary.LittleEndian.PutUint32(buf, uint32(args[1].Int()))
		default:
			binary.LittleEndian.PutUint64(buf, uint64(args[1].Int()))
		}
		return []script.Value{script.Bool(r.proc.Write(addr, buf) == nil)}, nil
	})
}

func (r *Runtime) writerFloat(size int) script.Value {
	return script.GoFunc("write", func(args []script.Value) ([]script.Value, error) {
		if len(args) < 2 || r.proc == nil {
			return []script.Value{script.Bool(false)}, nil
		}
		addr := uint64(args[0].Int())
		f, _ := args[1].Number()
		buf := make([]byte, size)
		if size == 4 {
			binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(f)))
		} else {
			binary.LittleEndian.PutUint64(buf, math.Float64bits(f))
		}
		return []script.Value{script.Bool(r.proc.Write(addr, buf) == nil)}, nil
	})
}

func (r *Runtime) getAddressSafe(args []script.Value) ([]script.Value, error) {
	if len(args) == 0 {
		return []script.Value{script.Int(0)}, nil
	}
	name := args[0].Str()
	if r.cfg.Resolve != nil {
		if addr, ok := r.cfg.Resolve(name); ok {
			return []script.Value{script.Int(int64(addr))}, nil
		}
	}
	return []script.Value{script.Int(0)}, nil
}

func (r *Runtime) showMessage(args []script.Value) ([]script.Value, error) {
	if r.cfg.Show != nil && len(args) > 0 {
		r.cfg.Show(args[0].Str())
	}
	return nil, nil
}

func (r *Runtime) openProcess(args []script.Value) ([]script.Value, error) {
	if r.cfg.OpenProcess == nil || len(args) == 0 {
		return []script.Value{script.Bool(false)}, nil
	}
	p, err := r.cfg.OpenProcess(args[0].Str())
	if err != nil || p == nil {
		return []script.Value{script.Bool(false)}, nil
	}
	r.proc = p
	return []script.Value{script.Bool(true)}, nil
}

// aobScan returns the first address of an array-of-bytes pattern anywhere in
// the target, or 0.
func (r *Runtime) aobScan(args []script.Value) ([]script.Value, error) {
	if len(args) == 0 || r.proc == nil {
		return []script.Value{script.Int(0)}, nil
	}
	pat, err := scan.ParseAOB(args[0].Str())
	if err != nil {
		return nil, err
	}
	regions, err := mem.Regions(r.proc.PID)
	if err != nil {
		return []script.Value{script.Int(0)}, nil
	}
	for _, reg := range regions {
		if !reg.Readable() || reg.Size() == 0 {
			continue
		}
		if addr, ok := scanRegion(r.proc, reg, pat); ok {
			return []script.Value{script.Int(int64(addr))}, nil
		}
	}
	return []script.Value{script.Int(0)}, nil
}

// aobScanModule returns the first address of a pattern within a named module,
// or 0.
func (r *Runtime) aobScanModule(args []script.Value) ([]script.Value, error) {
	if len(args) < 2 || r.proc == nil {
		return []script.Value{script.Int(0)}, nil
	}
	pat, err := scan.ParseAOB(args[1].Str())
	if err != nil {
		return nil, err
	}
	regions, err := mem.Regions(r.proc.PID)
	if err != nil {
		return []script.Value{script.Int(0)}, nil
	}
	module := args[0].Str()
	for _, reg := range regions {
		if !reg.Readable() || reg.Size() == 0 {
			continue
		}
		if filepath.Base(strings.TrimSuffix(reg.Path, " (deleted)")) != module {
			continue
		}
		if addr, ok := scanRegion(r.proc, reg, pat); ok {
			return []script.Value{script.Int(int64(addr))}, nil
		}
	}
	return []script.Value{script.Int(0)}, nil
}

func scanRegion(p *mem.Process, reg mem.Region, pat *scan.AOBPattern) (uint64, bool) {
	window := reg.Size()
	if window > 1<<24 {
		window = 1 << 24
	}
	data, _ := p.Read(reg.Start, int(window))
	last := len(data) - len(pat.Bytes)
	for i := 0; i <= last; i++ {
		if pat.Match(data[i:]) {
			return reg.Start + uint64(i), true
		}
	}
	return 0, false
}

func (r *Runtime) writeBytes(args []script.Value) ([]script.Value, error) {
	if len(args) < 2 || r.proc == nil {
		return []script.Value{script.Bool(false)}, nil
	}
	addr := uint64(args[0].Int())
	t := args[1].Table()
	if t == nil {
		return []script.Value{script.Bool(false)}, nil
	}
	arr := t.Array()
	buf := make([]byte, len(arr))
	for i, v := range arr {
		buf[i] = byte(v.Int())
	}
	return []script.Value{script.Bool(r.proc.Write(addr, buf) == nil)}, nil
}

func (r *Runtime) pidFromName(args []script.Value) ([]script.Value, error) {
	if len(args) == 0 {
		return []script.Value{script.Int(0)}, nil
	}
	procs, err := mem.List()
	if err != nil {
		return []script.Value{script.Int(0)}, nil
	}
	for i := range procs {
		if procs[i].Name == args[0].Str() {
			return []script.Value{script.Int(int64(procs[i].PID))}, nil
		}
	}
	return []script.Value{script.Int(0)}, nil
}

func (r *Runtime) nameFromPID(args []script.Value) ([]script.Value, error) {
	if len(args) == 0 {
		return []script.Value{script.Str("")}, nil
	}
	pid := int(args[0].Int())
	procs, err := mem.List()
	if err != nil {
		return []script.Value{script.Str("")}, nil
	}
	for i := range procs {
		if procs[i].PID == pid {
			return []script.Value{script.Str(procs[i].Name)}, nil
		}
	}
	return []script.Value{script.Str("")}, nil
}

func noopFunc(name string) script.Value {
	return script.GoFunc(name, func([]script.Value) ([]script.Value, error) {
		return []script.Value{script.ObjectVal(noop{})}, nil
	})
}

// noop accepts any member read or write, so optional API surfaces can be
// assigned to without erroring.
type noop struct{}

func (noop) Index(script.Value) (script.Value, bool)   { return script.ObjectVal(noop{}), true }
func (noop) SetIndex(script.Value, script.Value) error { return nil }

// addressList is getAddressList().
type addressList struct{ r *Runtime }

func (l addressList) Index(key script.Value) (script.Value, bool) {
	switch key.Str() {
	case "getCount":
		return script.GoFunc("getCount", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Int(int64(l.count()))}, nil
		}), true
	case "getMemoryRecordByDescription":
		return script.GoFunc("getMemoryRecordByDescription", func(args []script.Value) ([]script.Value, error) {
			if len(args) == 0 || l.r.cfg.Table == nil {
				return []script.Value{script.Nil()}, nil
			}
			if rec, ok := l.r.cfg.Table.Record(args[0].Str()); ok {
				return []script.Value{script.ObjectVal(record{rec})}, nil
			}
			return []script.Value{script.Nil()}, nil
		}), true
	case "getMemoryRecord":
		return script.GoFunc("getMemoryRecord", func(args []script.Value) ([]script.Value, error) {
			if len(args) == 0 || l.r.cfg.Table == nil {
				return []script.Value{script.Nil()}, nil
			}
			recs := l.r.cfg.Table.Records()
			i := int(args[0].Int())
			if i >= 0 && i < len(recs) {
				return []script.Value{script.ObjectVal(record{recs[i]})}, nil
			}
			return []script.Value{script.Nil()}, nil
		}), true
	case "Component":
		return script.ObjectVal(noop{}), true
	case "createMemoryRecord":
		return script.GoFunc("createMemoryRecord", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.ObjectVal(&detached{table: l.r.cfg.Table})}, nil
		}), true
	case "appendToEntry":
		return script.GoFunc("appendToEntry", l.r.appendToEntry), true
	}
	return script.Nil(), false
}

func (l addressList) SetIndex(script.Value, script.Value) error { return nil }

func (l addressList) count() int {
	if l.r.cfg.Table == nil {
		return 0
	}
	return len(l.r.cfg.Table.Records())
}

// record wraps a table Record.
type record struct{ r Record }

func (m record) Index(key script.Value) (script.Value, bool) {
	switch key.Str() {
	case "Description":
		return script.Str(m.r.Description()), true
	case "ID":
		return script.Int(int64(m.r.ID())), true
	case "Active":
		return script.Bool(m.r.Active()), true
	case "Address":
		return script.Int(int64(m.r.Address())), true
	case "Type":
		return script.Int(0), true
	case "Value":
		return script.Str(m.r.ValueString()), true
	case "getDescription":
		return script.GoFunc("getDescription", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Str(m.r.Description())}, nil
		}), true
	case "getID":
		return script.GoFunc("getID", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Int(int64(m.r.ID()))}, nil
		}), true
	case "getAddress":
		return script.GoFunc("getAddress", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Int(int64(m.r.Address()))}, nil
		}), true
	case "getValue":
		return script.GoFunc("getValue", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Str(m.r.ValueString())}, nil
		}), true
	case "isActive":
		return script.GoFunc("isActive", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Bool(m.r.Active())}, nil
		}), true
	case "setActive":
		return script.GoFunc("setActive", func(args []script.Value) ([]script.Value, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("setActive: missing value")
			}
			return nil, m.r.SetActive(truthy(args[1]))
		}), true
	case "setValue":
		return script.GoFunc("setValue", func(args []script.Value) ([]script.Value, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("setValue: missing value")
			}
			return nil, m.r.SetValueString(args[1].Str())
		}), true
	}
	return script.Nil(), false
}

func (m record) SetIndex(key, value script.Value) error {
	switch key.Str() {
	case "Active":
		return m.r.SetActive(truthy(value))
	case "Address":
		return m.r.SetAddress(uint64(value.Int()))
	case "Value":
		return m.r.SetValueString(value.Str())
	}
	return nil
}

func (r *Runtime) appendToEntry(args []script.Value) ([]script.Value, error) {
	if len(args) == 0 || r.cfg.Table == nil {
		return []script.Value{script.Bool(false)}, nil
	}
	d, ok := args[0].Object().(*detached)
	if !ok || d == nil {
		return []script.Value{script.Bool(false)}, nil
	}
	if d.typ == "" {
		d.typ = "dword"
	}
	rec, err := r.cfg.Table.Add(d.desc, d.addr, d.typ, d.val)
	if err != nil {
		return nil, err
	}
	d.persisted = rec
	return []script.Value{script.Bool(true)}, nil
}

func (r *Runtime) setAddress(args []script.Value) ([]script.Value, error) {
	if len(args) < 2 {
		return []script.Value{script.Bool(false)}, nil
	}
	d, ok := args[0].Object().(*detached)
	if !ok || d == nil {
		return []script.Value{script.Bool(false)}, nil
	}
	d.addr = uint64(args[1].Int())
	return []script.Value{script.Bool(true)}, nil
}

// detached is a record created by createMemoryRecord() before it is appended
// to the table.
type detached struct {
	desc      string
	addr      uint64
	typ       string
	val       string
	active    bool
	table     Table
	persisted Record
}

func (d *detached) Index(key script.Value) (script.Value, bool) {
	switch key.Str() {
	case "Description":
		return script.Str(d.desc), true
	case "Address":
		return script.Int(int64(d.addr)), true
	case "Type":
		return script.Str(d.typ), true
	case "Value":
		return script.Str(d.val), true
	case "Active":
		return script.Bool(d.active), true
	case "getAddress":
		return script.GoFunc("getAddress", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Int(int64(d.addr))}, nil
		}), true
	case "getValue":
		return script.GoFunc("getValue", func([]script.Value) ([]script.Value, error) {
			return []script.Value{script.Str(d.val)}, nil
		}), true
	}
	return script.Nil(), false
}

func (d *detached) SetIndex(key, value script.Value) error {
	switch key.Str() {
	case "Description":
		d.desc = value.Str()
	case "Address":
		d.addr = uint64(value.Int())
	case "Type":
		d.typ = value.Str()
	case "Value":
		d.val = value.Str()
	case "Active":
		d.active = truthy(value)
	}
	return nil
}

func truthy(v script.Value) bool {
	switch v.Kind() {
	case script.KindNil:
		return false
	case script.KindBool:
		return v.Bool()
	default:
		return true
	}
}
