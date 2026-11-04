package wire

// Refusal reasons of the session prompt route, beside the session ones.
const (
	ReasonPromptHarnessNotSupported = "harness-not-supported"
	ReasonPromptPrefixNotAllowed    = "prefix-not-allowed"
	ReasonPromptTextTooLong         = "text-too-long"
)

// SessionPromptRequest is the body of POST /v1/sessions/prompt. LastMessage is
// the id of the last chat message the page saw; the web mux refuses a send once
// the transcript has moved past it.
type SessionPromptRequest struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	LastMessage string `json:"last_message,omitempty"`
	// Continue replaces Text: the server types /clear and /gx-design <epic>.
	Continue bool `json:"continue,omitempty"`
}
