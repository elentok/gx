package webauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// fileTokenTTL is how long a hosted-file token serves its folder.
const fileTokenTTL = time.Hour

// FileToken is what a verified hosted-file token carries.
type FileToken struct {
	Dir     string // the folder of the HTML file the token was made for
	LoginID string // the web login it was made for
}

// MintFileToken makes an opaque token for dir, bound to a web login. The key
// is made when the gate is, so a restart ends every token.
func (g *Gate) MintFileToken(dir, loginID string) string {
	exp := g.now().Add(fileTokenTTL).Unix()
	payload := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(exp, 10) + "\n" + loginID + "\n" + dir))
	return payload + "." + g.signFileToken(payload)
}

// VerifyFileToken checks the signature and the expiry. It does not check that
// the login is still live; LoginLive does.
func (g *Gate) VerifyFileToken(token string) (FileToken, bool) {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(g.signFileToken(payload))) {
		return FileToken{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return FileToken{}, false
	}
	parts := strings.SplitN(string(raw), "\n", 3)
	if len(parts) != 3 {
		return FileToken{}, false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || !g.now().Before(time.Unix(exp, 0)) {
		return FileToken{}, false
	}
	return FileToken{Dir: parts[2], LoginID: parts[1]}, true
}

// LoginLive reports whether a web login id is still in the login store.
func (g *Gate) LoginLive(id string) bool {
	for _, l := range g.List() {
		if l.ID == id {
			return true
		}
	}
	return false
}

func (g *Gate) signFileToken(payload string) string {
	mac := hmac.New(sha256.New, g.fileKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func newFileKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("webauth: no randomness: " + err.Error())
	}
	return key
}
