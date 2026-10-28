package syshealth

import "sort"

// topN is the length of each cause list.
const topN = 3

// CauseKind is the kind of a cause row. The values go on the wire.
type CauseKind string

const (
	KindAgent      CauseKind = "agent"
	KindSession    CauseKind = "session"
	KindProcess    CauseKind = "process"
	KindOtherUsers CauseKind = "other-users"
)

const (
	serverLabel      = "gx server"
	unownedClaude    = "claude (not gx)"
	otherUsersLabel  = "other users"
	startMatchSlack  = 1
	reparentedToInit = 1
)

// OwnerKind is what a known pid belongs to.
type OwnerKind int

const (
	OwnerTicket OwnerKind = iota + 1
	OwnerSession
	OwnerServer
)

// Owner is who a known pid belongs to. Name is the ticket address or the
// session name; the server has none.
type Owner struct {
	Kind OwnerKind
	Name string
}

// OwnerKey is a known pid with its start time in Unix seconds. A pid alone
// never matches: pids get reused.
type OwnerKey struct {
	PID       int
	StartUnix int64
}

// Cause is one row of a cause list.
type Cause struct {
	Kind  CauseKind
	Label string
	// Busiest is the display name of the tree's busiest process (agent rows).
	Busiest string
	// Ticket is the ticket address of an agent row.
	Ticket string
	// CPU is the share of the whole machine, in percent.
	CPU      float64
	MemBytes uint64
}

// CauseLists are the top rows by CPU and by memory.
type CauseLists struct {
	CPU    []Cause
	Memory []Cause
}

// group is the processes of one row, summed.
type group struct {
	owner *Owner // nil for a single unowned process
	// procs are the members, in table order.
	procs []proc
	cpu   float64
	mem   uint64
}

type proc struct {
	p   Process
	cpu float64
}

type procKey struct {
	pid   int
	start int64
}

// TopCauses builds the two cause lists from two reads of the process table and
// the pids gx knows. argv is called only for rows that make a list.
func TopCauses(prev, cur ProcessTable, owners map[OwnerKey]Owner, argv func(pid int) []string) CauseLists {
	busy, reason := BusyPercent(&prev.Reading, cur.Reading)
	shares := cpuShares(prev, cur, reason == "")

	byPID := make(map[int]Process, len(cur.Processes))
	for _, p := range cur.Processes {
		if !p.Unreadable {
			byPID[p.PID] = p
		}
	}

	var groups []*group
	byOwner := map[Owner]*group{}
	var readableCPU float64
	for _, p := range cur.Processes {
		if p.Unreadable {
			continue
		}
		cpu := shares[procKey{p.PID, p.StartUnix}]
		readableCPU += cpu
		o := ownerOf(p, byPID, owners)
		var g *group
		if o == nil {
			g = &group{}
			groups = append(groups, g)
		} else if g = byOwner[*o]; g == nil {
			g = &group{owner: o}
			byOwner[*o] = g
			groups = append(groups, g)
		}
		g.procs = append(g.procs, proc{p, cpu})
		g.cpu += cpu
		g.mem += p.MemBytes
	}

	cpuRows := rank(groups, func(g *group) float64 { return g.cpu }, func(g *group) Cause {
		return g.cause(g.cpu, byCPU, argv)
	})
	if reason == "" && hasUnreadable(prev, cur) {
		other := busy - readableCPU
		if other > 0 {
			cpuRows = append(cpuRows, Cause{Kind: KindOtherUsers, Label: otherUsersLabel, CPU: other})
			sort.SliceStable(cpuRows, func(i, j int) bool { return cpuRows[i].CPU > cpuRows[j].CPU })
			if len(cpuRows) > topN {
				cpuRows = cpuRows[:topN]
			}
		}
	}
	memRows := rank(groups, func(g *group) float64 { return float64(g.mem) }, func(g *group) Cause {
		return g.cause(g.cpu, byMem, argv)
	})
	return CauseLists{CPU: cpuRows, Memory: memRows}
}

