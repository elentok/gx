package triage

import (
	"slices"
	"strconv"
	"strings"
)

// GroupLimitTokens is the combined estimate a group must stay under.
const GroupLimitTokens = 45_000

// parseEstimate reads "12K" or "12000" as tokens; ok is false for anything else.
func parseEstimate(s string) (tokens int, ok bool) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := 1
	if k, found := strings.CutSuffix(s, "K"); found {
		s, mult = k, 1000
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n * mult, true
}

// groupable reports whether a row may join a group: a ready-epic row. Map
// rows never group.
func (r *Row) groupable() bool {
	return !r.gone() && r.Plan != nil && r.Plan.Action == "ready"
}

// assignGroups marks the rows that Plan proposes to promote as one ticket:
// ready rows linked by overlapWith whose estimates all exist and sum to under
// the limit. Any other overlap stays a chain.
func (r *Record) assignGroups() {
	for i := range r.Rows {
		r.Rows[i].Group = ""
	}
	index := map[string]int{}
	parent := make([]int, len(r.Rows))
	for i := range r.Rows {
		parent[i] = i
		index[r.Rows[i].Address] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	for i := range r.Rows {
		if !r.Rows[i].groupable() {
			continue
		}
		for _, other := range r.Rows[i].Plan.OverlapWith {
			if j, ok := index[other]; ok && r.Rows[j].groupable() {
				parent[find(i)] = find(j)
			}
		}
	}
	members := map[int][]int{}
	for i := range r.Rows {
		if r.Rows[i].groupable() {
			members[find(i)] = append(members[find(i)], i)
		}
	}
	for _, idx := range members {
		if len(idx) < 2 {
			continue
		}
		total, all := 0, true
		for _, i := range idx {
			n, ok := parseEstimate(r.Rows[i].Estimate)
			all = all && ok
			total += n
		}
		if !all || total >= GroupLimitTokens {
			continue
		}
		slices.Sort(idx)
		for _, i := range idx {
			r.Rows[i].Group = r.Rows[idx[0]].Address
		}
	}
}

// Row finds a row by address.
func (r *Record) Row(address string) (*Row, error) { return r.row(address) }

// GroupMembers lists the addresses of the rows in a group.
func (r *Record) GroupMembers(group string) []string {
	var out []string
	for _, row := range r.Rows {
		if group != "" && row.Group == group {
			out = append(out, row.Address)
		}
	}
	return out
}

// GroupEdges are the chain edges of a group: the union of its members'
// overlapWith, minus the members themselves.
func (r *Record) GroupEdges(group string) []string {
	members := r.GroupMembers(group)
	var out []string
	for _, row := range r.Rows {
		if row.Group != group || row.Plan == nil {
			continue
		}
		for _, o := range row.Plan.OverlapWith {
			if !slices.Contains(members, o) && !slices.Contains(out, o) {
				out = append(out, o)
			}
		}
	}
	slices.Sort(out)
	return out
}

// Ungroup undoes the group the row is in, at step approve. Its members go back
// to chained rows by their overlapWith.
func (s *Store) Ungroup(id string, revision int, address string) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "ungroup", "approve"); err != nil {
			return err
		}
		row, err := rec.row(address)
		if err != nil {
			return err
		}
		if row.Group == "" {
			return phaseErr("row %s is not in a group", address)
		}
		group := row.Group
		for i := range rec.Rows {
			if rec.Rows[i].Group == group {
				rec.Rows[i].Group = ""
			}
		}
		return nil
	})
}
