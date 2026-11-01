package migrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tokenledger"
)

// Attribution says which group, project and harness a rebuilt session's tokens
// are counted under. Project is "" for a session with no findable project.
type Attribution struct {
	Group   string
	Project string
	Harness string
}

// Attributions is the attribution of the sessions the data of one project
// names. A session it does not know is Other, no project, Claude (see Of).
type Attributions struct {
	sessions map[string]Attribution
}

// Of is the attribution of session id: what the project's data says, else
// Other with no project on Claude. It never guesses from a folder or a prompt.
func (a Attributions) Of(id string) Attribution {
	if at, ok := a.sessions[id]; ok {
		return at
	}
	return Attribution{Group: tokenledger.GroupOther, Harness: string(harness.Claude)}
}

// NoProject lists, sorted, the known sessions that belong to no project.
func (a Attributions) NoProject() []string {
	var ids []string
	for id, at := range a.sessions {
		if at.Project == "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// InProject lists, sorted, the sessions attributed to project.
func (a Attributions) InProject(project string) []string {
	var ids []string
	for id, at := range a.sessions {
		if at.Project == project {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// attrSessionMeta is the part of a session's meta.json the attribution reads.
type attrSessionMeta struct {
	ID      string `json:"id"`
	Harness string `json:"harness"`
	Project string `json:"project"`
	Ticket  string `json:"ticket"`
	Design  bool   `json:"design"`
}

// Attribute reads the data that survives for the ticket-store project at root,
// strongest source first: ticket session_ids (Queue, One-off or, in a map
// epic, Design), then conversation records, then session meta dirs. A session
// no source names is not listed (see Attributions.Of).
func Attribute(dirs config.Dirs, root string) (Attributions, error) {
	project := tickets.ProjectName(root)
	out := Attributions{sessions: map[string]Attribution{}}
	set := func(id, group, proj, h string) {
		if id == "" {
			return
		}
		if h != string(harness.Codex) {
			h = string(harness.Claude)
		}
		// The first source wins group and project; any source may name Codex.
		if at, ok := out.sessions[id]; ok {
			if h == string(harness.Codex) {
				at.Harness = h
				out.sessions[id] = at
			}
			return
		}
		out.sessions[id] = Attribution{Group: group, Project: proj, Harness: h}
	}

	if err := attributeTickets(root, project, set); err != nil {
		return out, err
	}

	dir, err := conversations.DirIn(dirs)
	if err != nil {
		return out, err
	}
	recs, err := conversations.List(dir)
	if err != nil {
		return out, err
	}
	for _, r := range recs {
		proj := ticketProject(r.Ticket)
		if proj != "" && proj != project {
			continue
		}
		group := tokenledger.GroupOther
		if proj == project && designRecord(root, project, r) {
			group = tokenledger.GroupDesign
		}
		set(r.ID, group, proj, r.Harness)
	}

	data, err := dirs.DataDir()
	if err != nil {
		return out, err
	}
	entries, err := os.ReadDir(filepath.Join(data, "sessions"))
	if err != nil && !os.IsNotExist(err) {
		return out, err
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(data, "sessions", e.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var m attrSessionMeta
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		proj := m.Project
		if proj == "" {
			proj = ticketProject(m.Ticket)
		}
		if proj != "" && proj != project {
			continue
		}
		group := tokenledger.GroupOther
		if m.Design {
			group = tokenledger.GroupDesign
		}
		set(m.ID, group, proj, m.Harness)
	}
	return out, nil
}

func attributeTickets(root, project string, set func(id, group, proj, h string)) error {
	live, err := tickets.Load(root)
	if err != nil {
		return err
	}
	archived, err := tickets.LoadArchived(root)
	if err != nil {
		return err
	}
	for _, epics := range [][]tickets.Epic{live, archived} {
		for _, epic := range epics {
			group := tokenledger.GroupQueue
			switch {
			case epic.IsMap:
				group = tokenledger.GroupDesign
			case epic.Name == tickets.OneOffsEpic || epic.Name == tickets.ArchivedOneOffsEpic:
				group = tokenledger.GroupOneOff
			}
			for _, t := range epic.Tickets {
				st, err := schema.ParseTicket(t.FilePath())
				if err != nil {
					return err
				}
				for _, id := range st.SessionIDs {
					set(id, group, project, "")
				}
			}
		}
	}
	return nil
}

// ticketProject is the project of a "project:epic/id" address, "" when it has
// none.
func ticketProject(address string) string {
	p, _, ok := strings.Cut(address, ":")
	if !ok {
		return ""
	}
	return p
}
