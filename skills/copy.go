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

// identityIfExists returns path's on-disk identity under mode: its symlink
// target under ModeSymlink, or its content hash under ModeManagedCopy.
// Either way, "" means nothing exists there, matching PathHashes' "empty
// means absent" convention.
func identityIfExists(path string, mode InstallMode) (string, error) {
	if mode != ModeSymlink {
		return hashIfExists(path)
	}
	target, err := os.Readlink(path)
	if err == nil {
		return target, nil
	}
	if os.IsNotExist(err) {
		return "", nil
	}
	if _, statErr := os.Lstat(path); statErr == nil {
		// Exists but isn't a symlink: occupied by unrelated content. The NUL
		// prefix can never equal a real symlink target, so this never
		// spuriously matches a recorded LinkTarget.
		return "\x00non-symlink", nil
	}
	return "", nil
}

// managedIdentity returns f's recorded identity: its LinkTarget under
// symlink mode, its Hash under managed-copy mode - whichever is set.
func managedIdentity(f ManagedFile) string {
	if f.LinkTarget != "" {
		return f.LinkTarget
	}
	return f.Hash
}

// sourceFileIdentity returns f's identity as it would be recorded were it
// installed under mode: its own absolute path under ModeSymlink (a
// symlink's content *is* its target), its content hash under
// ModeManagedCopy.
func sourceFileIdentity(f SourceFile, mode InstallMode) (string, error) {
	if mode == ModeSymlink {
		return f.AbsPath, nil
	}
	return hashFile(f.AbsPath)
}

// placeFile creates dstPath as a copy or a symlink of srcPath, depending on
// mode, replacing whatever - file or symlink - previously occupied
// dstPath. Callers must have already authorized the overwrite via an
// ownership/force check; placeFile itself doesn't check.
func placeFile(srcPath, dstPath string, mode InstallMode) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if mode == ModeSymlink {
		return os.Symlink(srcPath, dstPath)
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, data, 0644)
}
