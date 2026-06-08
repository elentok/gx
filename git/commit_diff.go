package git

import (
	"strings"
)

type CommitFile struct {
	Status     string
	Path       string
	RenameFrom string
}

func CommitFilesForRef(repoRoot, ref string) ([]CommitFile, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	args := []string{
		"show",
		"--find-renames",
		"--format=",
		"--name-status",
		ref,
	}
	if isStashRef(ref) {
		args = []string{"stash", "show", "-u", "--name-status", ref}
	}
	out, _, err := run(repoRoot, args)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var files []CommitFile
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		file := CommitFile{Status: parts[0]}
		switch {
		case strings.HasPrefix(parts[0], "R") || strings.HasPrefix(parts[0], "C"):
			if len(parts) >= 3 {
				file.RenameFrom = parts[1]
				file.Path = parts[2]
			}
		default:
			file.Path = parts[1]
		}
		if file.Path == "" {
			continue
		}
		files = append(files, file)
	}
	return files, nil
}

func CommitFileDiffForRef(repoRoot, ref, path string, contextLines int) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	args, err := singleFileDiffArgs(repoRoot, ref, path, contextLines, false)
	if err != nil {
		return "", err
	}
	out, _, err := run(repoRoot, args)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func CommitFileDiffWithDeltaForRef(repoRoot, ref, path string, contextLines, renderWidth int, sideBySide bool) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	rawArgs, err := singleFileDiffArgs(repoRoot, ref, path, contextLines, false)
	if err != nil {
		return "", err
	}
	raw, _, err := run(repoRoot, rawArgs)
	if err != nil {
		return "", err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if out, deltaErr := colorizeWithDelta(repoRoot, raw, sideBySide, renderWidth); deltaErr == nil {
		return out, nil
	}
	colorArgs, err := singleFileDiffArgs(repoRoot, ref, path, contextLines, true)
	if err != nil {
		return "", err
	}
	out, _, err := run(repoRoot, colorArgs)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func isStashRef(ref string) bool {
	return strings.HasPrefix(ref, "stash@{") || ref == "stash"
}

func singleFileDiffArgs(repoRoot, ref, path string, contextLines int, color bool) ([]string, error) {
	if !isStashRef(ref) {
		args := []string{
			"show",
			"--find-renames",
			diffContextArg(contextLines),
		}
		if color {
			args = append(args, "--color=always")
		} else {
			args = append(args, "--no-color")
		}
		args = append(args, "--format=", ref, "--", path)
		return args, nil
	}

	targetRef, err := stashTargetRef(repoRoot, ref, path)
	if err != nil {
		return nil, err
	}

	args := []string{
		"diff",
		"--find-renames",
		diffContextArg(contextLines),
	}
	if color {
		args = append(args, "--color=always")
	} else {
		args = append(args, "--no-color")
	}
	args = append(args, ref+"^1", targetRef, "--", path)
	return args, nil
}

// stashTargetRef resolves the "new" side of a stash diff: the stash commit
// itself for tracked changes, or its third parent (ref^3, the untracked-files
// commit) for files absent from the stash's own tree.
func stashTargetRef(repoRoot, ref, path string) (string, error) {
	exists, err := pathExistsInTree(repoRoot, ref, path)
	if err != nil {
		return "", err
	}
	if !exists {
		return ref + "^3", nil
	}
	return ref, nil
}

// commitDiffEndpoints resolves the two object endpoints an image diff compares,
// using the exact same rules the text diff (singleFileDiffArgs) uses so the two
// can never disagree about which versions they show: a regular commit compares
// its first parent (ref^1) against ref; a stash compares its base (ref^1)
// against the stash commit (or ref^3 for untracked files); renames take the old
// path from RenameFrom.
func commitDiffEndpoints(repoRoot, ref string, file CommitFile) (oldRef, oldPath, newRef, newPath string, err error) {
	oldPath = file.Path
	if file.RenameFrom != "" {
		oldPath = file.RenameFrom
	}
	newPath = file.Path

	if !isStashRef(ref) {
		return ref + "^1", oldPath, ref, newPath, nil
	}

	newRef, err = stashTargetRef(repoRoot, ref, newPath)
	if err != nil {
		return "", "", "", "", err
	}
	return ref + "^1", oldPath, newRef, newPath, nil
}

// CommitImageDiffBlobs returns the raw old/new bytes for an image file changed
// in ref, resolving the two endpoints via commitDiffEndpoints (so they match the
// unified text diff exactly). oldOK/newOK report side presence — false for the
// absent side of an added or deleted file, an expected state rather than an
// error.
func CommitImageDiffBlobs(repoRoot, ref string, file CommitFile) (oldBytes, newBytes []byte, oldOK, newOK bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	oldRef, oldPath, newRef, newPath, err := commitDiffEndpoints(repoRoot, ref, file)
	if err != nil {
		return nil, nil, false, false
	}
	oldBytes, oldOK = gitObjectBytes(repoRoot, oldRef+":"+oldPath)
	newBytes, newOK = gitObjectBytes(repoRoot, newRef+":"+newPath)
	return oldBytes, newBytes, oldOK, newOK
}

func pathExistsInTree(repoRoot, treeish, path string) (bool, error) {
	_, _, err := run(repoRoot, []string{"cat-file", "-e", treeish + ":" + path})
	if err == nil {
		return true, nil
	}
	if _, _, parentErr := run(repoRoot, []string{"rev-parse", "--verify", treeish}); parentErr != nil {
		return false, parentErr
	}
	return false, nil
}
