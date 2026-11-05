// Package repair holds the repair verbs shared by the CLI and the server.
package repair

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/verify"
	"github.com/elentok/gx/tickets"
)

// VerifyResult is the --json payload of `gx server tickets verify`: always the full,
// unfiltered list, with a landing in flight reported beside it.
type VerifyResult struct {
	Tickets         []verify.TicketVerification `json:"tickets"`
	LandingInFlight *merge.MergeMarker          `json:"landing_in_flight"`
	// OrphanLandLock is a land lock with no marker: a land mid-flight, or a
	// crash leftover when its owner is not running.
	OrphanLandLock *merge.MergeLockOwner `json:"orphan_land_lock"`
}

// VerifyRun is everything Verify needs.
type VerifyRun struct {
	EpicPath        string
	ID              string // empty verifies the whole epic
	FeatureWorktree string
	WorktreeDir     string
	WorkspaceID     string
	// MergeLockStateDir is the state dir the epic's merge lock lives under.
	MergeLockStateDir string
	Deps              verify.VerifyDeps
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

	events, _, err := runlog.ReadEvents(filepath.Dir(epicPath), epicName)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("reading run log: %w", err)
	}
	verifications, err := verify.VerifyEpic(run.Deps, verify.VerifyParams{
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
		verifications = []verify.TicketVerification{}
	}

	lockDir, err := merge.MergeLockDirIn(run.MergeLockStateDir, epicPath)
	if err != nil {
		return VerifyResult{}, err
	}
	marker, err := merge.ReadMergeMarker(lockDir)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("reading land marker: %w", err)
	}
	if marker != nil && marker.Epic != epicName {
		marker = nil
	}
	orphan, err := merge.OrphanMergeLock(lockDir)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("reading land lock: %w", err)
	}
	return VerifyResult{Tickets: verifications, LandingInFlight: marker, OrphanLandLock: orphan}, nil
}
