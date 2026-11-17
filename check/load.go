package check

import (
	"runtime"

	"github.com/elentok/gx/brief"
)

// HighLoadPerCore is the 1-minute load average per core above which a time
// limit only warns: a slow run on a busy machine says little about the code,
// and there is no cheap re-run to tell the two apart. The re-run logic reuses it.
const HighLoadPerCore = 1.5

// SystemLoad is the 1-minute load average of the machine; 0 when unknown.
func SystemLoad() float64 { return brief.SystemLoad() }

// highLoad reports whether load is above HighLoadPerCore on this machine.
func highLoad(load float64) bool {
	return load/float64(runtime.NumCPU()) > HighLoadPerCore
}
