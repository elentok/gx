package tickets

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// LintEntry is one ticket the claim lint reads: a manifest entry (Name is its
// slug) or an epic ticket (Name is its ID).
type LintEntry struct {
	Name      string
	Type      string
	Body      string
	BlockedBy []string
}

// CommandProblem is one thing wrong with a quoted gx command. Words and Flag
// say what to look for when deciding whether the name is planned.
type CommandProblem struct {
	Words []string // subcommand words up to and including the unknown one
	Flag  string   // the unknown flag, with Words naming its command
	Text  string
}

// LintEnv holds everything the checks look names up in. Command is gx's own
// command tree. Root is the repo root; a nil GoTests or Flows skips that check.
type LintEnv struct {
	Root    string
	Command func(words, flags []string) []CommandProblem
	GoTests []string
	Flows   map[int]bool
	TopDirs []string
}

// LintFinding is one line of the lint's output.
type LintFinding struct {
	Entry string
	Text  string
	// Refuses is true for a miss that stops the run: one in an implement or
	// research ticket.
	Refuses bool
}

// LintReport is the lint's result, in print order: Misses, Warnings, Planned,
// Skipped.
type LintReport struct {
	Misses   []LintFinding
	Warnings []LintFinding
	Planned  []LintFinding
	Skipped  []string
}

// Refused reports whether any miss stops the run.
func (r LintReport) Refused() bool {
	for _, m := range r.Misses {
		if m.Refuses {
			return true
		}
	}
	return false
}

// LintClaims checks the names in the bodies of entries. existing are the epic's
// tickets that a blocked_by ID can name; entries may repeat some of them.
func LintClaims(entries, existing []LintEntry, env LintEnv) LintReport {
	var r LintReport
	if env.GoTests == nil {
		r.Skipped = append(r.Skipped, "-run patterns: no Go test files found")
	}
	if env.Flows == nil {
		r.Skipped = append(r.Skipped, "flow ids: no flow catalog (webui/e2e/CATALOG.md) found")
	}
	byName := map[string]LintEntry{}
	for _, e := range existing {
		byName[e.Name] = e
	}
	for _, e := range entries {
		byName[e.Name] = e
	}
	all := append(append([]LintEntry{}, existing...), entries...)
	for _, e := range entries {
		var pb strings.Builder
		pb.WriteString(plannedText(e, byName))
		if e.Type == string(schema.TypeVerify) {
			for _, o := range all {
				if o.Type == string(schema.TypeImplement) {
					fmt.Fprintln(&pb, buildText(o))
				}
			}
		}
		planned := pb.String()
		for _, c := range ExtractClaims(e.Body, env.TopDirs) {
			if c.Kind == ClaimLineRef {
				r.Warnings = append(r.Warnings, LintFinding{e.Name, fmt.Sprintf("%s: name a symbol, not a line", c.Text), false})
				continue
			}
			for _, p := range claimProblems(c, env) {
				f := LintFinding{Entry: e.Name, Text: p.text}
				switch {
				case p.planned != nil && p.planned(planned):
					r.Planned = append(r.Planned, f)
				case e.Type == string(schema.TypeVerify):
					r.Warnings = append(r.Warnings, f)
				default:
					f.Refuses = e.Type == string(schema.TypeImplement) || e.Type == string(schema.TypeResearch)
					r.Misses = append(r.Misses, f)
				}
			}
		}
	}
	return r
}

func buildText(e LintEntry) string {
	return schema.Section(e.Body, "What to build") + "\n" + schema.Section(e.Body, "Acceptance criteria")
}

// plannedText is What to build and Acceptance criteria of e and of every
// transitive blocker that byName can resolve.
func plannedText(e LintEntry, byName map[string]LintEntry) string {
	var sb strings.Builder
	seen := map[string]bool{}
	var visit func(LintEntry)
	visit = func(x LintEntry) {
		fmt.Fprintln(&sb, buildText(x))
		for _, ref := range x.BlockedBy {
			if b, ok := byName[ref]; ok && !seen[ref] {
				seen[ref] = true
				visit(b)
			}
		}
	}
	seen[e.Name] = true
	visit(e)
	return sb.String()
}

type claimProblem struct {
	text    string
	planned func(sections string) bool // nil: a name that can never be planned
}

