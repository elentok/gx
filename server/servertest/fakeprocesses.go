package servertest

import (
	"sync"

	"github.com/elentok/gx/syshealth"
)

// FakeProcesses is a process table source a test steers between reads. The
// CPUTicks of a process passed to Set is what it uses in each read; the fake
// adds them up, so the shares between two reads are those values over
// ticksPerRead.
type FakeProcesses struct {
	mu      sync.Mutex
	procs   []syshealth.Process
	cum     map[int]uint64
	idle    uint64
	total   uint64
	reads   int
	err     error
	release chan struct{}
}

// NewFakeProcesses returns a source with an empty table.
func NewFakeProcesses() *FakeProcesses { return &FakeProcesses{cum: map[int]uint64{}} }

// Set replaces the table every later read returns.
func (f *FakeProcesses) Set(procs ...syshealth.Process) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.procs = procs
}

// Fail makes later reads fail with err; nil makes them succeed again.
func (f *FakeProcesses) Fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// Block makes later reads wait until Release.
func (f *FakeProcesses) Block() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.release = make(chan struct{})
}

// Release lets blocked reads go on and stops blocking.
func (f *FakeProcesses) Release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.release != nil {
		close(f.release)
		f.release = nil
	}
}

// Reads counts the reads started so far, failed and blocked ones included.
func (f *FakeProcesses) Reads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

// Read is the server.Config ProcessSource.
func (f *FakeProcesses) Read() (syshealth.ProcessTable, error) {
	f.mu.Lock()
	f.reads++
	release := f.release
	f.mu.Unlock()
	if release != nil {
		<-release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return syshealth.ProcessTable{}, f.err
	}
	var used uint64
	out := make([]syshealth.Process, len(f.procs))
	for i, p := range f.procs {
		used += p.CPUTicks
		f.cum[p.PID] += p.CPUTicks
		p.CPUTicks = f.cum[p.PID]
		out[i] = p
	}
	f.total += ticksPerRead
	f.idle += ticksPerRead - min(used, ticksPerRead)
	return syshealth.ProcessTable{
		Reading:   syshealth.Reading{CPUIdle: f.idle, CPUTotal: f.total, TickRate: 100, Cores: 4},
		Processes: out,
	}, nil
}
