package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	totpPeriod    = 30 * time.Second
	totpDigits    = 6
	totpSkewSteps = 1
	totpIssuer    = "gx"
)

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// newTOTPSecret returns a random 160-bit secret as unpadded base32.
func newTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return totpEncoding.EncodeToString(raw), nil
}

// totpStepAt is the step number (RFC 6238 T) that contains t.
func totpStepAt(t time.Time) int64 {
	return t.Unix() / int64(totpPeriod/time.Second)
}

// TOTPStepAt is totpStepAt, exported for gxfixture.
func TOTPStepAt(t time.Time) int64 { return totpStepAt(t) }

// TOTPCode computes the 6-digit code for a step. It is exported for gxfixture and tests.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := totpEncoding.DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	if err != nil {
		return "", fmt.Errorf("decode TOTP secret: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, bin%mod), nil
}

// verifyTOTP checks code against the steps around now. It returns the matched
// step, and refuses any step at or below lastStep so a code works once.
func verifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.TrimSpace(code)
	current := totpStepAt(now)
	for step := current - totpSkewSteps; step <= current+totpSkewSteps; step++ {
		if step <= lastStep {
			continue
		}
		want, err := TOTPCode(secret, step)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// totpURI is the otpauth:// URI behind the QR code and the "Add to authenticator app" link.
func totpURI(secret, account string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", totpIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(int(totpPeriod/time.Second)))
	label := url.PathEscape(totpIssuer + ":" + account)
	return "otpauth://totp/" + label + "?" + q.Encode()
}
