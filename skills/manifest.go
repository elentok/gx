// Package skills defines gx's durable record of agent-skill installations
// and the ownership rules that decide whether it's safe to overwrite or
// remove content on disk.
package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CurrentSchemaVersion is the manifest schema version this build writes and
// the highest version it knows how to read.
const CurrentSchemaVersion = 1

// InstallMode records how a skill's managed files were placed on disk.
type InstallMode string

const (
	// ModeManagedCopy means gx copied the skill's files; ManagedFile.Hash
	// holds each file's content hash.
	ModeManagedCopy InstallMode = "managed-copy"
	// ModeSymlink means gx symlinked the skill's files to their source;
	// ManagedFile.LinkTarget holds each symlink's intended target.
	ModeSymlink InstallMode = "symlink"
)

// ManagedFile is one path gx installed as part of a skill, relative to the
// agent root(s) it was installed under.
type ManagedFile struct {
	Path string `json:"path"`
	// Hash is the installed content's hash, set when Mode is ModeManagedCopy.
	Hash string `json:"hash,omitempty"`
	// LinkTarget is the symlink's intended target, set when Mode is ModeSymlink.
	LinkTarget string `json:"link-target,omitempty"`
}

// Manifest is gx's durable record of a single skill installation: what was
// installed, how, from where, and into which agent roots.
type Manifest struct {
	SchemaVersion int           `json:"schema-version"`
	Mode          InstallMode   `json:"mode"`
	Source        string        `json:"source"`
	AgentRoots    []string      `json:"agent-roots"`
	Files         []ManagedFile `json:"files"`
}

var (
	// ErrNotExist means no manifest is present at the requested path.
	ErrNotExist = errors.New("manifest does not exist")
	// ErrMalformed means a manifest is present but its content can't be parsed.
	ErrMalformed = errors.New("manifest is malformed")
	// ErrUnsupportedVersion means a manifest parses but declares a schema
	// version newer than this build supports.
	ErrUnsupportedVersion = errors.New("manifest schema version is unsupported")
)

var userConfigDirFn = os.UserConfigDir

// ManifestPath returns the manifest file path for the skill identified by
// id, following gx's existing user configuration-directory convention
// (typically ~/.config/gx/skills/<id>.json).
func ManifestPath(id string) (string, error) {
	base, err := userConfigDirFn()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "gx", "skills", id+".json"), nil
}

// Load reads and validates the manifest at path. It returns ErrNotExist if
// no manifest is there, ErrMalformed if the content can't be parsed as a
// manifest, or ErrUnsupportedVersion if it parses but declares a schema
// version newer than CurrentSchemaVersion.
func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, ErrNotExist
		}
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %s: %v", ErrMalformed, path, err)
	}
	if m.SchemaVersion == 0 {
		return Manifest{}, fmt.Errorf("%w: %s: missing schema-version", ErrMalformed, path)
	}
	if m.SchemaVersion > CurrentSchemaVersion {
		return Manifest{}, fmt.Errorf("%w: %s: schema version %d", ErrUnsupportedVersion, path, m.SchemaVersion)
	}

	return m, nil
}

// Save writes m to path atomically: the manifest is fully written to a
// temporary file in the same directory and only then renamed into place, so
// a reader never observes a partially written manifest. SchemaVersion is
// set to CurrentSchemaVersion when unset.
func Save(path string, m Manifest) error {
	if m.SchemaVersion == 0 {
		m.SchemaVersion = CurrentSchemaVersion
	}

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	b = append(b, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create manifest dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp manifest: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp manifest: %w", err)
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return fmt.Errorf("chmod temp manifest: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp manifest into place: %w", err)
	}
	return nil
}
