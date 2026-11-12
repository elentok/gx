package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// notGenerated lists the exported wire types the web UI does not use, so
// tygo.yaml leaves their files out. A type moved out of a listed file lands in
// an unlisted one; this list makes that a visible edit instead of a silent
// deletion from the generated TypeScript.
var notGenerated = map[string]bool{
	"AFKRequest": true, "AFKResult": true, "AddProjectRequest": true, "AddProjectResult": true,
	"AskHookOption": true, "AskHookQuestion": true, "AskHookRequest": true, "AskHookResult": true,
	"CPUSpikeResult": true, "ChangedRequest": true, "ConfigStatus": true, "DefaultHarnessStatus": true,
	"Event": true, "Explanation": true, "Handshake": true, "History": true, "IterationInfo": true,
	"LimitSample": true, "LimitsReportResult": true, "LockInfo": true, "OneOffRequest": true,
	"OneOffResult": true, "ProjectDefaultHarness": true, "ProjectInfo": true, "ProjectRequest": true,
	"ProjectResult": true, "QueueEntry": true, "QueueItem": true, "QueueRequest": true,
	"QueueResult": true, "ReloadRecord": true, "ReloadResult": true, "RepairRequest": true,
	"RepairResult": true, "Route": true, "SaveChecksRequest": true, "SaveChecksResult": true,
	"SetDefaultHarnessRequest": true, "SetDefaultHarnessResult": true, "ViModeRequest": true,
	"ViModeResult": true, "WebInfo": true, "WebLogin": true, "WebLoginsResult": true, "WebMode": true,
}

func tygoFiles(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "tygo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Packages []struct {
			Path         string   `yaml:"path"`
			IncludeFiles []string `yaml:"include_files"`
		} `yaml:"packages"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	files := map[string]bool{}
	for _, p := range cfg.Packages {
		if strings.HasSuffix(p.Path, "/server/wire") {
			for _, f := range p.IncludeFiles {
				files[f] = true
			}
		}
	}
	if len(files) == 0 {
		t.Fatal("tygo.yaml lists no server/wire files")
	}
	return files
}

func exportedTypes(t *testing.T, file string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			if ts := s.(*ast.TypeSpec); ts.Name.IsExported() {
				names = append(names, ts.Name.Name)
			}
		}
	}
	return names
}

// TestTygoCoversWireTypes keeps `make web-types` from dropping exports: every
// file tygo.yaml lists must exist and declare an exported type, and every
// exported type elsewhere must be in notGenerated.
func TestTygoCoversWireTypes(t *testing.T) {
	listed := tygoFiles(t)
	for f := range listed {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("tygo.yaml lists %s, which is missing: update include_files", f)
		} else if len(exportedTypes(t, f)) == 0 {
			t.Errorf("tygo.yaml lists %s, which declares no exported types: update include_files", f)
		}
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var unlisted []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || listed[f] {
			continue
		}
		for _, name := range exportedTypes(t, f) {
			if !notGenerated[name] {
				unlisted = append(unlisted, f+": "+name)
			}
		}
	}
	sort.Strings(unlisted)
	for _, u := range unlisted {
		t.Errorf("%s is in a file tygo.yaml does not list: add the file to include_files or the type to notGenerated", u)
	}
}

// TestGeneratedTypesAreUnique catches tygo merging two const groups that share
// a name prefix (both `Reason*` groups became `type Reason`), which breaks the
// webui build with TS2300.
func TestGeneratedTypesAreUnique(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "webui", "src", "generated", "server.ts"))
	if err != nil {
		t.Fatal(err)
	}
	decl := regexp.MustCompile(`(?m)^export (?:type|interface|const) (\w+)\b`)
	seen := map[string]bool{}
	for _, m := range decl.FindAllStringSubmatch(string(src), -1) {
		if seen[m[1]] {
			t.Errorf("generated/server.ts declares %s twice: rename a Go const group so its names stop sharing a prefix", m[1])
		}
		seen[m[1]] = true
	}
}
