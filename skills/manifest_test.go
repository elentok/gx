package skills

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestPathUsesUserConfigDir(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	got, err := ManifestPath("my-skill")
	if err != nil {
		t.Fatalf("ManifestPath: %v", err)
	}
	want := filepath.Join(tmp, "gx", "skills", "my-skill.json")
	if got != want {
		t.Fatalf("ManifestPath = %q, want %q", got, want)
	}
}

func TestManifestPathPropagatesUserConfigDirError(t *testing.T) {
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { userConfigDirFn = prev })

	if _, err := ManifestPath("my-skill"); err == nil {
		t.Fatal("ManifestPath: expected error, got nil")
	}
}

func TestLoadMissingReturnsErrNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	_, err := Load(path)
	if !errors.Is(err, ErrNotExist) {
		t.Fatalf("Load: err = %v, want ErrNotExist", err)
	}
}

func TestLoadMalformedReturnsErrMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("Load: err = %v, want ErrMalformed", err)
	}
}

func TestLoadMissingSchemaVersionReturnsErrMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"mode":"managed-copy"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("Load: err = %v, want ErrMalformed", err)
	}
}

func TestLoadUnsupportedVersionReturnsErrUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"schema-version":999}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("Load: err = %v, want ErrUnsupportedVersion", err)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "manifest.json")
	m := Manifest{
		Mode:       ModeManagedCopy,
		Source:     "github.com/example/skills//foo",
		AgentRoots: []string{"/home/user/.claude/skills", "/home/user/.codex/skills"},
		Files: []ManagedFile{
			{Path: "foo/SKILL.md", Hash: "abc123"},
			{Path: "foo/scripts/run.sh", Hash: "def456"},
		},
	}

	if err := Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, CurrentSchemaVersion)
	}
	if got.Mode != m.Mode || got.Source != m.Source {
		t.Fatalf("Load = %+v, want Mode/Source to match %+v", got, m)
	}
	if len(got.Files) != len(m.Files) || got.Files[0] != m.Files[0] || got.Files[1] != m.Files[1] {
		t.Fatalf("Files = %+v, want %+v", got.Files, m.Files)
	}
}

func TestSaveLeavesNoTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	if err := Save(path, Manifest{Mode: ModeSymlink, Source: "src"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "manifest.json" {
		t.Fatalf("dir entries = %v, want only manifest.json (no leftover temp file)", entries)
	}
}
