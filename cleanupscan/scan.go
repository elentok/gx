// Package cleanupscan classifies a repo's epics, branches and worktrees for
// `gx cleanup scan` and the server. It must not import cobra.
package cleanupscan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// EpicScan is one epic's classification, as reported by `gx cleanup scan`.
type EpicScan struct {
	Name                string `json:"name"`
	AllDone             bool   `json:"all_done"`
	MergedToTrunk       bool   `json:"merged_to_main"`
	HasCodeReviewTicket bool   `json:"has_code_review_ticket"`
	CodeReviewDone      bool   `json:"code_review_done"`
}

// HousekeepingScan is the repo-wide, once-per-run housekeeping report.
type HousekeepingScan struct {
	// Skipped is true when the scratch root resolves inside a bare git
	// directory, where nothing under .scratch is trackable in the first
	// place.
	Skipped bool `json:"skipped"`
	// TrackedFiles lists any git-tracked files found under .scratch (repo-root
	// relative paths). Non-empty means .scratch/ leaked into git tracking.
	TrackedFiles []string `json:"tracked_files"`
	// GitignoreHasScratch is whether .gitignore has a .scratch entry.
	GitignoreHasScratch bool `json:"gitignore_has_scratch"`
}

// Result is the full `gx cleanup scan --json` payload.
type Result struct {
	Epics        []EpicScan       `json:"epics"`
	Worktrees    []WorktreeScan   `json:"worktrees"`
	Housekeeping HousekeepingScan `json:"housekeeping"`
	// AtticRefs lists branches `gx server tickets start-over` set aside. Report-only: nothing
	// in gx prunes them, and scan must never learn to.
	AtticRefs []string `json:"attic_refs"`
}

const atticRefPrefix = "ralph-loop/attic/"

func scanAtticRefs(repo git.Repo) ([]string, error) {
	branches, err := git.ListBranches(repo)
	if err != nil {
		return nil, err
	}
	refs := []string{}
	for _, b := range branches {
		if !b.IsRemote && strings.HasPrefix(b.Name, atticRefPrefix) {
			refs = append(refs, b.Name)
		}
	}
	return refs, nil
}

// Scan resolves the repo at cwd, classifies every epic under .scratch
// (tickets.Load already excludes .archive and other dot-prefixed directories)
// and computes the repo-wide housekeeping report.
func Scan(dirs config.Dirs, cwd string) (Result, error) {
	cfg, err := dirs.Load()
	if err != nil {
		return Result{}, err
	}
	return ScanIn(cfg.TicketStore.Path, cwd)
}

// ScanIn is Scan over an explicit ticket store.
func ScanIn(storePath, cwd string) (Result, error) {
	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return Result{}, fmt.Errorf("not inside a git repo: %w", err)
	}
	// A "merged?" check against an unresolved trunk can't be trusted, so
	// stop before reporting anything the gx-cleanup skill would delete on.
	openRepo, err := ticketgit.OpenRepoIn(storePath, cwd)
	if err != nil {
		return Result{}, err
	}
	repo := *openRepo

	root, err := tickets.RootIn(storePath, cwd)
	if err != nil {
		return Result{}, err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return Result{}, err
	}

	result := Result{Epics: []EpicScan{}}
	activeEpics := []tickets.Epic{}
	for _, epic := range epics {
		// Containers that gx tickets recreates on demand are never work to
		// archive: archiving one makes the next close split its history.
		if epic.Name == tickets.FollowUpsEpic || epic.Name == tickets.ArchivedFollowUpsEpic {
			continue
		}
		activeEpics = append(activeEpics, epic)
		// The standing one-offs epic never finishes and never merges as a unit.
		if tickets.IsOneOffsEpicName(epic.Name) {
			continue
		}
		scan, err := scanEpic(repo, epic)
		if err != nil {
			return Result{}, err
		}
		result.Epics = append(result.Epics, scan)
	}

	// Archived epics are done, so their iteration branches resolve to done
	// tickets and earn a delete recommendation once merged.
	archived, err := tickets.LoadArchived(root)
	if err != nil {
		return Result{}, err
	}
	result.Worktrees, err = scanWorktrees(repo, append(activeEpics, archived...))
	if err != nil {
		return Result{}, err
	}

	result.Housekeeping, err = scanHousekeeping(*info)
	if err != nil {
		return Result{}, err
	}

	result.AtticRefs, err = scanAtticRefs(repo)
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func scanEpic(repo git.Repo, epic tickets.Epic) (EpicScan, error) {
	scan := EpicScan{
		Name:    epic.Name,
		AllDone: epic.AllDone(),
	}

	for _, t := range epic.Tickets {
		if t.Type == "code-review" {
			scan.HasCodeReviewTicket = true
			if t.IsDone() {
				scan.CodeReviewDone = true
			}
		}
	}

	// merged_at outlives the branch, which may be deleted once merged.
	if !epic.MergedAt.IsZero() {
		scan.MergedToTrunk = true
		return scan, nil
	}
	branches, err := git.ListBranches(repo)
	if err != nil {
		return EpicScan{}, err
	}
	branchExists := false
	for _, b := range branches {
		if b.Name == epic.Name {
			branchExists = true
			break
		}
	}
	if branchExists {
		merged, err := pr.IsCommitMergedToTrunk(repo.Root, repo.Trunk, epic.Name)
		if err != nil {
			return EpicScan{}, err
		}
		scan.MergedToTrunk = merged
	}

	return scan, nil
}

// scanHousekeeping runs the tracked-files/.gitignore checks against a real
// working tree: the current worktree for a bare/`.bare`-trick repo, or the
// repo root itself for a plain repo. A bare repo's own git directory has no
// index or checked-out .gitignore, so when cwd resolves there directly (no
// linked worktree in play) there is nothing trackable to check — skip.
func scanHousekeeping(info git.DirInfo) (HousekeepingScan, error) {
	checkoutDir := info.WorktreeRoot
	if !info.Repo.IsBare {
		checkoutDir = info.Repo.Root
	}
	if checkoutDir == "" {
		return HousekeepingScan{Skipped: true, TrackedFiles: []string{}}, nil
	}

	tracked, err := git.TrackedFilesUnder(checkoutDir, ".scratch")
	if err != nil {
		return HousekeepingScan{}, err
	}
	if tracked == nil {
		tracked = []string{}
	}

	return HousekeepingScan{
		TrackedFiles:        tracked,
		GitignoreHasScratch: gitignoreHasScratch(checkoutDir),
	}, nil
}

// gitignoreHasScratch reports whether repoRoot's .gitignore has a line that
// ignores .scratch (with or without a trailing slash). A missing .gitignore
// is treated the same as one without the entry.
func gitignoreHasScratch(repoRoot string) bool {
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".gitignore"))
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == ".scratch" || line == ".scratch/" {
			return true
		}
	}
	return false
}
