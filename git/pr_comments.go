package git

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
