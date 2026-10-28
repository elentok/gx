package tickets

import (
	"strings"
	"testing"
)

func questionBody(summary, block string) string {
	b := "## Question\n\n" + summary + "\n\nMore detail.\n"
	if block != "" {
		b += "\n```gx-question\n" + block + "```\n"
	}
	return b + "\n## Handoff\n\nResume at step 2.\n"
}

func TestParseQuestion_Kinds(t *testing.T) {
	tests := []struct {
		name  string
		block string
		check func(t *testing.T, q Question)
	}{
		{"no block is free text", "", func(t *testing.T, q Question) {
			if q.Kind != QuestionText || q.Detail != "More detail." {
				t.Fatalf("got %+v", q)
			}
		}},
		{"text", "type: text\n", func(t *testing.T, q Question) {
			if q.Kind != QuestionText {
				t.Fatalf("got %+v", q)
			}
		}},
		{"yes-no", "type: yes-no\nyes: ship it\nno: wait\nrecommended: no\n", func(t *testing.T, q Question) {
			if q.Kind != QuestionYesNo || q.Yes != "ship it" || q.No != "wait" || q.Recommended != "no" {
				t.Fatalf("got %+v", q)
			}
		}},
		{"options", "type: options\noptions:\n  - id: a\n    label: Use A\n    consequence: fast\n  - id: b\n    label: Use B\nrecommended: a\n", func(t *testing.T, q Question) {
			if q.Kind != QuestionOptions || len(q.Options) != 2 || q.Options[0].Consequence != "fast" || q.Recommended != "a" {
				t.Fatalf("got %+v", q)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := questionBody("Which one?", tt.block)
			strict, err := ParseQuestionStrict(body)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, strict)
			if strict.Summary != "Which one?" || strict.Handoff != "Resume at step 2." {
				t.Fatalf("got %+v", strict)
			}
			tt.check(t, ParseQuestionLenient(body))
		})
	}
}

func TestParseQuestionStrict_Errors(t *testing.T) {
	opts := func(n int) string {
		s := "type: options\noptions:\n"
		for i := 0; i < n; i++ {
			s += "  - id: o" + string(rune('a'+i)) + "\n    label: L\n"
		}
		return s
	}
	tests := []struct {
		name, block, want string
	}{
		{"bad yaml", "type: [\n", "invalid gx-question YAML"},
		{"colon in label", "type: options\noptions:\n  - id: a\n    label: Fix: now\n  - id: b\n    label: B\n", "wrap the label, consequence, yes and no values in double quotes"},
		{"unknown type", "type: poll\n", "unknown question type"},
		{"duplicate ids", "type: options\noptions:\n  - {id: a, label: A}\n  - {id: a, label: B}\n", "duplicate option id"},
		{"recommended outside options", opts(2) + "recommended: zz\n", "not an option id"},
		{"recommended not yes/no", "type: yes-no\nrecommended: maybe\n", "not yes or no"},
		{"one option", opts(1), "2 to 6 options, got 1"},
		{"seven options", opts(7), "2 to 6 options, got 7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := questionBody("Which one?", tt.block)
			_, err := ParseQuestionStrict(body)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
			q := ParseQuestionLenient(body)
			if q.Kind != QuestionText || len(q.Options) != 0 || q.Summary != "Which one?" {
				t.Fatalf("lenient should read free text, got %+v", q)
			}
		})
	}
}

func TestParseQuestion_SummaryTooLong(t *testing.T) {
	body := questionBody(strings.Repeat("é", MaxQuestionSummary+1), "")
	if _, err := ParseQuestionStrict(body); err == nil {
		t.Fatal("strict should fail on long summary")
	}
	if !ParseQuestionLenient(body).SummaryTooLong() {
		t.Fatal("lenient should flag long summary")
	}
	ok := questionBody(strings.Repeat("é", MaxQuestionSummary), "")
	if _, err := ParseQuestionStrict(ok); err != nil {
		t.Fatal(err)
	}
}

func TestParseQuestionStrict_MissingSection(t *testing.T) {
	if _, err := ParseQuestionStrict("## Handoff\n\nx\n"); err == nil {
		t.Fatal("want error")
	}
}
