package bgtx

// CardPage is one subagent still running and one finished, then the assistant
// text, if any. The running one is a background launch because a text that ends
// the turn also ends a foreground call.
func CardPage(text string) func(b *Lines) {
	return func(b *Lines) {
		b.subBackground(30 * minute)
		b.subFinished(5 * minute)
		if text != "" {
			b.add(sec, map[string]any{"type": "assistant", "message": map[string]any{"id": "msg_card_page", "role": "assistant",
				"content": []any{map[string]any{"type": "text", "text": text}}}})
		}
	}
}
