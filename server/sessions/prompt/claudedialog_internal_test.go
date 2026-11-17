package prompt

import (
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestClaudeUpdatedInput_ClickOrder(t *testing.T) {
	held := wire.AskHookRequest{Questions: []wire.AskHookQuestion{{
		Question:    "Which?",
		MultiSelect: true,
		Options:     []wire.AskHookOption{{Label: "A"}, {Label: "B"}},
	}}}
	tests := []struct {
		name    string
		answer  wire.SessionAnswer
		want    string
		refusal string
	}{
		{"Other between picks", wire.SessionAnswer{Choices: []string{"2", "other", "1"}, Text: "mine"}, "B, mine, A", ""},
		{"text without Other goes last", wire.SessionAnswer{Choices: []string{"2", "1"}, Text: "mine"}, "B, A, mine", ""},
		{"Other without text", wire.SessionAnswer{Choices: []string{"other"}}, "", "question 1 needs text for Other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, refusal := claudeUpdatedInput(held, []wire.SessionAnswer{tt.answer})
			if refusal != tt.refusal {
				t.Fatalf("refusal = %q, want %q", refusal, tt.refusal)
			}
			if tt.refusal != "" {
				return
			}
			var got struct {
				Answers map[string]string `json:"answers"`
			}
			if err := json.Unmarshal(input, &got); err != nil {
				t.Fatal(err)
			}
			if got.Answers["Which?"] != tt.want {
				t.Fatalf("answer = %q, want %q", got.Answers["Which?"], tt.want)
			}
		})
	}
}

func TestCodexQuestionText_FallsBackToTheTitle(t *testing.T) {
	tests := []struct {
		name string
		q    wire.QuestionCardQuestion
		want string
	}{
		{"raw text wins", wire.QuestionCardQuestion{Title: "Header: Which?", Question: "Which?"}, "Which?"},
		{"empty raw falls back", wire.QuestionCardQuestion{Title: "Header: Which?"}, "Header: Which?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codexQuestionText(tt.q); got != tt.want {
				t.Fatalf("codexQuestionText = %q, want %q", got, tt.want)
			}
		})
	}
}
