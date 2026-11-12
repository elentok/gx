package ticketsflow

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/tickets/ticketspec"
)

const (
	maxCallerLines = 12
	// A slow gopls falls back to rg rather than stalling the draft.
	goplsTimeout = 20 * time.Second
)

// draftMisses is one line per path, test pattern, flow id or symbol the plan
// names that the checkout at root lacks. It also fills each ticket's Callers.
// The judgment checks (what a change hides, which side wins, signature fallout)
// are the agent's: nothing here claims one.
func draftMisses(plan *ticketspec.DraftPlan, root, commit string, procEnv []string) []string {
	env := ticketspec.LintEnv{
		Root:    root,
		GoTests: ticketspec.LoadGoTestNames(root),
		Flows:   ticketspec.LoadFlowCatalog(root),
		TopDirs: topLevelDirs(root),
	}
	var misses []string
	for _, t := range plan.Tickets {
		body := ticketspec.RenderDraftBody(t, plan.Scenario, "x", "x")
		for _, m := range ticketspec.ClaimMisses(body, env) {
			misses = append(misses, t.Slug+": "+m)
		}
	}

	symbols := planSymbols(plan)
	callers := map[string]string{}
	rgOK := true
	if _, err := exec.LookPath("rg"); err != nil {
		rgOK = false
		misses = append(misses, "symbols: rg not found on PATH, so symbol existence and callers are unchecked")
	}
	if rgOK {
		finder := newCallerFinder(root, commit, procEnv)
		for _, s := range symbols {
			text, found := finder.callers(s)
			if !found {
				for _, t := range plan.Tickets {
					if slices.Contains(t.EntrySymbols, s) || slices.Contains(t.ChangedSymbols, s) {
						misses = append(misses, t.Slug+": `"+s+"`: no such symbol")
					}
				}
				continue
			}
			callers[s] = text
		}
	}
	for i := range plan.Tickets {
		plan.Tickets[i].Callers = callers
	}
	return misses
}

// planSymbols is every entry and changed symbol of the plan, once.
func planSymbols(plan *ticketspec.DraftPlan) []string {
	var out []string
	for _, t := range plan.Tickets {
		for _, s := range append(slices.Clone(t.EntrySymbols), t.ChangedSymbols...) {
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

type callerFinder struct {
	root, commit string
	env          []string
	gopls        bool
}

func newCallerFinder(root, commit string, env []string) *callerFinder {
	_, err := exec.LookPath("gopls")
	return &callerFinder{root: root, commit: commit, env: env, gopls: err == nil}
}

func (f *callerFinder) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = f.root
	cmd.Env = append([]string{}, f.env...)
	return cmd
}

// callers returns the labelled caller list of symbol and whether the symbol
// exists at all. Go symbols go through gopls when it is on PATH; every `gopls
// -remote=auto` call of a run shares one daemon, so that is one session.
func (f *callerFinder) callers(symbol string) (string, bool) {
	name := symbol[strings.LastIndex(symbol, ".")+1:]
	if f.gopls {
		if pos, ok := f.goDefinition(name); ok {
			ctx, cancel := context.WithTimeout(context.Background(), goplsTimeout)
			out, err := f.command(ctx, "gopls", "-remote=auto", "references", pos).CombinedOutput()
			cancel()
			if err == nil {
				return fmt.Sprintf("verified at %s: %s", f.commit, f.summarize(refLines(string(out)))), true
			}
		}
	}
	lines := f.rgLines(name)
	if len(lines) == 0 {
		return "", false
	}
	return "rg result, not verified: " + f.summarize(lines), true
}

func defRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`^(?:func\s+(?:\([^)]*\)\s*)?|type\s+|var\s+|const\s+)(` + regexp.QuoteMeta(name) + `)\b`)
}

// goDefinition returns the file:line:col of the Go declaration of name.
func (f *callerFinder) goDefinition(name string) (string, bool) {
	re := defRe(name)
	cmd := f.command(context.Background(), "rg", "-n", "--no-heading", "-g", "*.go", "-e", strings.TrimPrefix(re.String(), "^"), ".")
	out, _ := cmd.Output()
	for l := range strings.SplitSeq(string(out), "\n") {
		parts := strings.SplitN(l, ":", 3)
		if len(parts) < 3 {
			continue
		}
		if m := re.FindStringSubmatchIndex(parts[2]); m != nil {
			return fmt.Sprintf("%s:%s:%d", strings.TrimPrefix(parts[0], "./"), parts[1], m[2]+1), true
		}
	}
	return "", false
}

// rgLines is the file:line of every word match of name.
func (f *callerFinder) rgLines(name string) []string {
	cmd := f.command(context.Background(), "rg", "-n", "--no-heading", "-w", "-F", "-e", name, ".")
	out, _ := cmd.Output()
	var lines []string
	for l := range strings.SplitSeq(string(out), "\n") {
		if parts := strings.SplitN(l, ":", 3); len(parts) == 3 {
			lines = append(lines, strings.TrimPrefix(parts[0], "./")+":"+parts[1])
		}
	}
	return lines
}

// refLines turns gopls `file:line:col-col` lines into file:line.
func refLines(out string) []string {
	var lines []string
	for l := range strings.SplitSeq(out, "\n") {
		parts := strings.Split(strings.TrimSpace(l), ":")
		if len(parts) < 3 {
			continue
		}
		if _, err := strconv.Atoi(parts[len(parts)-2]); err != nil {
			continue
		}
		lines = append(lines, strings.Join(parts[:len(parts)-1], ":"))
	}
	return lines
}

func (f *callerFinder) summarize(lines []string) string {
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, f.root+"/")
	}
	if len(lines) == 0 {
		return "none found"
	}
	if len(lines) > maxCallerLines {
		return fmt.Sprintf("%s, and %d more", strings.Join(lines[:maxCallerLines], ", "), len(lines)-maxCallerLines)
	}
	return strings.Join(lines, ", ")
}
