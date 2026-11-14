// Package rangediff answers the Diff page's git questions for a three-dot
// range between two typed refs: which refs resolve, what the merge base is,
// which files changed and the patch of one file.
package rangediff

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/elentok/gx/git/internal/run"
)

const (
	// MaxFiles caps the file list; past it FileList.Truncated is set.
	MaxFiles = 5000
	// LargeLines and LargeBytes are the patch size above which a file is large.
	LargeLines = 2000
	LargeBytes = 60 * 1024
)

// Side says which of the two typed refs a ref came from.
type Side int

const (
	First Side = iota + 1
	Second
)

func (s Side) String() string {
	if s == First {
		return "first"
	}
	return "second"
}

// UnknownRefError is returned when a typed ref does not resolve to a commit.
type UnknownRefError struct {
	Ref  string
	Side Side
}

func (e *UnknownRefError) Error() string {
	return fmt.Sprintf("unknown %s ref: %q", e.Side, e.Ref)
}

// Range is a resolved three-dot range. Only SHAs are kept: later git calls
// never see the typed refs.
type Range struct {
	FirstSHA  string
	SecondSHA string
	// MergeBase is the diff base. Empty when NoCommonHistory.
	MergeBase       string
	NoCommonHistory bool
}

// Resolve checks both refs and finds their merge base. Two commits with no
// common ancestor are a result (NoCommonHistory), not an error.
func Resolve(dir, first, second string) (Range, error) {
	a, err := resolveRef(dir, first, First)
	if err != nil {
		return Range{}, err
	}
	b, err := resolveRef(dir, second, Second)
	if err != nil {
		return Range{}, err
	}
	out, _, err := run.RunWith(dir, run.NonInteractiveEnv(),
		run.RunOptions{AllowExit: map[int]bool{1: true}}, "merge-base", a, b)
	if err != nil {
		return Range{}, err
	}
	return Range{FirstSHA: a, SecondSHA: b, MergeBase: out, NoCommonHistory: out == ""}, nil
}

func resolveRef(dir, ref string, side Side) (string, error) {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", &UnknownRefError{Ref: ref, Side: side}
	}
	sha, _, err := run.Exec(dir, []string{"rev-parse", "--verify", "--end-of-options", ref + "^{commit}"})
	if err != nil {
		return "", &UnknownRefError{Ref: ref, Side: side}
	}
	return sha, nil
}

// File is one changed file of the range. It is the single source for the
// file entry shared by the list, patch and blob calls.
type File struct {
	Path string
	// OldPath is the rename source; empty unless Status is R.
	OldPath   string
	Status    string
	Added     int
	Deleted   int
	Large     bool
	Binary    bool
	Generated bool
}

// FileList is the changed files of a range, at most MaxFiles of them.
type FileList struct {
	Files     []File
	Truncated bool
}

// ListFiles lists what the second ref changed since the merge base.
func ListFiles(dir string, r Range) (FileList, error) {
	if r.NoCommonHistory {
		return FileList{}, nil
	}
	files, err := diffFiles(dir, []string{"diff"}, []string{r.MergeBase, r.SecondSHA, "--"})
	if err != nil {
		return FileList{}, err
	}
	return capFiles(files), nil
}

func capFiles(files []File) FileList {
	list := FileList{Files: files}
	if len(files) > MaxFiles {
		list.Files = files[:MaxFiles]
		list.Truncated = true
	}
	return list
}

// diffFiles lists the files one git diff-like call changes. cmd is "diff" or
// "show --format=" and tail names what to compare.
func diffFiles(dir string, cmd, tail []string) ([]File, error) {
	base := append(append([]string{}, cmd...), "--no-color", "--find-renames", "--no-ext-diff")
	env := run.NonInteractiveEnv()
	call := func(extra ...string) ([]byte, error) {
		out, _, err := run.RunRaw(dir, env, append(append(append([]string{}, base...), extra...), tail...)...)
		return out, err
	}

	status, err := call("--name-status", "-z")
	if err != nil {
		return nil, err
	}
	numstat, err := call("--numstat", "-z")
	if err != nil {
		return nil, err
	}
	patch, err := call("--patch")
	if err != nil {
		return nil, err
	}

	files, err := parseNameStatus(string(status))
	if err != nil {
		return nil, err
	}
	if err := applyNumstat(files, string(numstat)); err != nil {
		return nil, err
	}
	applySizes(files, string(patch))
	for i := range files {
		files[i].Generated = isGenerated(files[i].Path)
	}
	return files, nil
}