// cpuShares is each process's share of the machine over the window, keyed by
// pid and start time. A pid whose start time changed has no entry before it, so
// it starts fresh. Without a usable window every share is zero.
func cpuShares(prev, cur ProcessTable, windowOK bool) map[procKey]float64 {
	shares := map[procKey]float64{}
	if !windowOK {
		return shares
	}
	total, ok := tickDelta(prev.Reading.CPUTotal, cur.Reading.CPUTotal, cur.Reading.Wrap32)
	if !ok || total == 0 {
		return shares
	}
	before := make(map[procKey]uint64, len(prev.Processes))
	for _, p := range prev.Processes {
		if !p.Unreadable {
			before[procKey{p.PID, p.StartUnix}] = p.CPUTicks
		}
	}
	for _, p := range cur.Processes {
		k := procKey{p.PID, p.StartUnix}
		old, seen := before[k]
		if p.Unreadable || !seen || p.CPUTicks < old {
			continue
		}
		shares[k] = 100 * float64(p.CPUTicks-old) / float64(total)
	}
	return shares
}

// hasUnreadable reports whether any process was hidden or vanished between the
// reads, so its CPU is missing from the readable sum.
func hasUnreadable(prev, cur ProcessTable) bool {
	now := make(map[int]bool, len(cur.Processes))
	for _, p := range cur.Processes {
		if p.Unreadable {
			return true
		}
		now[p.PID] = true
	}
	for _, p := range prev.Processes {
		if !p.Unreadable && !now[p.PID] {
			return true
		}
	}
	return false
}

// ownerOf walks up the parent pids inside the same read until a known pid
// whose start time matches. A process reparented to pid 1 has lost its link.
//
// The gx server is a known pid but owns only itself: its git and cleanup
// children are other processes with their own rows, so a match on the server
// above the process itself does not count.
func ownerOf(p Process, byPID map[int]Process, owners map[OwnerKey]Owner) *Owner {
	seen := map[int]bool{}
	for cur := p; !seen[cur.PID]; {
		seen[cur.PID] = true
		if o, ok := lookupOwner(cur, owners); ok {
			if o.Kind == OwnerServer && cur.PID != p.PID {
				return nil
			}
			return &o
		}
		if cur.PPID <= reparentedToInit {
			return nil
		}
		parent, ok := byPID[cur.PPID]
		if !ok {
			return nil
		}
		cur = parent
	}
	return nil
}

func lookupOwner(p Process, owners map[OwnerKey]Owner) (Owner, bool) {
	for d := int64(-startMatchSlack); d <= startMatchSlack; d++ {
		if o, ok := owners[OwnerKey{p.PID, p.StartUnix + d}]; ok {
			return o, true
		}
	}
	return Owner{}, false
}

type busiestBy int

const (
	byCPU busiestBy = iota
	byMem
)

// cause turns a group into a row. The busiest process of a tree is the one
// with the most of the list's own metric.
func (g *group) cause(cpu float64, by busiestBy, argv func(int) []string) Cause {
	c := Cause{CPU: cpu, MemBytes: g.mem}
	switch {
	case g.owner == nil:
		p := g.procs[0].p
		p.Argv = argv(p.PID)
		c.Kind = KindProcess
		c.Label = DisplayName(p)
		if IsClaude(p) {
			c.Label = unownedClaude
		}
	case g.owner.Kind == OwnerTicket:
		c.Kind, c.Label, c.Ticket = KindAgent, g.owner.Name, g.owner.Name
		c.Busiest = g.busiestName(by, argv)
	case g.owner.Kind == OwnerSession:
		c.Kind, c.Label = KindSession, g.owner.Name
	default:
		c.Kind, c.Label = KindProcess, serverLabel
	}
	return c
}

func (g *group) busiestName(by busiestBy, argv func(int) []string) string {
	best := g.procs[0]
	for _, q := range g.procs[1:] {
		if by == byCPU && q.cpu > best.cpu || by == byMem && q.p.MemBytes > best.p.MemBytes {
			best = q
		}
	}
	best.p.Argv = argv(best.p.PID)
	return DisplayName(best.p)
}

// rank sorts the groups by metric, biggest first, and builds rows for the top
// ones only, so argv is read for those alone. Zero rows never list.
func rank(groups []*group, metric func(*group) float64, build func(*group) Cause) []Cause {
	sorted := make([]*group, 0, len(groups))
	for _, g := range groups {
		if metric(g) > 0 {
			sorted = append(sorted, g)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return metric(sorted[i]) > metric(sorted[j]) })
	if len(sorted) > topN {
		sorted = sorted[:topN]
	}
	rows := make([]Cause, 0, len(sorted))
	for _, g := range sorted {
		rows = append(rows, build(g))
	}
	return rows
}
