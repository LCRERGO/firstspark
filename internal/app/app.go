// Package app wires the configuration, engine and UI together and provides
// the command line interface.
package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/internal/ui"
	"github.com/LCRERGO/firstspark/pkg/cheattable"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/customtype"
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
	export := fs.String("export", "", "export results to a .CT file")
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

	if *showList {
		return listProcesses()
	}
	if _, err := customtype.LoadAndRegister(config.CustomTypesPath()); err != nil {
		return err
	}
	if *pid > 0 {
		return headlessScan(cfg, *pid, *typ, *mode, *value, *value2, *compare, *next, *export)
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

func headlessScan(cfg config.Config, pid int, typ, mode, value, value2, compare, next, export string) error {
	proc, err := mem.Find(pid)
	if err != nil {
		return err
	}
	opts := scan.DefaultOptions()
	opts.Alignment = cfg.Scan.Alignment
	opts.SnapshotLimit = cfg.Scan.SnapshotLimit
	opts.Epsilon = cfg.Scan.FloatEpsilon
	opts.WritableOnly = cfg.Scan.WritableOnly
	opts.MaxResults = cfg.UI.ResultLimit

	if typ == "" {
		typ = cfg.Scan.ValueType
	}
	vt, err := scan.ParseValueType(typ)
	if err != nil {
		return err
	}
	opts.Type = vt

	if mode != "" {
		sm, err := scan.ParseScanMode(mode)
		if err != nil {
			return err
		}
		opts.Mode = sm
	}
	if compare != "" {
		cmp, err := scan.ParseCompareOp(compare)
		if err != nil {
			return err
		}
		opts.Compare = cmp
	}
	if opts.Mode == scan.ModeExact || opts.Mode == scan.ModeIncreasedBy || opts.Mode == scan.ModeDecreasedBy {
		v, err := scan.ParseValue(vt, value)
		if err != nil {
			return err
		}
		opts.Value = v
	}
	if opts.Mode == scan.ModeBetween {
		v, err := scan.ParseValue(vt, value)
		if err != nil {
			return err
		}
		opts.Value = v
		v2, err := scan.ParseValue(vt, value2)
		if err != nil {
			return err
		}
		opts.Value2 = v2
	}

	session := scan.NewSession(proc, opts)
	if err := session.First(context.Background(), nil); err != nil {
		return err
	}
	fmt.Println(i18n.Tf("cli.first_scan", map[string]any{"Count": session.Count()}))

	if next != "" {
		nm, err := scan.ParseScanMode(next)
		if err != nil {
			return err
		}
		session.SetMode(nm)
		if nm == scan.ModeIncreasedBy || nm == scan.ModeDecreasedBy || nm == scan.ModeExact {
			v, err := scan.ParseValue(vt, value)
			if err != nil {
				return err
			}
			session.SetValue(v)
		}
		if nm == scan.ModeBetween {
			v, err := scan.ParseValue(vt, value)
			if err != nil {
				return err
			}
			session.SetValue(v)
			v2, err := scan.ParseValue(vt, value2)
			if err != nil {
				return err
			}
			session.SetValue2(v2)
		}
		if err := session.Next(context.Background(), nil); err != nil {
			return err
		}
		fmt.Println(i18n.Tf("cli.next_scan", map[string]any{"Mode": nm, "Count": session.Count()}))
	}

	results := session.Results()
	limit := cfg.UI.ResultLimit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	for _, r := range results {
		fmt.Printf("0x%x = %s\n", r.Addr, r.Prev.String())
	}

	if export != "" {
		tbl := &cheattable.Table{}
		for _, r := range results {
			tbl.Add("", fmt.Sprintf("0x%x", r.Addr), r.Prev.Type.String(), r.Prev.String())
		}
		if err := tbl.Save(export); err != nil {
			return err
		}
		fmt.Println(i18n.Tf("cli.exported", map[string]any{"Count": len(results), "Path": export}))
	}
	return nil
}
