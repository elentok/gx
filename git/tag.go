package git

import (
	"fmt"
	"strconv"
	"strings"
)

// LastTag returns the most recent tag reachable from HEAD, or "v0.0.0" if none.
func LastTag(root string) string {
	tag := runAllowFail(root, []string{"describe", "--tags", "--abbrev=0"})
	if tag == "" {
		return "v0.0.0"
	}
	return tag
}

// ParseVersion parses "vMAJOR.MINOR.PATCH" into its integer components.
func ParseVersion(tag string) (major, minor, patch int, err error) {
	parts := strings.SplitN(strings.TrimPrefix(tag, "v"), ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("cannot parse version from tag %q", tag)
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, fmt.Errorf("cannot parse version from tag %q", tag)
	}
	return major, minor, patch, nil
}

// CreateAnnotatedTag creates an annotated tag at HEAD.
func CreateAnnotatedTag(root, tag, message string) error {
	_, _, err := run(root, []string{"tag", "-a", tag, "-m", message})
	return err
}

// PushOrigin pushes the current branch to origin.
func PushOrigin(root string) error {
	_, _, err := run(root, []string{"push", "origin"})
	return err
}

// PushTag pushes a specific tag to origin.
func PushTag(root, tag string) error {
	_, _, err := run(root, []string{"push", "origin", tag})
	return err
}
