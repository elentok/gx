package projects

import (
	"bytes"
	"context"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

const (
	defaultMigrationFailurePoll = 2 * time.Second
)

// Failures reads the saved failed migrations of the projects now in the ticket
// store. The CLI writes the record, so each call reads the file.
func (m *Migrator) Failures() []wire.MigrationFailure {
	dirs, err := tickets.ProjectDirs(m.cfg().TicketStore)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(dirs))
	dirOf := make(map[string]string, len(dirs))
	for _, d := range dirs {
		name := tickets.ProjectName(d)
		names = append(names, name)
		dirOf[name] = d
	}
	recorded, err := migratefail.Read(m.cfg().StateDir, names)
	if err != nil {
		m.log.Warn("read migration failures", "err", err)
		return nil
	}
	out := make([]wire.MigrationFailure, 0, len(recorded))
	for _, f := range recorded {
		out = append(out, wire.MigrationFailure{Project: f.Project, Error: f.Error, RetryCommand: "gx tickets migrate '" + strings.ReplaceAll(dirOf[f.Project], "'", `'\''`) + "'"})
	}
	return out
}

func (m *Migrator) readFailureFile() []byte {
	data, _ := os.ReadFile(migratefail.Path(m.cfg().StateDir))
	return data
}

// keepMigrationFailuresWatched announces a change to the record file until ctx
// ends. The baseline is an empty file, not a read at startup: a write racing
// the watcher's start would be taken as the baseline and never announced, and
// a record that already exists costs one harmless event. It polls: the writers
// are other processes, and a rename over the file is the only change they make.
func (m *Migrator) KeepFailuresWatched(ctx context.Context) {
	interval := m.cfg().MigrationFailurePollInterval
	if interval <= 0 {
		interval = defaultMigrationFailurePoll
	}
	read := m.readFailureFile
	var last []byte
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if cur := read(); !bytes.Equal(cur, last) {
				last = cur
				m.back.FailuresChanged()
			}
		}
	}
}
