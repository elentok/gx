package server

import (
	"crypto/rand"
	"math/big"
	"strings"
	"time"
)

const (
	// setupCodeAlphabet leaves out 0 O 1 I L, which look alike.
	setupCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	setupCodeLength   = 8
	setupCodeLifetime = time.Hour
)

// newSetupCode returns a random code, undashed and uppercase.
func newSetupCode() (string, error) {
	max := big.NewInt(int64(len(setupCodeAlphabet)))
	var b strings.Builder
	for i := 0; i < setupCodeLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(setupCodeAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// formatSetupCode shows a code as K7QM-3XPD.
func formatSetupCode(code string) string {
	return code[:setupCodeLength/2] + "-" + code[setupCodeLength/2:]
}

// parseSetupCode accepts a code with or without the dash, in any case.
func parseSetupCode(s string) (string, bool) {
	code := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", ""))
	if len(code) != setupCodeLength {
		return "", false
	}
	for _, r := range code {
		if !strings.ContainsRune(setupCodeAlphabet, r) {
			return "", false
		}
	}
	return code, true
}

func setupCodeExpired(createdAt, now time.Time) bool {
	return !now.Before(createdAt.Add(setupCodeLifetime))
}
