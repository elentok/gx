package grillround_test

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/transcript"
)

const (
	yesNoBlock = "\n\n```gx-question\ntype: yes-no\nyes: Go ahead\nno: Stop here\nrecommended: yes\n```"
	optsBlock  = "\n\n```gx-question\ntype: options\nrecommended: a\noptions:\n  - id: a\n    label: SQLite\n    consequence: Fast reads\n  - id: b\n    label: Files on disk\n```"
	defaults   = "\n\n---\n\n**Decided unless you object**\n\n- Q3: keep it.\n- Q4: move it.\n- Q5: drop it."
	runEnd     = "✅ Design: done. Next: go on"
)

var base = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// chat builds messages with ids m0, m1, ... and rising timestamps. A string
// starting with "> " is the person, "tools" a tools entry, anything else agent
// text.
func chat(parts ...string) []transcript.ChatMessage {
	var msgs []transcript.ChatMessage
	for i, p := range parts {
		m := transcript.ChatMessage{ID: "m" + strconv.Itoa(i), At: base.Add(time.Duration(i) * time.Minute)}
		switch {
		case len(p) > 2 && p[:2] == "> ":
			m.Role, m.Kind, m.Text = transcript.ChatRolePerson, transcript.ChatKindPrompt, p[2:]
		case p == "tools":
			m.Role, m.Kind, m.ToolCount = transcript.ChatRoleAgent, transcript.ChatKindTools, 3
		default:
			m.Role, m.Kind, m.Text = transcript.ChatRoleAgent, transcript.ChatKindText, p
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func item(msg, questions, words int) grillround.Item {
	return grillround.Item{MessageID: "m" + strconv.Itoa(msg), At: base.Add(time.Duration(msg) * time.Minute), Questions: questions, Words: words}
}

func TestCount(t *testing.T) {
	alpha := "❓ **Q1** - **Alpha**: one two"
	cases := []struct {
		name string
		msgs []transcript.ChatMessage
		want []grillround.Item
	}{
		{"one card with yes-no labels", chat("❓ **Q1** - **Which store?**: SQLite or files?" + yesNoBlock), []grillround.Item{item(0, 1, 9)}},
		{"multi-card round", chat(alpha + "\n\n❓ **Q2** - **Beta**: three"), []grillround.Item{item(0, 2, 5)}},
		{"options card counts labels and consequences, not keys", chat("❓ **Q1** - **Store**: Pick one." + optsBlock), []grillround.Item{item(0, 1, 9)}},
		{"defaults list counts words, not questions", chat("❓ **Q1** - **Alpha**: one\n\n❓ **Q2** - **Beta**: two" + defaults), []grillround.Item{item(0, 2, 13)}},
		{"no defaults list", chat("❓ **Q1** - **Alpha**: one\n\n❓ **Q2** - **Beta**: two"), []grillround.Item{item(0, 2, 4)}},
		{"accept these defaults", chat("❓ **Q1** - **Accept these defaults?**: Say so." + yesNoBlock + defaults), []grillround.Item{item(0, 1, 5+4+9)}},
		{
			"shared understanding keeps its summary and adds none after",
			chat("We agreed on A and B.\n\n❓ **Q4** - **Shared understanding**: Is this right?", "> yes", "Recorded.\n\n"+runEnd),
			[]grillround.Item{item(0, 1, 11)},
		},
		{
			"summary in its own message",
			chat(alpha, "> a", "Recording the answer.", "Summary: we picked a.", runEnd),
			[]grillround.Item{item(0, 1, 3), item(3, 0, 4)},
		},
		{
			"summary shares a message with the run-end line",
			chat(alpha, "> a", "We picked a.\n\n"+runEnd),
			[]grillround.Item{item(0, 1, 3), item(2, 0, 3)},
		},
		{
			"summary shares a message with the next round",
			chat(alpha, "> a", "We picked a.\n\n❓ **Q2** - **Beta**: two"),
			[]grillround.Item{item(0, 1, 3), item(2, 0, 3), item(2, 1, 2)},
		},
		{"plain text and tool notes count nothing", chat("Looking at it.", "tools", "Still looking.", runEnd), nil},
		{
			"markdown and glyphs are stripped",
			chat("Intro words here.\n\n❓ **Q1** - **Title**: See [the docs](http://x.y/z) for `code`.\n➡️ I recommend **yes**.\n# Head\n- one"),
			[]grillround.Item{item(0, 1, 11)},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := grillround.Count(c.msgs); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Count = %+v, want %+v", got, c.want)
			}
		})
	}
}
