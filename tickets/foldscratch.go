package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/git"
)

// StrayScratchDir is an old-style per-worktree `.scratch` directory found
// during a startup fold scan, together with the epic-slug subdirectories it
// contains (bare directory names, no path-building).
type StrayScratchDir struct {
	WorktreeName string
	Path         string
	EpicSlugs    []string
}

// FoldAction is one epic-slug move decided by PlanFold: from a stray
// worktree `.scratch` dir into the canonical root, either directly (no
// collision) or flagged for the caller to resolve.
type FoldAction struct {
	EpicSlug     string
	WorktreeName string
	From         string
	To           string
	Collides     bool
}

// CollisionResolution is the user's chosen resolution for one colliding
// FoldAction.
type CollisionResolution int

const (
	ResolveMerge CollisionResolution = iota
	ResolveAutoRename
)

// ResolveCollision is called once per colliding FoldAction to get the user's
// choice of how to resolve it.
type ResolveCollision func(FoldAction) (CollisionResolution, error)

// PlanFold decides, for every epic slug in every stray scratch dir, whether
// it can move directly into canonicalRoot or collides with an epic slug
// that's already there (or already claimed by an earlier stray in strays).
// Pure: canonicalEpicSlugs and strays are pre-read by the caller, so this has
// no filesystem dependency and is unit-testable against synthetic fixtures.
func PlanFold(canonicalRoot string, canonicalEpicSlugs []string, strays []StrayScratchDir) []FoldAction {
	claimed := make(map[string]bool, len(canonicalEpicSlugs))
	for _, slug := range canonicalEpicSlugs {
		claimed[slug] = true
	}

	var actions []FoldAction
	for _, stray := range strays {
		for _, slug := range stray.EpicSlugs {
			actions = append(actions, FoldAction{
				EpicSlug:     slug,
				WorktreeName: stray.WorktreeName,
				From:         filepath.Join(stray.Path, slug),
				To:           filepath.Join(canonicalRoot, slug),
				Collides:     claimed[slug],
			})
			claimed[slug] = true
		}
	}
	return actions
}

// ApplyFold executes actions in order: non-colliding ones move directly;
// colliding ones ask resolve for a decision. A merge unions the stray epic's
// entries into the canonical one without ever overwriting an entry already
// present there; an auto-rename moves the stray epic to a
// `<slug>-<worktree>` sibling instead.
func ApplyFold(actions []FoldAction, resolve ResolveCollision) error {
	for _, action := range actions {
		if !action.Collides {
			if err := moveDir(action.From, action.To); err != nil {
				return fmt.Errorf("folding %s: %w", action.EpicSlug, err)
			}
			continue
		}

		choice, err := resolve(action)
		if err != nil {
			return fmt.Errorf("resolving collision for %s: %w", action.EpicSlug, err)
		}

		switch choice {
		case ResolveMerge:
			if err := mergeDir(action.From, action.To); err != nil {
				return fmt.Errorf("merging %s: %w", action.EpicSlug, err)
			}
		case ResolveAutoRename:
			renamed := action.To + "-" + action.WorktreeName
			if err := moveDir(action.From, renamed); err != nil {
				return fmt.Errorf("renaming %s: %w", action.EpicSlug, err)
			}
		default:
			return fmt.Errorf("unknown collision resolution %d for %s", choice, action.EpicSlug)
		}
	}
	return nil
}

// GatherStrayScratchDirs scans repo's linked worktrees for old-style
// per-worktree `.scratch` directories (predating Repo.ScratchRoot()
// centralizing them at the bare repo root) and reads each one's immediate
// subdirectories as epic slugs. Worktrees with no `.scratch` dir, or an
// empty one, are omitted.
func GatherStrayScratchDirs(repo git.Repo) ([]StrayScratchDir, error) {
	worktrees, err := git.ListWorktrees(repo)
	if err != nil {
		return nil, err
	}

	canonicalRoot := repo.ScratchRoot()

	var strays []StrayScratchDir
	for _, wt := range worktrees {
		strayPath := filepath.Join(wt.Path, ".scratch")
		if strayPath == canonicalRoot {
			continue
		}

		slugs, err := listEpicSlugs(strayPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if len(slugs) == 0 {
			continue
		}

		strays = append(strays, StrayScratchDir{WorktreeName: wt.Name, Path: strayPath, EpicSlugs: slugs})
	}
	return strays, nil
}

// FoldStrayScratchDirs is the full startup migration path for a bare-repo
// checkout: it discovers stray per-worktree `.scratch` dirs, plans the fold,
// and applies it, calling resolve for any collisions. It's a no-op for
// non-bare repos (stray per-worktree `.scratch` dirs are only possible in
// the bare-repo layout that Repo.ScratchRoot() replaced) and when nothing
// stray is found.
func FoldStrayScratchDirs(repo git.Repo, resolve ResolveCollision) error {
	if !repo.IsBare {
		return nil
	}

	strays, err := GatherStrayScratchDirs(repo)
	if err != nil {
		return err
	}
	if len(strays) == 0 {
		return nil
	}

	canonicalRoot := repo.ScratchRoot()
	canonicalEpicSlugs, err := listEpicSlugs(canonicalRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	actions := PlanFold(canonicalRoot, canonicalEpicSlugs, strays)
	return ApplyFold(actions, resolve)
}

func listEpicSlugs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, entry := range entries {
		if entry.IsDir() {
			slugs = append(slugs, entry.Name())
		}
	}
	return slugs, nil
}

func moveDir(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
		return err
	}
	return os.Rename(from, to)
}

// mergeDir unions from's entries into to, moving each entry that doesn't
// already exist at the destination and leaving conflicting entries behind in
// from. If anything is left behind, that's reported as an error rather than
// silently dropped or overwritten.
func mergeDir(from, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}

	var leftover []string
	for _, entry := range entries {
		fromPath := filepath.Join(from, entry.Name())
		toPath := filepath.Join(to, entry.Name())
		if _, err := os.Stat(toPath); err == nil {
			leftover = append(leftover, entry.Name())
			continue
		}
		if err := os.Rename(fromPath, toPath); err != nil {
			return err
		}
	}

	if len(leftover) > 0 {
		return fmt.Errorf("%d entr(ies) already present at %s, left in place: %v", len(leftover), to, leftover)
	}
	return os.Remove(from)
}
