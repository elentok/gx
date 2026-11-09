package config

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config/registry"
)

// The guard keeps the settings registry in step with the config structs: a new
// key must reach the page or be excluded with a reason.

const guardRule = `.ai/index.md "Adding a config key"`

type guardInput struct {
	Raw, Project, Config, Notifications reflect.Type
	Entries                             []registry.Entry
	Exclusions                          []registry.Exclusion
}

type guardNode struct {
	path registry.KeyPath
	typ  reflect.Type
	leaf bool
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func jsonName(f reflect.StructField) string {
	return strings.Split(f.Tag.Get("json"), ",")[0]
}

// walkKeys lists the JSON keys of t. A key for which stop is true is a leaf
// even if its Go type is a struct, so an exclusion may cover a whole block.
func walkKeys(t reflect.Type, stop func(registry.KeyPath) bool) []guardNode {
	var out []guardNode
	var rec func(t reflect.Type, prefix registry.KeyPath)
	rec = func(t reflect.Type, prefix registry.KeyPath) {
		t = derefType(t)
		if t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := jsonName(f)
			if name == "-" {
				continue
			}
			if f.Anonymous && name == "" {
				rec(f.Type, prefix)
				continue
			}
			if name == "" || !f.IsExported() {
				continue
			}
			path := append(slices.Clone(prefix), name)
			ft := derefType(f.Type)
			if stop(path) || ft.Kind() != reflect.Struct {
				out = append(out, guardNode{path, ft, true})
				continue
			}
			out = append(out, guardNode{path, ft, false})
			rec(ft, path)
		}
	}
	rec(t, nil)
	return out
}

// normalizeKey makes a key path comparable with Go field names.
func normalizeKey(segments []string) string {
	var parts []string
	for _, s := range segments {
		parts = append(parts, strings.ToLower(strings.ReplaceAll(s, "-", "")))
	}
	return strings.Join(parts, ".")
}

// loadedKinds maps the normalized path of every duration and size field of
// the loaded config to its type.
func loadedKinds(t reflect.Type) map[string]registry.ValueType {
	out := map[string]registry.ValueType{}
	var rec func(t reflect.Type, prefix []string)
	rec = func(t reflect.Type, prefix []string) {
		t = derefType(t)
		if t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			path := append(slices.Clone(prefix), f.Name)
			switch derefType(f.Type) {
			case reflect.TypeOf(time.Duration(0)):
				out[normalizeKey(path)] = registry.TypeDuration
			case reflect.TypeOf(ByteSize(0)):
				out[normalizeKey(path)] = registry.TypeSize
			default:
				rec(f.Type, path)
			}
		}
	}
	rec(t, nil)
	return out
}

var typeNames = map[registry.ValueType]string{
	registry.TypeBool: "bool", registry.TypeNumber: "number", registry.TypeDuration: "duration",
	registry.TypeSize: "size", registry.TypeEnum: "enum", registry.TypeString: "string",
	registry.TypeStringList: "string list", registry.TypeNumberList: "number list",
	registry.TypeSecret: "secret", registry.TypeChecks: "checks",
}

func typeName(v registry.ValueType) string { return typeNames[v] }

// allowedTypes is what a key of this Go type may be typed as in the registry.
func allowedTypes(t reflect.Type, path registry.KeyPath, loaded map[string]registry.ValueType) []registry.ValueType {
	switch {
	case t == reflect.TypeOf(json.RawMessage(nil)):
		return []registry.ValueType{registry.TypeChecks}
	case t.Kind() == reflect.Bool:
		return []registry.ValueType{registry.TypeBool}
	case t.Kind() >= reflect.Int && t.Kind() <= reflect.Float64:
		return []registry.ValueType{registry.TypeNumber}
	case t.Kind() == reflect.String:
		if k, ok := loaded[normalizeKey(path)]; ok {
			return []registry.ValueType{k}
		}
		return []registry.ValueType{registry.TypeString, registry.TypeEnum, registry.TypeSecret}
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.String:
		return []registry.ValueType{registry.TypeStringList}
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Float64:
		return []registry.ValueType{registry.TypeNumberList}
	}
	return nil
}

