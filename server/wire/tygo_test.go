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

const wirePath = "github.com/elentok/gx/server/wire"

// tygoPackage is one tygo.yaml package at or below server/wire.
type tygoPackage struct {
	dir    string // relative to the repo root, e.g. server/wire/ops
	output string // output_path, relative to the repo root
	files  map[string]bool
}

func tygoPackages(t *testing.T) []tygoPackage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "tygo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Packages []struct {
			Path         string   `yaml:"path"`
			OutputPath   string   `yaml:"output_path"`
			IncludeFiles []string `yaml:"include_files"`
		} `yaml:"packages"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	var pkgs []tygoPackage
	for _, p := range cfg.Packages {
		if p.Path != wirePath && !strings.HasPrefix(p.Path, wirePath+"/") {
			continue
		}
		files := map[string]bool{}
		for _, f := range p.IncludeFiles {
			files[f] = true
		}
		pkgs = append(pkgs, tygoPackage{
			dir:    strings.TrimPrefix(p.Path, "github.com/elentok/gx/"),
			output: p.OutputPath,
			files:  files,
		})
	}
	if len(pkgs) == 0 {
		t.Fatal("tygo.yaml has no server/wire package")
	}
	return pkgs
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
	for _, pkg := range tygoPackages(t) {
		dir := filepath.Join("..", "..", pkg.dir)
		for f := range pkg.files {
			path := filepath.Join(dir, f)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("tygo.yaml lists %s/%s, which is missing: update include_files", pkg.dir, f)
			} else if len(exportedTypes(t, path)) == 0 {
				t.Errorf("tygo.yaml lists %s/%s, which declares no exported types: update include_files", pkg.dir, f)
			}
		}
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		var unlisted []string
		for _, path := range files {
			f := filepath.Base(path)
			if strings.HasSuffix(f, "_test.go") || pkg.files[f] {
				continue
			}
			for _, name := range exportedTypes(t, path) {
				if !notGenerated[name] {
					unlisted = append(unlisted, f+": "+name)
				}
			}
		}
		sort.Strings(unlisted)
		for _, u := range unlisted {
			t.Errorf("%s/%s is in a file tygo.yaml does not list: add the file to include_files or the type to notGenerated", pkg.dir, u)
		}
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
	decl := regexp.MustCompile(`(?m)^export (?:type|interface|const) (\w+)\b`)
	seen := map[string]string{} // name -> output file that declared it
	for _, pkg := range tygoPackages(t) {
		src, err := os.ReadFile(filepath.Join("..", "..", pkg.output))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range decl.FindAllStringSubmatch(string(src), -1) {
			if first, dup := seen[m[1]]; dup {
				t.Errorf("%s declares %s twice (first in %s): rename a Go const group so its names stop sharing a prefix, or a type declared in two wire packages", pkg.output, m[1], first)
			}
			seen[m[1]] = pkg.output
		}
	}
}
