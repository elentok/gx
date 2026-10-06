package tickets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets/schema"
)

// MigrateIntoStore copies the old .scratch tree at oldRoot (archive and event
// logs included) into projectDir, creating project.json when missing. It only
// reads oldRoot, which stays as a backup until cutover.
//
// It is all-or-nothing: any claimed ticket in the old tree, or any file or
// ticket address that already exists in the project, refuses the whole copy
// before a byte is written. That makes a re-run refuse rather than overwrite.
// Returns the number of files copied.
func MigrateIntoStore(oldRoot, projectDir, name, repo string) (int, error) {
	var files []string
	var problems []error

	err := filepath.WalkDir(oldRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(oldRoot, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		problems = append(problems, migrateFileProblems(path, filepath.Join(projectDir, rel))...)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("reading old tree %s: %w", oldRoot, err)
	}
	if err := errors.Join(problems...); err != nil {
		return 0, err
	}

	if err := ensureProjectFile(projectDir, name, repo); err != nil {
		return 0, err
	}
	for _, rel := range files {
		if err := copyFileInto(filepath.Join(oldRoot, rel), filepath.Join(projectDir, rel)); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

// migrateFileProblems lists why src can't be copied to dst: dst already
// exists, another ticket in dst's directory has the same address, or src is a
// claimed ticket.
func migrateFileProblems(src, dst string) []error {
	var problems []error
	if _, err := os.Lstat(dst); err == nil {
		problems = append(problems, fmt.Errorf("already exists, refusing to overwrite: %s", dst))
	}

	_, id, _, isTicket := parseTicketFilename(filepath.Base(src))
	if !isTicket || filepath.Base(filepath.Dir(src)) != "issues" {
		return problems
	}

	if existing, _ := filepath.Glob(filepath.Join(filepath.Dir(dst), id+"-*.md")); len(existing) > 0 && existing[0] != dst {
		problems = append(problems, fmt.Errorf("ticket address already exists, refusing to overwrite: %s", existing[0]))
	}
	if raw, err := os.ReadFile(src); err == nil {
		if t, err := schema.ParseTicketRaw(string(raw), src); err == nil && t.Ticket.Status == schema.StatusClaimed {
			problems = append(problems, fmt.Errorf("ticket is claimed, release it first: %s", src))
		}
	}
	return problems
}

func ensureProjectFile(projectDir, name, repo string) error {
	if _, err := config.ReadProjectFile(projectDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := json.MarshalIndent(config.ProjectFile{Name: &name, Repo: &repo}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(projectDir, config.ProjectFileName), append(data, '\n'))
}

func copyFileInto(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(dst, data); err != nil {
		return fmt.Errorf("copying %s: %w", src, err)
	}
	return nil
}
