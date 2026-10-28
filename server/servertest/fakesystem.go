package servertest

import (
	"math"
	"sync"

	"github.com/elentok/gx/syshealth"
)

// ticksPerRead is how many CPU ticks one FakeSystem reading adds.
const ticksPerRead = 1000

// FakeSystem is a CPU and memory source a test steers between readings. Each
// Read advances the cumulative ticks so the busy percent between two readings
// is the busy value set for that reading.
type FakeSystem struct {
	mu       sync.Mutex
	busy     float64
	queue    []float64
	mem      float64
	pressure int
	idle     uint64
	total    uint64
	reads    int
	err      error
	cores    int
	tickRate int
}

// NewFakeSystem returns a source reporting an idle CPU and no memory use.
func NewFakeSystem() *FakeSystem { return &FakeSystem{cores: 4, tickRate: 100} }

// SetBusy sets the CPU busy percent of every later reading.
func (f *FakeSystem) SetBusy(pct float64) { f.update(func() { f.busy = pct }) }

// QueueBusy makes the next readings use pcts in order, then fall back to SetBusy's value.
func (f *FakeSystem) QueueBusy(pcts ...float64) {
	f.update(func() { f.queue = append(f.queue, pcts...) })
}

// SetMem sets the memory percent used of every later reading.
func (f *FakeSystem) SetMem(pct float64) { f.update(func() { f.mem = pct }) }

// SetPressure sets the macOS pressure level of every later reading; 0 is no
// pressure value, as on Linux.
func (f *FakeSystem) SetPressure(level int) { f.update(func() { f.pressure = level }) }

// Fail makes later readings fail with err; nil makes them succeed again.
// Pass syshealth.ErrUnavailable for a machine with no backend.
func (f *FakeSystem) Fail(err error) { f.update(func() { f.err = err }) }

// Reads counts the readings taken so far, failed ones included.
func (f *FakeSystem) Reads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func (f *FakeSystem) update(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

// Read is the server.Config SystemHealth source.
func (f *FakeSystem) Read() (syshealth.Reading, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if f.err != nil {
		return syshealth.Reading{}, f.err
	}
	busy := f.busy
	if len(f.queue) > 0 {
		busy, f.queue = f.queue[0], f.queue[1:]
	}
	f.total += ticksPerRead
	f.idle += uint64(math.Round((100 - busy) * ticksPerRead / 100))
	const memTotal = 16 << 30
	return syshealth.Reading{
		CPUIdle: f.idle, CPUTotal: f.total, TickRate: f.tickRate, Cores: f.cores,
		MemPercent: f.mem, MemTotal: memTotal, MemUsed: uint64(f.mem / 100 * memTotal),
		Pressure: f.pressure,
	}, nil
}
