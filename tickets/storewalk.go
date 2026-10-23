package tickets

// StoreEpic is one epic of the ticket store with the project it belongs to.
// Archived is true for an epic under the project's ".archive/" directory.
type StoreEpic struct {
	Project  string
	Archived bool
	Epic     Epic
}

// LoadStoreEpics returns every epic of every project in the ticket store:
// live epics (which include follow-ups-archived and one-offs-archived, being
// ordinary epic directories) and each project's ".archive/" epics. Unlike the
// listing walkers, it hides nothing.
func LoadStoreEpics(storePath string) ([]StoreEpic, error) {
	dirs, err := ProjectDirs(storePath)
	if err != nil {
		return nil, err
	}
	var out []StoreEpic
	for _, dir := range dirs {
		project := ProjectName(dir)
		live, err := Load(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range live {
			out = append(out, StoreEpic{Project: project, Epic: e})
		}
		archived, err := LoadArchived(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range archived {
			out = append(out, StoreEpic{Project: project, Archived: true, Epic: e})
		}
	}
	return out, nil
}
