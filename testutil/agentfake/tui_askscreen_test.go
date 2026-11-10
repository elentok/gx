package agentfake

import (
	"testing"

	"github.com/elentok/gx/agentrunner/claudeui"
)

func TestAskScreen_RowsParseAsTheRealDialog(t *testing.T) {
	t.Parallel()
	opts := []AskOption{{Label: "S", Description: "about S"}, {Label: "M"}}
	for name, q := range map[string]AskQuestion{
		"single": {Header: "Size", Question: "Which size?", Options: opts},
		"multi":  {Header: "Size", Question: "Which sizes?", MultiSelect: true, Options: opts},
	} {
		u := &askUI{spec: AskScreen{Questions: []AskQuestion{q, q}}, done: make([]bool, 2), typed: make([]string, 2), checked: [][]bool{make([]bool, 3), make([]bool, 3)}}
		d, ok := claudeui.ParseAskDialog(u.rows())
		if !ok || d.Multi != q.MultiSelect || len(d.Options) != 3 || d.Cursor != claudeui.AskCursorOption {
			t.Errorf("%s: parsed %+v, %v from %q", name, d, ok, u.rows())
		}
		u.cur = 2
		if d, ok := claudeui.ParseAskDialog(u.rows()); !ok || !d.Review {
			t.Errorf("%s: review parsed %+v, %v from %q", name, d, ok, u.rows())
		}
	}
}
