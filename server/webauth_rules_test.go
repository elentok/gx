package server

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B SHA-1 secret "12345678901234567890", base32.
const rfc6238Secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestWebAuthRulesTOTPVectors(t *testing.T) {
	// The RFC lists 8 digits; ours are the last 6 of those.
	for _, tc := range []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	} {
		got, err := TOTPCode(rfc6238Secret, tc.unix/30)
		if err != nil || got != tc.want {
			t.Errorf("t=%d: got %q, %v; want %q", tc.unix, got, err, tc.want)
		}
	}
}

func TestWebAuthRulesTOTPSkewAndReplay(t *testing.T) {
	now := time.Unix(1111111111, 0)
	step := totpStepAt(now)
	codeAt := func(s int64) string {
		c, err := TOTPCode(rfc6238Secret, s)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	for _, tc := range []struct {
		name   string
		step   int64
		last   int64
		wantOK bool
	}{
		{"current", step, 0, true},
		{"one back", step - 1, 0, true},
		{"one ahead", step + 1, 0, true},
		{"two back", step - 2, 0, false},
		{"two ahead", step + 2, 0, false},
		{"replay of the last step", step, step, false},
		{"older than the last step", step - 1, step, false},
		{"newer than the last step", step + 1, step, true},
	} {
		got, ok := verifyTOTP(rfc6238Secret, codeAt(tc.step), now, tc.last)
		if ok != tc.wantOK || (ok && got != tc.step) {
			t.Errorf("%s: got step %d ok=%v; want step %d ok=%v", tc.name, got, ok, tc.step, tc.wantOK)
		}
	}
	if _, ok := verifyTOTP(rfc6238Secret, "000000x", now, 0); ok {
		t.Error("garbage code accepted")
	}
}

func TestWebAuthRulesTOTPSecretAndURI(t *testing.T) {
	secret, err := newTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TOTPCode(secret, 1); err != nil {
		t.Fatalf("new secret is not usable: %v", err)
	}
	u, err := url.Parse(totpURI(secret, "me@host"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "otpauth" || u.Host != "totp" {
		t.Errorf("uri = %s", u)
	}
	q := u.Query()
	if q.Get("secret") != secret || q.Get("digits") != "6" || q.Get("period") != "30" || q.Get("issuer") == "" {
		t.Errorf("query = %v", q)
	}
}

func TestWebAuthRulesSetupCode(t *testing.T) {
	for i := 0; i < 200; i++ {
		code, err := newSetupCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 8 || strings.ContainsAny(code, "0O1IL") {
			t.Fatalf("bad code %q", code)
		}
	}
	if strings.ContainsAny(setupCodeAlphabet, "0O1IL") {
		t.Error("alphabet has look-alikes")
	}
	if got := formatSetupCode("K7QM3XPD"); got != "K7QM-3XPD" {
		t.Errorf("format = %q", got)
	}
	for _, in := range []string{"K7QM-3XPD", "k7qm3xpd", " k7qm-3xpd "} {
		if got, ok := parseSetupCode(in); !ok || got != "K7QM3XPD" {
			t.Errorf("parse(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "K7QM-3XP", "K7QM-3XPDD", "K7QM-3XP0", "K7QM-3XPL"} {
		if _, ok := parseSetupCode(in); ok {
			t.Errorf("parse(%q) accepted", in)
		}
	}
	made := time.Unix(1000, 0)
	if setupCodeExpired(made, made.Add(59*time.Minute)) {
		t.Error("59 minutes old is expired")
	}
	if !setupCodeExpired(made, made.Add(time.Hour)) {
		t.Error("an hour old is not expired")
	}
}

func TestWebAuthRulesDeviceName(t *testing.T) {
	for _, tc := range []struct{ ua, want string }{
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1", "Safari on iPhone"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0 Mobile/15E148 Safari/604.1", "Chrome on iPhone"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", "Chrome on Mac"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "Safari on Mac"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0", "Edge on Windows"},
		{"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0", "Firefox on Linux"},
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36", "Chrome on Android"},
		{"curl/8.0", "Unknown device"},
		{"", "Unknown device"},
	} {
		if got := deviceName(tc.ua); got != tc.want {
			t.Errorf("deviceName(%q) = %q, want %q", tc.ua, got, tc.want)
		}
	}
	if got := deviceName("Windows " + strings.Repeat("x", 100)); len(got) > deviceNameMaxLen {
		t.Errorf("name too long: %q", got)
	}
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func failN(l *authLimiter, ip string, n int) {
	for i := 0; i < n; i++ {
		l.Fail(ip)
	}
}

func TestWebAuthRulesLimiterPerIP(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newAuthLimiter(clk.now)

	failN(l, "1.1.1.1", 4)
	if w := l.Wait("1.1.1.1"); w != 0 {
		t.Fatalf("after 4 failures wait = %d", w)
	}
	l.Fail("1.1.1.1")
	if w := l.Wait("1.1.1.1"); w != 60 {
		t.Fatalf("after 5 failures wait = %d, want 60", w)
	}
	if w := l.Wait("2.2.2.2"); w != 0 {
		t.Errorf("other IP wait = %d", w)
	}
	clk.advance(30 * time.Second)
	if w := l.Wait("1.1.1.1"); w != 30 {
		t.Errorf("wait after 30s = %d, want 30", w)
	}
	clk.advance(30 * time.Second)
	if w := l.Wait("1.1.1.1"); w != 0 {
		t.Fatalf("wait after a minute = %d", w)
	}

	failN(l, "1.1.1.1", 5)
	if w := l.Wait("1.1.1.1"); w != 120 {
		t.Fatalf("second round wait = %d, want 120", w)
	}

	// Rounds keep doubling and stop at an hour.
	for round := 3; round <= 10; round++ {
		clk.advance(2 * time.Hour)
		failN(l, "1.1.1.1", 5)
		want := min(60<<(round-1), 3600)
		if w := l.Wait("1.1.1.1"); w != want {
			t.Fatalf("round %d wait = %d, want %d", round, w, want)
		}
	}

	l.Success("1.1.1.1")
	if w := l.Wait("1.1.1.1"); w != 0 {
		t.Errorf("wait after success = %d", w)
	}
	failN(l, "1.1.1.1", 5)
	if w := l.Wait("1.1.1.1"); w != 60 {
		t.Errorf("first round after success = %d, want 60", w)
	}
}

func TestWebAuthRulesLimiterGlobal(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newAuthLimiter(clk.now)

	for i := 0; i < 49; i++ {
		l.Fail(fmt.Sprintf("10.0.0.%d", i))
	}
	if w := l.Wait("10.9.9.9"); w != 0 {
		t.Fatalf("wait after 49 failures = %d", w)
	}
	l.Fail("10.0.0.49")
	for _, ip := range []string{"10.0.0.0", "10.9.9.9"} {
		if w := l.Wait(ip); w != 900 {
			t.Errorf("%s wait = %d, want 900", ip, w)
		}
	}
	clk.advance(15 * time.Minute)
	if w := l.Wait("10.9.9.9"); w != 0 {
		t.Errorf("wait after lockout = %d", w)
	}

	failN(l, "3.3.3.3", 5)
	l.Reset()
	if w := l.Wait("3.3.3.3"); w != 0 {
		t.Errorf("wait after reset = %d", w)
	}
}

func TestWebAuthRulesLimiterGlobalWindow(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newAuthLimiter(clk.now)
	for i := 0; i < 49; i++ {
		l.Fail(fmt.Sprintf("10.0.0.%d", i))
	}
	clk.advance(61 * time.Minute)
	l.Fail("10.0.1.1")
	if w := l.Wait("10.9.9.9"); w != 0 {
		t.Errorf("failures older than an hour counted: wait = %d", w)
	}
}
