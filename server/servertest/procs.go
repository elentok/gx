package servertest

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/elentok/gx/nativerunner"
)

// sysctlProcs is set by procs_darwin.go, the only file that cannot compile
// on Linux.
var sysctlProcs nativerunner.ProcessTable

// NewProcessTable reads the kernel's process table in process, so a lookup
// spawns no `ps`. The harness's PTY runner takes it as Pty.Procs.
func NewProcessTable() nativerunner.ProcessTable {
	if runtime.GOOS == "darwin" && sysctlProcs != nil {
		return sysctlProcs
	}
	return procfsTable{root: "/proc"}
}

// procfsTable reads /proc. It compiles everywhere so its parsing is tested
// on every OS.
type procfsTable struct{ root string }

func (t procfsTable) Lookup(pid int) (nativerunner.Process, bool, error) {
	dir := t.root + "/" + strconv.Itoa(pid)
	stat, err := os.ReadFile(dir + "/stat")
	if errors.Is(err, fs.ErrNotExist) {
		return nativerunner.Process{}, false, nil
	}
	if err != nil {
		return nativerunner.Process{}, false, err
	}
	start, ok := procfsStart(stat)
	if !ok {
		return nativerunner.Process{}, false, errors.New("malformed " + dir + "/stat")
	}
	// A zombie or another user's process has no readable cmdline.
	cmdline, _ := os.ReadFile(dir + "/cmdline")
	args := strings.Join(strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00"), " ")
	return nativerunner.Process{Start: start, Cmdline: args}, true, nil
}

// procfsStart is field 22 (starttime) of /proc/<pid>/stat. The comm field
// (2) may contain spaces and parens, so count from the last ")".
func procfsStart(stat []byte) (string, bool) {
	i := bytes.LastIndexByte(stat, ')')
	if i < 0 {
		return "", false
	}
	fields := strings.Fields(string(stat[i+1:]))
	// fields[0] is field 3 (state), so field 22 is fields[19].
	if len(fields) < 20 {
		return "", false
	}
	return fields[19], true
}
