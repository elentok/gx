package git

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PR represents one outgoing GitHub pull request.
type PR struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	IsDraft   bool      `json:"isDraft"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Repo is the owning repo ("owner/name"), always populated by the
	// GraphQL search query ListOpenPRs runs (see issues/12 and
	// issues/13-comments-popup.md, which fetches a single PR's comments by
	// repo+number and needs this regardless of scope).
	Repo string `json:"-"`

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

// prClosedListFields is the field set fetched for each recently-closed PR:
// enough to render a marker, title, and closed date with no facets.
const prClosedListFields = "number,title,state,mergedAt,closedAt,url"

// closedPRWindow bounds how far back ListClosedPRs looks for recently-closed
// PRs.
const closedPRWindow = 14 * 24 * time.Hour

// ClosedPR represents one recently-closed outgoing GitHub pull request
// (merged or closed-unmerged), rendered with no facets.
type ClosedPR struct {
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	State    string    `json:"state"`
	MergedAt time.Time `json:"mergedAt"`
	ClosedAt time.Time `json:"closedAt"`
	URL      string    `json:"url"`

	// Repo is the owning repo ("owner/name"), populated only by the
	// --all-repos GraphQL fetch (listClosedPRsAllRepos).
	Repo string `json:"-"`
}

// IsMerged reports whether the PR was merged rather than closed-unmerged.
func (pr ClosedPR) IsMerged() bool {
	return pr.State == "MERGED"
}

// ListClosedPRs returns the current user's outgoing PRs closed (merged or
// closed-unmerged) in the last two weeks, most recently closed first. When
// allRepos is false the search is scoped to the repo at dir; when true it
// spans every repo the user has closed PRs in.
func ListClosedPRs(dir string, allRepos bool) ([]ClosedPR, error) {
	cutoff := time.Now().Add(-closedPRWindow).Format("2006-01-02")
	if allRepos {
		return listClosedPRsAllRepos(dir, cutoff)
	}
	out, err := runGH(dir, []string{
		"pr", "list",
		"--state", "closed",
		"--search", "closed:>" + cutoff,
		"--limit", "100",
		"--author", "@me",
		"--json", prClosedListFields,
	})
	if err != nil {
		return nil, classifyPRListError(err)
	}
	prs, err := parseClosedPRList(out)
	if err != nil {
		return nil, err
	}
	sortClosedPRs(prs)
	return prs, nil
}

// listClosedPRsAllRepos fetches every repo's recently-closed PRs in a single
// GraphQL search query (see issues/12-all-mode-graphql-migration.md), rather
// than a repo-discovery search plus one `gh pr list` call per repo.
func listClosedPRsAllRepos(dir, cutoff string) ([]ClosedPR, error) {
	nodes, err := runGraphQLPRSearch[closedPRSearchNode](dir, closedPRsSearchQuery, "is:pr author:@me is:closed closed:>"+cutoff)
	if err != nil {
		return nil, err
	}
	prs := make([]ClosedPR, len(nodes))
	for i, n := range nodes {
		prs[i] = n.toClosedPR()
	}
	sortClosedPRs(prs)
	return prs, nil
}

// sortClosedPRs orders prs by closed date, most recent first.
func sortClosedPRs(prs []ClosedPR) {
	sort.Slice(prs, func(i, j int) bool {
		return prs[i].ClosedAt.After(prs[j].ClosedAt)
	})
}

// parseClosedPRList decodes the JSON array produced by
// `gh pr list --state closed ...`.
func parseClosedPRList(jsonOut string) ([]ClosedPR, error) {
	var prs []ClosedPR
	if strings.TrimSpace(jsonOut) == "" {
		return prs, nil
	}
	if err := json.Unmarshal([]byte(jsonOut), &prs); err != nil {
		return nil, fmt.Errorf("parsing gh pr list --state closed output: %w", err)
	}
	return prs, nil
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

// ListOpenPRs returns the current user's outgoing open PRs: actionable PRs
// first (green group, then red group), each group most-recently-updated
// first, followed by non-actionable PRs, also most-recently-updated first.
// When allRepos is false the search is scoped to the repo at dir via a
// repo: qualifier; when true it spans every repo the user has open PRs in.
// Always goes through the GraphQL search query (see
// issues/12-all-mode-graphql-migration.md) rather than `gh pr list`, so the
// comment-count facet never costs a full comment-body fetch (see
// issues/13-comments-popup.md).
func ListOpenPRs(dir string, allRepos bool) ([]PR, error) {
	searchQuery := "is:pr author:@me is:open"
	if !allRepos {
		repo, err := currentRepoNameWithOwner(dir)
		if err != nil {
			return nil, classifyPRListError(err)
		}
		searchQuery += " repo:" + repo
	}
	nodes, err := runGraphQLPRSearch[prSearchNode](dir, openPRsSearchQuery, searchQuery)
	if err != nil {
		return nil, err
	}
	prs := make([]PR, len(nodes))
	for i, n := range nodes {
		prs[i] = n.toPR()
	}
	sortPRs(prs)
	return prs, nil
}

// currentRepoNameWithOwner resolves the "owner/name" of the repo at dir.
// GraphQL search has no notion of "current directory", so scoping
// ListOpenPRs's search query to a single repo needs this explicit qualifier.
func currentRepoNameWithOwner(dir string) (string, error) {
	return runGH(dir, []string{"repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"})
}

// runGHSearchPRs runs `gh search prs --author @me --json <jsonFields>` with
// the given extra filter args. Used only by the all-repos existence probe
// (AnyPRsExist) — the full open/closed fetches go through
// runGraphQLPRSearch instead.
func runGHSearchPRs(dir, jsonFields string, extraArgs ...string) (string, error) {
	args := append([]string{
		"search", "prs",
		"--author", "@me",
		"--limit", "100",
		"--json", jsonFields,
	}, extraArgs...)
	out, err := runGH(dir, args)
	if err != nil {
		return "", classifyPRListError(err)
	}
	return out, nil
}

// openPRsSearchQuery fetches every field needed to render an open-PR row and
// derive its facets in one shot: identity fields, the CI rollup state for
// the tip commit, mergeability, review decision/bodies, and comment/
// review-request counts.
const openPRsSearchQuery = `
query($searchQuery: String!) {
  search(query: $searchQuery, type: ISSUE, first: 100) {
    nodes {
      ... on PullRequest {
        number
        title
        url
        isDraft
        updatedAt
        mergeable
        reviewDecision
        repository { name owner { login } }
        commits(last: 1) {
          nodes { commit { statusCheckRollup { state } } }
        }
        reviews(last: 50) { nodes { state body } }
        comments { totalCount }
        reviewRequests { totalCount }
      }
    }
  }
}`

// closedPRsSearchQuery fetches only what a closed-PR row needs (no facets).
const closedPRsSearchQuery = `
query($searchQuery: String!) {
  search(query: $searchQuery, type: ISSUE, first: 100) {
    nodes {
      ... on PullRequest {
        number
        title
        url
        state
        mergedAt
        closedAt
        repository { name owner { login } }
      }
    }
  }
}`

// prSearchRepo is the repository identity shape shared by both search node
// types.
type prSearchRepo struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func (r prSearchRepo) nameWithOwner() string {
	return r.Owner.Login + "/" + r.Name
}

// prSearchNode is the shape of one `... on PullRequest` node returned by
// openPRsSearchQuery.
type prSearchNode struct {
	Number         int          `json:"number"`
	Title          string       `json:"title"`
	URL            string       `json:"url"`
	IsDraft        bool         `json:"isDraft"`
	UpdatedAt      time.Time    `json:"updatedAt"`
	Mergeable      string       `json:"mergeable"`
	ReviewDecision string       `json:"reviewDecision"`
	Repository     prSearchRepo `json:"repository"`
	Commits        struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	Reviews struct {
		Nodes []PRReview `json:"nodes"`
	} `json:"reviews"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	ReviewRequests struct {
		TotalCount int `json:"totalCount"`
	} `json:"reviewRequests"`
}

