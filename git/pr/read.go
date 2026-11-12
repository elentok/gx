package pr

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/elentok/gx/git/internal/run"
)

// PreflightError is a failed Preflight. Reason is meant for a person.
type PreflightError struct {
	Reason string
	Err    error
}

func (e *PreflightError) Error() string { return e.Reason }
func (e *PreflightError) Unwrap() error { return e.Err }

// Preflight checks that gh is installed and logged in, that Dir has an origin
// remote, and that gh can see that repo. It returns the "owner/repo" and
// changes nothing; failures are *PreflightError.
func (r Runner) Preflight() (string, error) {
	if _, err := lookPathIn(r.Env, "gh"); err != nil {
		return "", &PreflightError{Reason: "gh is not installed", Err: err}
	}
	url, _, err := run.Run(r.Dir, r.Env, "remote", "get-url", "origin")
	if err != nil || url == "" {
		return "", &PreflightError{Reason: "this repo has no `origin` remote", Err: err}
	}
	repo, ok := repoFromRemoteURL(url)
	if !ok {
		return "", &PreflightError{Reason: fmt.Sprintf("cannot read owner/repo from origin %q", url)}
	}
	_, err = r.Run("repo", "view", repo, "--json", "nameWithOwner", "-q", ".nameWithOwner")
	if err != nil {
		var listErr *PRListError
		if errors.As(classifyPRListError(err), &listErr) && listErr.Kind == PRListErrorUnauthenticated {
			return "", &PreflightError{Reason: "gh is not logged in, run `gh auth login`", Err: err}
		}
		return "", &PreflightError{Reason: fmt.Sprintf("gh cannot access %s", repo), Err: err}
	}
	return repo, nil
}

// repoFromRemoteURL extracts "owner/repo" from the usual git remote forms
// (git@host:o/r.git, https://host/o/r, ssh://git@host/o/r.git).
func repoFromRemoteURL(url string) (string, bool) {
	url = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(url), "/"), ".git")
	parts := strings.FieldsFunc(url, func(c rune) bool { return c == '/' || c == ':' })
	if len(parts) < 2 || strings.Contains(url, " ") {
		return "", false
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1], true
}

// HeadPR is a PR found by its head branch.
type HeadPR struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	State       string `json:"state"` // OPEN, MERGED or CLOSED
	IsDraft     bool   `json:"isDraft"`
	HeadSHA     string `json:"headRefOid"`
	MergeCommit string `json:"-"`
}

// FindByHead returns the PR whose head is branch, or nil when there is none.
// An open PR wins over a merged one, which wins over a closed one; State tells
// them apart.
func (r Runner) FindByHead(repo, branch string) (*HeadPR, error) {
	out, err := r.Run("pr", "list", "-R", repo, "--head", branch, "--state", "all",
		"--limit", "30", "--json", "number,url,state,isDraft,headRefOid,mergeCommit")
	if err != nil {
		return nil, classifyPRListError(err)
	}
	var raw []struct {
		HeadPR
		MergeCommit struct {
			OID string `json:"oid"`
		} `json:"mergeCommit"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parsing gh pr list response: %w", err)
	}
	rank := map[string]int{"OPEN": 0, "MERGED": 1, "CLOSED": 2}
	var best *HeadPR
	for _, n := range raw {
		p := n.HeadPR
		p.MergeCommit = n.MergeCommit.OID
		if best == nil || rank[p.State] < rank[best.State] {
			best = &p
		}
	}
	return best, nil
}

// WatchState is the cheap per-PR state the watcher polls.
type WatchState struct {
	State       string
	IsDraft     bool
	HeadSHA     string
	MergeCommit string
	// Mergeable is set only while State is OPEN; GitHub's value on a closed
	// PR means nothing.
	Mergeable string
	// Rollup is the head commit's status-check rollup state (SUCCESS, FAILURE,
	// ERROR, PENDING, EXPECTED), empty when the head has no checks.
	Rollup string
}

// WatchQuery fetches the cheap fields of many PRs in one GraphQL call.
func (r Runner) WatchQuery(repo string, numbers []int) (map[int]WatchState, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, fmt.Errorf("repo %q is not owner/name", repo)
	}
	var b strings.Builder
	b.WriteString("query($o:String!,$n:String!){repository(owner:$o,name:$n){")
	for _, num := range numbers {
		fmt.Fprintf(&b, "p%d:pullRequest(number:%d){state isDraft headRefOid mergeCommit{oid} mergeable commits(last:1){nodes{commit{statusCheckRollup{state}}}}} ", num, num)
	}
	b.WriteString("}}")
	var env struct {
		Errors graphQLErrors `json:"errors"`
		Data   struct {
			Repository map[string]*struct {
				State       string `json:"state"`
				IsDraft     bool   `json:"isDraft"`
				HeadSHA     string `json:"headRefOid"`
				MergeCommit *struct {
					OID string `json:"oid"`
				} `json:"mergeCommit"`
				Mergeable string `json:"mergeable"`
				Commits   struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								State string `json:"state"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := r.graphQL(b.String(), []string{"-f", "o=" + owner, "-f", "n=" + name}, &env); err != nil {
		return nil, err
	}
	if err := env.Errors.err(); err != nil {
		return nil, err
	}
	out := map[int]WatchState{}
	for _, num := range numbers {
		n := env.Data.Repository["p"+strconv.Itoa(num)]
		if n == nil {
			continue
		}
		s := WatchState{State: n.State, IsDraft: n.IsDraft, HeadSHA: n.HeadSHA}
		if n.MergeCommit != nil {
			s.MergeCommit = n.MergeCommit.OID
		}
		if n.State == "OPEN" {
			s.Mergeable = n.Mergeable
		}
		if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			s.Rollup = n.Commits.Nodes[0].Commit.StatusCheckRollup.State
		}
		out[num] = s
	}
	return out, nil
}

