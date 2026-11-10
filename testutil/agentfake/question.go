package agentfake

// QuestionOption is one choice of a request_user_input question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// QuestionSpec is one question of a request_user_input call.
type QuestionSpec struct {
	ID       string           `json:"id"`
	Header   string           `json:"header"`
	Question string           `json:"question"`
	Options  []QuestionOption `json:"options"`
}

// QuestionCall is what an ActionQuestion of the Codex fake opens: a
// request_user_input call written to the rollout, and its overlay.
type QuestionCall struct {
	CallID    string         `json:"call_id"`
	Questions []QuestionSpec `json:"questions"`
	// Shown, when set, is what the overlay draws in place of Questions, for a
	// screen that does not match the call.
	Shown []QuestionSpec `json:"shown,omitempty"`
	// Quiet writes the call and nothing else: no Blocked report, no overlay,
	// until an ActionQuestionShow.
	Quiet bool `json:"quiet,omitempty"`
}
