// Package mem provides Linux process introspection and cross-process memory
// access built on process_vm_readv(2) and process_vm_writev(2).
package mem

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Process describes a single running process.
type Process struct {
	PID     int
	PPID    int
	Name    string
	Cmdline string
	UID     int
	Exe     string
}

// List returns every process visible in /proc, sorted by PID.
func List() ([]Process, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("mem: read /proc: %w", err)
	}
	procs := make([]Process, 0, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		p, err := Find(pid)
		if err != nil {
			continue
		}
		procs = append(procs, *p)
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].PID < procs[j].PID })
	return procs, nil
}

// Find loads the process metadata for pid from /proc.
func Find(pid int) (*Process, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("mem: invalid pid %d", pid)
	}
	base := fmt.Sprintf("/proc/%d", pid)
	if _, err := os.Stat(base); err != nil {
		return nil, fmt.Errorf("mem: pid %d: %w", pid, err)
	}
	p := &Process{PID: pid}
	if b, err := os.ReadFile(filepath.Join(base, "comm")); err == nil {
		p.Name = strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile(filepath.Join(base, "cmdline")); err == nil {
		p.Cmdline = strings.Join(strings.Split(strings.TrimRight(string(b), "\x00"), "\x00"), " ")
	}
	if b, err := os.ReadFile(filepath.Join(base, "status")); err == nil {
		p.UID = parseUID(string(b))
	}
	if b, err := os.ReadFile(filepath.Join(base, "stat")); err == nil {
		p.PPID = parsePPID(string(b))
	}
	if link, err := os.Readlink(filepath.Join(base, "exe")); err == nil {
		p.Exe = link
	}
	return p, nil
}

// Exists reports whether the process is still alive.
func (p *Process) Exists() bool {
	if p == nil || p.PID <= 0 {
		return false
	}
	_, err := os.Stat(fmt.Sprintf("/proc/%d", p.PID))
	return err == nil
}

// parsePPID extracts the parent pid from /proc/<pid>/stat. The comm field can
// contain spaces and parentheses, so parsing starts after the last ')'.
func parsePPID(stat string) int {
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return 0
	}
	fields := strings.Fields(stat[i+1:])
	if len(fields) < 2 {
		return 0
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return ppid
}

func parseUID(status string) int {
	for _, line := range strings.Split(status, "\n") {
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if uid, err := strconv.Atoi(fields[1]); err == nil {
				return uid
			}
		}
	}
	return -1
}
