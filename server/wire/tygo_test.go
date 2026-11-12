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

// agentConstValues returns the string values of every const of the named type
// declared in ralphloop/agent/agent.go, in source order.
func agentConstValues(t *testing.T, typeName string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", "ralphloop", "agent", "agent.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var vals []string
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != typeName {
				continue
			}
			for _, v := range vs.Values {
				if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					vals = append(vals, strings.Trim(lit.Value, `"`))
				}
			}
		}
	}
	return vals
}

// TestTygoAgentUnionsMatchGoConsts guards the hard-coded agent.ContextLevel
// and agent.Tier unions in tygo.yaml. A Go constant added or removed without
// a tygo.yaml edit would leave the generated TypeScript union stale.
func TestTygoAgentUnionsMatchGoConsts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "tygo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Packages []struct {
			TypeMappings map[string]string `yaml:"type_mappings"`
		} `yaml:"packages"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile(`"([^"]*)"`)
	for _, c := range []struct{ mapping, goType string }{
		{"agent.ContextLevel", "ContextLevel"},
		{"agent.Tier", "Tier"},
	} {
		union, found := "", false
		for _, p := range cfg.Packages {
			if u, ok := p.TypeMappings[c.mapping]; ok {
				union, found = u, true
			}
		}
		if !found {
			t.Errorf("tygo.yaml has no type_mappings entry for %s", c.mapping)
			continue
		}
		var got []string
		for _, m := range quoted.FindAllStringSubmatch(union, -1) {
			got = append(got, m[1])
		}
		want := agentConstValues(t, c.goType)
		if len(want) == 0 {
			t.Fatalf("found no %s constants in ralphloop/agent/agent.go", c.goType)
		}
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("tygo.yaml %s union is %v, Go constants are %v: update type_mappings", c.mapping, got, want)
		}
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
