package ticketsflow

import (
	"context"
	"errors"
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
	goplsTimeout = 8 * time.Second
)

// draftMisses is one line per path, test pattern, flow id or symbol the plan
// names that the checkout at root lacks. It also fills each ticket's Callers.
// The judgment checks (what a change hides, which side wins, signature fallout)
// are the agent's: nothing here claims one.
func draftMisses(plan *ticketspec.DraftPlan, root string, finder *callerFinder) []string {
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
	misses = append(misses, draftFacts(plan, root, finder, rgOK)...)
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
	noRemote     bool
	// goplsFailure is the first gopls failure of the run; rgFallbacks counts
	// the symbols answered by rg after it.
	goplsFailure string
	rgFallbacks  int
}

func newCallerFinder(root, commit string, env []string, noGopls bool) *callerFinder {
	_, err := exec.LookPath("gopls")
	return &callerFinder{root: root, commit: commit, env: env, gopls: err == nil && !noGopls}
}

// goplsNote is the one-line notice that gopls failed and rg stood in, or "".
func (f *callerFinder) goplsNote() string {
	if f.goplsFailure == "" {
		return ""
	}
	return fmt.Sprintf("gopls failed (%s); used rg for %d symbol(s)", f.goplsFailure, f.rgFallbacks)
}

// failGopls turns gopls off for the rest of the run and remembers why.
func (f *callerFinder) failGopls(out string, err error) {
	f.gopls = false
	if f.goplsFailure != "" {
		return
	}
	reason, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	if errors.Is(err, context.DeadlineExceeded) {
		reason = "timed out"
	}
	if reason == "" {
		reason = err.Error()
	}
	f.goplsFailure = reason
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
			out, err := f.goplsReferences(pos)
			if err == nil {
				return fmt.Sprintf("verified at %s: %s", f.commit, f.summarize(refLines(out))), true
			}
			f.failGopls(out, err)
		}
	}
	lines := f.rgLines(name)
	if len(lines) == 0 {
		return "", false
	}
	if f.goplsFailure != "" {
		f.rgFallbacks++
	}
	return "rg result, not verified: " + f.summarize(lines), true
}

// goplsReferences prefers the shared daemon. When the daemon cannot be dialed
// (seen even unsandboxed) it runs in-process and stays there for the rest of
// the run, so the dial failure is paid once.
func (f *callerFinder) goplsReferences(pos string) (string, error) {
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), goplsTimeout)
		defer cancel()
		out, err := f.command(ctx, "gopls", args...).CombinedOutput()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return string(out), err
	}
	if !f.noRemote {
		out, err := run("-remote=auto", "references", pos)
		if err == nil {
			return out, nil
		}
		f.noRemote = true
	}
	return run("references", pos)
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
