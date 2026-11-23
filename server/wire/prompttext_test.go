package wire

import "testing"

func TestValidatePromptText_BangBehindAHiddenCharacterIsRefused(t *testing.T) {
	zwsp := "\xe2\x80\x8b"
	bom := "\xef\xbb\xbf"
	for _, text := range []string{"!x", zwsp + "!x", bom + zwsp + "!x"} {
		if _, res := ValidatePromptText(text, "text"); res == nil || res.Reason != ReasonPromptPrefixNotAllowed {
			t.Errorf("ValidatePromptText(%q) = %v, want the ! refusal", text, res)
		}
	}
}
