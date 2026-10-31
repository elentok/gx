package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CodexHomeDuplicates returns the paths under codexHome/skills that share a
// name with a gx skill directory in files. Codex reads both that root and
// ~/.agents/skills, so it would find such a skill twice. It only reports;
// nothing is deleted.
func CodexHomeDuplicates(codexHome string, files []SourceFile) []string {
	if codexHome == "" {
		return nil
	}
	seen := map[string]bool{}
	var dups []string
	for _, f := range files {
		name, _, nested := strings.Cut(filepath.ToSlash(f.RelPath), "/")
		if !nested || seen[name] {
			continue
		}
		seen[name] = true
		path := filepath.Join(codexHome, "skills", name)
		if _, err := os.Lstat(path); err == nil {
			dups = append(dups, path)
		}
	}
	sort.Strings(dups)
	return dups
}
