package prflow

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// MaxTextTries is how many pr-text tickets one run gets: the first try and two
// retries. A reset in the activity log starts the count again.
const MaxTextTries = 3

const (
	textSlug       = "pr-text"
	epicLabel      = "Epic:"
	worktreeLabel  = "Worktree:"
	skillLabel     = "Skill:"
	blockKindKey   = "land_block_kind"
	blockReasonKey = "land_block_reason"
)

// TextJob is what a pr-text agent is told: the epic, the feature worktree it
// reads, and the skill that overrides the bundled default rules (empty: none).
type TextJob struct {
	Epic     string
	Worktree string
	Skill    string
}

func (j TextJob) body(id string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n# %s — Write the PR text for %s\n\n## What to build\n\n", id, j.Epic)
	b.WriteString("Write the pull request title and body. Read the feature worktree; do not change it.\n\n")
	fmt.Fprintf(&b, "%s %s\n%s %s\n", epicLabel, j.Epic, worktreeLabel, j.Worktree)
	if j.Skill != "" {
		fmt.Fprintf(&b, "%s %s\n", skillLabel, j.Skill)
	}
	return b.String()
}

func jobFromBody(body string) TextJob {
	var j TextJob
	for line := range strings.SplitSeq(body, "\n") {
		switch {
		case strings.HasPrefix(line, epicLabel):
			j.Epic = strings.TrimSpace(strings.TrimPrefix(line, epicLabel))
		case strings.HasPrefix(line, worktreeLabel):
			j.Worktree = strings.TrimSpace(strings.TrimPrefix(line, worktreeLabel))
		case strings.HasPrefix(line, skillLabel):
			j.Skill = strings.TrimSpace(strings.TrimPrefix(line, skillLabel))
		}
	}
	return j
}

// StartPRText writes one pr-text ticket into the epic at epicPath. It writes
// none, and reports false, while one is open or claimed.
func StartPRText(epicPath string, job TextJob) (id string, started bool, err error) {
	if slices.ContainsFunc(tickets.LoadEpicAt(epicPath).Tickets, func(t tickets.Ticket) bool {
		return t.Type == string(schema.TypePRText) && (t.Status == string(schema.StatusOpen) || t.Status == string(schema.StatusClaimed))
	}) {
		return "", false, nil
	}
	_, id, err = ticketwrite.WriteNewTicket(epicPath, "", textSlug, func(id string) (schema.Ticket, string) {
		return schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusOpen, Type: schema.TypePRText}, job.body(id)
	})
	return id, err == nil, err
}

// ParseText reads a pr-text Resolution: a one-line `Title:` and a non-empty
// `Body:` that runs to the end. The error is the reason it is not usable.
func ParseText(resolution string) (title, body string, err error) {
	lines := strings.Split(resolution, "\n")
	bodyAt := -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "Title:") && title == "":
			title = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
		case strings.HasPrefix(line, "Body:") && bodyAt < 0:
			bodyAt = i
		}
	}
	if title == "" {
		return "", "", errors.New("the Resolution has no Title: line")
	}
	if bodyAt < 0 {
		return "", "", errors.New("the Resolution has no Body: section")
	}
	lines[bodyAt] = strings.TrimPrefix(lines[bodyAt], "Body:")
	body = strings.TrimSpace(strings.Join(lines[bodyAt:], "\n"))
	if body == "" {
		return "", "", errors.New("the Body: section is empty")
	}
	return title, pr.TrimBody(body), nil
}

func resolutionOf(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return schema.Section(schema.ParseBody(string(raw)), "Resolution"), nil
}

// CheckText is the reason the finished pr-text ticket's Resolution is unusable,
// "" when it is fine.
func CheckText(ticketPath string) (string, error) {
	res, err := resolutionOf(ticketPath)
	if err != nil {
		return "", err
	}
	if _, _, err := ParseText(res); err != nil {
		return err.Error(), nil
	}
	return "", nil
}

// AfterInvalidText runs once a pr-text attempt is cancelled: it forks the next
// ticket, or after the last try stores pr-text-failed with reason on the epic.
// forked is the new ticket's id, "" when none was forked.
func AfterInvalidText(epicPath, ticketPath, reason string) (forked string, err error) {
	since, err := ResetTime(epicPath)
	if err != nil {
		return "", err
	}
	tries := 0
	for _, t := range tickets.LoadEpicAt(epicPath).Tickets {
		if t.Type == string(schema.TypePRText) && !t.CreatedAt.Before(since) {
			tries++
		}
	}
	if tries >= MaxTextTries {
		return "", tickets.OverwriteEpicFields(epicPath, map[string]any{
			blockKindKey: "pr-text-failed", blockReasonKey: reason,
		})
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		return "", err
	}
	id, _, err := StartPRText(epicPath, jobFromBody(schema.ParseBody(string(raw))))
	return id, err
}

// LatestText is the newest done valid pr-text Resolution, unless a "PR closed
// without merging" entry is newer than it.
func LatestText(epicPath string) (title, body string, ok bool) {
	var best tickets.Ticket
	for _, t := range tickets.LoadEpicAt(epicPath).Tickets {
		if t.Type == string(schema.TypePRText) && t.Status == string(schema.StatusDone) && t.Number >= best.Number {
			if res, err := resolutionOf(t.Path); err == nil {
				if _, _, err := ParseText(res); err == nil {
					best = t
				}
			}
		}
	}
	if best.Path == "" {
		return "", "", false
	}
	res, _ := resolutionOf(best.Path)
	title, body, _ = ParseText(res)
	done := best.CreatedAt
	if st, err := schema.ParseTicket(best.Path); err == nil && !st.IterationFinishedAt.IsZero() {
		done = st.IterationFinishedAt
	}
	if closed, err := LastClosed(epicPath); err == nil && closed.After(done) {
		return "", "", false
	}
	return title, body, true
}
