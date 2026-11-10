package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ThirdPartySourcePrefix starts a Skill source install's manifest Source; the
// source folder's real path follows it.
const ThirdPartySourcePrefix = "skill source: "

// ThirdPartyRoot returns the real path of the source a manifest installs, and
// false when m is not a Skill source install.
func ThirdPartyRoot(m Manifest) (string, bool) {
	root, ok := strings.CutPrefix(m.Source, ThirdPartySourcePrefix)
	return root, ok && root != ""
}

// SourceID names a Skill source's manifest, derived from its real path. It is
// stable and safe as a file name.
func SourceID(realDir string) string {
	sum := sha256.Sum256([]byte(realDir))
	return "source-" + hex.EncodeToString(sum[:8])
}

// ScanSource returns the names of dir's direct subfolders that hold a
// SKILL.md, sorted. A symlink to a folder counts, and so does a symlinked
// SKILL.md. Loose files and folders without SKILL.md are skipped.
func ScanSource(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		sub, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil || !sub.IsDir() {
			continue
		}
		marker, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md"))
		if err != nil || !marker.Mode().IsRegular() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// SourceSkillFiles lists the files to install for the skills of a source.
// Link mode needs only each skill's SKILL.md, which names the folder to link.
// Copy mode lists every regular file under each skill folder.
func SourceSkillFiles(dir string, names []string, copyFiles bool) ([]SourceFile, error) {
	var files []SourceFile
	for _, name := range names {
		skillDir := filepath.Join(dir, name)
		if !copyFiles {
			files = append(files, SourceFile{RelPath: filepath.Join(name, "SKILL.md"), AbsPath: filepath.Join(skillDir, "SKILL.md")})
			continue
		}
		// The trailing separator makes WalkDir follow a symlinked skill folder.
		err := filepath.WalkDir(skillDir+string(filepath.Separator), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(skillDir, path)
			if err != nil {
				return err
			}
			files = append(files, SourceFile{RelPath: filepath.Join(name, rel), AbsPath: path})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
