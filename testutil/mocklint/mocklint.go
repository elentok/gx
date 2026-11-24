// Package mocklint checks a stored mock file for values a mock must not
// invent: raw colors, system-ui, scripts and switcher markup, plus the
// state and width markup a mock check needs.
package mocklint

import (
	"fmt"
	"regexp"
	"strings"
)

// SwitcherClass is the root class of the standard variant switcher pill
// (skills/gx-prototype/switcher-snippet.html). A stored mock has no switcher.
const SwitcherClass = "mk-sw"

// Finding is one problem; it names the file and the rule.
type Finding struct {
	File string
	Rule string
	Msg  string
}

func (f Finding) String() string { return fmt.Sprintf("%s: %s: %s", f.File, f.Rule, f.Msg) }

const (
	RuleColor      = "raw-color"
	RuleSystemUI   = "system-ui"
	RuleScript     = "script"
	RuleSwitcher   = "switcher"
	RuleNoState    = "missing-state"
	RuleNoWidth    = "missing-width"
	RuleStateWidth = "state-without-width"
)

var (
	// The one exempt block: the <style> that carries data-mock-inlined.
	exemptStyle = regexp.MustCompile(`(?is)<style\b[^>]*\sdata-mock-inlined\b[^>]*>.*?</style>`)
	styleElem   = regexp.MustCompile(`(?is)<style\b[^>]*>(.*?)</style>`)
	styleAttr   = regexp.MustCompile(`(?is)\sstyle\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	// A hex followed by `{` before any `;` or `}` is an id selector, not a value.
	hexColor   = regexp.MustCompile(`(#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4}))\b[^{;}]*(?:[;}]|$)`)
	funcColor  = regexp.MustCompile(`(?i)\b(?:rgba?|hsla?)\s*\(`)
	namedColor = regexp.MustCompile(`(?i)(?:^|[^\w.#-])(` + strings.Join(cssNamedColors, "|") + `)(?:[^\w-]|$)`)
	systemUI   = regexp.MustCompile(`(?i)system-ui`)
	scriptTag  = regexp.MustCompile(`(?i)<script\b`)
	switcher   = regexp.MustCompile(`(?:^|[\s"'.])` + SwitcherClass + `(?:[\s"'{.:\[,>]|$)`)
	stateTag   = regexp.MustCompile(`(?is)<[a-z][^>]*\sdata-mock-state\b[^>]*>`)
	widthAttr  = regexp.MustCompile(`\sdata-mock-width\b`)
)

// Lint returns one finding per problem in the mock file's text.
func Lint(file, text string) []Finding {
	text = exemptStyle.ReplaceAllString(text, "")
	var out []Finding
	add := func(rule, msg string) { out = append(out, Finding{file, rule, msg}) }

	for _, css := range cssSpans(text) {
		if m := hexColor.FindStringSubmatch(css); m != nil {
			add(RuleColor, "hex color "+m[1]+" outside the data-mock-inlined style")
		}
		if m := funcColor.FindString(css); m != "" {
			add(RuleColor, "color function "+m+" outside the data-mock-inlined style")
		}
		if m := namedColor.FindStringSubmatch(css); m != nil {
			add(RuleColor, "named color "+m[1]+" outside the data-mock-inlined style")
		}
	}
	if systemUI.MatchString(text) {
		add(RuleSystemUI, "system-ui outside the data-mock-inlined style")
	}
	if scriptTag.MatchString(text) {
		add(RuleScript, "a stored mock holds no <script> element")
	}
	if switcher.MatchString(text) {
		add(RuleSwitcher, "switcher markup ("+SwitcherClass+") in a stored mock")
	}
	states := stateTag.FindAllString(text, -1)
	if len(states) == 0 {
		add(RuleNoState, "no data-mock-state markup")
	}
	hasWidth := widthAttr.MatchString(text)
	if !hasWidth {
		add(RuleNoWidth, "no data-mock-width markup")
	}
	for _, tag := range states {
		if hasWidth && !widthAttr.MatchString(tag) {
			add(RuleStateWidth, "state section has no data-mock-width: "+tag)
		}
	}
	return out
}

// cssSpans returns the text of every <style> element and style="" attribute.
func cssSpans(text string) []string {
	var spans []string
	for _, m := range styleElem.FindAllStringSubmatch(text, -1) {
		spans = append(spans, m[1])
	}
	for _, m := range styleAttr.FindAllStringSubmatch(styleElem.ReplaceAllString(text, ""), -1) {
		spans = append(spans, m[1]+m[2])
	}
	return spans
}
