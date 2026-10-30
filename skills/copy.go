package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// hashFile returns the hex-encoded sha256 of the file at path.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// managedIdentity returns f's recorded identity: its LinkTarget under
// symlink mode, its Hash under managed-copy mode - whichever is set.
func managedIdentity(f ManagedFile) string {
	if f.LinkTarget != "" {
		target, err := physicalPath(f.LinkTarget)
		if err == nil {
			return target
		}
		return filepath.Clean(f.LinkTarget)
	}
	return f.Hash
}
