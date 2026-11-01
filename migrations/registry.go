// Package migrations holds the one ordered registry of named migration steps
// that bring a ticket tracker up to date. cmd (gx tickets migrate,
// gx restart-check) and server both run it.
//
// Contract for contributors: a released step name is never reused or renamed.
// To change a step's behaviour, add a new step.
package migrations

import (
	"errors"
	"fmt"
	"slices"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
)

// Env is what a step needs to run.
type Env struct {
	Dirs config.Dirs
	// Root is the tracker the step reads and writes.
	Root string
	// LookupRoot is the tracker that repo, run-log and conversation lookups
	// resolve against: Root itself, except in a dry run on a copy.
	LookupRoot string
	// DryRun keeps the steps that write outside Root report-only.
	DryRun bool
	// Skip reports whether the named step is to be skipped. Nil skips nothing.
	Skip func(name string) bool
}

// StepFunc runs one step and returns the files it changed.
type StepFunc func(Env) ([]tickets.MigrationChange, error)

// Step is one named migration step.
type Step struct {
	Name string
	Run  StepFunc
	// Append adds the step's changes as they are. Otherwise a file the earlier
	// steps already changed is folded into its entry, so the file count stays
	// one per file.
	Append bool
}

// Registry is an ordered list of steps with unique names.
type Registry struct {
	steps []Step
}

// Register adds s after the steps already registered. It panics on an empty or
// duplicate name.
func (r *Registry) Register(s Step) {
	if s.Name == "" {
		panic("migrations: step with an empty name")
	}
	if s.Run == nil {
		panic(fmt.Sprintf("migrations: step %q has no function", s.Name))
	}
	if slices.ContainsFunc(r.steps, func(o Step) bool { return o.Name == s.Name }) {
		panic(fmt.Sprintf("migrations: duplicate step name %q", s.Name))
	}
	r.steps = append(r.steps, s)
}

// Names lists the step names in run order.
func (r *Registry) Names() []string {
	names := make([]string, len(r.steps))
	for i, s := range r.steps {
		names[i] = s.Name
	}
	return names
}

// earlyError marks a failure before any change was made, where nothing is printed.
type earlyError struct{ err error }

func (e earlyError) Error() string { return e.err.Error() }
func (e earlyError) Unwrap() error { return e.err }

// Run runs the steps in order and stops at the first failure. The changes made
// up to it are returned with the error. early reports a failure before any step
// ran, where nothing is printed.
func (r *Registry) Run(env Env) (changes []tickets.MigrationChange, early bool, err error) {
	for _, s := range r.steps {
		if env.Skip != nil && env.Skip(s.Name) {
			continue
		}
		got, err := s.Run(env)
		if s.Append {
			changes = append(changes, got...)
		} else {
			changes = mergeChanges(changes, got)
		}
		if err != nil {
			var e earlyError
			if errors.As(err, &e) {
				return changes, true, e.err
			}
			return changes, false, err
		}
	}
	return changes, false, nil
}

// Default is the registry of every released step, in run order.
var Default = &Registry{}

// Run runs the Default registry.
func Run(env Env) ([]tickets.MigrationChange, bool, error) {
	return Default.Run(env)
}

// mergeChanges adds extra to changes, folding a file that already has an entry
// into it so the file count stays one per file.
func mergeChanges(changes, extra []tickets.MigrationChange) []tickets.MigrationChange {
	for _, e := range extra {
		i := slices.IndexFunc(changes, func(c tickets.MigrationChange) bool { return c.Path == e.Path })
		if i < 0 {
			changes = append(changes, e)
			continue
		}
		changes[i].Notes = append(changes[i].Notes, e.Notes...)
	}
	return changes
}
