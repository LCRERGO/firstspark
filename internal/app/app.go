// Package app wires the configuration, engine and UI together and provides
// the command line interface.
package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/internal/ui"
	"github.com/LCRERGO/firstspark/pkg/cheattable"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/customtype"
	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// Version is the application version.
const Version = "0.2.0"

// Run parses args and dispatches to the GUI or a headless command.
func Run(args []string) error {
	fs := flag.NewFlagSet("firstspark", flag.ContinueOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "configuration file")
	showList := fs.Bool("list", false, "list processes and exit")
	pid := fs.Int("pid", 0, "target pid for a headless scan")
	typ := fs.String("type", "", "value type (byte|word|dword|qword|float|double|string|aob)")
	mode := fs.String("mode", "", "scan mode (exact|unknown)")
	value := fs.String("value", "", "value to scan for")
	value2 := fs.String("value2", "", "upper bound for a between scan")
	compare := fs.String("compare", "", "comparison operator (== != > >= < <=)")
	next := fs.String("next", "", "next scan mode (changed|unchanged|increased|decreased|...)")
	exec := fs.String("exec", "", "executable region filter (any|only|non)")
	cow := fs.Bool("cow", false, "scan copy-on-write regions only")
	start := fs.String("start", "", "scan range start address (hex)")
	stop := fs.String("stop", "", "scan range stop address (hex)")
	export := fs.String("export", "", "export results to a .CT file")
	logLevel := fs.String("log-level", "", "log level (debug|info|warn|error)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println("firstspark", Version)
		return nil
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	if err := i18n.Init(cfg.UI.Language); err != nil {
		return err
	}

	level := cfg.Log.Level
	if *logLevel != "" {
		level = *logLevel
	}
	logPath := cfg.Log.File
	if logPath == "" {
		logPath = config.LogPath()
	}
	headless := *showList || *pid > 0
	if err := log.Setup(level, logPath, headless || os.Getenv("DISPLAY") == ""); err != nil {
		fmt.Fprintln(os.Stderr, "firstspark: log:", err)
	}
	defer log.Close()
	log.Info("firstspark starting", "version", Version, "level", level, "log", logPath)

	if *showList {
		return listProcesses()
	}
	if _, err := customtype.LoadAndRegister(config.CustomTypesPath()); err != nil {
		return err
	}
	if *pid > 0 {
		return headlessScan(cfg, *pid, scanFlags{
			typ: *typ, mode: *mode, value: *value, value2: *value2, compare: *compare,
			next: *next, exec: *exec, start: *start, stop: *stop, cow: *cow, export: *export,
		})
	}
	return ui.Run(cfg)
}

func listProcesses() error {
	procs, err := mem.List()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PID\tUID\tNAME\tCMDLINE")
	for _, p := range procs {
		cmd := p.Cmdline
		if len(cmd) > 80 {
			cmd = cmd[:80] + "..."
		}
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", p.PID, p.UID, p.Name, cmd)
	}
	return w.Flush()
}

// scanFlags carries the headless scan command line options.
type scanFlags struct {
	typ, mode, value, value2, compare, next string
	exec, start, stop                       string
	cow                                     bool
	export                                  string
}

func headlessScan(cfg config.Config, pid int, f scanFlags) error {
	proc, err := mem.Find(pid)
	if err != nil {
		return err
	}
	log.Info("headless scan", "pid", pid, "type", f.typ, "mode", f.mode, "next", f.next)
	opts, vt, err := headlessOptions(cfg, f)
	if err != nil {
		return err
	}

	session := scan.NewSession(proc, opts)
	if err := session.First(context.Background(), nil); err != nil {
		return err
	}
	fmt.Println(i18n.Tf("cli.first_scan", map[string]any{"Count": session.Count()}))

	if f.next != "" {
		if err := headlessNext(session, opts, vt, f); err != nil {
			return err
		}
	}

	results := session.Results()
	if limit := cfg.UI.ResultLimit; limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	printResults(results)
	if f.export != "" {
		return exportResults(results, f.export)
	}
	return nil
}

// headlessOptions turns the CLI flags into scan options and the resolved value
// type (used again by the --next scan).
func headlessOptions(cfg config.Config, f scanFlags) (scan.Options, scan.ValueType, error) {
	opts := scan.DefaultOptions()
	opts.Alignment = cfg.Scan.Alignment
	opts.SnapshotLimit = cfg.Scan.SnapshotLimit
	opts.Epsilon = cfg.Scan.FloatEpsilon
	opts.WritableOnly = cfg.Scan.WritableOnly
	opts.MaxResults = cfg.UI.ResultLimit

	if f.typ == "" {
		f.typ = cfg.Scan.ValueType
	}
	vt, err := scan.ParseValueType(f.typ)
	if err != nil {
		return opts, vt, err
	}
	opts.Type = vt
	if err := applyScanFlags(&opts, f); err != nil {
		return opts, vt, err
	}
	if err := applyScanValue(&opts, vt, f); err != nil {
		return opts, vt, err
	}
	return opts, vt, nil
}

// applyScanFlags applies the mode, comparison and region-filter flags.
func applyScanFlags(opts *scan.Options, f scanFlags) error {
	if f.mode != "" {
		sm, err := scan.ParseScanMode(f.mode)
		if err != nil {
			return err
		}
		opts.Mode = sm
	}
	if f.compare != "" {
		cmp, err := scan.ParseCompareOp(f.compare)
		if err != nil {
			return err
		}
		opts.Compare = cmp
	}
	if f.exec != "" {
		em, err := scan.ParseExecutableMode(f.exec)
		if err != nil {
			return err
		}
		opts.Executable = em
	}
	opts.CopyOnWrite = f.cow
	start, err := parseHexAddr(f.start)
	if err != nil {
		return err
	}
	stop, err := parseHexAddr(f.stop)
	if err != nil {
		return err
	}
	opts.Start, opts.Stop = start, stop
	return nil
}

// applyScanValue parses the value (and between bound) for the selected mode.
func applyScanValue(opts *scan.Options, vt scan.ValueType, f scanFlags) error {
	switch {
	case opts.Type == scan.TypeGrouped:
		gp, err := scan.ParseGrouped(f.value)
		if err != nil {
			return err
		}
		opts.Grouped = gp
	case modeTakesValue(opts.Mode):
		v, err := scan.ParseValue(vt, f.value)
		if err != nil {
			return err
		}
		opts.Value = v
	}
	if opts.Mode == scan.ModeBetween {
		v, err := scan.ParseValue(vt, f.value)
		if err != nil {
			return err
		}
		opts.Value = v
		v2, err := scan.ParseValue(vt, f.value2)
		if err != nil {
			return err
		}
		opts.Value2 = v2
	}
	return nil
}

// headlessNext applies and runs the --next scan.
func headlessNext(session *scan.Session, opts scan.Options, vt scan.ValueType, f scanFlags) error {
	nm, err := scan.ParseScanMode(f.next)
	if err != nil {
		return err
	}
	session.SetMode(nm)
	if modeTakesValue(nm) && opts.Type != scan.TypeGrouped {
		v, err := scan.ParseValue(vt, f.value)
		if err != nil {
			return err
		}
		session.SetValue(v)
	}
	if nm == scan.ModeBetween {
		v, err := scan.ParseValue(vt, f.value)
		if err != nil {
			return err
		}
		session.SetValue(v)
		v2, err := scan.ParseValue(vt, f.value2)
		if err != nil {
			return err
		}
		session.SetValue2(v2)
	}
	if err := session.Next(context.Background(), nil); err != nil {
		return err
	}
	fmt.Println(i18n.Tf("cli.next_scan", map[string]any{"Mode": nm, "Count": session.Count()}))
	return nil
}

func printResults(results []scan.Result) {
	for _, r := range results {
		fmt.Printf("0x%x = %s\n", r.Addr, r.Value.String())
	}
}

func exportResults(results []scan.Result, path string) error {
	tbl := &cheattable.Table{}
	for _, r := range results {
		tbl.Add("", fmt.Sprintf("0x%x", r.Addr), r.Value.Type.String(), r.Value.String())
	}
	if err := tbl.Save(path); err != nil {
		return err
	}
	fmt.Println(i18n.Tf("cli.exported", map[string]any{"Count": len(results), "Path": path}))
	return nil
}

// modeTakesValue reports whether a mode consumes the scan value.
func modeTakesValue(m scan.ScanMode) bool {
	switch m {
	case scan.ModeExact, scan.ModeBigger, scan.ModeSmaller, scan.ModeIncreasedBy, scan.ModeDecreasedBy:
		return true
	default:
		return false
	}
}

// parseHexAddr parses an optional hexadecimal address; an empty string is 0.
func parseHexAddr(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid address %q", s)
	}
	return n, nil
}
