package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// stubRecovery stands in for a recovery command's run* function.
func stubRecovery(jsonMode bool, runErr error) (stdout, stderr string, err error) {
	var out, errBuf bytes.Buffer
	err = finishRecovery(&out, &errBuf, jsonMode, map[string]string{"outcome": "landed"}, "landed ok", runErr)
	return out.String(), errBuf.String(), err
}

func TestFinishRecovery_SuccessJSON(t *testing.T) {
	out, _, err := stubRecovery(true, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if out != "{\"actor\":\"recovery\",\"outcome\":\"landed\",\"via\":\"direct\"}\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestFinishRecovery_SuccessHuman(t *testing.T) {
	out, _, err := stubRecovery(false, nil)
	if err != nil || out != "landed ok\n" {
		t.Errorf("out=%q err=%v", out, err)
	}
}

func TestFinishRecovery_RefusalJSON(t *testing.T) {
	out, errOut, err := stubRecovery(true, &RefusalError{Reason: ReasonLandLocked, Message: "locked"})
	assertExit1(t, err)
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	var env map[string]any
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatal(jerr)
	}
	want := map[string]any{"refused": true, "reason": "land_locked", "message": "locked", "via": "direct", "actor": "recovery"}
	if fmt.Sprint(env) != fmt.Sprint(want) {
		t.Errorf("envelope = %v, want %v", env, want)
	}
}

func TestFinishRecovery_PlainErrorUsesFallbackReason(t *testing.T) {
	out, _, err := stubRecovery(true, errors.New("boom"))
	assertExit1(t, err)
	var env RefusalEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatal(jerr)
	}
	if !env.Refused || env.Reason != ReasonError || env.Message != "boom" {
		t.Errorf("envelope = %+v", env)
	}
}

func TestFinishRecovery_RefusalHuman(t *testing.T) {
	out, errOut, err := stubRecovery(false, &RefusalError{Reason: ReasonForkChildren, Message: "has fork children"})
	assertExit1(t, err)
	if out != "" || errOut != "has fork children\n" {
		t.Errorf("stdout=%q stderr=%q", out, errOut)
	}
}

func assertExit1(t *testing.T, err error) {
	t.Helper()
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("err = %v, want ExitError{1}", err)
	}
}
