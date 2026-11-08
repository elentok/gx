package evolve

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/tickets"
)

// Flags of a map's rows. A map has no run log or metrics, so these replace the
// queue flags.
const (
	flagSpecRoundCap = "spec-round-cap"
	flagReReview     = "re-review"
	flagScopeChanged = "scope-changed"
	flagManyRounds   = "many-rounds"
	flagHuhCluster   = "huh-cluster"
)

// Thresholds of the map flags.
const (
	specRoundCap     = 2 // spec-round- tickets in the map
	manyRounds       = 3 // grilling rounds on one ticket
	huhClusterTicket = 2 // Huh? answers on one ticket
	huhClusterMap    = 3 // Huh? answers across the map
)

const (
	specRoundPrefix  = "spec-round-"
	reReviewHeading  = "## Re-review findings"
	scopeChangedMark = "(stories/scope changed)"
)

// HuhCluster lists the Huh? questions of one ticket that sits in a cluster; the
// question text is the skill text that wrote it.
type HuhCluster struct {
	Ticket    string                  `json:"ticket"`
	Questions []conversations.HuhItem `json:"questions"`
}

// applyMapFlags fills a map table's grilling columns, flags and Huh? clusters
// from the epic's tickets and the conversation records.
func applyMapFlags(t evolveTarget, table *InputsTable, recs []conversations.Record) {
	byID := map[string]tickets.Ticket{}
	specRounds := 0
	for _, tk := range t.epic.Tickets {
		byID[tk.Identifier] = tk
		if isSpecRound(tk) {
			specRounds++
		}
	}

	type stats struct {
		rounds, words int
		huh           []conversations.HuhItem
	}
	perTicket := map[string]*stats{}
	statsOf := func(id string) *stats {
		if perTicket[id] == nil {
			perTicket[id] = &stats{}
		}
		return perTicket[id]
	}
	seen := map[string]bool{} // an item's message id counts once across records
	for _, r := range recs {
		prefix := t.project + ":" + t.name + "/"
		id, ok := strings.CutPrefix(r.Ticket, prefix)
		if !ok {
			continue
		}
		s := statsOf(id)
		for _, it := range r.Grilling {
			if seen[it.MessageID] {
				continue
			}
			seen[it.MessageID] = true
			s.words += it.Words
			if it.Questions > 0 {
				s.rounds++
			}
		}
		s.huh = append(s.huh, r.Huh...)
	}

	huhTotal := 0
	huhCount := map[string]int{}
	for i := range table.Rows {
		id := table.Rows[i].ID
		// The ticket log is the count of record; the record backs it up.
		huhCount[id] = max(table.Rows[i].Events[countHuh], len(statsOf(id).huh))
		huhTotal += huhCount[id]
	}

	for i := range table.Rows {
		row := &table.Rows[i]
		tk := byID[row.ID]
		s := statsOf(row.ID)
		row.GrillingRounds, row.Words = s.rounds, s.words
		add := func(cond bool, f string) {
			if cond {
				row.Flags = append(row.Flags, f)
			}
		}
		add(specRounds >= specRoundCap && isSpecRound(tk), flagSpecRoundCap)
		add(strings.Contains(tk.Body, reReviewHeading), flagReReview)
		add(strings.Contains(tk.Body, scopeChangedMark), flagScopeChanged)
		add(row.GrillingRounds >= manyRounds, flagManyRounds)
		cluster := huhCount[row.ID] > 0 && (huhCount[row.ID] >= huhClusterTicket || huhTotal >= huhClusterMap)
		add(cluster, flagHuhCluster)
		if cluster && len(s.huh) > 0 {
			table.HuhClusters = append(table.HuhClusters, HuhCluster{Ticket: row.ID, Questions: s.huh})
		}
	}
}

// printHuhClusters lists each clustered ticket's Huh? question text.
func printHuhClusters(w io.Writer, table InputsTable) error {
	for _, c := range table.HuhClusters {
		if _, err := fmt.Fprintf(w, "huh-cluster %s:\n", c.Ticket); err != nil {
			return err
		}
		for _, q := range c.Questions {
			if _, err := fmt.Fprintf(w, "  - %s\n", strings.Join(strings.Fields(q.Question), " ")); err != nil {
				return err
			}
		}
	}
	return nil
}

// isSpecRound reports whether tk is a spec-round ticket, named by its slug.
func isSpecRound(tk tickets.Ticket) bool {
	_, slug, _ := strings.Cut(filepath.Base(tk.Path), "-")
	return strings.HasPrefix(slug, specRoundPrefix)
}