// toPR converts the GraphQL node into a PR with facet-equivalent data. The
// tip commit's single rolled-up CI state is expanded into a synthetic
// one-element StatusCheckRollup so PR.CIState() classifies it the same way
// it would the REST API's per-check array; comment/review-request counts
// become placeholder slices of the right length since only their length is
// ever read.
func (n prSearchNode) toPR() PR {
	var rollupState string
	if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		rollupState = n.Commits.Nodes[0].Commit.StatusCheckRollup.State
	}
	return PR{
		Number:            n.Number,
		Title:             n.Title,
		URL:               n.URL,
		IsDraft:           n.IsDraft,
		UpdatedAt:         n.UpdatedAt,
		Repo:              n.Repository.nameWithOwner(),
		Mergeable:         n.Mergeable,
		ReviewDecision:    n.ReviewDecision,
		StatusCheckRollup: graphQLRollupToChecks(rollupState),
		Reviews:           n.Reviews.Nodes,
		Comments:          make([]json.RawMessage, n.Comments.TotalCount),
		ReviewRequests:    make([]json.RawMessage, n.ReviewRequests.TotalCount),
	}
}

// graphQLRollupToChecks maps GraphQL's single rolled-up commit
// statusCheckRollup.state onto the synthetic one-element PRStatusCheck slice
// PR.CIState() expects, preserving CINone/CIFailed/CIRunning/CIPassed
// classification without needing the individual per-check array the REST
// API returns.
func graphQLRollupToChecks(state string) []PRStatusCheck {
	switch state {
	case "":
		return nil
	case "SUCCESS":
		return []PRStatusCheck{{Status: "COMPLETED", Conclusion: "SUCCESS"}}
	case "FAILURE", "ERROR":
		return []PRStatusCheck{{Status: "COMPLETED", Conclusion: "FAILURE"}}
	default: // PENDING, EXPECTED
		return []PRStatusCheck{{Status: "IN_PROGRESS"}}
	}
}