// maxPRChecksPages bounds the contexts paging so a misbehaving cursor cannot loop.
const maxPRChecksPages = 10

const prChecksQuery = `query($o:String!,$n:String!,$p:Int!,$c:String){
  repository(owner:$o,name:$n){ pullRequest(number:$p){ commits(last:1){ nodes{ commit{ oid
    statusCheckRollup{ contexts(first:100,after:$c){ pageInfo{ hasNextPage endCursor } nodes{ __typename
      ... on CheckRun{ databaseId name status conclusion detailsUrl
        checkSuite{ workflowRun{ databaseId workflow{ databaseId } } } }
      ... on StatusContext{ context state targetUrl } } } } } } } } }
}`

// PRChecks fetches the per-check detail of a PR's current head, for the
// classifier (ClassifyChecks). It pages the contexts connection; an error on
// any page returns no partial list.
func (r Runner) PRChecks(repo string, number int) (headSHA string, checks []Check, err error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return "", nil, fmt.Errorf("repo %q is not owner/name", repo)
	}
	cursor := ""
	for range maxPRChecksPages {
		sha, page, next, err := r.prChecksPage(owner, name, number, cursor)
		if err != nil {
			return "", nil, err
		}
		headSHA = sha
		checks = append(checks, page...)
		if next == "" {
			return headSHA, checks, nil
		}
		cursor = next
	}
	return "", nil, fmt.Errorf("PR %s#%d has more than %d pages of checks", repo, number, maxPRChecksPages)
}

// prChecksPage reads one page of contexts. next is the cursor of the following
// page, or "" when this was the last.
func (r Runner) prChecksPage(owner, name string, number int, cursor string) (headSHA string, checks []Check, next string, err error) {
	var env struct {
		Errors graphQLErrors `json:"errors"`
		Data   struct {
			Repository struct {
				PullRequest struct {
					Commits struct {
						Nodes []struct {
							Commit struct {
								OID               string `json:"oid"`
								StatusCheckRollup *struct {
									Contexts struct {
										PageInfo struct {
											HasNextPage bool   `json:"hasNextPage"`
											EndCursor   string `json:"endCursor"`
										} `json:"pageInfo"`
										Nodes []checkContextNode `json:"nodes"`
									} `json:"contexts"`
								} `json:"statusCheckRollup"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	args := []string{"-f", "o=" + owner, "-f", "n=" + name, "-F", "p=" + strconv.Itoa(number)}
	if cursor != "" {
		args = append(args, "-f", "c="+cursor)
	}
	if err := r.graphQL(prChecksQuery, args, &env); err != nil {
		return "", nil, "", err
	}
	if err := env.Errors.err(); err != nil {
		return "", nil, "", err
	}
	nodes := env.Data.Repository.PullRequest.Commits.Nodes
	if len(nodes) == 0 {
		return "", nil, "", nil
	}
	commit := nodes[0].Commit
	if rollup := commit.StatusCheckRollup; rollup != nil {
		for _, n := range rollup.Contexts.Nodes {
			checks = append(checks, n.toCheck(commit.OID))
		}
		if rollup.Contexts.PageInfo.HasNextPage {
			next = rollup.Contexts.PageInfo.EndCursor
		}
	}
	return commit.OID, checks, next, nil
}

type checkContextNode struct {
	Typename   string `json:"__typename"`
	DatabaseID int64  `json:"databaseId"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
	CheckSuite struct {
		WorkflowRun *struct {
			DatabaseID int64 `json:"databaseId"`
			Workflow   struct {
				DatabaseID int64 `json:"databaseId"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
}

func (n checkContextNode) toCheck(headSHA string) Check {
	if n.Typename != "CheckRun" {
		// A legacy status has no status/conclusion pair: map it onto one.
		c := Check{Name: n.Context, HeadSHA: headSHA, URL: n.TargetURL, Status: "COMPLETED"}
		switch n.State {
		case "SUCCESS":
			c.Conclusion = "SUCCESS"
		case "FAILURE", "ERROR":
			c.Conclusion = "FAILURE"
		default:
			c.Status = "PENDING"
		}
		return c
	}
	c := Check{
		Name: n.Name, HeadSHA: headSHA, Status: n.Status, Conclusion: n.Conclusion,
		URL: n.DetailsURL, JobID: strconv.FormatInt(n.DatabaseID, 10),
	}
	if wr := n.CheckSuite.WorkflowRun; wr != nil {
		c.RunID = strconv.FormatInt(wr.DatabaseID, 10)
		c.WorkflowID = strconv.FormatInt(wr.Workflow.DatabaseID, 10)
	}
	return c
}

func (r Runner) graphQL(query string, extra []string, target any) error {
	out, err := r.Run(append([]string{"api", "graphql", "-f", "query=" + query}, extra...)...)
	if err != nil {
		return classifyPRListError(err)
	}
	if err := json.Unmarshal([]byte(out), target); err != nil {
		return fmt.Errorf("parsing gh api graphql response: %w", err)
	}
	return nil
}
