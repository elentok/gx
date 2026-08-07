package claudehistory

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Conversation holds cheap metadata extracted from a single .jsonl transcript.
type Conversation struct {
	Path         string
	SessionID    string
	Title        string
	LastAccessed time.Time
}

// ConversationMeta parses path cheaply to extract title and last-accessed time.
// It does not count turns or tokens.
func ConversationMeta(path string) (Conversation, error) {
	f, err := os.Open(path)
	if err != nil {
		return Conversation{}, err
	}
	defer f.Close()

	var (
		aiTitle       string
		lastPrompt    string
		customTitle   string
		firstUserText string
		sessionID     string
		lastAccessed  time.Time
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		var rec transcriptLine
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}

		if rec.SessionID != "" && sessionID == "" {
			sessionID = rec.SessionID
		}

		if rec.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
				lastAccessed = t
			}
		}

		switch rec.Type {
		case "ai-title":
			if aiTitle == "" {
				aiTitle = rec.AITitle
			}
		case "last-prompt":
			if rec.LastPrompt != "" {
				lastPrompt = rec.LastPrompt
			}
		case "custom-title":
			if rec.CustomTitle != "" {
				customTitle = rec.CustomTitle
			}
		case "user":
			if firstUserText == "" && !rec.IsSidechain && rec.ParentUUID == "" {
				firstUserText = extractUserMessageText(rec.RawMessage)
			}
		}
	}

	return Conversation{
		Path:         path,
		SessionID:    sessionID,
		Title:        buildConversationTitle(customTitle, aiTitle, lastPrompt, firstUserText, sessionID),
		LastAccessed: lastAccessed,
	}, scanner.Err()
}

// ListConversations returns all conversations in dir (a project directory),
// sorted most-recently-accessed first. Metadata is parsed in parallel.
func ListConversations(dir string) ([]Conversation, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}

	if len(paths) == 0 {
		return nil, nil
	}

	type result struct {
		conv Conversation
		err  error
	}
	results := make([]result, len(paths))
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			c, err := ConversationMeta(p)
			results[i] = result{conv: c, err: err}
		}(i, p)
	}
	wg.Wait()

	var convs []Conversation
	for _, r := range results {
		if r.err == nil && (r.conv.SessionID != "" || r.conv.Title != "") {
			convs = append(convs, r.conv)
		}
	}

	sort.Slice(convs, func(i, j int) bool {
		return convs[i].LastAccessed.After(convs[j].LastAccessed)
	})

	return convs, nil
}

type transcriptLine struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	AITitle     string          `json:"aiTitle"`
	LastPrompt  string          `json:"lastPrompt"`
	CustomTitle string          `json:"customTitle"`
	ParentUUID  string          `json:"parentUuid"`
	IsSidechain bool            `json:"isSidechain"`
	RawMessage  json.RawMessage `json:"message"`
}

func extractUserMessageText(rawMsg json.RawMessage) string {
	return decodeMessageText(rawMsg, cleanUserText)
}

// decodeMessageText decodes message.content, which rg/Claude Code transcripts
// encode as either a plain string or a list of content blocks, and returns
// the first non-empty text run through postProcess. Shared by the assistant-
// and user-text extractors, which differ only in postProcess.
func decodeMessageText(rawMsg json.RawMessage, postProcess func(string) string) string {
	if len(rawMsg) == 0 {
		return ""
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(rawMsg, &msg); err != nil || len(msg.Content) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return postProcess(s)
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type != "text" {
			continue
		}
		if text := postProcess(b.Text); text != "" {
			return text
		}
	}
	return ""
}

// leadingXMLRE matches a single XML-like tag at the start of a string.
// Used to strip slash-command metadata noise like <command-message>...</command-message>.
var leadingXMLRE = regexp.MustCompile(`^<[^>]+>[^<]*</[^>]+>\s*`)

func cleanUserText(s string) string {
	s = strings.TrimSpace(s)
	for leadingXMLRE.MatchString(s) {
		s = strings.TrimSpace(leadingXMLRE.ReplaceAllLiteralString(s, ""))
	}
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// acknowledgementWords are short replies that confirm/approve a prior
// message rather than describing the conversation's topic (e.g. the closing
// "agree" of a /grill flow). They're skipped as a title candidate since
// they're never informative on their own.
var acknowledgementWords = map[string]struct{}{
	"agree": {}, "agreed": {}, "yes": {}, "yep": {}, "yeah": {}, "yup": {},
	"ok": {}, "okay": {}, "sure": {}, "sounds good": {}, "lgtm": {},
	"continue": {}, "go ahead": {}, "do it": {}, "ship it": {},
	"approved": {}, "confirmed": {}, "correct": {}, "looks good": {},
}

func isAcknowledgement(s string) bool {
	_, ok := acknowledgementWords[strings.ToLower(strings.Trim(s, " \t\n."))]
	return ok
}

func buildConversationTitle(customTitle, aiTitle, lastPrompt, firstUser, sessionID string) string {
	for _, candidate := range []string{customTitle, aiTitle, lastPrompt, firstUser} {
		t := stripLeadingSlashToken(candidate)
		if t == "" || isAcknowledgement(t) {
			continue
		}
		return t
	}
	return sessionID
}

// stripLeadingSlashToken strips a leading /command token so that
// "/grill generate a logo" → "generate a logo" and "/clear" → "".
func stripLeadingSlashToken(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "/") {
		rest := s[1:]
		idx := strings.IndexAny(rest, " \t\n")
		if idx < 0 {
			return ""
		}
		s = strings.TrimSpace(rest[idx:])
	}
	return s
}
