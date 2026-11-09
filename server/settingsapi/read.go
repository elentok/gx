package settingsapi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/server/wire"
)

// Read returns the entries that apply to the target with the values its file
// holds. Secrets carry only whether they are set. pendingRestart is passed
// through for the page's "Restart needed" notice.
func (w *Writer) Read(t Target, pendingRestart []string) (wire.SettingsReadResponse, error) {
	resp := wire.SettingsReadResponse{PendingRestart: pendingRestart, Entries: []wire.SettingsEntry{}}
	globalDoc, err := readDoc(w.GlobalPath)
	if err != nil {
		return resp, err
	}

	var targetDoc []byte
	var entries []registry.Entry
	if t.Project == "" {
		targetDoc = globalDoc
		for _, e := range w.Registry.All() {
			if !e.ProjectOnly {
				entries = append(entries, e)
			}
		}
	} else {
		dir, err := w.ProjectDir(t.Project)
		if err != nil {
			return resp, err
		}
		if targetDoc, err = readDoc(filepath.Join(dir, config.ProjectFileName)); err != nil {
			return resp, err
		}
		if resp.Project, err = projectInfo(t.Project, targetDoc); err != nil {
			return resp, err
		}
		// The project view's order is the sidebar's: Project, the overridable sections, Checks.
		for _, s := range w.Registry.ProjectView() {
			entries = append(entries, s.Entries...)
		}
	}

	for _, e := range entries {
		resp.Entries = append(resp.Entries, buildEntry(e, t.Project != "", targetDoc, globalDoc))
	}
	return resp, nil
}

func buildEntry(e registry.Entry, project bool, targetDoc, globalDoc []byte) wire.SettingsEntry {
	path := e.Key
	if project {
		path = e.ProjectPath()
	}
	out := wire.SettingsEntry{
		Key: e.Key.String(), Path: e.Key, Type: typeName(e.Type), Section: e.Section.String(),
		Description: e.Description, Enum: e.Enum, Min: e.Min, Integer: e.Integer,
		Restart: e.Restart, Risky: e.Risky, ConfirmText: e.ConfirmText, ReadOnly: e.ReadOnly,
		Label: e.Label, RiskyOffOnly: e.RiskyOffOnly, Override: string(e.Override),
	}
	raw, set := lookup(targetDoc, path)
	out.Set = set
	if set && !e.WriteOnly() {
		out.Value = raw
	}
	if v, ok := config.DefaultValue(e.Key); ok {
		out.Default, _ = json.Marshal(v)
	}
	if project {
		out.GlobalKey, out.GlobalValue = globalValue(e, globalDoc)
		// The Identity row reads the entry, not Project.VCS: an unset vcs means git.
		if e.Key.String() == "vcs" && !set {
			out.Default = json.RawMessage(`"git"`)
		}
	}
	return out
}

// globalValue is the current effective value of the global key a project
// entry overrides or caps: the global file's value, else the default.
func globalValue(e registry.Entry, globalDoc []byte) (string, json.RawMessage) {
	key := e.GlobalKey
	if len(key) == 0 {
		if !e.ProjectOverridable {
			return "", nil
		}
		key = e.Key
	}
	if raw, ok := lookup(globalDoc, key); ok {
		return key.String(), raw
	}
	if v, ok := config.DefaultValue(key); ok {
		raw, _ := json.Marshal(v)
		return key.String(), raw
	}
	return key.String(), nil
}

func projectInfo(name string, doc []byte) (*wire.SettingsProject, error) {
	pf, err := config.ParseProjectFile(doc)
	if err != nil {
		return nil, err
	}
	// An unset vcs means git: only a repo-less project writes "none".
	info := &wire.SettingsProject{Name: name, VCS: "git"}
	if pf.Name != nil {
		info.Name = *pf.Name
	}
	if pf.VCS != nil {
		info.VCS = *pf.VCS
	}
	if pf.Repo != nil {
		info.Repo = *pf.Repo
		_, err := os.Stat(filepath.Join(info.Repo, check.RepoConfigPath))
		info.RepoChecksFile = err == nil
	}
	return info, nil
}

// readDoc reads a settings file; a missing file reads as an empty document.
func readDoc(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []byte("{}"), nil
	}
	return data, err
}

// lookup finds the raw value at a key path. Segments are matched whole: a
// segment may itself contain dots.
func lookup(doc []byte, path registry.KeyPath) (json.RawMessage, bool) {
	cur := json.RawMessage(doc)
	for _, seg := range path {
		var m map[string]json.RawMessage
		if json.Unmarshal(cur, &m) != nil {
			return nil, false
		}
		next, ok := m[seg]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

var typeNames = map[registry.ValueType]string{
	registry.TypeBool: "bool", registry.TypeNumber: "number", registry.TypeDuration: "duration",
	registry.TypeSize: "size", registry.TypeEnum: "enum", registry.TypeString: "string",
	registry.TypeStringList: "string-list", registry.TypeNumberList: "number-list",
	registry.TypeSecret: "secret", registry.TypeChecks: "checks",
}

func typeName(t registry.ValueType) string { return typeNames[t] }
