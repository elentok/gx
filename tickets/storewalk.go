package tickets

// StoreEpic is one epic of the ticket store with the project it belongs to.
// Archived is true for an epic under the project's ".archive/" directory.
type StoreEpic struct {
	Project  string
	Archived bool
	Epic     Epic
}

// backfillEpics calls step on every non-map epic of the project at root, live
// and archived, and collects the changes it reports (also on error, so a
// partial run still reports what it wrote). step may fill in the epic's
// tickets in memory for the steps that follow it on the same epic.
func backfillEpics(root string, step func(epic *Epic) ([]MigrationChange, error)) ([]MigrationChange, error) {
	live, err := Load(root)
	if err != nil {
		return nil, err
	}
	archived, err := LoadArchived(root)
	if err != nil {
		return nil, err
	}
	var changes []MigrationChange
	for _, epics := range [][]Epic{live, archived} {
		for i := range epics {
			if epics[i].IsMap {
				continue
			}
			got, err := step(&epics[i])
			changes = append(changes, got...)
			if err != nil {
				return changes, err
			}
		}
	}
	return changes, nil
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
