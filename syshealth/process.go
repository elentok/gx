package syshealth

// Process is one row of the process table.
type Process struct {
	PID  int
	PPID int
	// StartUnix is the process start in Unix seconds.
	StartUnix int64
	// CPUTicks is cumulative CPU time (user + system) in ticks of the
	// ProcessTable's Reading.TickRate.
	CPUTicks uint64
	// MemBytes is resident memory.
	MemBytes uint64
	Comm     string
	// Argv is empty until a separate argv read fills it.
	Argv []string
	// Unreadable marks a pid that vanished or is hidden; only PID is set.
	Unreadable bool
}

// ProcessTable is one read of every process, with the machine reading taken
// at the same moment, so callers share one window for rows and busy total.
type ProcessTable struct {
	Reading   Reading
	Processes []Process
}
