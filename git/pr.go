package git

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// prListFields is the field set fetched for each PR: identity/display fields
// plus the raw facet inputs needed to derive CI/approval/mergeable/comment
// state and the actionable marker.
const prListFields = "number,title,url,isDraft,updatedAt,statusCheckRollup,reviewDecision,reviews,mergeable,comments,reviewRequests"

// PR represents one outgoing GitHub pull request.
type PR struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	IsDraft   bool      `json:"isDraft"`
	UpdatedAt time.Time `json:"updatedAt"`

	Mergeable         string            `json:"mergeable"`
	ReviewDecision    string            `json:"reviewDecision"`
	StatusCheckRollup []PRStatusCheck   `json:"statusCheckRollup"`
	Reviews           []PRReview        `json:"reviews"`
	Comments          []json.RawMessage `json:"comments"`
	ReviewRequests    []json.RawMessage `json:"reviewRequests"`
}

type PRStatusCheck struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type PRReview struct {
	State string `json:"state"`
	Body  string `json:"body"`
}

// CIState is the rolled-up status-check state for a PR.
type CIState int

const (
	CINone CIState = iota
	CIRunning
	CIFailed
	CIPassed
)

// ApprovalState is a PR's review-approval state.
type ApprovalState int

const (
	ApprovalNotYet ApprovalState = iota
	ApprovalApproved
	ApprovalChangesRequested
	ApprovalCommentedOnly
)

// MergeableState is a PR's merge-conflict state.
type MergeableState int

const (
	MergeableChecking MergeableState = iota
	MergeableClean
	MergeableConflicting
)

// Marker is the 3-state actionable summary derived from a PR's facets.
type Marker int

const (
	MarkerNeutral Marker = iota
	MarkerGreen
	MarkerRed
)

var failedCheckConclusions = map[string]bool{
	"FAILURE":         true,
	"TIMED_OUT":       true,
	"ACTION_REQUIRED": true,
	"CANCELLED":       true,
}

// CIState rolls up StatusCheckRollup into a single state: any failing
// conclusion wins, else any still-running check wins, else passed (or none
// if there are no checks at all).
func (pr PR) CIState() CIState {
	if len(pr.StatusCheckRollup) == 0 {
		return CINone
	}
	for _, c := range pr.StatusCheckRollup {
		if failedCheckConclusions[c.Conclusion] {
			return CIFailed
		}
	}
	for _, c := range pr.StatusCheckRollup {
		if c.Status != "COMPLETED" {
			return CIRunning
		}
	}
	return CIPassed
}

// ApprovalState derives the review-approval state. reviewersRequested is only
// meaningful when the returned state is ApprovalNotYet: it distinguishes
// "waiting on requested reviewers" (not actionable) from "no reviewers
// requested" (blocked on the PR owner).
func (pr PR) ApprovalState() (state ApprovalState, reviewersRequested bool) {
	switch pr.ReviewDecision {
	case "APPROVED":
		return ApprovalApproved, false
	case "CHANGES_REQUESTED":
		return ApprovalChangesRequested, false
	}
	for _, r := range pr.Reviews {
		if r.State == "COMMENTED" {
			return ApprovalCommentedOnly, false
		}
	}
	return ApprovalNotYet, len(pr.ReviewRequests) > 0
}

// MergeableState derives the merge-conflict state from the mergeable field.
func (pr PR) MergeableState() MergeableState {
	switch pr.Mergeable {
	case "CONFLICTING":
		return MergeableConflicting
	case "MERGEABLE":
		return MergeableClean
	default:
		return MergeableChecking
	}
}

// CommentCount is a lower-bound count of issue comments plus non-empty review
// bodies (inline diff comments are excluded).
func (pr PR) CommentCount() int {
	count := len(pr.Comments)
	for _, r := range pr.Reviews {
		if strings.TrimSpace(r.Body) != "" {
			count++
		}
	}
	return count
}

// Facets bundles a PR's derived facet states, computed once and reused for
// both marker classification and row rendering (avoids re-deriving each
// facet independently for the marker and for the facet line).
type Facets struct {
	CI                 CIState
	Approval           ApprovalState
	ReviewersRequested bool
	Mergeable          MergeableState
	CommentCount       int
}

// Facets derives all of a PR's facet states in one pass.
func (pr PR) Facets() Facets {
	approval, reviewersRequested := pr.ApprovalState()
	return Facets{
		CI:                 pr.CIState(),
		Approval:           approval,
		ReviewersRequested: reviewersRequested,
		Mergeable:          pr.MergeableState(),
		CommentCount:       pr.CommentCount(),
	}
}

// Marker classifies the facets as green (actionable, mergeable-clean), red
// (actionable, blocked on the PR owner), or neutral (waiting on others).
func (f Facets) Marker() Marker {
	blocked := f.CI == CIFailed ||
		f.Approval == ApprovalChangesRequested ||
		f.Mergeable == MergeableConflicting ||
		(f.Approval == ApprovalNotYet && !f.ReviewersRequested)
	if blocked {
		return MarkerRed
	}

	if f.CI == CIPassed && f.Approval == ApprovalApproved && f.Mergeable == MergeableClean {
		return MarkerGreen
	}

	return MarkerNeutral
}

