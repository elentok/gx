package ghfake

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Run is the fake gh: it logs the call, fires a scripted failure if one
// matches, and otherwise answers from the state file. It returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	dir := os.Getenv(DirEnv)
	if dir == "" {
		fmt.Fprintf(stderr, "ghfake: %s is not set\n", DirEnv)
		return 2
	}
	line, _ := json.Marshal(args)
	log, err := os.OpenFile(filepath.Join(dir, "calls.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintf(log, "%s\n", line)
		log.Close()
	}
	code := 0
	err = Locked(dir, func(s *State) error {
		if f := s.takeFailure(args); f != nil {
			fmt.Fprintln(stderr, f.Stderr)
			code = f.Code
			return nil
		}
		var out string
		out, code = s.answer(args, stderr)
		fmt.Fprintln(stdout, out)
		return nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "ghfake: %v\n", err)
		return 2
	}
	return code
}

func (s *State) takeFailure(args []string) *Failure {
	joined := strings.Join(args, " ")
	for i := range s.Failures {
		f := s.Failures[i]
		if !strings.HasPrefix(joined, f.Match) {
			continue
		}
		if f.Times == 1 {
			s.Failures = append(s.Failures[:i], s.Failures[i+1:]...)
		} else if f.Times > 1 {
			s.Failures[i].Times--
		}
		return &f
	}
	return nil
}

func (s *State) answer(args []string, stderr io.Writer) (string, int) {
	fail := func(code int, msg string) (string, int) {
		fmt.Fprintln(stderr, msg)
		return "", code
	}
	if len(args) < 2 {
		return fail(2, "ghfake: unhandled call "+strings.Join(args, " "))
	}
	switch args[0] + " " + args[1] {
	case "auth status":
		if s.LoggedOut {
			return fail(1, "You are not logged into any GitHub hosts. To log in, run: gh auth login")
		}
		return "Logged in", 0
	case "repo view":
		if s.LoggedOut {
			return fail(4, "To get started with GitHub CLI, please run: gh auth login")
		}
		if s.NoRepoAccess {
			return fail(1, "GraphQL: Could not resolve to a Repository (HTTP 404)")
		}
		return args[2], 0
	case "pr list":
		return s.prList(flagValue(args, "--head")), 0
	case "pr view":
		return s.prView(args[2:]), 0
	case "pr create":
		return s.prCreate(args), 0
	case "pr ready":
		if p := s.byNumber(args[2]); p != nil {
			p.Draft = false
		}
		return "", 0
	case "run rerun":
		s.rerun(flagValue(args, "--job"))
		return "", 0
	case "api graphql":
		return s.graphQL(args), 0
	}
	return fail(2, "ghfake: unhandled call "+strings.Join(args, " "))
}

func (s *State) byNumber(n string) *PR {
	for i := range s.PRs {
		if strconv.Itoa(s.PRs[i].Number) == n {
			return &s.PRs[i]
		}
	}
	return nil
}

func nullable(sha string) any {
	if sha == "" {
		return nil
	}
	return map[string]string{"oid": sha}
}

func (s *State) prList(head string) string {
	list := []map[string]any{}
	for _, p := range s.PRs {
		if p.Head == head {
			list = append(list, map[string]any{"number": p.Number, "url": p.URL, "state": p.State,
				"isDraft": p.Draft, "headRefOid": p.HeadSHA, "mergeCommit": nullable(p.MergeCommit)})
		}
	}
	return mustJSON(list)
}

// prView answers `pr view [number]` with the PR's URL (the callers ask for
// `--json url -q .url`); with no number it is the newest PR.
func (s *State) prView(rest []string) string {
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		if p := s.byNumber(rest[0]); p != nil {
			return p.URL
		}
		return ""
	}
	if n := len(s.PRs); n > 0 {
		return s.PRs[n-1].URL
	}
	return ""
}

func (s *State) prCreate(args []string) string {
	c := Created{Repo: flagValue(args, "-R"), Head: flagValue(args, "--head"), Base: flagValue(args, "--base"), Title: flagValue(args, "--title")}
	if body, err := os.ReadFile(flagValue(args, "--body-file")); err == nil {
		c.Body = string(body)
	}
	s.Created = append(s.Created, c)
	n := 1
	for _, p := range s.PRs {
		n = max(n, p.Number+1)
	}
	revParse := exec.Command("git", "rev-parse", "--verify", c.Head)
	revParse.Env = os.Environ()
	sha, _ := revParse.Output()
	p := PR{Number: n, URL: fmt.Sprintf("https://github.com/%s/pull/%d", c.Repo, n), Head: c.Head, Base: c.Base,
		State: "OPEN", Draft: true, HeadSHA: strings.TrimSpace(string(sha)), Mergeable: "MERGEABLE"}
	s.PRs = append(s.PRs, p)
	return p.URL
}

