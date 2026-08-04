package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Bundle is gx's canonical, installable skill bundle (see README.md),
// embedded into the gx binary at compile time so every production
// installation method (release, Homebrew, go install, local build) ships the
// same content.
//
//go:embed README.md local-tracker.md gx-to-tickets gx-tdd
var Bundle embed.FS

// BundleID identifies gx's canonical skill bundle in the manifest store
// (see ManifestPath).
const BundleID = "gx-canonical"

// BundleSource is the manifest's human-readable Source for an install of
// Bundle.
const BundleSource = "gx built-in canonical skill bundle"

// BundleFiles returns every file in Bundle, relative to the bundle root, in
// deterministic (lexical, directory-walk) order.
func BundleFiles() ([]string, error) {
	var files []string
	err := fs.WalkDir(Bundle, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk embedded bundle: %w", err)
	}
	return files, nil
}

// ExtractBundle writes every file in Bundle into dir (created as needed),
// returning a SourceFile per file whose AbsPath points at the extracted
// copy - Install and Plan read source content from disk, not from the
// embed.FS directly.
func ExtractBundle(dir string) ([]SourceFile, error) {
	rels, err := BundleFiles()
	if err != nil {
		return nil, err
	}

	sources := make([]SourceFile, 0, len(rels))
	for _, rel := range rels {
		data, err := Bundle.ReadFile(rel)
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", rel, err)
		}
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			return nil, fmt.Errorf("create %s: %w", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, data, 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", abs, err)
		}
		sources = append(sources, SourceFile{RelPath: rel, AbsPath: abs})
	}
	return sources, nil
}
