package wire

// BrokenRuleReport is the request of POST /v1/broken-rules: a rule the web UI
// found broken. Every field is required.
type BrokenRuleReport struct {
	Rule      string `json:"rule"` // a short slug, for example "text-box-name"
	Message   string `json:"message"`
	Value     string `json:"value"` // the value that broke the rule
	URL       string `json:"url"`
	UserAgent string `json:"user_agent"`
}

// BrokenRuleResult answers a BrokenRuleReport. A refused report has one of the
// reasons "rate-limited", "too-large", "incomplete" and "bad-rule".
type BrokenRuleResult struct {
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
}
