package git

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

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
