package celua

import (
	"testing"
	"time"

	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/script"
)

func TestCreateAndAppendRecord(t *testing.T) {
	tab := &fakeTable{recs: map[string]*fakeRecord{}}
	r := New(Config{Table: tab})
	err := r.Eval(`
mr = createMemoryRecord()
mr.Description = "new"
mr.Address = 4096
mr.Type = "4 Bytes"
mr.Value = "99"
appendToEntry(mr)
`)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	rec, ok := tab.recs["new"]
	if !ok {
		t.Fatalf("record not added: %v", tab.added)
	}
	if rec.addr != 4096 || rec.value != "99" {
		t.Fatalf("record = %+v", rec)
	}
	if err := r.Eval(`getAddressList().getMemoryRecordByDescription("new").Address = 8192`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if rec.addr != 8192 {
		t.Fatalf("setAddress via record = %d, want 8192", rec.addr)
	}
}

func TestTimersFireOnSchedule(t *testing.T) {
	now := time.Unix(0, 0)
	var shown []string
	r := New(Config{Now: func() time.Time { return now }, Show: func(s string) { shown = append(shown, s) }})
	if err := r.Eval(`createTimer(nil, 1000, function() showMessage("tick") end)`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if err := r.RunTimers(); err != nil {
		t.Fatalf("RunTimers: %v", err)
	}
	if len(shown) != 0 {
		t.Fatalf("timer fired early: %v", shown)
	}
	now = now.Add(1100 * time.Millisecond)
	if err := r.RunTimers(); err != nil {
		t.Fatalf("RunTimers: %v", err)
	}
	if len(shown) != 1 || shown[0] != "tick" {
		t.Fatalf("timer did not fire: %v", shown)
	}
}

func TestDelayedExecuteFiresOnce(t *testing.T) {
	now := time.Unix(0, 0)
	var count int
	r := New(Config{Now: func() time.Time { return now }, Show: func(string) { count++ }})
	if err := r.Eval(`delayedExecute(500, function() showMessage("once") end)`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	now = now.Add(time.Second)
	if err := r.RunTimers(); err != nil {
		t.Fatalf("RunTimers: %v", err)
	}
	if err := r.RunTimers(); err != nil {
		t.Fatalf("RunTimers: %v", err)
	}
	if count != 1 {
		t.Fatalf("one-shot fired %d times", count)
	}
}

func TestByteTable(t *testing.T) {
	b := &byteTable{data: []byte{10, 20, 30}}
	if v, _ := b.Index(script.Int(1)); v.Int() != 20 {
		t.Fatalf("byte[1] = %d", v.Int())
	}
	sizeFn, _ := b.Index(script.Str("getSize"))
	rets, err := script.CallValue(sizeFn)
	if err != nil || len(rets) != 1 || rets[0].Int() != 3 {
		t.Fatalf("getSize = %v, %v", rets, err)
	}
	byteFn, _ := b.Index(script.Str("getByte"))
	rets, err = script.CallValue(byteFn, script.Nil(), script.Int(2))
	if err != nil || rets[0].Int() != 30 {
		t.Fatalf("getByte(2) = %v, %v", rets, err)
	}
}

type fakeRecord struct {
	desc    string
	id      int
	active  bool
	addr    uint64
	value   string
	setCnt  int
	valueCt int
}

func (f *fakeRecord) Description() string           { return f.desc }
func (f *fakeRecord) ID() int                       { return f.id }
func (f *fakeRecord) Active() bool                  { return f.active }
func (f *fakeRecord) Address() uint64               { return f.addr }
func (f *fakeRecord) TypeName() string              { return "dword" }
func (f *fakeRecord) ValueString() string           { return f.value }
func (f *fakeRecord) SetActive(b bool) error        { f.active = b; f.setCnt++; return nil }
func (f *fakeRecord) SetAddress(a uint64) error     { f.addr = a; return nil }
func (f *fakeRecord) SetValueString(s string) error { f.value = s; f.valueCt++; return nil }

type fakeTable struct {
	recs  map[string]*fakeRecord
	added []string
}

func (t *fakeTable) Record(desc string) (Record, bool) {
	r, ok := t.recs[desc]
	if !ok {
		return nil, false
	}
	return r, true
}

func (t *fakeTable) Records() []Record {
	out := make([]Record, 0, len(t.recs))
	for _, r := range t.recs {
		out = append(out, r)
	}
	return out
}

func (t *fakeTable) Add(desc string, addr uint64, typeName, value string) (Record, error) {
	r := &fakeRecord{desc: desc, addr: addr, value: value}
	if t.recs == nil {
		t.recs = map[string]*fakeRecord{}
	}
	t.recs[desc] = r
	t.added = append(t.added, desc)
	return r, nil
}

func TestEvalAddressListActive(t *testing.T) {
	rec := &fakeRecord{desc: "_PointerCodeLocations", id: 7}
	tbl := &fakeTable{recs: map[string]*fakeRecord{"_PointerCodeLocations": rec}}
	r := New(Config{Table: tbl})
	if err := r.Eval(`getAddressList().getMemoryRecordByDescription("_PointerCodeLocations").Active = true`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if !rec.active || rec.setCnt != 1 {
		t.Fatalf("record active=%v setCnt=%d", rec.active, rec.setCnt)
	}
}

func TestEvalShowMessageAndVersion(t *testing.T) {
	var shown []string
	r := New(Config{Version: 7.4, Show: func(s string) { shown = append(shown, s) }})
	if err := r.Eval(`showMessage("hello")`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if len(shown) != 1 || shown[0] != "hello" {
		t.Fatalf("shown = %v", shown)
	}
}

func TestEvalOpenProcess(t *testing.T) {
	var opened string
	r := New(Config{OpenProcess: func(name string) (*mem.Process, error) {
		opened = name
		return &mem.Process{PID: 42, Name: name}, nil
	}})
	if err := r.Eval(`openProcess("game.exe")`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if opened != "game.exe" || r.Proc() == nil || r.Proc().PID != 42 {
		t.Fatalf("opened=%q proc=%+v", opened, r.Proc())
	}
}

func TestEvalNewAPISurface(t *testing.T) {
	r := New(Config{})
	err := r.Eval(`
a = AobScan("90 90")
b = AobScanModule("libc.so.6", "90 90")
c = getOpenedProcessID()
d = getProcessIDFromProcessName("no-such-process")
e = getProcessNameFromProcessID(1)
f = getAddressSafe("nope")
writeBytes(0, {1, 2, 3})
getMainForm().Visible = false
`)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
}

func TestGlobalsPersistAcrossChunks(t *testing.T) {
	var shown string
	r := New(Config{Show: func(s string) { shown = s }})
	if err := r.Eval(`function helper(x) return x + 1 end; shared = 5`); err != nil {
		t.Fatalf("first Eval: %v", err)
	}
	if err := r.Eval(`if helper(shared) == 6 then showMessage("ok") end`); err != nil {
		t.Fatalf("second Eval: %v", err)
	}
	if shown != "ok" {
		t.Fatalf("persisted globals not visible: %q", shown)
	}
}

func TestEvalUnknownMemberErrors(t *testing.T) {
	r := New(Config{})
	if err := r.Eval(`x = getAddressList().nope`); err == nil {
		t.Fatal("expected an error for an unknown member")
	}
}

func TestEvalNoopSurface(t *testing.T) {
	r := New(Config{})
	if err := r.Eval(`
createTimer(nil, 1000, function() end)
getAddressList().Component[1].OnSectionClick = nil
`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
}