// checkSide compares one file's struct keys with the entries and exclusions
// that address that file.
func checkSide(file string, t reflect.Type, entries map[string]registry.Entry, entryPath func(registry.Entry) registry.KeyPath,
	excl map[string]bool, loaded map[string]registry.ValueType) []string {
	covered := func(p registry.KeyPath) bool {
		_, ok := entries[p.String()]
		return ok || excl[p.String()]
	}
	nodes := walkKeys(t, func(p registry.KeyPath) bool { return covered(p) })
	seen := map[string]guardNode{}
	for _, n := range nodes {
		seen[n.path.String()] = n
	}
	var out []string
	for _, n := range nodes {
		if n.leaf && !covered(n.path) {
			out = append(out, "config key "+quote(n.path)+" in "+file+" has no registry entry. "+
				"Add an entry in config/registry, or an exclusion with a reason; see "+guardRule+".")
		}
	}
	for k, e := range entries {
		n, ok := seen[k]
		if !ok {
			out = append(out, "registry entry "+quote(e.Key)+" is for "+quote(entryPath(e))+", which is no longer a key in "+
				file+". Remove the entry from config/registry.")
			continue
		}
		allowed := allowedTypes(n.typ, n.path, loaded)
		if len(allowed) > 0 && !slices.Contains(allowed, e.Type) {
			var names []string
			for _, a := range allowed {
				names = append(names, typeName(a))
			}
			out = append(out, "registry entry "+quote(e.Key)+" is typed "+typeName(e.Type)+" but its key in "+file+
				" is a "+n.typ.String()+". Change the entry's Type to "+strings.Join(names, " or ")+".")
		}
	}
	for k := range excl {
		// A key below an excluded block is never walked, so an exclusion may
		// name a child of another exclusion.
		if _, ok := seen[k]; !ok && !underExclusion(k, excl) {
			out = append(out, "exclusion "+`"`+k+`"`+" is for a key no longer in "+file+
				". Remove it from config/registry/exclusions.go.")
		}
	}
	return out
}

func underExclusion(k string, excl map[string]bool) bool {
	for other := range excl {
		if strings.HasPrefix(k, other+".") {
			return true
		}
	}
	return false
}

func quote(k registry.KeyPath) string { return `"` + k.String() + `"` }

// secretKeys lists the keys of secret:"true" fields, minus the chat id, which
// is shown rather than write-only.
func secretKeys(t reflect.Type, prefix registry.KeyPath) []string {
	var out []string
	for _, n := range walkKeys(t, func(registry.KeyPath) bool { return false }) {
		f, ok := fieldAt(t, n.path)
		if !ok || f.Tag.Get("secret") != "true" {
			continue
		}
		k := append(slices.Clone(prefix), n.path...)
		if k.String() == "notifications.telegram.chat-id" {
			continue
		}
		out = append(out, k.String())
	}
	return out
}

func fieldAt(t reflect.Type, path registry.KeyPath) (reflect.StructField, bool) {
	var f reflect.StructField
	for _, seg := range path {
		t = derefType(t)
		found := false
		for i := 0; i < t.NumField(); i++ {
			if jsonName(t.Field(i)) == seg {
				f, found = t.Field(i), true
				break
			}
		}
		if !found {
			return f, false
		}
		t = f.Type
	}
	return f, true
}

// settingsGuardProblems returns every mismatch between the config structs and
// the registry, each naming the key and the fix.
func settingsGuardProblems(in guardInput) []string {
	global := map[string]registry.Entry{}
	project := map[string]registry.Entry{}
	for _, e := range in.Entries {
		if !e.ProjectOnly {
			global[e.Key.String()] = e
		}
		if e.AppliesToProject() {
			project[e.ProjectPath().String()] = e
		}
	}
	exclGlobal, exclProject := map[string]bool{}, map[string]bool{}
	for _, x := range in.Exclusions {
		if x.Project {
			exclProject[x.Key.String()] = true
		} else {
			exclGlobal[x.Key.String()] = true
		}
	}
	loaded := loadedKinds(in.Config)
	out := checkSide("config.json", in.Raw, global, func(e registry.Entry) registry.KeyPath { return e.Key }, exclGlobal, loaded)
	out = append(out, checkSide("project.json", in.Project, project, registry.Entry.ProjectPath, exclProject, loaded)...)

	want := secretKeys(in.Notifications, registry.Key("notifications"))
	var have []string
	for _, e := range in.Entries {
		if e.WriteOnly() {
			have = append(have, e.Key.String())
		}
	}
	for _, k := range want {
		if !slices.Contains(have, k) {
			out = append(out, "field "+`"`+k+`"`+` is tagged secret:"true" but has no registry entry typed secret. Type its entry as secret.`)
		}
	}
	for _, k := range have {
		if !slices.Contains(want, k) {
			out = append(out, `registry entry "`+k+`" is typed secret but its field lacks the secret:"true" tag. Add the tag to the field in config/notifications.go, or type the entry otherwise.`)
		}
	}
	slices.Sort(out)
	return out
}

