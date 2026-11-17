package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// runProcCap runs scripts/proc-cap with a ps stub that reports 10 processes, so
// the cap is 10 plus the headroom. headroom "" leaves GX_PROC_HEADROOM unset.
func runProcCap(t *testing.T, headroom string, args ...string) (string, int) {
	t.Helper()
	script, err := filepath.Abs("proc-cap")
	if err != nil {
		t.Fatal(err)
	}
	stubs := t.TempDir()
	ps := "#!/usr/bin/env bash\nseq 10\n"
	if err := os.WriteFile(filepath.Join(stubs, "ps"), []byte(ps), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(script, args...)
	cmd.Env = []string{"PATH=" + stubs + ":" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	if headroom != "" {
		cmd.Env = append(cmd.Env, "GX_PROC_HEADROOM="+headroom)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out)), code
}

func skipIfHardLimitBelow(t *testing.T, want int) {
	t.Helper()
	out, err := exec.Command("sh", "-c", "ulimit -H -u").Output()
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimSpace(string(out))
	if s == "unlimited" {
		return
	}
	hard, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	if hard < want {
		t.Skipf("hard process limit %d is below the %d this case expects", hard, want)
	}
}

func TestProcCap_DefaultHeadroomIs800(t *testing.T) {
	skipIfHardLimitBelow(t, 810)
	out, code := runProcCap(t, "", "sh", "-c", "ulimit -S -u")
	if code != 0 || out != "810" {
		t.Fatalf("soft limit = %q (exit %d), want 810", out, code)
	}
}

func TestProcCap_HeadroomCanBeOverridden(t *testing.T) {
	out, code := runProcCap(t, "5", "sh", "-c", "ulimit -S -u")
	if code != 0 || out != "15" {
		t.Fatalf("soft limit = %q (exit %d), want 15", out, code)
	}
}

func TestProcCap_HelpNamesTheDefault(t *testing.T) {
	out, code := runProcCap(t, "", "--help")
	if code != 0 || !strings.Contains(out, "800") {
		t.Fatalf("help (exit %d) does not name 800:\n%s", code, out)
	}
}
