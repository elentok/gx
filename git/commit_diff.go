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
	out, _, err := run(repoRoot, []string{
		"show",
		"--find-renames",
		"--format=",
		"--name-status",
		ref,
	})
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

func CommitFileDiffForRef(repoRoot, ref, path string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	out, _, err := run(repoRoot, []string{
		"show",
		"--find-renames",
		"--unified=1",
		"--format=",
		ref,
		"--",
		path,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
