package files

import (
	"path/filepath"
	"strings"
)

var (
	deniedNames = map[string]bool{
		".git": true, ".ssh": true, ".aws": true, ".npmrc": true, ".netrc": true,
		".git-credentials": true, ".gnupg": true, ".kube": true, ".docker": true, ".pgpass": true,
	}
	deniedSuffixes = []string{".pem", ".key", ".p12", ".pfx"}
	privateKeys    = []string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa"}
)

// denied reports whether any segment of path is on the deny list.
func denied(path string) bool {
	for seg := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if deniedSegment(strings.ToLower(seg)) {
			return true
		}
	}
	return false
}

func deniedSegment(s string) bool {
	if deniedNames[s] || strings.HasPrefix(s, ".env") {
		return true
	}
	for _, suf := range deniedSuffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	if strings.HasSuffix(s, ".pub") {
		return false
	}
	for _, k := range privateKeys {
		if strings.HasPrefix(s, k) {
			return true
		}
	}
	return false
}
