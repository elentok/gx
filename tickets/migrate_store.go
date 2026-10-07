package tickets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets/schema"
)

// storeFile is one file to write into the project: content already converted
// to the store shape, plus the old file it came from (for error messages and
// the claimed check).
type storeFile struct {
	src  string
	rel  string
	data []byte
}

// MigrateIntoStore copies the old .scratch tree at oldRoot (archive and event
// logs included) into projectDir, creating project.json when missing, and
// converts it to the store shape on the way: epic.yaml / map.md become the
// epic's ticket.md, and every ticket gets the legacy frontmatter fixes and
// `type: task` rewritten to `type: implement`. It only reads oldRoot, which
// stays as a backup until cutover.
//
// It is all-or-nothing: any claimed ticket in the old tree, or any file or
// ticket address that already exists in the project, refuses the whole copy
// before a byte is written. That makes a re-run refuse rather than overwrite.
// The converted result is validated project-wide (see ValidateProject) in a
// temp dir before anything is written, so an invalid ticket refuses the whole
// copy. With dryRun it stops there and never touches projectDir.
// Returns the number of files written (or that would be written).
func MigrateIntoStore(oldRoot, projectDir, name, repo string, dryRun bool) (int, error) {
	var files []storeFile
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
		problems = append(problems, migrateFileProblems(path, filepath.Join(projectDir, rel))...)
		if isEpicSidecar(rel) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, storeFile{src: path, rel: rel, data: convertTicketFile(path, raw)})
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("reading old tree %s: %w", oldRoot, err)
	}

	epicFiles, err := convertEpics(oldRoot)
	if err != nil {
		return 0, err
	}
	for _, f := range epicFiles {
		problems = append(problems, migrateFileProblems(f.src, filepath.Join(projectDir, f.rel))...)
	}
	files = append(files, epicFiles...)

	if err := errors.Join(problems...); err != nil {
		return 0, err
	}

	if err := validateStaged(files, projectDir, name, repo); err != nil {
		return 0, err
	}
	if dryRun {
		return len(files), nil
	}

	if err := ensureProjectFile(projectDir, name, repo); err != nil {
		return 0, err
	}
	for _, f := range files {
		if err := writeStoreFile(f, filepath.Join(projectDir, f.rel)); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

// validateStaged writes files into a temp project and validates it. Errors
// name tickets by the path they would have in projectDir, not the temp dir.
func validateStaged(files []storeFile, projectDir, name, repo string) error {
	tmp, err := os.MkdirTemp("", "gx-migrate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := ensureProjectFile(tmp, name, repo); err != nil {
		return err
	}
	for _, f := range files {
		if err := writeStoreFile(f, filepath.Join(tmp, f.rel)); err != nil {
			return err
		}
	}
	if err := ValidateProject(tmp); err != nil {
		return errors.New(strings.ReplaceAll(err.Error(), tmp, projectDir))
	}
	return nil
}

// isEpicSidecar reports whether rel is an epic.yaml or map.md directly under
// an epic directory: the old-shape files that ticket.md replaces.
func isEpicSidecar(rel string) bool {
	dir, base := filepath.Split(rel)
	if dir == "" || strings.Contains(strings.Trim(dir, string(filepath.Separator)), string(filepath.Separator)) {
		return false
	}
	return base == "epic.yaml" || base == "map.md"
}

// convertTicketFile returns raw with the legacy fixes applied and type: task
// rewritten, or raw untouched when path isn't a ticket, doesn't parse, or
// already has the new shape.
func convertTicketFile(path string, raw []byte) []byte {
	if _, _, _, ok := parseTicketFilename(filepath.Base(path)); !ok {
		return raw
	}
	if parent := filepath.Base(filepath.Dir(path)); parent != "issues" && parent != ".archive" {
		return raw
	}
	old, err := schema.ParseTicketRaw(string(raw), path)
	if err != nil {
		return raw
	}
	t, notes := migrateTicket(old)
	if t.Type == schema.TypeTask {
		t.Type = schema.TypeImplement
		notes = append(notes, "type: task -> implement")
	}
	if len(notes) == 0 {
		return raw
	}
	out, err := schema.MarshalTicket(t, schema.ParseBody(string(raw)))
	if err != nil {
		return raw
	}
	return out
}

// convertEpics builds a ticket.md for every epic that still has an epic.yaml
// or map.md and no ticket.md yet. The map's text becomes the body and the epic
// is marked kind: map; the sidecar's timestamps carry over, and a completed
// epic is marked done.
func convertEpics(oldRoot string) ([]storeFile, error) {
	entries, err := os.ReadDir(oldRoot)
	if err != nil {
		return nil, err
	}
	var out []storeFile
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(oldRoot, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "ticket.md")); err == nil {
			continue
		}
		sidecar, sidecarErr := os.ReadFile(filepath.Join(dir, "epic.yaml"))
		body, bodyErr := os.ReadFile(filepath.Join(dir, "map.md"))
		if sidecarErr != nil && bodyErr != nil {
			continue
		}
		data, err := epicTicketMD(sidecar, body)
		if err != nil {
			return nil, fmt.Errorf("converting epic %s: %w", dir, err)
		}
		out = append(out, storeFile{src: dir, rel: filepath.Join(e.Name(), "ticket.md"), data: data})
	}
	return out, nil
}

func epicTicketMD(sidecar, body []byte) ([]byte, error) {
	var wire epicYAML
	if err := yaml.Unmarshal(sidecar, &wire); err != nil {
		return nil, err
	}
	fm := struct {
		Kind        string     `yaml:"kind,omitempty"`
		Status      string     `yaml:"status"`
		StartedAt   *time.Time `yaml:"started_at,omitempty"`
		CompletedAt *time.Time `yaml:"completed_at,omitempty"`
	}{Status: string(schema.StatusOpen), StartedAt: wire.StartedAt, CompletedAt: wire.CompletedAt}
	if wire.CompletedAt != nil {
		fm.Status = string(schema.StatusDone)
	}
	if body != nil {
		fm.Kind = KindMap
	}
	head, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(head) + "---\n" + string(body)), nil
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

func writeStoreFile(f storeFile, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(dst, f.data); err != nil {
		return fmt.Errorf("copying %s: %w", f.src, err)
	}
	return nil
}
