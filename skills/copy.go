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

// hashIfExists is hashFile, except a missing path yields an empty hash
// instead of an error - the same "empty means absent" convention
// PathHashes uses.
func hashIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// copyFile copies srcPath to dstPath, creating dstPath's parent
// directories as needed.
func copyFile(srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(dstPath, data, 0644)
}
