package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Bundle is gx's canonical, installable skill bundle (see gx.md),
// embedded into the gx binary at compile time so every production
// installation method (release, Homebrew, go install, local build) ships the
// same content.
//
//go:embed gx.md gx-local-tracker.md gx-to-tickets gx-tdd gx-implement gx-resolving-merge-conflicts gx-investigate gx-cleanup gx-merge gx-code-review
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

// DevSourceFiles returns a SourceFile per Bundle entry, pointing at root's
// on-disk skill bundle (its "skills/" directory) instead of the embedded
// copy - the source `gx skills install --dev` links to, so edits under a
// contributor's checkout show up immediately.
//
// It fails without returning any files if root's skills/ directory is
// missing any file Bundle embeds, so a checkout that isn't gx's own source
// tree (or one that's missing files) is refused before any installation
// change is made.
func DevSourceFiles(root string) ([]SourceFile, error) {
	rels, err := BundleFiles()
	if err != nil {
		return nil, err
	}

	bundleDir := filepath.Join(root, "skills")
	sources := make([]SourceFile, 0, len(rels))
	for _, rel := range rels {
		abs := filepath.Join(bundleDir, rel)
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("%s doesn't contain gx's skill bundle (missing skills/%s): %w", root, rel, err)
		}
		sources = append(sources, SourceFile{RelPath: rel, AbsPath: abs})
	}
	return sources, nil
}
