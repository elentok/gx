package modelrules

import "testing"

func TestParseID(t *testing.T) {
	tests := []struct {
		id     string
		family string
		v      Version
		ok     bool
	}{
		{"gpt-6-sol", "sol", Version{6, 0}, true},
		{"gpt-5.4-mini", "mini", Version{5, 4}, true},
		{"claude-opus-5", "opus", Version{5, 0}, true},
		{"claude-fable-5-1", "fable", Version{5, 1}, true},
		{"claude-fable-5-1-20261101", "fable", Version{5, 1}, true},
		{"gpt-reserve", "", Version{}, false},
		{"codex-auto-review", "", Version{}, false},
		{"claude-weird", "", Version{}, false},
		{"opus", "", Version{}, false},
	}
	for _, tt := range tests {
		f, v, ok := ParseID(tt.id)
		if f != tt.family || v != tt.v || ok != tt.ok {
			t.Errorf("ParseID(%q) = %q %v %v", tt.id, f, v, ok)
		}
	}
}

func TestVersionCompareIsNumeric(t *testing.T) {
	if (Version{5, 10}).Compare(Version{5, 9}) <= 0 {
		t.Error("5.10 must be newer than 5.9")
	}
}

func e(value, id string) Entry { return Entry{Value: value, ID: id} }

func TestResolve(t *testing.T) {
	list := []Entry{
		{Value: "fable", ID: "claude-fable-5-1"},
		{Value: "claude-fable-5-1", ID: "claude-fable-5-1"},
		{Value: "alias", ID: "only-id"},
	}
	tests := []struct {
		name, value, wantValue string
		ok                     bool
	}{
		{"value match", "fable", "fable", true},
		{"id match", "only-id", "alias", true},
		{"no match", "nope", "", false},
	}
	for _, tc := range tests {
		e, ok := Resolve(list, tc.value)
		if ok != tc.ok || (ok && e.Value != tc.wantValue) {
			t.Errorf("%s: Resolve(%q) = %+v, %v; want value %q, ok %v", tc.name, tc.value, e, ok, tc.wantValue, tc.ok)
		}
	}
}

func TestLatest(t *testing.T) {
	hidden := func(en Entry) Entry { en.Hidden = true; return en }
	succ := func(en Entry, s string) Entry { en.Successor = s; return en }
	tests := []struct {
		name  string
		model string
		list  []Entry
		want  string
		ok    bool
	}{
		{"highest version, numeric", "claude-fable-5-9",
			[]Entry{e("claude-fable-5-10", "claude-fable-5-10"), e("claude-fable-5-9", "claude-fable-5-9")},
			"claude-fable-5-10", true},
		{"alias never decides", "fable",
			[]Entry{e("fable", "claude-fable-5-1"), e("claude-fable-5-1", "claude-fable-5-1"), e("claude-fable-5-2", "claude-fable-5-2")},
			"claude-fable-5-2", true},
		{"alias already latest", "fable",
			[]Entry{e("fable", "claude-fable-5-1"), e("claude-fable-5-1", "claude-fable-5-1")},
			"", false},
		{"hidden never counts", "claude-fable-5-1",
			[]Entry{e("claude-fable-5-1", "claude-fable-5-1"), hidden(e("claude-fable-5-2", "claude-fable-5-2"))},
			"", false},
		{"visible successor wins", "gpt-5-sol",
			[]Entry{succ(e("gpt-5-sol", "gpt-5-sol"), "gpt-5.1-sol"), e("gpt-5.1-sol", "gpt-5.1-sol"), e("gpt-7-sol", "gpt-7-sol")},
			"gpt-5.1-sol", true},
		{"hidden successor ignored", "gpt-5-sol",
			[]Entry{succ(e("gpt-5-sol", "gpt-5-sol"), "gpt-9-sol"), hidden(e("gpt-9-sol", "gpt-9-sol")), e("gpt-6-sol", "gpt-6-sol")},
			"gpt-6-sol", true},
		{"other family ignored", "gpt-5-sol",
			[]Entry{e("gpt-5-sol", "gpt-5-sol"), e("gpt-9-mini", "gpt-9-mini")},
			"", false},
		{"unparseable model", "gpt-reserve",
			[]Entry{e("gpt-reserve", "gpt-reserve"), e("gpt-9-sol", "gpt-9-sol")}, "", false},
		{"empty list", "claude-fable-5-1", nil, "", false},
	}
	for _, tt := range tests {
		got, ok := Latest(tt.model, tt.list)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: Latest = %q %v, want %q %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestEffort(t *testing.T) {
	all := []string{"low", "medium", "high", "xhigh", "max"}
	tests := []struct {
		name      string
		cur       string
		supported []string
		codex     bool
		def       string
		want      string
	}{
		{"keeps listed", "high", all, false, "", "high"},
		{"codex default", "xhigh", []string{"low", "medium"}, true, "medium", "medium"},
		{"claude nearest lower", "max", []string{"low", "medium", "high"}, false, "", "high"},
		{"claude lowest when none lower", "low", []string{"medium", "high"}, false, "", "medium"},
		{"empty stays empty", "", all, false, "", ""},
		{"no list gives empty", "high", nil, false, "", ""},
	}
	for _, tt := range tests {
		if got := Effort(tt.cur, tt.supported, tt.codex, tt.def); got != tt.want {
			t.Errorf("%s: Effort = %q, want %q", tt.name, got, tt.want)
		}
	}
}
