package tickets

import "testing"

func TestParseRenderedStatus_RoundTripsWord(t *testing.T) {
	for s := StatusOpen; s <= StatusCancelled; s++ {
		got, ok := ParseRenderedStatus(s.Word())
		if !ok || got != s {
			t.Errorf("ParseRenderedStatus(%q) = %v, %v; want %v, true", s.Word(), got, ok, s)
		}
	}
	if _, ok := ParseRenderedStatus("nonsense"); ok {
		t.Error("ParseRenderedStatus(nonsense) reported ok")
	}
}
