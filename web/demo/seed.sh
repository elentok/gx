#!/usr/bin/env bash

# seed.sh — build a deterministic, throwaway git repo for the gx demos.
#
# Produces, under $TARGET_DIR (default web/demo/.work):
#
#   upstream.git/   – bare "remote" the worktrees track
#   upstream-work/  – scratch clone used to author the upstream history
#   worktrees/      – bare clone (.bare layout) with the linked worktrees gx shows
#   xdg/            – an XDG_CONFIG_HOME with a gx config (nerd fonts + image diffs on)
#
# Open the demos against it with:
#
#   cd web/demo/.work/worktrees && XDG_CONFIG_HOME=$PWD/../xdg gx
#
# Worktree states (what the worktrees table demonstrates):
#   main          synced        (0 ahead, 0 behind)  + 1 stash
#   feature-auth  diverged      (1 ahead, 1 behind)  + unstaged change
#   feature-api   behind        (0 ahead, 2 behind)
#   feature-ui    ahead         (2 ahead, 0 behind)  + untracked file + CHANGED IMAGE
#   bugfix-login  synced        (0 ahead, 0 behind)
#   refactor-db   synced        (0 ahead, 0 behind)  + staged + untracked files
#   chore-cleanup no tracking   (remote branch exists, local tracking not configured)
#
# The script is deterministic (pinned identity + commit dates) and idempotent
# (it wipes and rebuilds $TARGET_DIR on every run).

set -euo pipefail

# ── deterministic environment ────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET_DIR="${1:-$SCRIPT_DIR/.work}"
# Resolve to an absolute path so the `cd`s below are stable.
mkdir -p "$TARGET_DIR"
TARGET_DIR="$(cd "$TARGET_DIR" && pwd)"

export GIT_AUTHOR_NAME="Demo Author"
export GIT_AUTHOR_EMAIL="demo@example.com"
export GIT_COMMITTER_NAME="Demo Author"
export GIT_COMMITTER_EMAIL="demo@example.com"

# Pinned, monotonically increasing commit clock → reproducible history.
COMMIT_EPOCH=1700000000
COMMIT_STEP=0
function bump-clock() {
  COMMIT_STEP=$((COMMIT_STEP + 3600))
  local when=$((COMMIT_EPOCH + COMMIT_STEP))
  export GIT_AUTHOR_DATE="$when +0000"
  export GIT_COMMITTER_DATE="$when +0000"
}

function main() {
  rm -rf "$TARGET_DIR"
  mkdir -p "$TARGET_DIR"

  create-upstream
  create-worktrees-repo
  write-demo-config

  echo ""
  echo "Done! Demo repo created in $TARGET_DIR"
  echo ""
  echo "  cd $TARGET_DIR/worktrees && XDG_CONFIG_HOME=$TARGET_DIR/xdg gx"
}

# ── upstream ──────────────────────────────────────────────────────────────────

