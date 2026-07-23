//go:build gui

package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// iconResolver maps processes to icons. Lookup order is: X11 window icon,
// WM_CLASS -> StartupWMClass, executable -> .desktop Exec, Steam/Proton
// appid, then a generic fallback. Results are cached per PID.
type iconResolver struct {
	mu       sync.Mutex
	cache    map[int]fyne.Resource
	generic  fyne.Resource
	byExe    map[string]string
	byClass  map[string]string
	x        *x11Client
	loadOnce sync.Once
}

func newIconResolver() *iconResolver {
	return &iconResolver{cache: map[int]fyne.Resource{}, x: &x11Client{}}
}

// get returns the cached icon for pid, or the generic fallback.
func (r *iconResolver) get(pid int) fyne.Resource {
	r.mu.Lock()
	res, ok := r.cache[pid]
	r.mu.Unlock()
	if ok && res != nil {
		return res
	}
	return r.fallback()
}

// resolve looks up and caches the icon for pid. Safe to call concurrently.
func (r *iconResolver) resolve(pid int) {
	res := r.lookup(pid)
	if res == nil {
		res = r.fallback()
	}
	r.mu.Lock()
	r.cache[pid] = res
	r.mu.Unlock()
}

func (r *iconResolver) fallback() fyne.Resource {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.generic == nil {
		r.generic = theme.FileApplicationIcon()
	}
	return r.generic
}

func (r *iconResolver) lookup(pid int) fyne.Resource {
	windows := r.x.pidWindows()
	if win, ok := windows[pid]; ok {
		if img := r.x.icon(win); img != nil {
			if res := pngResource(img); res != nil {
				return res
			}
		}
		if cls := r.x.wmClass(win); cls != "" {
			if res := r.iconByName(r.classIcon(cls)); res != nil {
				return res
			}
		}
	}
	if exe := procExe(pid); exe != "" {
		if res := r.iconByName(r.exeIcon(exe)); res != nil {
			return res
		}
	}
	if id := steamAppID(pid); id != "" {
		if res := r.iconByName("steam_icon_" + id); res != nil {
			return res
		}
		if res := loadFileResource(steamGameIconPath(id)); res != nil {
			return res
		}
	}
	return nil
}

func (r *iconResolver) classIcon(class string) string {
	r.ensureDesktop()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byClass[strings.ToLower(class)]
}

func (r *iconResolver) exeIcon(exe string) string {
	base := exeBase(exe)
	if base == "" {
		return ""
	}
	r.ensureDesktop()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byExe[base]
}

func (r *iconResolver) iconByName(name string) fyne.Resource {
	if name == "" {
		return nil
	}
	path := name
	if !filepath.IsAbs(path) {
		path = lookupIconPath(name)
	}
	return loadFileResource(path)
}

// ensureDesktop builds the .desktop index once.
func (r *iconResolver) ensureDesktop() {
	r.loadOnce.Do(func() {
		byExe := map[string]string{}
		byClass := map[string]string{}
		for _, dir := range applicationDirs() {
			_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(path, ".desktop") {
					return nil
				}
				parseDesktopFile(path, byExe, byClass)
				return nil
			})
		}
		r.mu.Lock()
		r.byExe, r.byClass = byExe, byClass
		r.mu.Unlock()
	})
}

// wrapperBinaries are Exec tokens that launch another program rather than
// being the application itself.
var wrapperBinaries = map[string]bool{
	"env": true, "wine": true, "wine64": true, "wine-preloader": true,
	"wine64-preloader": true, "sh": true, "bash": true, "dash": true,
	"flatpak": true, "snap": true, "steam": true, "steam-launch-wrapper": true,
	"gamemoderun": true, "mangohud": true, "proton": true, "python": true,
	"python3": true, "java": true, "mono": true, "box86": true, "box64": true,
	"dosbox": true, "run": true, "xdg-open": true, "gtk-launch": true,
	"distrobox": true, "toolbox": true, "bwrap": true, "umu-run": true,
}

func parseDesktopFile(path string, byExe, byClass map[string]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	inEntry := false
	hidden := false
	var icon, exec, wmClass string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "Icon":
			icon = val
		case "Exec":
			exec = val
		case "StartupWMClass":
			wmClass = val
		case "Hidden", "NoDisplay":
			if strings.EqualFold(val, "true") {
				hidden = true
			}
		}
	}
	if hidden || icon == "" {
		return
	}
	if wmClass != "" {
		byClass[strings.ToLower(wmClass)] = icon
	}
	for _, tok := range strings.Fields(exec) {
		base := exeBase(tok)
		if base == "" || wrapperBinaries[base] {
			continue
		}
		if _, exists := byExe[base]; !exists {
			byExe[base] = icon
		}
	}
}