func parseNameStatus(out string) ([]File, error) {
	toks := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	files := []File{}
	for i := 0; i < len(toks) && toks[i] != ""; {
		letter := toks[i][:1]
		f := File{Status: letter}
		if letter == "R" || letter == "C" {
			if i+2 >= len(toks) {
				return nil, fmt.Errorf("rangediff: truncated name-status output")
			}
			f.OldPath, f.Path = toks[i+1], toks[i+2]
			i += 3
		} else {
			if i+1 >= len(toks) {
				return nil, fmt.Errorf("rangediff: truncated name-status output")
			}
			f.Path = toks[i+1]
			i += 2
		}
		files = append(files, f)
	}
	return files, nil
}

// applyNumstat fills counts and Binary. Git lists numstat entries in the same
// order as name-status.
func applyNumstat(files []File, out string) error {
	toks := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	n := 0
	for i := 0; i < len(toks) && toks[i] != ""; n++ {
		if n >= len(files) {
			return fmt.Errorf("rangediff: numstat has more entries than name-status")
		}
		parts := strings.SplitN(toks[i], "\t", 3)
		if len(parts) != 3 {
			return fmt.Errorf("rangediff: bad numstat entry %q", toks[i])
		}
		if parts[0] == "-" || parts[1] == "-" {
			files[n].Binary = true
		} else {
			files[n].Added, _ = strconv.Atoi(parts[0])
			files[n].Deleted, _ = strconv.Atoi(parts[1])
		}
		if parts[2] == "" { // rename: the two paths follow as their own tokens
			i += 3
		} else {
			i++
		}
	}
	if n != len(files) {
		return fmt.Errorf("rangediff: numstat has fewer entries than name-status")
	}
	return nil
}

// applySizes sets Large from the full patch. Git emits one "diff --git"
// section per file in the same order as the lists above, and a content line
// never starts with "diff --git" (it begins with +, - or a space).
func applySizes(files []File, patch string) {
	const marker = "diff --git "
	i := -1
	for rest := patch; ; {
		if strings.HasPrefix(rest, marker) {
			i++
		}
		next := strings.Index(rest, "\n"+marker)
		section := rest
		if next >= 0 {
			section = rest[:next+1]
		}
		if i >= 0 && i < len(files) {
			files[i].Large = len(section) > LargeBytes || strings.Count(section, "\n") > LargeLines
		}
		if next < 0 {
			return
		}
		rest = rest[next+1:]
	}
}

var generatedNames = map[string]bool{
	"package-lock.json":   true,
	"npm-shrinkwrap.json": true,
	"yarn.lock":           true,
	"pnpm-lock.yaml":      true,
	"bun.lock":            true,
	"bun.lockb":           true,
	"go.sum":              true,
	"Cargo.lock":          true,
	"Gemfile.lock":        true,
	"poetry.lock":         true,
	"uv.lock":             true,
	"composer.lock":       true,
	"Pipfile.lock":        true,
	"flake.lock":          true,
	"mix.lock":            true,
	"pubspec.lock":        true,
	"Podfile.lock":        true,
}

func isGenerated(p string) bool { return generatedNames[path.Base(p)] }

// Patch returns the unified patch of one file of the range, with no size cap.
// A binary file has no patch. A rename shows as one patch.
func Patch(dir string, r Range, f File) (string, error) {
	if f.Binary || r.NoCommonHistory {
		return "", nil
	}
	args := []string{"--literal-pathspecs", "diff", "--no-color", "--find-renames", "--no-ext-diff",
		r.MergeBase, r.SecondSHA, "--"}
	if f.OldPath != "" {
		args = append(args, f.OldPath)
	}
	args = append(args, f.Path)
	out, _, err := run.RunRaw(dir, run.NonInteractiveEnv(), args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
