package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// EpicScan is one epic's classification, as reported by `gx cleanup scan`.
type EpicScan struct {
	Name                string `json:"name"`
	AllDone             bool   `json:"all_done"`
	MergedToMain        bool   `json:"merged_to_main"`
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

// CleanupScanResult is the full `gx cleanup scan --json` payload.
type CleanupScanResult struct {
	Epics        []EpicScan       `json:"epics"`
	Worktrees    []WorktreeScan   `json:"worktrees"`
	Housekeeping HousekeepingScan `json:"housekeeping"`
}

// runCleanupScan resolves the current repo, classifies every epic under
// .scratch (excluding .archive), computes the repo-wide housekeeping report,
// and prints the result to w either as human-readable text or as JSON.
func runCleanupScan(cwd string, jsonOut bool, w io.Writer) error {
	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}
	repo := info.Repo

	epics, err := tickets.Load(repo.ScratchRoot())
	if err != nil {
		return err
	}

	result := CleanupScanResult{Epics: []EpicScan{}}
	activeEpics := []tickets.Epic{}
	for _, epic := range epics {
		if epic.Name == ".archive" {
			continue
		}
		activeEpics = append(activeEpics, epic)
		scan, err := scanEpic(repo, epic)
		if err != nil {
			return err
		}
		result.Epics = append(result.Epics, scan)
	}

	result.Worktrees, err = scanWorktrees(repo, activeEpics)
	if err != nil {
		return err
	}

	result.Housekeeping, err = scanHousekeeping(*info)
	if err != nil {
		return err
	}

	if jsonOut {
		return printCleanupScanJSON(w, result)
	}
	printCleanupScanText(w, result)
	return nil
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
		merged, err := git.IsCommitMergedToMain(repo.Root, epic.Name)
		if err != nil {
			return EpicScan{}, err
		}
		scan.MergedToMain = merged
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

func printCleanupScanJSON(w io.Writer, result CleanupScanResult) error {
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

func printCleanupScanText(w io.Writer, result CleanupScanResult) {
	fmt.Fprintln(w, "Epics:")
	if len(result.Epics) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, e := range result.Epics {
		codeReview := "no code-review ticket"
		if e.HasCodeReviewTicket {
			codeReview = "code-review done"
			if !e.CodeReviewDone {
				codeReview = "code-review pending"
			}
		}
		fmt.Fprintf(w, "  %s: done=%t merged=%t %s\n", e.Name, e.AllDone, e.MergedToMain, codeReview)
	}

	fmt.Fprintln(w, "Worktrees:")
	if len(result.Worktrees) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, ws := range result.Worktrees {
		detail := ws.Kind
		switch ws.Kind {
		case "iteration":
			detail = fmt.Sprintf("iteration epic=%s ticket=%s done=%t landed=%t", ws.Epic, ws.TicketID, ws.TicketDone, ws.Landed)
		case "feature":
			detail = fmt.Sprintf("feature epic=%s merged=%t", ws.Epic, ws.MergedToMain)
		case "other":
			detail = fmt.Sprintf("other merged=%t", ws.MergedToMain)
		}
		rec := ws.Recommendation
		if rec == "" {
			rec = "-"
		}
		fmt.Fprintf(w, "  %s: %s active=%t recommendation=%s\n", ws.Branch, detail, ws.Active, rec)
	}

	fmt.Fprintln(w, "Housekeeping:")
	if result.Housekeeping.Skipped {
		fmt.Fprintln(w, "  skipped (scratch root is inside a bare git directory)")
		return
	}
	if len(result.Housekeeping.TrackedFiles) == 0 {
		fmt.Fprintln(w, "  no tracked files under .scratch")
	} else {
		fmt.Fprintf(w, "  tracked files under .scratch: %s\n", strings.Join(result.Housekeeping.TrackedFiles, ", "))
	}
	if result.Housekeeping.GitignoreHasScratch {
		fmt.Fprintln(w, "  .scratch/ is gitignored")
	} else {
		fmt.Fprintln(w, "  .scratch/ is NOT gitignored")
	}
}
