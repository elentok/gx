package tickets

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/elentok/gx/tickets/schema"
)

// MaxQuestionSummary is the longest allowed question summary line, in runes.
const MaxQuestionSummary = 120

// Question kinds. A question with no gx-question block is QuestionText.
const (
	QuestionYesNo   = "yes-no"
	QuestionOptions = "options"
	QuestionText    = "text"
)

const (
	questionFence = "```gx-question"
	minOptions    = 2
	maxOptions    = 6
)

// QuestionOption is one choice of an options question.
type QuestionOption struct {
	ID          string `yaml:"id"`
	Label       string `yaml:"label"`
	Consequence string `yaml:"consequence"`
}

// Question is a parsed "## Question" + "## Handoff" body pair.
type Question struct {
	Summary     string
	Kind        string
	Yes, No     string // yes-no consequences
	Options     []QuestionOption
	Recommended string
	// Detail is the question text after the summary line, without the block.
	Detail  string
	Handoff string
}

// SummaryTooLong reports whether the summary exceeds MaxQuestionSummary. Strict
// parsing already fails on it; lenient callers (the web read, validate) use this.
func (q Question) SummaryTooLong() bool {
	return utf8.RuneCountInString(q.Summary) > MaxQuestionSummary
}

// questionBlock is the YAML wire form of the gx-question block.
type questionBlock struct {
	Type        string           `yaml:"type"`
	Yes         string           `yaml:"yes"`
	No          string           `yaml:"no"`
	Options     []QuestionOption `yaml:"options"`
	Recommended string           `yaml:"recommended"`
}

// ParseQuestionStrict parses body for `gx tickets ask`: any problem with the
// summary or the gx-question block is an error naming the problem.
func ParseQuestionStrict(body string) (Question, error) {
	q, block, hasBlock := splitQuestion(body)
	if q.Summary == "" {
		return q, errors.New("missing ## Question section or summary line")
	}
	if q.SummaryTooLong() {
		return q, fmt.Errorf("question summary is %d characters, max %d",
			utf8.RuneCountInString(q.Summary), MaxQuestionSummary)
	}
	if !hasBlock {
		return q, nil
	}
	if err := applyBlock(&q, block); err != nil {
		return q, err
	}
	return q, nil
}

// ParseQuestionLenient parses body for reading: a missing or bad gx-question
// block reads as a free-text question and never errors.
func ParseQuestionLenient(body string) Question {
	q, block, hasBlock := splitQuestion(body)
	if !hasBlock {
		return q
	}
	parsed := q
	if err := applyBlock(&parsed, block); err != nil {
		return q
	}
	return parsed
}

// splitQuestion extracts the free-text parts and the raw block. The returned
// Question is always free text (Kind QuestionText).
func splitQuestion(body string) (q Question, block string, hasBlock bool) {
	q.Kind = QuestionText
	q.Handoff = schema.Section(body, "Handoff")
	section := schema.Section(body, "Question")

	var rest []string
	inBlock := false
	for line := range strings.SplitSeq(section, "\n") {
		switch {
		case inBlock && strings.TrimSpace(line) == "```":
			inBlock = false
		case inBlock:
			block += line + "\n"
		case !hasBlock && strings.TrimSpace(line) == questionFence:
			inBlock, hasBlock = true, true
		default:
			rest = append(rest, line)
		}
	}
	text := strings.TrimSpace(strings.Join(rest, "\n"))
	q.Summary, q.Detail, _ = strings.Cut(text, "\n")
	q.Summary = strings.TrimSpace(q.Summary)
	q.Detail = strings.TrimSpace(q.Detail)
	return q, block, hasBlock
}

// applyBlock decodes and validates block into q.
func applyBlock(q *Question, block string) error {
	var b questionBlock
	dec := yaml.NewDecoder(bytes.NewReader([]byte(block)))
	dec.KnownFields(true)
	if err := dec.Decode(&b); err != nil {
		return fmt.Errorf("invalid gx-question YAML: %w", err)
	}
	switch b.Type {
	case QuestionYesNo:
		if len(b.Options) > 0 {
			return errors.New("yes-no question must not have options")
		}
		if b.Recommended != "" && b.Recommended != "yes" && b.Recommended != "no" {
			return fmt.Errorf("recommended %q is not yes or no", b.Recommended)
		}
	case QuestionOptions:
		if err := checkOptions(b.Options, b.Recommended); err != nil {
			return err
		}
	case QuestionText:
		if len(b.Options) > 0 || b.Recommended != "" {
			return errors.New("text question must not have options or recommended")
		}
	default:
		return fmt.Errorf("unknown question type %q", b.Type)
	}
	q.Kind, q.Yes, q.No = b.Type, b.Yes, b.No
	q.Options, q.Recommended = b.Options, b.Recommended
	return nil
}

func checkOptions(opts []QuestionOption, recommended string) error {
	if len(opts) < minOptions || len(opts) > maxOptions {
		return fmt.Errorf("options question needs %d to %d options, got %d", minOptions, maxOptions, len(opts))
	}
	seen := map[string]bool{}
	for _, o := range opts {
		if o.ID == "" || o.Label == "" {
			return errors.New("every option needs an id and a label")
		}
		if seen[o.ID] {
			return fmt.Errorf("duplicate option id %q", o.ID)
		}
		seen[o.ID] = true
	}
	if recommended != "" && !seen[recommended] {
		return fmt.Errorf("recommended %q is not an option id", recommended)
	}
	return nil
}