function create-upstream() {
  cd "$TARGET_DIR"
  git init -q --bare upstream.git

  git clone -q upstream.git upstream-work
  cd upstream-work
  git symbolic-ref HEAD refs/heads/main

  # ── main: project skeleton ────────────────────────────────────────────────
  mkdir -p src tests assets

  write-file README.md "# MyProject

A sample Go web service."
  write-file src/main.go "package main

func main() {
	startServer()
}"
  make-image assets/banner.png "#1e1e2e" "#cba6f7" "MyProject"
  git add assets/banner.png
  commit "Initial project setup"

  write-file src/server.go "package main

func startServer() {}
func stopServer()  {}"
  write-file src/config.go "package main

type Config struct {
	Port int
	Host string
}"
  commit "Add server and config"

  write-file tests/server_test.go "package main

import \"testing\"

func TestServer(t *testing.T) {}
func TestConfig(t *testing.T) {}"
  commit "Add server tests"

  git push -q origin main

  # ── feature-auth: auth module (branches from here) ────────────────────────
  git checkout -q -b feature-auth
  write-file src/auth.go "package main

func login(user, pass string) bool { return false }
func logout(token string)          {}"
  commit "Add auth module"
  write-file tests/auth_test.go "package main

import \"testing\"

func TestLogin(t *testing.T)  {}
func TestLogout(t *testing.T) {}"
  commit "Add auth tests"
  git push -q origin feature-auth
  git tag feature-auth-v1  # pin this state for the worktree

  # ── feature-api: API handler (branches from main) ─────────────────────────
  git checkout -q main
  git checkout -q -b feature-api
  write-file src/api.go "package main

func handleRequest(path string) {}
func handleError(err error)     {}"
  commit "Add API handler"
  git push -q origin feature-api
  git tag feature-api-v1  # pin this state for the worktree

  # ── feature-ui: UI renderer (branches from main) ──────────────────────────
  git checkout -q main
  git checkout -q -b feature-ui
  write-file src/ui.go "package main

func render(template string) string { return \"\" }
func layout(content string) string  { return content }"
  commit "Add UI renderer"
  git push -q origin feature-ui

  # ── bugfix-login: starts from main ────────────────────────────────────────
  git checkout -q main
  git checkout -q -b bugfix-login
  git push -q origin bugfix-login

  # ── main: advance two commits (makes feature-auth + feature-api "behind") ─
  git checkout -q main
  write-file src/logger.go "package main

import \"log\"

func logInfo(msg string)  { log.Println(\"INFO:\", msg) }
func logError(msg string) { log.Println(\"ERROR:\", msg) }"
  commit "Add structured logging"
  write-file src/metrics.go "package main

func recordMetric(name string, value float64) {}
func flushMetrics()                           {}"
  commit "Add metrics collection"
  git push -q origin main

  # ── feature-api: push 2 more upstream commits (worktree will be "behind") ─
  git checkout -q feature-api
  write-file src/api_v2.go "package main

func handleRequestV2(path string, version int) {}"
  commit "Add v2 API handler"
  write-file src/middleware.go "package main

func withLogging(next func()) func()  { return next }
func withAuth(next func()) func()     { return next }"
  commit "Add request middleware"
  git push -q origin feature-api

  # ── feature-auth: push 1 more upstream commit (worktree will "diverge") ───
  git checkout -q feature-auth
  write-file src/oauth.go "package main

func oauthLogin(token string) bool  { return false }
func oauthLogout(token string)      {}"
  commit "Add OAuth login flow"
  git push -q origin feature-auth

  # ── bugfix-login: merge main, add the fix ────────────────────────────────
  git checkout -q bugfix-login
  git merge -q --no-ff main -m "Merge main into bugfix-login"
  write-file src/auth.go "package main

import \"time\"

const loginTimeout = 30 * time.Second

func login(user, pass string) bool {
	// fixed: respect timeout
	return false
}
func logout(token string) {}"
  commit "Fix login timeout (#42)"
  git push -q origin bugfix-login

  # ── refactor-db: starts from main, one commit ─────────────────────────────
  git checkout -q main
  git checkout -q -b refactor-db
  write-file src/db.go "package main

func openDB(dsn string) error  { return nil }
func closeDB()                 {}
func queryDB(sql string) error { return nil }"
  commit "Add database layer"
  git push -q origin refactor-db

  # ── chore-cleanup: exists on remote but worktree won't track it ───────────
  git checkout -q main
  git checkout -q -b chore-cleanup
  write-file src/cleanup.go "package main

func removeDeprecated() {}
func archiveLogs()      {}"
  commit "Start cleanup work"
  git push -q origin chore-cleanup

  git checkout -q main
  git push -q origin --tags  # push all tags to upstream
}

# ── worktrees repo (.bare layout) ─────────────────────────────────────────────

function create-worktrees-repo() {
  cd "$TARGET_DIR"
  mkdir worktrees
  cd worktrees

  # gx's .bare trick: a bare repo in .bare/ with a .git file pointing at it,
  # so the directory reads as a normal repo whose worktrees live alongside.
  git init -q --bare .bare
  echo "gitdir: ./.bare" > .git
  git config remote.origin.url "$TARGET_DIR/upstream.git"
  git config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
  git remote update             # fetch all branches into refs/remotes/origin/*
  git fetch -q origin --tags    # fetch tags (not included in remote update)

  # main — synced
  git worktree add -q -b main main origin/main

  # feature-ui — 2 ahead after local commits, untracked WIP, and a CHANGED IMAGE
  git worktree add -q -b feature-ui feature-ui origin/feature-ui
  cd feature-ui
  write-file styles.go "package main

const (
	colorPrimary   = \"#3498db\"
	colorSecondary = \"#2ecc71\"
)"
  commit "Add theme colours"
  write-file animations.go "package main

func fadeIn(duration int)  {}
func fadeOut(duration int) {}
func slideIn()             {}"
  commit "Add fade and slide animations"
  # Untracked WIP file
  printf 'package main\n\n// TODO: bezier curve helpers\n' > curves.go
  # Unstaged image change → drives the inline image-diff demo.
  make-image assets/banner.png "#1e1e2e" "#a6e3a1" "MyProject UI"
  cd ..

  # feature-auth — pinned to v1: 1 local commit ahead, 1 upstream behind → diverged
  git worktree add -q -b feature-auth feature-auth feature-auth-v1
  cd feature-auth
  write-file src/session.go "package main

import \"time\"

type Session struct {
	Token     string
	ExpiresAt time.Time
}"
  commit "Add session management"
  # Unstaged modification
  printf 'package main\n\n// BUG: needs constant-time comparison\nfunc login(user, pass string) bool { return false }\nfunc logout(token string)          {}\n' > src/auth.go
  cd ..

  # feature-api — pinned to v1 (2 commits behind origin/feature-api)
  git worktree add -q -b feature-api feature-api feature-api-v1

  # bugfix-login — synced
  git worktree add -q -b bugfix-login bugfix-login origin/bugfix-login

  # chore-cleanup — remote branch exists but local branch has no tracking set
  git branch --no-track chore-cleanup origin/chore-cleanup
  git worktree add -q chore-cleanup chore-cleanup

  # refactor-db — synced, with staged + untracked changes
  git worktree add -q -b refactor-db refactor-db origin/refactor-db
  cd refactor-db
  printf 'package main\n\nimport "database/sql"\n\nfunc openDB(dsn string) (*sql.DB, error) { return sql.Open("sqlite3", dsn) }\nfunc closeDB(db *sql.DB)                  { db.Close() }\nfunc queryDB(db *sql.DB, q string) error  { return nil }\n' > src/db.go
  git add src/db.go
  printf 'package main\n\n// TODO: connection pooling\nconst maxConns = 10\n' > src/db_pool.go
  cd ..

  # A stash on main (refs/stash is shared across worktrees, so the Stash tab
  # has content) — created from a throwaway tweak, then stashed.
  cd main
  printf '\n// WIP: graceful shutdown\n' >> src/main.go
  bump-clock
  git stash push -q -m "WIP: graceful shutdown"
  cd ..
}

# ── demo gx config ────────────────────────────────────────────────────────────

function write-demo-config() {
  mkdir -p "$TARGET_DIR/xdg/gx"
  cat > "$TARGET_DIR/xdg/gx/config.json" <<'JSON'
{
  "use-nerdfont-icons": true,
  "image-diffs": true,
  "stage-diff-context-lines": 1
}
JSON
}

# ── helpers ───────────────────────────────────────────────────────────────────

function write-file() {
  local path="$1"
  local content="$2"
  mkdir -p "$(dirname "$path")"
  printf "%s\n" "$content" > "$path"
  git add "$path"
}

function commit() {
  bump-clock
  git commit -q -m "$1"
}

# make-image <path> <bg-hex> <fg-hex> <label>
# Deterministic PNG via ImageMagick so the image-diff demo has real pixels.
# Draws shapes only (no text) to avoid depending on a configured font: a bg
# fill plus a centered fg band, so before/after read as a clear visual change.
function make-image() {
  local path="$1" bg="$2" fg="$3" label="$4"  # label kept for caller readability
  mkdir -p "$(dirname "$path")"
  local im
  if command -v magick >/dev/null 2>&1; then im=magick
  elif command -v convert >/dev/null 2>&1; then im=convert
  else
    echo "warning: ImageMagick not found; writing placeholder for $path" >&2
    printf 'placeholder %s\n' "$label" > "$path"
    return
  fi
  "$im" -size 480x240 "xc:$bg" \
    -fill "$fg" -draw "roundrectangle 80,80 400,160 16,16" \
    "$path"
}

main "$@"
