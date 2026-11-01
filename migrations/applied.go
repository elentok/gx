package migrations

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// AppliedFile is the name of a project's applied list inside the project
// directory: one step name per line, append-only, blank lines and lines
// starting with "#" ignored. It is a file, not a directory, so tickets.Load
// (which only reads directories) never mistakes it for an epic. Only
// migration code writes it.
const AppliedFile = "applied-migrations.txt"

// readApplied returns the step names in the list at path. A missing file is an
// empty list.
func readApplied(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var names []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		names = append(names, line)
	}
	return names, sc.Err()
}

// recordApplied appends name to the list at path unless it is already there.
func recordApplied(path, name string) error {
	names, err := readApplied(path)
	if err != nil {
		return err
	}
	if slices.Contains(names, name) {
		return nil
	}
	prefix := ""
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = "\n"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(prefix + name + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// WriteFullApplied gives a newly created project directory a list that already
// holds every registered step, so the project is not migrated for nothing. Only
// creation paths call it; an existing project keeps the list it has.
func (r *Registry) WriteFullApplied(projectDir string) error {
	for _, name := range r.Names() {
		if err := recordApplied(filepath.Join(projectDir, AppliedFile), name); err != nil {
			return err
		}
	}
	return nil
}

// WriteFullApplied writes the Default registry's full list into projectDir.
func WriteFullApplied(projectDir string) error {
	return Default.WriteFullApplied(projectDir)
}

// UnknownApplied returns the sorted names, across the lists in the given
// project directories, that no registered step has. Callers warn once per run
// with the result; the names stay in the files.
func (r *Registry) UnknownApplied(projectDirs ...string) ([]string, error) {
	known := r.Names()
	var unknown []string
	for _, dir := range projectDirs {
		names, err := readApplied(dir + string(os.PathSeparator) + AppliedFile)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if !slices.Contains(known, n) && !slices.Contains(unknown, n) {
				unknown = append(unknown, n)
			}
		}
	}
	slices.Sort(unknown)
	return unknown, nil
}