func (s *State) rerun(job string) {
	res, ok := s.Reruns[job]
	if !ok {
		res = Result{Status: "COMPLETED", Conclusion: "SUCCESS"}
	}
	for sha, checks := range s.Checks {
		for i := range checks {
			if strconv.FormatInt(checks[i].JobID, 10) == job {
				s.Checks[sha][i].Status, s.Checks[sha][i].Conclusion = res.Status, res.Conclusion
			}
		}
	}
}

var watchAlias = regexp.MustCompile(`p(\d+):pullRequest`)

// graphQL answers the two queries git/pr makes: the watch query (one aliased
// pullRequest per number) and the per-check detail query (`-F p=<number>`).
func (s *State) graphQL(args []string) string {
	query := ""
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "query="); ok && i > 0 && args[i-1] == "-f" {
			query = v
		}
	}
	if strings.Contains(query, "contexts(") {
		return s.checksResponse(flagValue(args, "-F")[len("p="):])
	}
	repo := map[string]any{}
	for _, m := range watchAlias.FindAllStringSubmatch(query, -1) {
		var node any
		if p := s.byNumber(m[1]); p != nil {
			node = map[string]any{"state": p.State, "isDraft": p.Draft, "headRefOid": p.HeadSHA,
				"mergeCommit": nullable(p.MergeCommit), "mergeable": p.Mergeable,
				"commits": map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{
					"statusCheckRollup": s.rollup(p.HeadSHA)}}}}}
		}
		repo["p"+m[1]] = node
	}
	return mustJSON(map[string]any{"data": map[string]any{"repository": repo}})
}

// rollup is GitHub's rollup state for a head: null without checks, FAILURE if
// any failed, PENDING if any is not done, else SUCCESS.
func (s *State) rollup(headSHA string) any {
	checks := s.Checks[headSHA]
	if len(checks) == 0 {
		return nil
	}
	state := "SUCCESS"
	for _, c := range checks {
		failed, pending := c.Conclusion == "FAILURE" || c.Conclusion == "TIMED_OUT" || c.Conclusion == "STARTUP_FAILURE", c.Status != "COMPLETED"
		if c.StatusState != "" {
			failed = c.StatusState == "FAILURE" || c.StatusState == "ERROR"
			pending = c.StatusState == "PENDING" || c.StatusState == "EXPECTED"
		}
		if failed {
			return map[string]any{"state": "FAILURE"}
		}
		if pending {
			state = "PENDING"
		}
	}
	return map[string]any{"state": state}
}

func (s *State) checksResponse(number string) string {
	var commits []any
	if p := s.byNumber(number); p != nil {
		var rollup any
		if checks := s.Checks[p.HeadSHA]; len(checks) > 0 {
			nodes := []any{}
			for _, c := range checks {
				nodes = append(nodes, c.node())
			}
			rollup = map[string]any{"contexts": map[string]any{"nodes": nodes}}
		}
		commits = []any{map[string]any{"commit": map[string]any{"oid": p.HeadSHA, "statusCheckRollup": rollup}}}
	}
	pr := map[string]any{"commits": map[string]any{"nodes": commits}}
	return mustJSON(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": pr}}})
}

func (c Check) node() map[string]any {
	if c.StatusState != "" {
		return map[string]any{"__typename": "StatusContext", "context": c.Name, "state": c.StatusState, "targetUrl": c.DetailsURL}
	}
	var run any
	if c.RunID != 0 {
		run = map[string]any{"databaseId": c.RunID, "workflow": map[string]any{"databaseId": c.WorkflowID}}
	}
	return map[string]any{"__typename": "CheckRun", "databaseId": c.JobID, "name": c.Name, "status": c.Status,
		"conclusion": c.Conclusion, "detailsUrl": c.DetailsURL, "checkSuite": map[string]any{"workflowRun": run}}
}

// flagValue is the argument after flag, or "" when flag is absent. The `-F p=5`
// form is why the caller may need to cut a key= prefix.
func flagValue(args []string, flag string) string {
	for i, a := range args[:len(args)-1] {
		if a == flag {
			return args[i+1]
		}
	}
	return ""
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