// exeBase extracts a comparable executable basename from an Exec token,
// handling paths, quotes and Wine's backslash paths.
func exeBase(tok string) string {
	tok = strings.Trim(tok, `"'`)
	if tok == "" || strings.HasPrefix(tok, "-") ||
		strings.Contains(tok, "=") || strings.HasPrefix(tok, "%") {
		return ""
	}
	if i := strings.LastIndexAny(tok, `/\`); i >= 0 {
		tok = tok[i+1:]
	}
	return strings.TrimSuffix(strings.ToLower(tok), ".exe")
}

// lookupIconPath resolves an icon name using the Icon Theme specification.
func lookupIconPath(name string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		if fileExists(name) {
			return name
		}
		return ""
	}
	sizes := []string{"16x16", "22x22", "24x24", "32x32", "48x48", "64x64", "128x128", "256x256", "scalable"}
	exts := []string{".png", ".svg", ".xpm"}
	for _, base := range iconBaseDirs() {
		for _, th := range iconThemeChain() {
			for _, size := range sizes {
				for _, ext := range exts {
					if p := filepath.Join(base, th, size, "apps", name+ext); fileExists(p) {
						return p
					}
				}
			}
			for _, ext := range exts {
				if p := filepath.Join(base, th, name+ext); fileExists(p) {
					return p
				}
			}
		}
	}
	for _, ext := range exts {
		if p := filepath.Join("/usr/share/pixmaps", name+ext); fileExists(p) {
			return p
		}
	}
	return ""
}

func iconThemeChain() []string {
	seen := map[string]bool{}
	var chain []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		chain = append(chain, name)
	}
	current := currentIconTheme()
	add(current)
	for _, name := range inheritsOf(current) {
		add(name)
	}
	add("hicolor")
	return chain
}

func inheritsOf(theme string) []string {
	for _, base := range iconBaseDirs() {
		data, err := os.ReadFile(filepath.Join(base, theme, "index.theme"))
		if err != nil {
			continue
		}
		inTheme := false
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") {
				inTheme = line == "[Icon Theme]"
				continue
			}
			if !inTheme {
				continue
			}
			if key, val, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) == "Inherits" {
				var out []string
				for _, part := range strings.Split(val, ",") {
					out = append(out, strings.TrimSpace(part))
				}
				return out
			}
		}
	}
	return nil
}

func currentIconTheme() string {
	home, _ := os.UserHomeDir()
	for _, cfg := range []string{"gtk-3.0", "gtk-4.0"} {
		data, err := os.ReadFile(filepath.Join(home, ".config", cfg, "settings.ini"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if key, val, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) == "gtk-icon-theme-name" {
				return strings.TrimSpace(val)
			}
		}
	}
	return "hicolor"
}

func iconBaseDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".icons"),
		filepath.Join(xdgDataHome(), "icons"),
	}
	for _, d := range filepath.SplitList(xdgDataDirs()) {
		dirs = append(dirs, filepath.Join(d, "icons"))
	}
	return dirs
}

func applicationDirs() []string {
	dirs := []string{filepath.Join(xdgDataHome(), "applications")}
	for _, d := range filepath.SplitList(xdgDataDirs()) {
		dirs = append(dirs, filepath.Join(d, "applications"))
	}
	return dirs
}

func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func xdgDataDirs() string {
	if v := os.Getenv("XDG_DATA_DIRS"); v != "" {
		return v
	}
	return "/usr/local/share:/usr/share"
}

func steamGameIconPath(id string) string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".steam", "steam", "steam", "games", id+".png"),
		filepath.Join(home, ".local", "share", "Steam", "steam", "games", id+".png"),
		filepath.Join(home, ".steam", "root", "steam", "games", id+".png"),
	}
	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

func steamAppID(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return ""
	}
	for _, kv := range bytes.Split(data, []byte{0}) {
		if bytes.HasPrefix(kv, []byte("SteamAppId=")) {
			return string(kv[len("SteamAppId="):])
		}
	}
	return ""
}

func procExe(pid int) string {
	p, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ""
	}
	return p
}

func pngResource(img image.Image) fyne.Resource {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return fyne.NewStaticResource("icon.png", buf.Bytes())
}

func loadFileResource(path string) fyne.Resource {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return fyne.NewStaticResource(filepath.Base(path), data)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