// closedPRSearchNode is the shape of one `... on PullRequest` node returned
// by closedPRsSearchQuery.
type closedPRSearchNode struct {
	Number     int          `json:"number"`
	Title      string       `json:"title"`
	URL        string       `json:"url"`
	State      string       `json:"state"`
	MergedAt   time.Time    `json:"mergedAt"`
	ClosedAt   time.Time    `json:"closedAt"`
	Repository prSearchRepo `json:"repository"`
}

func (n closedPRSearchNode) toClosedPR() ClosedPR {
	return ClosedPR{
		Number:   n.Number,
		Title:    n.Title,
		State:    n.State,
		MergedAt: n.MergedAt,
		ClosedAt: n.ClosedAt,
		URL:      n.URL,
		Repo:     n.Repository.nameWithOwner(),
	}
}

// graphQLSearchEnvelope is the `{"data":{"search":{"nodes":[...]}}}` shape
// gh api graphql returns for both openPRsSearchQuery and
// closedPRsSearchQuery.
type graphQLSearchEnvelope[T any] struct {
	Errors graphQLErrors `json:"errors"`
	Data   struct {
		Search struct {
			Nodes []T `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
}

// graphQLErrors is the `errors` array `gh api graphql` responses carry
// alongside (or instead of) `data`, shared by every envelope shape so the
// "any errors? report the first one" check isn't repeated per envelope.
type graphQLErrors []struct {
	Message string `json:"message"`
}

func (e graphQLErrors) err() error {
	if len(e) == 0 {
		return nil
	}
	return fmt.Errorf("gh api graphql: %s", e[0].Message)
}

// runGraphQLRequest runs `gh api graphql` with the given query plus extra
// `-f`/`-F` flag args (e.g. `"-f", "searchQuery=..."` or `"-F",
// "number=5"`), decoding the JSON response into target. Shared by
// runGraphQLPRSearch and FetchPRComments so the "shell out to gh, then
// decode" mechanics aren't duplicated per query shape.
func runGraphQLRequest(dir, query string, extraArgs []string, target any) error {
	args := append([]string{"api", "graphql", "-f", "query=" + query}, extraArgs...)
	out, err := runGH(dir, args)
	if err != nil {
		return classifyPRListError(err)
	}
	if err := json.Unmarshal([]byte(out), target); err != nil {
		return fmt.Errorf("parsing gh api graphql response: %w", err)
	}
	return nil
}

// runGraphQLPRSearch runs a search-shaped GraphQL query with the given
// $searchQuery variable, decoding the resulting nodes as T (either
// prSearchNode or closedPRSearchNode). Shared by listOpenPRsAllRepos and
// listClosedPRsAllRepos so the "one GraphQL search query, across every repo"
// shape isn't duplicated for each PR kind.
func runGraphQLPRSearch[T any](dir, query, searchQuery string) ([]T, error) {
	var envelope graphQLSearchEnvelope[T]
	if err := runGraphQLRequest(dir, query, []string{"-f", "searchQuery=" + searchQuery}, &envelope); err != nil {
		return nil, err
	}
	if err := envelope.Errors.err(); err != nil {
		return nil, err
	}
	return envelope.Data.Search.Nodes, nil
}

// AnyPRsExist reports whether the user has any PRs at all (open or closed),
// scoped to the repo at dir when allRepos is false, or across every repo
// when true. Used to distinguish "no open PRs" from "no PRs found" when the
// open-PR list comes back empty.
func AnyPRsExist(dir string, allRepos bool) (bool, error) {
	if allRepos {
		out, err := runGHSearchPRs(dir, "number", "--limit", "1")
		if err != nil {
			return false, err
		}
		out = strings.TrimSpace(out)
		return out != "" && out != "[]", nil
	}
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

// PRComment is one entry in a PR's comment timeline shown in the comments
// popup (issues/13-comments-popup.md): either an issue comment or a
// non-empty review-summary body, unified so both render the same way.
type PRComment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

// prCommentsQuery fetches a single PR's issue comments and review bodies
// directly by repo+number (not via search, since the caller already knows
// exactly which PR it wants) — reuses the `gh api graphql` calling
// convention runGraphQLPRSearch introduced in issues/12 rather than adding a
// second gh-shelling mechanism.
const prCommentsQuery = `
query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      comments(first: 100) {
        nodes { author { login } body createdAt }
      }
      reviews(first: 100) {
        nodes { author { login } body submittedAt }
      }
    }
  }
}`

type prCommentsEnvelope struct {
	Errors graphQLErrors `json:"errors"`
	Data   struct {
		Repository struct {
			PullRequest struct {
				Comments struct {
					Nodes []struct {
						Author    struct{ Login string } `json:"author"`
						Body      string                 `json:"body"`
						CreatedAt time.Time              `json:"createdAt"`
					} `json:"nodes"`
				} `json:"comments"`
				Reviews struct {
					Nodes []struct {
						Author      struct{ Login string } `json:"author"`
						Body        string                 `json:"body"`
						SubmittedAt time.Time              `json:"submittedAt"`
					} `json:"nodes"`
				} `json:"reviews"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// FetchPRComments fetches the full comment timeline for one PR — its issue
// comments plus any non-empty review-summary bodies — sorted chronologically
// (oldest first). Called on demand only for the PR whose comments popup is
// open, never for every row up front.
func FetchPRComments(dir, repo string, number int) ([]PRComment, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, fmt.Errorf("FetchPRComments: invalid repo %q, expected owner/name", repo)
	}
	extraArgs := []string{
		"-f", "owner=" + owner,
		"-f", "name=" + name,
		"-F", "number=" + strconv.Itoa(number),
	}
	var envelope prCommentsEnvelope
	if err := runGraphQLRequest(dir, prCommentsQuery, extraArgs, &envelope); err != nil {
		return nil, err
	}
	return commentsFromEnvelope(envelope)
}

// commentsFromEnvelope converts a decoded prCommentsEnvelope into a
// chronologically sorted (oldest first) comment timeline: issue comments
// plus any non-empty review-summary bodies, empty ones dropped.
func commentsFromEnvelope(envelope prCommentsEnvelope) ([]PRComment, error) {
	if err := envelope.Errors.err(); err != nil {
		return nil, err
	}
	pr := envelope.Data.Repository.PullRequest
	comments := make([]PRComment, 0, len(pr.Comments.Nodes)+len(pr.Reviews.Nodes))
	for _, c := range pr.Comments.Nodes {
		comments = append(comments, PRComment{Author: c.Author.Login, Body: c.Body, CreatedAt: c.CreatedAt})
	}
	for _, r := range pr.Reviews.Nodes {
		if strings.TrimSpace(r.Body) == "" {
			continue
		}
		comments = append(comments, PRComment{Author: r.Author.Login, Body: r.Body, CreatedAt: r.SubmittedAt})
	}
	sort.Slice(comments, func(i, j int) bool {
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})
	return comments, nil
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