func contains(s string) func(string) bool {
	return func(sections string) bool { return strings.Contains(sections, s) }
}

func claimProblems(c Claim, env LintEnv) []claimProblem {
	switch c.Kind {
	case ClaimCommand:
		if env.Command == nil {
			return nil
		}
		var out []claimProblem
		for _, p := range env.Command(c.Words, c.Flags) {
			needles := append([]string{"gx " + strings.Join(p.Words, " ")}, p.Flag)
			out = append(out, claimProblem{c.Text + ": " + p.Text, func(s string) bool {
				for _, n := range needles {
					if n != "" && !strings.Contains(s, n) {
						return false
					}
				}
				return true
			}})
		}
		return out
	case ClaimPath, ClaimFileFunc:
		return fileProblems(c, env)
	case ClaimTestPattern:
		if env.GoTests == nil {
			return nil
		}
		if msg := testPatternProblem(c.Pattern, env.GoTests); msg != "" {
			return []claimProblem{{c.Text + ": " + msg, contains(c.Pattern)}}
		}
	case ClaimFlow:
		return flowProblems(c, env)
	}
	return nil
}

func fileProblems(c Claim, env LintEnv) []claimProblem {
	path := filepath.Join(env.Root, filepath.FromSlash(c.File))
	data, err := os.ReadFile(path)
	if err != nil {
		if info, serr := os.Stat(path); serr == nil && info.IsDir() && c.Kind == ClaimPath {
			return nil
		}
		return []claimProblem{{c.Text + ": no such file", contains(c.File)}}
	}
	if c.Kind == ClaimFileFunc && strings.HasSuffix(c.File, ".go") {
		re := regexp.MustCompile(`(?m)^(func\s+(\([^)]*\)\s*)?|type\s+|var\s+|const\s+)` + regexp.QuoteMeta(c.Func) + `\b`)
		if !re.Match(data) {
			return []claimProblem{{c.Text + ": no such function in " + c.File, contains(c.Func)}}
		}
	}
	return nil
}

func testPatternProblem(pattern string, tests []string) string {
	top, _, _ := strings.Cut(pattern, "/")
	re, err := regexp.Compile(top)
	if err != nil {
		return "invalid pattern: " + err.Error()
	}
	if slices.ContainsFunc(tests, re.MatchString) {
		return ""
	}
	return "matches no Test function"
}

func flowProblems(c Claim, env LintEnv) []claimProblem {
	if env.Flows == nil {
		return nil
	}
	taken := env.Flows[c.Flow]
	switch {
	case c.New && taken:
		return []claimProblem{{fmt.Sprintf("%s: flow %d is already in the catalog", c.Text, c.Flow), nil}}
	case !c.New && !taken:
		re := regexp.MustCompile(fmt.Sprintf(`(?i)new flow %d\b`, c.Flow))
		return []claimProblem{{fmt.Sprintf("%s: flow %d is not in the catalog", c.Text, c.Flow), re.MatchString}}
	}
	return nil
}

var (
	goTestFuncRe  = regexp.MustCompile(`(?m)^func (Test\w*)\(`)
	flowHeadingRe = regexp.MustCompile(`(?m)^## (\d+)\. `)
	skipWalkDirs  = map[string]bool{".git": true, "node_modules": true, ".worktrees": true, "vendor": true}
)

// LoadGoTestNames returns the Test function names in the repo's Go test files,
// or nil when there are none.
func LoadGoTestNames(root string) []string {
	var names []string
	found := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipWalkDirs[d.Name()] && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		found = true
		if data, err := os.ReadFile(p); err == nil {
			for _, m := range goTestFuncRe.FindAllSubmatch(data, -1) {
				names = append(names, string(m[1]))
			}
		}
		return nil
	})
	if found && names == nil {
		names = []string{}
	}
	return names
}

// LoadFlowCatalog returns the flow numbers in webui/e2e/CATALOG.md, or nil when
// the catalog does not exist.
func LoadFlowCatalog(root string) map[int]bool {
	data, err := os.ReadFile(filepath.Join(root, "webui", "e2e", "CATALOG.md"))
	if err != nil {
		return nil
	}
	flows := map[int]bool{}
	for _, m := range flowHeadingRe.FindAllSubmatch(data, -1) {
		n, _ := strconv.Atoi(string(m[1]))
		flows[n] = true
	}
	return flows
}
