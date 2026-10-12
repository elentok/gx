package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
)

func TestWriteProjectFile_WritesSchemaAndOmitsUnsetKeys(t *testing.T) {
	dir := t.TempDir()
	name, repo := "p", "/r"
	if err := WriteProjectFile(dir, config.ProjectFile{Name: &name, Repo: &repo}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, config.ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"$schema": "`+config.ProjectSchemaURL+`"`) {
		t.Errorf("project.json lacks $schema:\n%s", data)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("project.json writes unset keys as null:\n%s", data)
	}
	if _, err := config.ReadProjectFile(dir); err != nil {
		t.Errorf("written project.json does not read back: %v", err)
	}
}
