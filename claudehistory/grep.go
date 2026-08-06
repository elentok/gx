package claudehistory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// ErrRgNotFound is returned when rg (ripgrep) is not in PATH.
var ErrRgNotFound = errors.New("ripgrep (rg) not found in PATH — brew install ripgrep")

// GrepResult is one rg match decoded to a human-readable form.
type GrepResult struct {
	FilePath  string // path to the .jsonl file
	LineNum   int    // 1-indexed line in the file
	SessionID string // session ID (from ConversationMeta)
	ConvTitle string // conversation title (from ConversationMeta)
	Snippet   string // decoded single-line snippet centered on the match
	SnippetHL []int  // rune indices in Snippet for highlighting
	Preview   string // full decoded message text for the preview pane
}

// GrepTranscripts runs rg (case-insensitive) for query across dirs, decodes
// each match, and returns results sorted newest-conversation-first.
// Returns ErrRgNotFound if rg is not in PATH.
func GrepTranscripts(query string, dirs []string) ([]GrepResult, error) {
	if _, err := exec.LookPath("rg"); err != nil {
		return nil, ErrRgNotFound
	}

	args := append([]string{"--json", "-i", "-e", query}, dirs...)
	out, err := exec.Command("rg", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil // no matches
		}
		return nil, err
	}

	type rawResult struct {
		filePath string
		lineNum  int
		rawLine  string
	}

	var rawResults []rawResult
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var rec struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
			} `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.Type != "match" {
			continue
		}
		rawResults = append(rawResults, rawResult{
			filePath: rec.Data.Path.Text,
			lineNum:  rec.Data.LineNumber,
			rawLine:  rec.Data.Lines.Text,
		})
	}

	if len(rawResults) == 0 {
		return nil, nil
	}

	// Fetch ConversationMeta for unique files in parallel.
	unique := map[string]struct{}{}
	for _, r := range rawResults {
		unique[r.filePath] = struct{}{}
	}
	convCache := make(map[string]Conversation, len(unique))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for fp := range unique {
		wg.Add(1)
		go func(fp string) {
			defer wg.Done()
			c, err := ConversationMeta(fp)
			mu.Lock()
			if err == nil {
				convCache[fp] = c
			}
			mu.Unlock()
		}(fp)
	}
	wg.Wait()

	var results []GrepResult
	for _, r := range rawResults {
		snippet, hl, preview := GrepDecodeMatch([]byte(r.rawLine), query)
		if snippet == "" {
			continue
		}
		results = append(results, GrepResult{
			FilePath:  r.filePath,
			LineNum:   r.lineNum,
			SessionID: convCache[r.filePath].SessionID,
			ConvTitle: convCache[r.filePath].Title,
			Snippet:   snippet,
			SnippetHL: hl,
			Preview:   preview,
		})
	}

	// Sort newest conversation first.
	sort.SliceStable(results, func(i, j int) bool {
		ti := convCache[results[i].FilePath].LastAccessed
		tj := convCache[results[j].FilePath].LastAccessed
		return ti.After(tj)
	})

	return results, nil
}

// GrepDecodeMatch decodes a raw JSONL transcript line and returns a
// human-readable snippet centered on query, with highlight ranges, and the
// full decoded message text for the preview pane. Returns empty strings if
// the line has no searchable text content.
func GrepDecodeMatch(rawLine []byte, query string) (snippet string, highlights []int, preview string) {
	rawLine = bytes.TrimSpace(rawLine)
	if len(rawLine) == 0 {
		return "", nil, ""
	}

	var rec transcriptLine
	if err := json.Unmarshal(rawLine, &rec); err != nil {
		return "", nil, ""
	}

	var text string
	switch rec.Type {
	case "user":
		text = extractUserMessageText(rec.RawMessage)
	case "assistant":
		text = extractAssistantText(rec.RawMessage)
	case "ai-title":
		text = strings.TrimSpace(rec.AITitle)
	case "last-prompt":
		text = strings.TrimSpace(rec.LastPrompt)
	default:
		return "", nil, ""
	}

	if text == "" {
		return "", nil, ""
	}

	preview = text
	// Normalize whitespace for the single-line snippet (collapse newlines/tabs to spaces).
	snippetText := strings.Join(strings.Fields(text), " ")
	snippet, highlights = centerSnippet(snippetText, query)
	return snippet, highlights, preview
}

// centerSnippet finds query (case-insensitive) in text, returns a snippet
// centered on the match with at most ~120 visible runes, plus highlight ranges.
func centerSnippet(text, query string) (snippet string, highlights []int) {
	runes := []rune(text)

	lower := strings.ToLower(text)
	lowerQ := strings.ToLower(query)
	byteIdx := strings.Index(lower, lowerQ)

	const windowBefore = 40
	const maxLen = 120

	if byteIdx < 0 {
		// Query not found in decoded text (matched inside JSON encoding).
		if len(runes) > maxLen {
			return string(runes[:maxLen]) + "…", nil
		}
		return text, nil
	}

	matchRune := utf8.RuneCountInString(text[:byteIdx])
	queryRunes := utf8.RuneCountInString(query)

	start := 0
	prefix := ""
	if matchRune > windowBefore {
		start = matchRune - windowBefore
		prefix = "…"
	}

	end := min(len(runes), start+maxLen)
	snippet = prefix + string(runes[start:end])
	if end < len(runes) {
		snippet += "…"
	}

	// Highlight range relative to snippet (accounting for ellipsis prefix rune).
	prefixRunes := len([]rune(prefix))
	snipMatch := prefixRunes + matchRune - start
	for i := range queryRunes {
		highlights = append(highlights, snipMatch+i)
	}
	return snippet, highlights
}

// extractAssistantText extracts the first text block from an assistant message.
func extractAssistantText(rawMsg json.RawMessage) string {
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
		return strings.TrimSpace(s)
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			return strings.TrimSpace(b.Text)
		}
	}
	return ""
}
