package chatdrafts

import (
	"errors"
	"os"
	"path/filepath"
)

// Test hook names: a file of this name in the hook dir arms the hook.
const (
	HookSaveFail  = "save-fail"
	HookStartFail = "start-fail"
)

// ErrHookSaveFail is the error a draft create answers while save-fail is armed.
var ErrHookSaveFail = errors.New("test hook: draft save forced to fail")

// ErrHookStartFail is the error a Start answers while start-fail is armed. It
// only fires for worktree-folder drafts: Start checks it after the worktree
// step, so source and trunk drafts start normally.
var ErrHookStartFail = errors.New("test hook: start forced to fail")

// HookArmed reports whether the file name exists in dir. An empty dir is never
// armed, so the hooks stay off unless GX_FORK_HOOK_DIR is set.
func HookArmed(dir, name string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}
