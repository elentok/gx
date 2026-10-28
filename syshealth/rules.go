package syshealth

// State is what a Tray meter shows. Values go on the wire as-is.
type State string

const (
	StateNormal   State = "normal"
	StateHigh     State = "high"
	StateCritical State = "critical"
	StateUnknown  State = "unknown"
)

// Reason says why a meter is StateUnknown. Values go on the wire as-is.
type Reason string

const (
	// ReasonWaiting: no second reading yet, or no watched disk yet.
	ReasonWaiting Reason = "waiting"
	// ReasonUnavailable: no backend on this build.
	ReasonUnavailable Reason = "unavailable"
	// ReasonError: a read failed.
	ReasonError Reason = "error"
)

// Fixed thresholds in percent. They are not config keys.
const (
	CPUHigh     = 70
	CPUCritical = 90
	MemHigh     = 80
	MemCritical = 90
)

// AverageWindow is the number of readings in the one-minute average at the
// default 5-second interval. It counts readings, not time, so a short test
// interval gets a short window.
const AverageWindow = 12

// hysteresis is how far below a threshold (or past a level edge) a value must
// move before the state (or level) changes back.
const hysteresis = 5

const (
	levelStep = 20
	maxLevel  = 4
)

// BusyPercent is the CPU busy percent between two readings. A nil prev gives
// ReasonWaiting, as does a window with no ticks in it.
func BusyPercent(prev *Reading, cur Reading) (float64, Reason) {
	if prev == nil {
		return 0, ReasonWaiting
	}
	dIdle, okIdle := tickDelta(prev.CPUIdle, cur.CPUIdle, cur.Wrap32)
	dTotal, okTotal := tickDelta(prev.CPUTotal, cur.CPUTotal, cur.Wrap32)
	if !okIdle || !okTotal || dIdle > dTotal {
		return 0, ReasonError
	}
	if dTotal == 0 {
		return 0, ReasonWaiting
	}
	return 100 * (1 - float64(dIdle)/float64(dTotal)), ""
}

// tickDelta is cur - prev. With 32-bit counters a drop is a wrap; otherwise a
// drop means the counters are not monotonic.
func tickDelta(prev, cur uint64, wrap32 bool) (uint64, bool) {
	if wrap32 {
		return uint64(uint32(cur) - uint32(prev)), true
	}
	if cur < prev {
		return 0, false
	}
	return cur - prev, true
}

// Window keeps the last AverageWindow values.
type Window struct {
	vals []float64
}

// Add appends v and drops the oldest value past AverageWindow.
func (w *Window) Add(v float64) {
	w.vals = append(w.vals, v)
	if len(w.vals) > AverageWindow {
		w.vals = w.vals[len(w.vals)-AverageWindow:]
	}
}

// Average is the mean of the held values, or 0 for none.
func (w *Window) Average() float64 {
	if len(w.vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range w.vals {
		sum += v
	}
	return sum / float64(len(w.vals))
}

// NextState moves prev to the state for avg. A state rises when avg reaches
// its threshold and clears only once avg is 5 points below it. Pass
// StateUnknown (or "") for prev when there is no history.
func NextState(prev State, avg, high, critical float64) State {
	raw := StateNormal
	switch {
	case avg >= critical:
		raw = StateCritical
	case avg >= high:
		raw = StateHigh
	}
	if prev != StateHigh && prev != StateCritical {
		return raw
	}
	if stateRank(raw) >= stateRank(prev) {
		return raw
	}
	if prev == StateCritical {
		if avg >= critical-hysteresis {
			return StateCritical
		}
		prev = StateHigh
	}
	if avg >= high-hysteresis {
		return prev
	}
	return StateNormal
}

func stateRank(s State) int {
	switch s {
	case StateHigh:
		return 1
	case StateCritical:
		return 2
	}
	return 0
}

// NextLevel is the fill (0 to 4) for v in percent. Level L covers 20L to
// 20(L+1); it changes only when v is 5 points past an edge. Pass a negative
// prev when there is no history.
func NextLevel(prev int, v float64) int {
	if prev < 0 || prev > maxLevel {
		prev = min(max(int(v/levelStep), 0), maxLevel)
	}
	for prev < maxLevel && v >= float64(levelStep*(prev+1)+hysteresis) {
		prev++
	}
	for prev > 0 && v < float64(levelStep*prev-hysteresis) {
		prev--
	}
	return prev
}

// Disk is the plain input for one watched disk.
type Disk struct {
	Free, Total uint64
	// Warned and Held are the disk watcher's flags.
	Warned, Held bool
	CleanBelow   uint64
}

// PercentUsed is the used share of the disk, or 0 for an empty Total.
func (d Disk) PercentUsed() float64 {
	if d.Total == 0 || d.Free > d.Total {
		return 0
	}
	return 100 * float64(d.Total-d.Free) / float64(d.Total)
}

// State is critical when held or under the clean threshold, high when warned,
// else normal.
func (d Disk) State() State {
	switch {
	case d.Held || d.Free < d.CleanBelow:
		return StateCritical
	case d.Warned:
		return StateHigh
	}
	return StateNormal
}

// WorstDisk returns the index of the disk with the worst state, ties going to
// the highest percent used. It returns -1 for no disks.
func WorstDisk(disks []Disk) int {
	worst := -1
	for i, d := range disks {
		if worst < 0 {
			worst = i
			continue
		}
		w := disks[worst]
		rd, rw := stateRank(d.State()), stateRank(w.State())
		if rd > rw || (rd == rw && d.PercentUsed() > w.PercentUsed()) {
			worst = i
		}
	}
	return worst
}