// Marker classifies the PR as green (actionable, mergeable-clean), red
// (actionable, blocked on the PR owner), or neutral (waiting on others).
func (pr PR) Marker() Marker {
	return pr.Facets().Marker()
}

func markerSortRank(m Marker) int {
	switch m {
	case MarkerGreen:
		return 0
	case MarkerRed:
		return 1
	default:
		return 2
	}
}

// PRListErrorKind classifies a gh pr list failure so callers can render
// tailored inline messages without re-parsing gh's output themselves.
type PRListErrorKind int

const (
	PRListErrorGeneric PRListErrorKind = iota
	PRListErrorGHNotInstalled
	PRListErrorUnauthenticated
)

// PRListError wraps a gh pr list/related failure with a classified kind.
type PRListError struct {
	Kind PRListErrorKind
	Err  error
}

func (e *PRListError) Error() string { return e.Err.Error() }
func (e *PRListError) Unwrap() error { return e.Err }

// classifyPRListError distinguishes "gh not installed" and "gh
// unauthenticated" from gh's other failure modes (network, rate limit, no
// GitHub remote, ...), which fall back to gh's raw wrapped message.
func classifyPRListError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return &PRListError{Kind: PRListErrorGHNotInstalled, Err: err}
	}
	var runErr *RunError
	if errors.As(err, &runErr) && isGHAuthFailure(runErr.Stderr) {
		return &PRListError{Kind: PRListErrorUnauthenticated, Err: err}
	}
	return &PRListError{Kind: PRListErrorGeneric, Err: err}
}

func isGHAuthFailure(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "gh auth login")
}

// ListOpenPRs returns the current user's outgoing open PRs in the repo at
// dir: actionable PRs first (green group, then red group), each group
// most-recently-updated first, followed by non-actionable PRs, also
// most-recently-updated first.
func ListOpenPRs(dir string) ([]PR, error) {
	out, err := runGH(dir, []string{
		"pr", "list",
		"--author", "@me",
		"--json", prListFields,
	})
	if err != nil {
		return nil, classifyPRListError(err)
	}
	prs, err := parsePRList(out)
	if err != nil {
		return nil, err
	}
	sortPRs(prs)
	return prs, nil
}

// AnyPRsExist reports whether the user has any PRs at all (open or closed)
// in the repo at dir. Used to distinguish "no open PRs" from "no PRs found"
// when the open-PR list comes back empty.
func AnyPRsExist(dir string) (bool, error) {
	out, err := runGH(dir, []string{
		"pr", "list",
		"--author", "@me",
		"--state", "all",
		"--limit", "1",
		"--json", "number",
	})
	if err != nil {
		return false, classifyPRListError(err)
	}
	out = strings.TrimSpace(out)
	return out != "" && out != "[]", nil
}

// sortPRs orders prs actionable-first (green group, then red group), each
// group most-recently-updated first, followed by non-actionable PRs, also
// most-recently-updated first.
func sortPRs(prs []PR) {
	sort.Slice(prs, func(i, j int) bool {
		ri, rj := markerSortRank(prs[i].Marker()), markerSortRank(prs[j].Marker())
		if ri != rj {
			return ri < rj
		}
		return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
	})
}

// parsePRList decodes the JSON array produced by `gh pr list --json ...`.
func parsePRList(jsonOut string) ([]PR, error) {
	var prs []PR
	if strings.TrimSpace(jsonOut) == "" {
		return prs, nil
	}
	if err := json.Unmarshal([]byte(jsonOut), &prs); err != nil {
		return nil, fmt.Errorf("parsing gh pr list output: %w", err)
	}
	return prs, nil
}

// BranchPRURL returns the GitHub PR URL for the current branch in worktreeRoot.
// Uses `gh pr view` which returns an error if no open PR exists.
func BranchPRURL(worktreeRoot string) (string, error) {
	return runGH(worktreeRoot, []string{"pr", "view", "--json", "url", "-q", ".url"})
}

// CommitPRURL returns the GitHub PR URL for the given commit hash, or "" if no
// PR is found. An empty result is not an error — callers surface it as a warning.
func CommitPRURL(worktreeRoot, hash string) (string, error) {
	return runGH(worktreeRoot, []string{
		"pr", "list",
		"--search", hash,
		"--state", "all",
		"--json", "url",
		"-q", ".[0].url",
	})
}

// IsCommitMergedToMain reports whether hash is an ancestor of the repo's main
// branch. Returns false (not an error) when git exits with code 1.
func IsCommitMergedToMain(worktreeRoot, hash string) (bool, error) {
	repo, err := FindRepo(worktreeRoot)
	if err != nil {
		return false, fmt.Errorf("IsCommitMergedToMain: %w", err)
	}
	_, _, err = run(repo.Root, []string{"merge-base", "--is-ancestor", hash, repo.MainBranch})
	if err != nil {
		if runErr, ok := err.(*RunError); ok && runErr.Code == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// runGH executes a gh command in dir and returns trimmed stdout.
func runGH(dir string, args []string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", &RunError{
				Args:   args,
				Dir:    dir,
				Stdout: strings.TrimSpace(outBuf.String()),
				Stderr: strings.TrimSpace(errBuf.String()),
				Code:   exitErr.ExitCode(),
			}
		}
		return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimRight(outBuf.String(), "\r\n"), nil
}
