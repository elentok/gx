//go:build darwin

package servertest

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/elentok/gx/nativerunner"
)

// Build-tagged because unix.SysctlKinfoProc does not exist on Linux (ADR 0035).
func init() { sysctlProcs = darwinTable{} }

type darwinTable struct{}

func (darwinTable) Lookup(pid int) (nativerunner.Process, bool, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if errors.Is(err, unix.EIO) || (err == nil && int(kp.Proc.P_pid) != pid) {
		return nativerunner.Process{}, false, nil
	}
	if err != nil {
		return nativerunner.Process{}, false, err
	}
	st := kp.Proc.P_starttime
	start := strconv.FormatInt(st.Sec, 10) + "." + strconv.FormatInt(int64(st.Usec), 10)
	return nativerunner.Process{Start: start, Cmdline: procArgs(pid)}, true, nil
}

// procArgs is pid's argv joined by spaces, or "" when it can't be read (a
// zombie, another user's process).
func procArgs(pid int) string {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(raw) < 4 {
		return ""
	}
	argc := int(binary.LittleEndian.Uint32(raw[:4]))
	rest := raw[4:]
	// The exec path, then NUL padding, then argc NUL-ended strings.
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return ""
	}
	rest = bytes.TrimLeft(rest[i:], "\x00")
	args := make([]string, 0, argc)
	for range argc {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			break
		}
		args = append(args, string(rest[:j]))
		rest = rest[j+1:]
	}
	return strings.Join(args, " ")
}
