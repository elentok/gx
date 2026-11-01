package tickets

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// ClaimKind is the shape a ticket-body name was sorted into.
type ClaimKind string

const (
	ClaimCommand     ClaimKind = "command"      // `gx tickets add --slug x`
	ClaimTestPattern ClaimKind = "test-pattern" // `-run 'TestX'`
	ClaimFileFunc    ClaimKind = "file-func"    // `tickets/x.go:Func`
	ClaimPath        ClaimKind = "path"         // `tickets/x.go`
	ClaimLineRef     ClaimKind = "line-ref"     // `x.go:42` or prose "line 42"
	ClaimFlow        ClaimKind = "flow"         // prose "flow 3" / "new flow 3"
)

// Claim is one name a ticket body asserts exists. Which fields are set
// depends on Kind.
type Claim struct {
	Kind    ClaimKind
	Text    string   // the span, or the matched prose
	Section string   // "## " heading it came from; "" for text before the first heading
	Words   []string // command: subcommand words after "gx"
	Flags   []string // command: --flags, without any =value
	Pattern string   // test pattern: the regexp
	File    string   // path, file-func, line-ref
	Func    string   // file-func
	Flow    int      // flow
	New     bool     // flow: written as "new flow N"
}

// Fixed lists for span classification. Every skip and extension rule lives here.
var (
	claimExtensions = []string{"go", "md", "yaml", "yml", "json", "ts", "tsx", "js", "py", "sh", "sql", "toml"}
	// A span containing any of these is never a claim.
	claimSkipMarkers = []string{"*", "~", "$", "://"}
)

var (
	inlineSpanRe = regexp.MustCompile("`([^`\n]+)`")
	runFormRe    = regexp.MustCompile(`^-run (?:'([^']*)'|"([^"]*)")$`)
	fileFuncRe   = regexp.MustCompile(`^([^:\s]+):([A-Za-z_]\w*)$`)
	fileLineRe   = regexp.MustCompile(`^([^:\s]+):\d+(?:-\d+)?$`)
	proseFlowRe  = regexp.MustCompile(`(?i)\b(new )?flow (\d+)\b`)
	proseLineRe  = regexp.MustCompile(`(?i)\blines? (\d+)\b`)
)

// ExtractClaims reads the backticked spans of every section of body, plus the
// prose forms "flow N", "new flow N" and "line N". Fenced code blocks are
// skipped. topDirs are the repo's top-level directory names.
func ExtractClaims(body string, topDirs []string) []Claim {
	preamble, sections := schema.SplitSections(body)
	all := append([]schema.NamedSection{{Content: preamble}}, sections...)
	var claims []Claim
	for _, s := range all {
		text := stripFences(s.Content)
		for _, m := range inlineSpanRe.FindAllStringSubmatch(text, -1) {
			if c, ok := classifySpan(m[1], topDirs); ok {
				c.Section = s.Name
				claims = append(claims, c)
			}
		}
		prose := inlineSpanRe.ReplaceAllString(text, " ")
		claims = append(claims, proseClaims(prose, s.Name)...)
	}
	return claims
}

func stripFences(text string) string {
	var out []string
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func proseClaims(prose, section string) []Claim {
	var claims []Claim
	for _, m := range proseFlowRe.FindAllStringSubmatch(prose, -1) {
		n, _ := strconv.Atoi(m[2])
		claims = append(claims, Claim{Kind: ClaimFlow, Text: m[0], Section: section, Flow: n, New: m[1] != ""})
	}
	for _, m := range proseLineRe.FindAllStringSubmatch(prose, -1) {
		claims = append(claims, Claim{Kind: ClaimLineRef, Text: m[0], Section: section})
	}
	return claims
}

func classifySpan(span string, topDirs []string) (Claim, bool) {
	span = strings.TrimSpace(span)
	isGx := strings.HasPrefix(span, "gx ")
	run := runFormRe.FindStringSubmatch(span)
	if slices.ContainsFunc(claimSkipMarkers, func(m string) bool { return strings.Contains(span, m) }) {
		return Claim{}, false
	}
	if strings.Contains(span, " ") && !isGx && run == nil {
		return Claim{}, false
	}
	switch {
	case isGx:
		words, flags := commandParts(span)
		return Claim{Kind: ClaimCommand, Text: span, Words: words, Flags: flags}, true
	case run != nil:
		return Claim{Kind: ClaimTestPattern, Text: span, Pattern: run[1] + run[2]}, true
	}
	if m := fileLineRe.FindStringSubmatch(span); m != nil && hasKnownExt(m[1]) {
		return Claim{Kind: ClaimLineRef, Text: span, File: m[1]}, true
	}
	if m := fileFuncRe.FindStringSubmatch(span); m != nil && hasKnownExt(m[1]) {
		return Claim{Kind: ClaimFileFunc, Text: span, File: m[1], Func: m[2]}, true
	}
	if isPathSpan(span, topDirs) {
		return Claim{Kind: ClaimPath, Text: span, File: span}, true
	}
	return Claim{}, false
}

func hasKnownExt(name string) bool {
	i := strings.LastIndex(name, ".")
	return i >= 0 && slices.Contains(claimExtensions, name[i+1:])
}

func isPathSpan(span string, topDirs []string) bool {
	dir, _, hasSlash := strings.Cut(span, "/")
	if !hasSlash {
		return false
	}
	return hasKnownExt(span) || slices.Contains(topDirs, dir)
}

// commandParts splits "gx a b <x> --flag=v" into subcommand words (up to the
// first flag or placeholder) and the --flags.
func commandParts(span string) (words, flags []string) {
	inWords := true
	for _, tok := range strings.Fields(span)[1:] {
		switch {
		case strings.HasPrefix(tok, "--"):
			inWords = false
			flags = append(flags, strings.SplitN(tok, "=", 2)[0])
		case !inWords:
		case strings.ContainsAny(tok[:1], "-<[\"'"):
			inWords = false
		default:
			words = append(words, tok)
		}
	}
	return words, flags
}
