package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/chatcapture"
)

func TestChatCaptureScenario_ANotifyPostLandsInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-capture")
	if r.ChatCapture == "" {
		t.Fatalf("output = %+v, want chat_capture", r.Output)
	}
	cmd := exec.Command(gxBin, "notify", "hello")
	cmd.Env = r.Env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gx notify: %v\n%s", err, out)
	}

	var got []chatcapture.Line
	if !testutil.Poll(10*time.Second, 100*time.Millisecond, func() bool {
		data, _ := os.ReadFile(r.ChatCapture)
		got = nil
		for l := range strings.FieldsSeq(string(data)) {
			var line chatcapture.Line
			if json.Unmarshal([]byte(l), &line) == nil && line.Transport == "slack" {
				got = append(got, line)
			}
		}
		return len(got) > 0
	}) {
		t.Fatal("no line in the capture file")
	}
	if len(got) != 1 || got[0].Transport != "slack" || got[0].Text != "hello" {
		t.Fatalf("capture = %+v, want one slack line with text hello", got)
	}
}
