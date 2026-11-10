package pr

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MaxBodyChars is GitHub's limit on a PR body.
const MaxBodyChars = 65536

const trimmedLine = "\n\n_(description trimmed: too long for GitHub)_"

var prURLRe = regexp.MustCompile(`https?://\S+/pull/(\d+)`)

// CreateDraft opens a draft PR for head against base and returns its number and
// URL. --head is always passed: without it gh prompts when the branch is
// unpushed. Failures are classified like the read calls.
func (r Runner) CreateDraft(repo, head, base, title, bodyFile string) (int, string, error) {
	out, err := r.Run("pr", "create", "--draft", "--head", head, "--base", base,
		"--title", title, "--body-file", bodyFile, "-R", repo)
	if err != nil {
		return 0, "", classifyPRListError(err)
	}
	m := prURLRe.FindStringSubmatch(out)
	if m == nil {
		return 0, "", fmt.Errorf("gh pr create printed no PR URL: %q", out)
	}
	n, _ := strconv.Atoi(m[1])
	return n, m[0], nil
}

// MarkReady takes a draft PR out of draft. The caller checks isDraft first.
func (r Runner) MarkReady(repo string, number int) error {
	_, err := r.Run("pr", "ready", strconv.Itoa(number), "-R", repo)
	return classifyPRListError(err)
}

// RerunJob reruns one failed job. The caller decides when a rerun is allowed.
func (r Runner) RerunJob(repo string, jobID int64) error {
	_, err := r.Run("run", "rerun", "--job", strconv.FormatInt(jobID, 10), "-R", repo)
	return classifyPRListError(err)
}

// TrimBody returns body unchanged when it fits GitHub's limit; otherwise it
// cuts at a line boundary and ends with a short "trimmed" line, staying under
// MaxBodyChars.
func TrimBody(body string) string {
	if len(body) < MaxBodyChars {
		return body
	}
	budget := MaxBodyChars - len(trimmedLine) - 1
	cut := body[:budget]
	if i := strings.LastIndexByte(cut, '\n'); i >= 0 {
		cut = cut[:i]
	}
	return cut + trimmedLine
}
