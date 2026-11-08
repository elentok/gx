package main

import (
	"path/filepath"

	triagerec "github.com/elentok/gx/triage"
)

// seedTriage writes the scenario's Triage records with the triage package, so a
// seeded record is one the real verbs read.
func (f *fixture) seedTriage(sc scenario) error {
	for _, s := range sc.Triage {
		store := triagerec.NewStore(filepath.Join(f.store, s.Project))
		rec, err := store.Create()
		if err != nil {
			return err
		}
		s.Edit(rec)
		if err := store.Save(rec); err != nil {
			return err
		}
	}
	return nil
}
