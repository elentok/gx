// Package epics holds the epic read models (list, detail, summary, needs-you)
// and the epic-state rules they share with the deadlock check.
//
// It sits below the scheduler and the session layers: what it needs from the
// root server arrives through Deps, never by importing the root.
package epics
