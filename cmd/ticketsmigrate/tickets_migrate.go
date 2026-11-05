package ticketsmigrate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/projectlock"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketmigrate"
)

// runTicketsMigrateToStore copies the old tracker tree at oldRoot into the
// ticket store as the project for the repo at cwd (see
// ticketmigrate.MigrateIntoStore). The project is named projectFlag, else after the
// repo directory: for a bare layout, the directory holding .bare.
func runTicketsMigrateToStore(dirs config.Dirs, cwd, oldRoot, projectFlag string, dryRun bool, w io.Writer) error {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}
	cfg, err := dirs.Load()
	if err != nil {
		return err
	}
	if cfg.TicketStore.Path == "" {
		return errors.New("ticket-store.path is not set")
	}

	name := projectFlag
	if name == "" {
		dir := repo.Root
		if filepath.Base(dir) == ".bare" {
			dir = filepath.Dir(dir)
		}
		name = filepath.Base(dir)
	}

	n, err := ticketmigrate.MigrateIntoStore(oldRoot, filepath.Join(cfg.TicketStore.Path, name), name, repo.Root, dryRun)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintf(w, "dry run: would copy %d file(s) into project %q; all tickets valid\n", n, name)
		return nil
	}
	fmt.Fprintf(w, "copied %d file(s) into project %q\n", n, name)
	return nil
}

// runTicketsMigrate runs the migration registry (see migrations.Run) on every
// ticket under scratchRoot and reports what changed, per file, to w. The
// frontmatter migration validates the whole computed result before writing
// anything, so a result that would be invalid leaves every file untouched and
// this returns that validation error unchanged. With dryRun it runs on a copy
// and writes nothing.
func runTicketsMigrate(dirs config.Dirs, scratchRoot string, dryRun bool, w io.Writer) error {
	if dryRun {
		return runTicketsMigrateDry(dirs, scratchRoot, w)
	}
	// Waits for a running migration of the project; the wait hook prints why.
	unlock, err := projectlock.Exclusive(scratchRoot)
	if err != nil {
		return err
	}
	defer unlock()
	// By hand every step reruns, so SkipApplied stays false; the run still records.
	state, err := dirs.StateDir()
	if err != nil {
		return err
	}
	changes, early, err := migrations.Run(migrations.Env{
		Dirs: dirs, Root: scratchRoot, LookupRoot: scratchRoot,
		AppliedPath: filepath.Join(scratchRoot, migrations.AppliedFile),
		StateDir:    state, Pass: &migrations.Pass{},
	})
	if err == nil {
		if clearErr := clearMigrationFailure(dirs, scratchRoot); clearErr != nil {
			return clearErr
		}
	}
	if early {
		return err
	}
	if len(changes) == 0 && err == nil {
		fmt.Fprintln(w, "no changes")
		return nil
	}

	printChanges(w, changes)
	fmt.Fprintf(w, "%d file(s) changed\n", len(changes))
	return err
}

// warnUnknownApplied prints one warning naming the applied-list entries, across
// the given projects, that this binary has no step for. It is advisory: a
// list that cannot be read is left for the run itself to report.
func warnUnknownApplied(w io.Writer, projectDirs ...string) {
	unknown, err := migrations.Default.UnknownApplied(projectDirs...)
	if err != nil || len(unknown) == 0 {
		return
	}
	fmt.Fprintf(w, "warning: applied migrations unknown to this gx (a newer version ran them): %s\n", strings.Join(unknown, ", "))
}

// clearMigrationFailure drops the project's saved failure once its migration
// has passed.
func clearMigrationFailure(dirs config.Dirs, projectDir string) error {
	state, err := dirs.StateDir()
	if err != nil {
		return err
	}
	return migratefail.Clear(state, tickets.ProjectName(projectDir))
}

// runTicketsMigrateDry runs the whole migration chain on a scratch copy of the
// tracker root, so each step reads the previous step's output and the real
// files are never written. Any failure is returned with the real paths in it.
func runTicketsMigrateDry(dirs config.Dirs, scratchRoot string, w io.Writer) error {
	changes, err := DryRunChanges(dirs, scratchRoot, false)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "dry run: every migration step run on a copy, nothing written")
	printChanges(w, changes)
	fmt.Fprintf(w, "%d file(s) would change\n", len(changes))
	return nil
}

// DryRunChanges runs the migration chain on a scratch copy of scratchRoot and
// returns the files it would change, with real paths. With skipApplied the steps
// in the project's applied list are skipped, as the server's own run skips them.
// The list is read from the copy, so a run never touches the real one.
func DryRunChanges(dirs config.Dirs, scratchRoot string, skipApplied bool) ([]tickets.MigrationChange, error) {
	tmp, err := os.MkdirTemp("", "gx-migrate-dry-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	work := filepath.Join(tmp, filepath.Base(scratchRoot))
	if err := copyTracker(scratchRoot, work); err != nil {
		return nil, err
	}
	// The creation step reads the store's git history by path under the store root.
	if _, err := os.Stat(filepath.Join(filepath.Dir(scratchRoot), ".git")); err == nil {
		if err := os.Symlink(filepath.Join(filepath.Dir(scratchRoot), ".git"), filepath.Join(tmp, ".git")); err != nil {
			return nil, err
		}
	}

	state, err := dirs.StateDir()
	if err != nil {
		return nil, err
	}
	env := migrations.Env{Dirs: dirs, Root: work, LookupRoot: scratchRoot, DryRun: true, StateDir: state, Pass: &migrations.Pass{}}
	if skipApplied {
		env.SkipApplied, env.AppliedPath = true, filepath.Join(work, migrations.AppliedFile)
	}
	changes, _, err := migrations.Run(env)
	if err != nil {
		return nil, errors.New(strings.ReplaceAll(err.Error(), work, scratchRoot))
	}
	for i := range changes {
		changes[i].Path = scratchRoot + strings.TrimPrefix(changes[i].Path, work)
	}
	return changes, nil
}

// copyTracker copies the regular files under src into dst, keeping modes and
// modification times (the migration reads mtimes). Symlinks and .git are skipped.
func copyTracker(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
}

// printChanges writes one "<path>: <notes>" line per change.
func printChanges(w io.Writer, changes []tickets.MigrationChange) {
	for _, c := range changes {
		fmt.Fprintf(w, "%s: %s\n", c.Path, strings.Join(c.Notes, ", "))
	}
}
