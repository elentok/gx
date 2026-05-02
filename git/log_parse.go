package git

import "strings"

func parseDecorations(raw string) []RefDecoration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var out []RefDecoration
	for _, part := range strings.Split(raw, ", ") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch {
		case strings.HasPrefix(part, "tag: "):
			out = append(out, RefDecoration{Name: strings.TrimPrefix(part, "tag: "), Kind: RefDecorationTag})
		case strings.HasPrefix(part, "HEAD -> "):
			out = append(out, RefDecoration{Name: strings.TrimPrefix(part, "HEAD -> "), Kind: RefDecorationLocalBranch})
		case strings.HasPrefix(part, "origin/"):
			out = append(out, RefDecoration{Name: part, Kind: RefDecorationRemoteBranch})
		default:
			out = append(out, RefDecoration{Name: part, Kind: RefDecorationLocalBranch})
		}
	}
	return out
}

func initials(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "?"
	}
	if len(parts) == 1 {
		r := []rune(parts[0])
		if len(r) == 0 {
			return "?"
		}
		if len(r) == 1 {
			return strings.ToUpper(string(r[0]))
		}
		return strings.ToUpper(string(r[:2]))
	}
	first := []rune(parts[0])
	last := []rune(parts[len(parts)-1])
	if len(first) == 0 || len(last) == 0 {
		return "?"
	}
	return strings.ToUpper(string(first[0]) + string(last[0]))
}
