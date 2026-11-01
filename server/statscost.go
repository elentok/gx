package server

import "github.com/elentok/gx/tokenledger"

// Cost sources for one day of the cost charts. A day uses exactly one.
const (
	costSourceLive    = "live"
	costSourceRebuilt = "rebuilt"
	costSourceStamp   = "stamp"
)

// costSourceInput is everything pickCostSource needs. Days are local-day labels
// in tokenledger.DayLayout, oldest first; Design and Impl have one entry per day.
type costSourceInput struct {
	Days         []string
	Rows         []tokenledger.Row
	FirstRebuilt string // "" when the ledger has no rebuilt day
	FirstLive    string // "" when the ledger has no live day
	Design, Impl costSeries
	Project      string // "" is All projects, the only filter that sees "no project"
}

// harnessCost is one harness's share of a group's day.
type harnessCost struct {
	Tokens  int64
	Dollars float64
}

// groupCost is one agent group's day. Harness is empty on stamp days: stamps
// do not record it.
type groupCost struct {
	Tokens, Dollars StatsAmount
	Harness         map[string]harnessCost
}

// dayCost is one day of the cost charts, from a single source.
type dayCost struct {
	Source string
	Groups map[string]groupCost
}

var costGroups = []string{
	tokenledger.GroupQueue, tokenledger.GroupDesign, tokenledger.GroupOneOff, tokenledger.GroupOther,
}

// pickCostSource picks one source per day: live rows from the first live day,
// rebuilt rows from the first rebuilt day up to the first live day, ticket
// stamps otherwise. Inside the live and rebuilt ranges a day without rows is a
// zero, never no-data. Day labels compare as strings (ISO dates).
func pickCostSource(in costSourceInput) []dayCost {
	byDay := map[string][]tokenledger.Row{}
	for _, r := range in.Rows {
		if in.Project == "" || r.Project == in.Project {
			byDay[r.Day] = append(byDay[r.Day], r)
		}
	}
	designTokens, designSpend := in.Design.amounts()
	implTokens, implSpend := in.Impl.amounts()

	out := make([]dayCost, len(in.Days))
	for i, day := range in.Days {
		d := dayCost{Groups: map[string]groupCost{}}
		switch {
		case in.FirstLive != "" && day >= in.FirstLive:
			d.Source = costSourceLive
		case in.FirstRebuilt != "" && day >= in.FirstRebuilt:
			d.Source = costSourceRebuilt
		default:
			d.Source = costSourceStamp
		}
		if d.Source == costSourceStamp {
			for _, g := range costGroups {
				d.Groups[g] = groupCost{}
			}
			d.Groups[tokenledger.GroupDesign] = groupCost{Tokens: designTokens[i], Dollars: designSpend[i]}
			d.Groups[tokenledger.GroupQueue] = groupCost{Tokens: implTokens[i], Dollars: implSpend[i]}
		} else {
			d.Groups = ledgerGroups(byDay[day])
		}
		out[i] = d
	}
	return out
}

// ledgerGroups sums one day's rows per group, with every group present.
func ledgerGroups(rows []tokenledger.Row) map[string]groupCost {
	groups := make(map[string]groupCost, len(costGroups))
	for _, g := range costGroups {
		groups[g] = groupCost{Harness: map[string]harnessCost{}}
	}
	for _, r := range rows {
		g, ok := groups[r.Group]
		if !ok {
			g, r.Group = groups[tokenledger.GroupOther], tokenledger.GroupOther
		}
		g.Tokens.Value += float64(r.Tokens)
		g.Dollars.Value += r.Dollars
		h := g.Harness[r.Harness]
		h.Tokens += r.Tokens
		h.Dollars += r.Dollars
		g.Harness[r.Harness] = h
		groups[r.Group] = g
	}
	return groups
}
