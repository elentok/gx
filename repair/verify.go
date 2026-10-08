// Package repair holds the repair verbs shared by the CLI and the server.
package repair

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// VerifyResult is the --json payload of `gx server tickets verify`: always the full,
// unfiltered list, with a landing in flight reported beside it.
type VerifyResult struct {
	Tickets         []ralphloop.TicketVerification `json:"tickets"`
	LandingInFlight *ralphloop.LandMarker          `json:"landing_in_flight"`
	// OrphanLandLock is a land lock with no marker: a land mid-flight, or a
	// crash leftover when its owner is not running.
	OrphanLandLock *ralphloop.LandLockOwner `json:"orphan_land_lock"`
}

// VerifyRun is everything Verify needs.
type VerifyRun struct {
	EpicPath        string
	ID              string // empty verifies the whole epic
	FeatureWorktree string
	WorktreeDir     string
	WorkspaceID     string
	Deps            ralphloop.VerifyDeps
}

// Verify is write-free and takes no land lock: reading owns nothing, so it
// works while a land or a live tab is in flight.
func Verify(run VerifyRun) (VerifyResult, error) {
	epicPath := filepath.Clean(run.EpicPath)
	epicName := filepath.Base(epicPath)
	epics, err := tickets.Load(filepath.Dir(epicPath))
	if err != nil {
		return VerifyResult{}, fmt.Errorf("loading epics under %s: %w", filepath.Dir(epicPath), err)
	}
	var epic *tickets.Epic
	for i := range epics {
		if epics[i].Name == epicName {
			epic = &epics[i]
		}
	}
	if epic == nil {
		return VerifyResult{}, fmt.Errorf("epic not found: %s", epicPath)
	}

	selected := epic.Tickets
	if run.ID != "" {
		selected = nil
		for _, t := range epic.Tickets {
			if t.DisplayNumber() == run.ID {
				selected = append(selected, t)
			}
		}
		if len(selected) == 0 {
			return VerifyResult{}, fmt.Errorf("ticket %s not found in epic %s", run.ID, epicName)
		}
	}

	events, _, err := ralphloop.ReadEvents(filepath.Dir(epicPath), epicName)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("reading run log: %w", err)
	}
	verifications, err := ralphloop.VerifyEpic(run.Deps, ralphloop.VerifyParams{
		Epic:            epicName,
		FeatureWorktree: run.FeatureWorktree,
		WorktreeDir:     run.WorktreeDir,
		WorkspaceID:     run.WorkspaceID,
		Tickets:         selected,
		Events:          events,
	})
	if err != nil {
		return VerifyResult{}, err
	}
	if verifications == nil {
		verifications = []ralphloop.TicketVerification{}
	}

	lockDir, err := ralphloop.LandLockDir(epicPath)
	if err != nil {
		return VerifyResult{}, err
	}
	marker, err := ralphloop.ReadLandMarker(lockDir)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("reading land marker: %w", err)
	}
	if marker != nil && marker.Epic != epicName {
		marker = nil
	}
	orphan, err := ralphloop.OrphanLandLock(lockDir)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("reading land lock: %w", err)
	}
	return VerifyResult{Tickets: verifications, LandingInFlight: marker, OrphanLandLock: orphan}, nil
}
