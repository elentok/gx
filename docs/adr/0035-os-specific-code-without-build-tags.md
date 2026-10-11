# OS-specific behavior without build tags

gx is used on macOS today and Linux support is planned. Code that differs by OS (the first case is
OS notifications) does not use `_darwin.go` / `_other.go` build tags. Each backend lives in a plain
file that compiles on every OS and builds a command (`argv`) run through an injectable runner. One
selection function picks the backend from injected `goos` and `lookPath` values. So every backend's
tests and every selection branch run on every OS: a Mac laptop catches a broken Linux backend, and a
Linux CI catches a broken macOS one.

## Considered Options

- **Build-tagged files** (`notify_darwin.go`, `notify_other.go`) — the usual Go idiom, but each
  file's tests only run on its own OS, so one side of the code is never tested where you work.

## Consequences

- macOS-only code is compiled into the Linux binary. It is small and never runs there.
- Build tags stay acceptable only when code cannot compile on another OS (a syscall or cgo API that
  does not exist there).
