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

	targetRef := ref
	if exists, err := pathExistsInTree(repoRoot, ref, path); err != nil {
		return nil, err
	} else if !exists {
		targetRef = ref + "^3"
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
