package ticketsflow

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// reviewBundleMaxBytes caps each diff file so a reviewer reads it in one Read.
const reviewBundleMaxBytes = 100 * 1024

// Review areas, in the order their files are written.
const (
	areaGo    = "go"
	areaTests = "tests"
	areaWeb   = "web"
	areaDocs  = "docs"
	areaOther = "other"
)

var reviewAreas = []string{areaGo, areaTests, areaWeb, areaDocs, areaOther}

// reviewExcluded names why a changed file stays out of every diff file, or "".
func reviewExcluded(file string) string {
	base := path.Base(file)
	switch {
	case strings.HasSuffix(base, ".lock"), base == "pnpm-lock.yaml", base == "package-lock.json",
		base == "yarn.lock", base == "go.sum":
		return "lockfile"
	case strings.Contains("/"+file, "/testdata/"), strings.Contains("/"+file, "/fixtures/"):
		return "fixture"
	}
	return ""
}

// reviewArea picks the area whose reviewer owns file.
func reviewArea(file string) string {
	base := path.Base(file)
	switch {
	case strings.HasSuffix(base, ".md"), strings.HasPrefix(file, "docs/"), strings.HasPrefix(file, "skills/"):
		return areaDocs
	case strings.HasSuffix(base, "_test.go"), strings.Contains(base, ".test."), strings.HasSuffix(base, ".e2e.ts"):
		return areaTests
	case strings.HasPrefix(file, "webui/"):
		return areaWeb
	case strings.HasSuffix(base, ".go"):
		return areaGo
	}
	return areaOther
}

// reviewFile is one changed file's place in the bundle.
type reviewFile struct {
	Name   string
	Area   string
	Reason string // set when excluded
	Parts  []string
}

type diffChunk struct {
	area string
	body strings.Builder
	n    int
}

func runTicketsReviewBundle(d Deps, cwd, arg, outDir string, w io.Writer) error {
	projectDir, project, name, err := resolveEpicTarget(d.dirs(), cwd, arg)
	if err != nil {
		return err
	}
	root, err := git.Toplevel(cwd)
	if err != nil {
		return err
	}
	env := d.Env.Env
	base, err := git.MergeBase(root, "main", "HEAD")
	if err != nil || base == "" {
		return fmt.Errorf("cannot resolve the merge-base of main and HEAD: %v", err)
	}
	names, _, err := git.Run(root, env, "diff", "--name-only", base+"...HEAD")
	if err != nil {
		return err
	}
	changed := strings.Fields(names)
	if len(changed) == 0 {
		return fmt.Errorf("empty diff: nothing changed since %s", base)
	}
	commits, _, err := git.Run(root, env, "log", base+"..HEAD", "--oneline")
	if err != nil {
		return err
	}

	epics, err := tickets.Load(projectDir)
	if err != nil {
		return err
	}
	var epic *tickets.Epic
	for i := range epics {
		if epics[i].Name == name {
			epic = &epics[i]
		}
	}
	if epic == nil {
		return &tickets.AddressError{Code: tickets.CodeUnknownEpic, Msg: fmt.Sprintf("no epic %q in project %q", name, project)}
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	files, chunks, err := reviewDiffs(root, env, base, changed)
	if err != nil {
		return err
	}
	var written []string
	for _, c := range chunks {
		file := fmt.Sprintf("diff-%s-%d.diff", c.area, c.n)
		if err := os.WriteFile(filepath.Join(outDir, file), []byte(c.body.String()), 0o644); err != nil {
			return err
		}
		written = append(written, fmt.Sprintf("%s (%d bytes)", file, c.body.Len()))
	}

	spec := reviewSpec(root, name, filepath.Join(projectDir, name))
	skills := reviewSkillPaths(d, root)
	address := project + ":" + name
	if err := os.WriteFile(filepath.Join(outDir, "deviations.md"), []byte(reviewDeviations(address, *epic)), 0o644); err != nil {
		return err
	}

	var idx strings.Builder
	fmt.Fprintf(&idx, "# Review bundle for %s\n\nmerge-base: %s\nspec: %s\n\n## Commits\n\n%s\n\n## Review skills\n\n%s\n\n## Files\n\n",
		address, base, spec, strings.TrimSpace(commits), strings.Join(skills, "\n"))
	for _, f := range files {
		switch {
		case f.Reason != "":
			fmt.Fprintf(&idx, "- %s: excluded (%s)\n", f.Name, f.Reason)
		default:
			fmt.Fprintf(&idx, "- %s: %s, %s\n", f.Name, f.Area, strings.Join(f.Parts, ", "))
		}
	}
	fmt.Fprintf(&idx, "\n## Diff files\n\n%s\n\nSpec deviations and tickets filed after the code-review ticket: deviations.md\n", strings.Join(written, "\n"))
	if err := os.WriteFile(filepath.Join(outDir, "index.md"), []byte(idx.String()), 0o644); err != nil {
		return err
	}

	fmt.Fprintf(w, "merge-base: %s\nspec: %s\ncommits:\n%s\nreview skills:\n%s\nbundle: %s\nindex: %s\n",
		base, spec, strings.TrimSpace(commits), strings.Join(skills, "\n"), outDir, filepath.Join(outDir, "index.md"))
	return nil
}

// reviewDiffs diffs each reviewed file once and packs the diffs into chunks of
// at most reviewBundleMaxBytes per area. A single oversized file is cut into
// several chunks.
func reviewDiffs(root string, env []string, base string, changed []string) ([]reviewFile, []*diffChunk, error) {
	sort.Strings(changed)
	var files []reviewFile
	byArea := map[string][]*diffChunk{}
	add := func(f *reviewFile, text string) {
		list := byArea[f.Area]
		for len(text) > 0 {
			if len(list) == 0 || list[len(list)-1].body.Len() >= reviewBundleMaxBytes {
				list = append(list, &diffChunk{area: f.Area, n: len(list) + 1})
			}
			c := list[len(list)-1]
			room := reviewBundleMaxBytes - c.body.Len()
			if len(text) > room && c.body.Len() > 0 && len(text) <= reviewBundleMaxBytes {
				// Keep a file whole when it fits in a fresh chunk.
				list = append(list, &diffChunk{area: f.Area, n: len(list) + 1})
				c = list[len(list)-1]
				room = reviewBundleMaxBytes
			}
			cut := min(room, len(text))
			c.body.WriteString(text[:cut])
			text = text[cut:]
			if !slices.Contains(f.Parts, fmt.Sprintf("diff-%s-%d.diff", f.Area, c.n)) {
				f.Parts = append(f.Parts, fmt.Sprintf("diff-%s-%d.diff", f.Area, c.n))
			}
		}
		byArea[f.Area] = list
	}
	for _, name := range changed {
		f := reviewFile{Name: name, Area: reviewArea(name), Reason: reviewExcluded(name)}
		if f.Reason == "" {
			text, _, err := git.Run(root, env, "diff", base+"...HEAD", "--", name)
			if err != nil {
				return nil, nil, err
			}
			add(&f, text+"\n")
		}
		files = append(files, f)
	}
	var chunks []*diffChunk
	for _, area := range reviewAreas {
		chunks = append(chunks, byArea[area]...)
	}
	return files, chunks, nil
}

// reviewSpec names the first spec that exists, or says the ticket bodies stand
// in for it.
func reviewSpec(root, epic, epicDir string) string {
	candidates := []string{filepath.Join(root, "docs", "specs", epic+".md")}
	if trimmed, ok := strings.CutSuffix(epic, tickets.ImplEpicSuffix); ok {
		candidates = append(candidates, filepath.Join(root, "docs", "specs", trimmed+".md"))
	}
	candidates = append(candidates, filepath.Join(epicDir, "spec.md"))
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			if rel, err := filepath.Rel(root, c); err == nil && !strings.HasPrefix(rel, "..") {
				return rel
			}
			return c
		}
	}
	return "none: the epic's ticket bodies are the spec"
}

