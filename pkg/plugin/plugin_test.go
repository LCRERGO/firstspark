package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

func writePlugin(t *testing.T, root, id, manifest, init string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "init.lua"), []byte(init), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestManifestValidate(t *testing.T) {
	for name, m := range map[string]*Manifest{
		"no id":   {},
		"bad api": {ID: "x", API: 99},
		"bad cap": {ID: "x", Permissions: []Capability{"bogus"}},
	} {
		if err := m.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	m := &Manifest{ID: "x", Permissions: []Capability{CapMemoryRead}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.Entry != "init.lua" || m.API != APIVersion {
		t.Errorf("defaults not applied: %+v", m)
	}
	if !m.Grants(CapMemoryRead) || m.Grants(CapMemoryWrite) {
		t.Error("Grants mismatch")
	}
}

func TestLoadAndLifecycle(t *testing.T) {
	root := t.TempDir()
	var logged []string
	api := API{Log: func(s string) { logged = append(logged, s) }}
	writePlugin(t, root, "hello", "id: hello\napi: 1\n", `
function on_load(ctx)
  firstspark.log("loaded " .. ctx.id)
end
function on_tick(ms)
  firstspark.log("tick " .. ms)
end
`)
	p, err := Load(filepath.Join(root, "hello"), api)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Tick(50); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	want := []string{"loaded hello", "tick 50"}
	if fmt.Sprint(logged) != fmt.Sprint(want) {
		t.Fatalf("logged = %v, want %v", logged, want)
	}
}

var gateProbe byte = 42

func TestCapabilityGating(t *testing.T) {
	root := t.TempDir()
	proc, err := mem.Find(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	addr := uint64(uintptr(unsafe.Pointer(&gateProbe)))
	src := fmt.Sprintf(`
function on_load(ctx)
  local b = firstspark.memory.read_u8(%d)
  firstspark.log("val " .. b)
end
`, addr)

	// Without the capability the memory table is absent and the call errors.
	writePlugin(t, root, "nogrant", "id: nogrant\n", src)
	noGrant, err := Load(filepath.Join(root, "nogrant"), API{Read: proc.Read, Write: proc.Write, Log: func(string) {}})
	if err != nil {
		t.Fatal(err)
	}
	if err := noGrant.Start(); err == nil {
		t.Fatal("expected an error calling a gated function")
	}

	// With memory_read granted the read succeeds.
	writePlugin(t, root, "granted", "id: granted\npermissions: [memory_read]\n", src)
	var logged []string
	granted, err := Load(filepath.Join(root, "granted"), API{Read: proc.Read, Write: proc.Write, Log: func(s string) { logged = append(logged, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if err := granted.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(logged) != 1 || logged[0] != "val 42" {
		t.Fatalf("logged = %v, want [val 42]", logged)
	}
}

func TestManagerTickAutoDisable(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "bad", "id: bad\n", `
function on_tick(ms)
  local x = nil
  x.y = 1
end
`)
	m := NewManager(root, API{Log: func(string) {}})
	m.Load([]string{"bad"}, true)
	if len(m.Plugins()) != 1 {
		t.Fatalf("plugin not loaded: %v", m.Errors())
	}
	for i := 0; i < maxTickFailures; i++ {
		m.Tick(50)
	}
	if len(m.Plugins()) != 0 {
		t.Fatal("plugin was not disabled after repeated failures")
	}
}

func TestManagerMenuAndRun(t *testing.T) {
	root := t.TempDir()
	var logged []string
	writePlugin(t, root, "menu", `id: menu
name: Menu Plugin
menu:
  - label: Greet
    action: greet
`, `
function greet()
  firstspark.log("hi")
end
`)
	m := NewManager(root, API{Log: func(s string) { logged = append(logged, s) }})
	m.Load([]string{"menu"}, true)
	menu := m.Menu()
	if len(menu) != 1 || menu[0].Item.Action != "greet" {
		t.Fatalf("menu = %+v", menu)
	}
	if err := m.Run("menu", "greet"); err != nil {
		t.Fatal(err)
	}
	if len(logged) != 1 || logged[0] != "hi" {
		t.Fatalf("logged = %v", logged)
	}
}

func TestManagerRequiresUISkippedHeadless(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "gui", "id: gui\nrequires: [ui]\n", "function on_load(ctx) end\n")
	m := NewManager(root, API{Log: func(string) {}})
	m.Load([]string{"gui"}, false)
	if len(m.Plugins()) != 0 {
		t.Fatal("ui plugin should be skipped headless")
	}
}
