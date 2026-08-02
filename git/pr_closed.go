package git

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

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