// reviewSkillPaths lists each configured review skill with its SKILL.md path
// from the repo, or "unresolved".
func reviewSkillPaths(d Deps, root string) []string {
	names := config.DefaultSkillsConfig().CodeReview
	if d.LoadConfig != nil {
		if cfg, err := d.LoadConfig(); err == nil && len(cfg.Skills.CodeReview) > 0 {
			names = cfg.Skills.CodeReview
		}
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		p := filepath.Join(root, "skills", n, "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			p = "unresolved"
		}
		out = append(out, fmt.Sprintf("- %s: %s", n, p))
	}
	return out
}

// reviewDeviations writes the epic's Spec deviations once, then the tickets
// filed after the code-review ticket (higher number), so the spec axis does
// not flag them as scope creep.
func reviewDeviations(address string, e tickets.Epic) string {
	reviewNum := -1
	for _, tk := range e.Tickets {
		if tk.Type == string(schema.TypeCodeReview) && (reviewNum < 0 || tk.Number < reviewNum) {
			reviewNum = tk.Number
		}
	}
	var dev, late strings.Builder
	for _, tk := range e.Tickets {
		if tk.Type == string(schema.TypeCodeReview) {
			continue
		}
		addr := fmt.Sprintf("%s/%s", address, tk.Identifier)
		if sec := schema.Section(tk.Body, "Spec deviations"); sec != "" {
			for line := range strings.SplitSeq(sec, "\n") {
				if strings.TrimSpace(line) != "" {
					fmt.Fprintf(&dev, "%s: %s\n", addr, strings.TrimSpace(line))
				}
			}
		}
		if reviewNum >= 0 && tk.Number > reviewNum {
			fmt.Fprintf(&late, "- %s %s\n", addr, tk.Title)
		}
	}
	none := func(s string) string {
		if s == "" {
			return "none\n"
		}
		return s
	}
	return "# Known spec deviations\n\n" + none(dev.String()) +
		"\n# Tickets filed after the code-review ticket\n\nThese are follow-ups of the review, not scope creep.\n\n" + none(late.String())
}

func newTicketsReviewBundleCmd(d Deps) *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:   "review-bundle <project:epic>",
		Short: "write the code-review inputs (per-area diffs, index, spec deviations) into a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			if outDir == "" {
				if outDir, err = os.MkdirTemp("", "review-bundle-"); err != nil {
					return err
				}
			}
			return runTicketsReviewBundle(d, cwd, args[0], outDir, c.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "directory to write the bundle into (default: a new temp directory)")
	return cmd
}
