// Package settingsapi is the one writer of settings files. Apply is pure: it
// takes bytes and returns bytes, so every caller shares one set of rules.
package settingsapi

import (
	"encoding/json"
	"errors"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/keyedit"
	"github.com/elentok/gx/config/registry"
)

// Scope selects the file a change applies to.
type Scope int

const (
	ScopeGlobal Scope = iota
	ScopeProject
)

// Change sets Key to Value, or removes Key. Key is a registry key; in project
// scope the entry's project path is what is written.
type Change struct {
	Key    registry.KeyPath
	Value  json.RawMessage
	Remove bool
}

func parseSize(s string) (int64, error) {
	b, err := config.ParseByteSize(s)
	return int64(b), err
}

// Apply returns the new file bytes, or a *Refusal and no bytes.
func Apply(reg *registry.Registry, scope Scope, doc []byte, changes []Change) ([]byte, error) {
	for _, c := range changes {
		if err := checkChange(reg, scope, c); err != nil {
			return nil, err
		}
	}
	if len(changes) > 1 {
		for _, c := range changes {
			out, err := edit(reg, scope, doc, []Change{c})
			if err == nil {
				err = validate(scope, out)
			}
			if err != nil {
				return nil, refuse(reasonOf(err), c.Key)
			}
		}
	}
	out, err := edit(reg, scope, doc, changes)
	if err != nil {
		return nil, err
	}
	if err := validate(scope, out); err != nil {
		keys := make([]registry.KeyPath, len(changes))
		for i, c := range changes {
			keys[i] = c.Key
		}
		return nil, refuse(reasonOf(err), keys...)
	}
	return out, nil
}

func reasonOf(err error) string {
	if r, ok := errors.AsType[*Refusal](err); ok {
		return r.Reason
	}
	return err.Error()
}

func checkChange(reg *registry.Registry, scope Scope, c Change) error {
	e, ok := reg.Lookup(c.Key)
	if !ok {
		return refuse("Unknown setting.", c.Key)
	}
	if e.ReadOnly {
		return refuse("This setting cannot be changed here.", c.Key)
	}
	if scope == ScopeProject && !e.AppliesToProject() {
		return refuse("This setting cannot be set per project.", c.Key)
	}
	if scope == ScopeGlobal && e.ProjectOnly {
		return refuse("This setting exists only in a project.", c.Key)
	}
	if c.Remove {
		return nil
	}
	if err := registry.CheckValue(e, c.Value, parseSize); err != nil {
		return refuse(err.Error(), c.Key)
	}
	return nil
}

func edit(reg *registry.Registry, scope Scope, doc []byte, changes []Change) ([]byte, error) {
	for _, c := range changes {
		e, _ := reg.Lookup(c.Key)
		path := []string(e.Key)
		if scope == ScopeProject {
			path = e.ProjectPath()
		}
		var err error
		if c.Remove {
			doc, err = keyedit.Remove(doc, path)
		} else {
			doc, err = keyedit.Set(doc, path, c.Value)
		}
		if err != nil {
			return nil, refuse(err.Error(), c.Key)
		}
	}
	return doc, nil
}

func validate(scope Scope, doc []byte) error {
	if scope == ScopeGlobal {
		cfg, err := config.Parse(doc)
		if err != nil {
			return err
		}
		return agentrunner.ValidateSetting(cfg.AgentRunner)
	}
	pf, err := config.ParseProjectFile(doc)
	if err != nil {
		return err
	}
	if err := config.ValidateProjectFile(pf); err != nil {
		return err
	}
	if len(pf.Checks) > 0 {
		if _, err := check.ParseConfig(pf.Checks); err != nil {
			return err
		}
	}
	return nil
}
