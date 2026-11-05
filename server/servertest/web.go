package servertest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/webauth"
)

// LoginPassword is the password SignIn sets on the account.
const LoginPassword = "correct horse battery"

var webHTTP = WebHTTPClient()

// WebClient is the one client every test uses to reach the web listener.
func WebClient() *http.Client { return webHTTP }

// StartWeb starts a harness with a web listener and returns it with its web URL.
func StartWeb(t *testing.T) (*Harness, string) {
	t.Helper()
	h := StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hs.WebURL == "" {
		t.Fatalf("handshake has no web_url: %+v", hs)
	}
	return h, hs.WebURL
}

// WebBase returns the scheme and host of a web URL, for example "http://127.0.0.1:8080".
func WebBase(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u.Scheme + "://" + u.Host
}

type loginEntry struct {
	once   sync.Once
	cookie *http.Cookie
}

// logins holds one signed-in cookie per harness: setup runs once, because a
// second sign-in in the same TOTP step would be refused as a replay.
var logins sync.Map

// Login signs the harness in through the real setup routes and returns the cookie.
func Login(t *testing.T, stateDir, base string) *http.Cookie {
	t.Helper()
	e, _ := logins.LoadOrStore(stateDir+"|"+WebBase(base), &loginEntry{})
	entry := e.(*loginEntry)
	entry.once.Do(func() {
		h := &Harness{Client: apiclient.New(server.SocketPath(stateDir))}
		entry.cookie, _ = SignIn(t, h, WebBase(base), time.Now)
	})
	if entry.cookie == nil {
		t.Fatal("sign-in failed earlier in this harness")
	}
	return entry.cookie
}

// WebSend sends an authenticated request with the given headers and an empty
// JSON body, and returns the status code.
func WebSend(t *testing.T, h *Harness, method, url string, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(Login(t, h.StateDir, WebBase(url)))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// SignIn sets the account up through the real routes and returns the cookie and
// the TOTP secret. now is the clock the server runs on.
func SignIn(t *testing.T, h *Harness, base string, now func() time.Time) (*http.Cookie, string) {
	t.Helper()
	info, err := h.Client.Web.Info(context.Background())
	if err != nil || !info.SetupPending || info.SetupCode == "" {
		t.Fatalf("web info %+v, %v; want setup pending with a code", info, err)
	}
	_, body := WebPostJSON(t, base+"/v1/web/auth/setup-start", `{"code":"`+info.SetupCode+`"}`, nil, "", JSONPost(base))
	var start struct{ Secret string }
	if err := json.Unmarshal([]byte(body), &start); err != nil || start.Secret == "" {
		t.Fatalf("setup-start body %q: %v", body, err)
	}
	code, err := webauth.TOTPCode(start.Secret, now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	resp, body := webPost(t, base+"/v1/web/auth/setup-finish",
		`{"code":"`+info.SetupCode+`","password":"`+LoginPassword+`","totp_code":"`+code+`"}`, nil, "", JSONPost(base))
	if ReasonOf(t, body) != "" || len(resp.Cookies()) != 1 {
		t.Fatalf("setup-finish body %q, %d cookies", body, len(resp.Cookies()))
	}
	return resp.Cookies()[0], start.Secret
}

// WebDoCookie sends a request with an optional cookie.
func WebDoCookie(t *testing.T, method, url string, cookie *http.Cookie) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// WebDo sends an authenticated request.
func WebDo(t *testing.T, h *Harness, method, url string) (int, string) {
	t.Helper()
	return WebDoCookie(t, method, url, Login(t, h.StateDir, WebBase(url)))
}

// WebPostJSON sends a POST with a JSON body. A nil cookie sends none; host, when
// set, replaces the Host header. It returns the status and the body.
func WebPostJSON(t *testing.T, url, body string, cookie *http.Cookie, host string, headers map[string]string) (int, string) {
	t.Helper()
	resp, out := webPost(t, url, body, cookie, host, headers)
	return resp.StatusCode, out
}

func webPost(t *testing.T, url, body string, cookie *http.Cookie, host string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if host != "" {
		req.Host = host
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, string(out)
}

// JSONPost returns the headers a page sends with a JSON write.
func JSONPost(base string) map[string]string {
	return map[string]string{"Origin": base, "Content-Type": "application/json"}
}

// ReasonOf returns the refusal reason in a response body.
func ReasonOf(t *testing.T, body string) string {
	t.Helper()
	var res struct {
		Refused bool   `json:"refused"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return res.Reason
}
