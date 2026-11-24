package mocklint

import (
	"os"
	"slices"
	"strings"
	"testing"
)

const okBody = `<section data-mock-state="a" data-mock-width="390"><p>hi</p></section>`

func page(head, body string) string {
	return "<html><head>" + head + "</head><body>" + body + "</body></html>"
}

func rules(fs []Finding) []string {
	var r []string
	for _, f := range fs {
		r = append(r, f.Rule)
	}
	return r
}

func TestLint(t *testing.T) {
	inlined := `<style data-mock-inlined>:root{--c:#fff;--f:system-ui;color:white;background:rgb(0,0,0)}</style>`
	cases := []struct {
		name string
		text string
		want string // rule of the single finding, "" for none
	}{
		{"clean with exempt block", page(inlined, okBody), ""},
		{"tokens in var()", page(`<style>p{color:var(--c)}</style>`, okBody), ""},
		{"class named like a color", page(`<style>.red,#add{margin:0}</style>`, okBody), ""},
		{"hex in style", page(`<style>p{color:#abc}</style>`, okBody), RuleColor},
		{"hex in attr", page("", `<section style="color:#ff0000" data-mock-state="a" data-mock-width="1"></section>`), RuleColor},
		{"rgba", page(`<style>p{color:rgba(0,0,0,.5)}</style>`, okBody), RuleColor},
		{"hsl", page(`<style>p{color:hsl(0 0% 0%)}</style>`, okBody), RuleColor},
		{"named color", page(`<style>p{color:white}</style>`, okBody), RuleColor},
		{"root outside exempt block", page(`<style>:root{--c:red}</style>`, okBody), RuleColor},
		{"system-ui", page(`<style>p{font-family:system-ui}</style>`, okBody), RuleSystemUI},
		{"script", page("", okBody+`<script>1</script>`), RuleScript},
		{"switcher", page("", okBody+`<div class="mk-sw"></div>`), RuleSwitcher},
		{"no state markup", page("", `<p data-mock-width="1"></p>`), RuleNoState},
		{"no width markup", page("", `<section data-mock-state="a"></section>`), RuleNoWidth},
		{"state without width", page("", okBody+`<section data-mock-state="b"></section>`), RuleStateWidth},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rules(Lint("m.html", c.text))
			switch {
			case c.want == "" && len(got) != 0:
				t.Fatalf("want no findings, got %v", got)
			case c.want != "" && (len(got) != 1 || got[0] != c.want):
				t.Fatalf("want [%s], got %v", c.want, got)
			}
		})
	}
}

func TestFindingNamesFileAndRule(t *testing.T) {
	fs := Lint("dir/m.html", page(`<style>p{color:#abc}</style>`, okBody))
	if len(fs) != 1 {
		t.Fatalf("findings: %v", fs)
	}
	msg := fs[0].String()
	if !strings.Contains(msg, "dir/m.html") || !strings.Contains(msg, RuleColor) {
		t.Fatalf("message %q lacks file or rule", msg)
	}
}

func TestSwitcherSnippetCarriesClass(t *testing.T) {
	data, err := os.ReadFile("../../skills/gx-prototype/switcher-snippet.html")
	if err != nil {
		t.Fatal(err)
	}
	if SwitcherClass != "mk-sw" || !strings.Contains(string(data), `class="`+SwitcherClass+`"`) {
		t.Fatalf("snippet must carry class %q", SwitcherClass)
	}
	if !slices.Contains(rules(Lint("snippet", string(data))), RuleSwitcher) {
		t.Fatal("the snippet itself must trip the switcher rule")
	}
}