func realGuardInput() guardInput {
	return guardInput{
		Raw: reflect.TypeOf(rawConfig{}), Project: reflect.TypeOf(ProjectFile{}),
		Config: reflect.TypeOf(Config{}), Notifications: reflect.TypeOf(NotificationsConfig{}),
		Entries: registry.Default().All(), Exclusions: registry.Exclusions(),
	}
}

func TestSettingsGuard_RegistryMatchesConfigStructs(t *testing.T) {
	for _, p := range settingsGuardProblems(realGuardInput()) {
		t.Error(p)
	}
}

func TestSettingsGuard_RemovedEntryNamesKeyAndRule(t *testing.T) {
	in := realGuardInput()
	in.Entries = slices.DeleteFunc(slices.Clone(in.Entries), func(e registry.Entry) bool {
		return e.Key.Equal(registry.Key("budget", "soft-limit"))
	})
	got := strings.Join(settingsGuardProblems(in), "\n")
	for _, want := range []string{`"budget.soft-limit"`, guardRule} {
		if !strings.Contains(got, want) {
			t.Errorf("problems %q lack %q", got, want)
		}
	}
}

// guardRaw and guardCfg are in-memory stand-ins for rawConfig and Config.
type guardRaw struct {
	Name  *string `json:"name"`
	Block *struct {
		Wait *string `json:"wait"`
		Size *string `json:"size"`
	} `json:"block"`
	Notifications *guardNotif `json:"notifications"`
}

type guardCfg struct {
	Block struct {
		Wait time.Duration
		Size ByteSize
	}
}

type guardNotif struct {
	Token string `json:"token" secret:"true"`
	Chat  string `json:"chat"`
}

func guardBase() guardInput {
	return guardInput{
		Raw: reflect.TypeOf(guardRaw{}), Project: reflect.TypeOf(struct{}{}),
		Config: reflect.TypeOf(guardCfg{}), Notifications: reflect.TypeOf(guardNotif{}),
		Entries: []registry.Entry{
			{Key: registry.Key("name"), Type: registry.TypeString},
			{Key: registry.Key("block", "wait"), Type: registry.TypeDuration},
			{Key: registry.Key("block", "size"), Type: registry.TypeSize},
			{Key: registry.Key("notifications", "token"), Type: registry.TypeSecret},
			{Key: registry.Key("notifications", "chat"), Type: registry.TypeString},
		},
	}
}

func guardHas(t *testing.T, in guardInput, wants ...string) {
	t.Helper()
	got := strings.Join(settingsGuardProblems(in), "\n")
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("problems %q lack %q", got, w)
		}
	}
}

func TestSettingsGuard_RuleDocumented(t *testing.T) {
	data, err := os.ReadFile("../.ai/index.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	_, section, ok := strings.Cut(doc, "## Adding a config key")
	if !ok {
		t.Fatal(".ai/index.md lacks the heading \"Adding a config key\"")
	}
	if i := strings.Index(section, "\n## "); i >= 0 {
		section = section[:i]
	}
	for _, want := range []string{"config/registry", "-update", "TestSettingsGuard"} {
		if !strings.Contains(section, want) {
			t.Errorf("section lacks %q", want)
		}
	}
}

func TestSettingsGuard_InMemory(t *testing.T) {
	t.Run("baseline has no problems", func(t *testing.T) {
		if got := settingsGuardProblems(guardBase()); len(got) > 0 {
			t.Errorf("problems: %v", got)
		}
	})
	t.Run("missing entry", func(t *testing.T) {
		in := guardBase()
		in.Entries = in.Entries[1:]
		guardHas(t, in, `"name"`, guardRule)
	})
	t.Run("entry without a key", func(t *testing.T) {
		in := guardBase()
		in.Entries = append(in.Entries, registry.Entry{Key: registry.Key("gone"), Type: registry.TypeBool})
		guardHas(t, in, `"gone"`, "no longer a key")
	})
	t.Run("vanished exclusion", func(t *testing.T) {
		in := guardBase()
		in.Exclusions = []registry.Exclusion{{Key: registry.Key("old")}}
		guardHas(t, in, `"old"`, "exclusions.go")
	})
	t.Run("duration typed as number", func(t *testing.T) {
		in := guardBase()
		in.Entries[1].Type = registry.TypeNumber
		guardHas(t, in, `"block.wait"`, "typed number", "duration")
	})
	t.Run("secret entry without the tag", func(t *testing.T) {
		in := guardBase()
		in.Entries[4].Type = registry.TypeSecret
		guardHas(t, in, `"notifications.chat"`, `lacks the secret:"true" tag`)
	})
	t.Run("tagged field without a secret entry", func(t *testing.T) {
		in := guardBase()
		in.Entries[3].Type = registry.TypeString
		guardHas(t, in, `"notifications.token"`, "typed secret")
	})
}
