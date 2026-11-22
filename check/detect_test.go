package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetect_GuardsScriptIsCandidateOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"guards":"node guards.js"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Detect(root)["guards"]
	if len(got) != 1 || !strings.Contains(got[0].Source, "guards") {
		t.Fatalf("guards candidates = %v, want the package.json script", got)
	}
	if !strings.HasPrefix(DefaultLine["guards"], "none") {
		t.Errorf("DefaultLine[guards] = %q, want it to say none", DefaultLine["guards"])
	}
}
