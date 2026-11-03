#!/usr/bin/env bash

# Creates two repos under ~/tmp/git-helpers-test:
#
#   upstream.git/   – bare "server" repo
#   upstream-work/  – normal clone used to author commits
#   worktrees/      – bare clone of upstream.git with linked worktrees
#
# Worktree states after setup:
#   main          synced        (0 ahead, 0 behind)
#   feature-auth  diverged      (1 ahead, 1 behind) + modified file
#   feature-api   behind        (0 ahead, 2 behind)
#   feature-ui    ahead         (2 ahead, 0 behind) + untracked file
#   bugfix-login  synced        (0 ahead, 0 behind)
#   refactor-db   synced        (0 ahead, 0 behind) + staged + untracked files
#   chore-cleanup no tracking   (remote exists but local tracking not configured)

set -euo pipefail

TEST_DIR=~/tmp/git-helpers-test

function main() {
	rm -rf "$TEST_DIR"
	mkdir -p "$TEST_DIR"

	create-upstream
	create-worktrees-repo

	echo ""
	echo "Done! Repos created in $TEST_DIR"
	echo ""
	echo "  cd $TEST_DIR/worktrees && gx"
}

# ── upstream ──────────────────────────────────────────────────────────────────

function create-upstream() {
	cd "$TEST_DIR"
	git init --bare upstream.git

	git clone upstream.git upstream-work
	cd upstream-work

	# ── main: project skeleton ────────────────────────────────────────────────
	mkdir -p src tests

	write-file README.md "# MyProject

A sample Go web service."
	write-file src/main.go "package main

func main() {
	startServer()
}"
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

	git push origin main

	# ── feature-auth: auth module (branches from here) ────────────────────────
	git checkout -b feature-auth
	write-file src/auth.go "package main

func login(user, pass string) bool { return false }
func logout(token string)          {}"
	commit "Add auth module"
	write-file tests/auth_test.go "package main

import \"testing\"

func TestLogin(t *testing.T)  {}
func TestLogout(t *testing.T) {}"
	commit "Add auth tests"
	git push origin feature-auth
	git tag feature-auth-v1 # pin this state for the worktree

	# ── feature-api: API handler (branches from main) ─────────────────────────
	git checkout main
	git checkout -b feature-api
	write-file src/api.go "package main

func handleRequest(path string) {}
func handleError(err error)     {}"
	commit "Add API handler"
	git push origin feature-api
	git tag feature-api-v1 # pin this state for the worktree

	# ── feature-ui: UI renderer (branches from main) ──────────────────────────
	git checkout main
	git checkout -b feature-ui
	write-file src/ui.go "package main

func render(template string) string { return \"\" }
func layout(content string) string  { return content }"
	commit "Add UI renderer"
	git push origin feature-ui

	# ── bugfix-login: starts from main ────────────────────────────────────────
	git checkout main
	git checkout -b bugfix-login
	git push origin bugfix-login

	# ── main: advance two commits (makes feature-auth + feature-api "behind") ─
	git checkout main
	write-file src/logger.go "package main

import \"log\"

func logInfo(msg string)  { log.Println(\"INFO:\", msg) }
func logError(msg string) { log.Println(\"ERROR:\", msg) }"
	commit "Add structured logging"
	write-file src/metrics.go "package main

func recordMetric(name string, value float64) {}
func flushMetrics()                           {}"
	commit "Add metrics collection"
	git push origin main

	# ── feature-api: push 2 more upstream commits (worktree will be "behind") ─
	git checkout feature-api
	write-file src/api_v2.go "package main

func handleRequestV2(path string, version int) {}"
	commit "Add v2 API handler"
	write-file src/middleware.go "package main

func withLogging(next func()) func()  { return next }
func withAuth(next func()) func()     { return next }"
	commit "Add request middleware"
	git push origin feature-api

	# ── feature-auth: push 1 more upstream commit (worktree will "diverge") ───
	git checkout feature-auth
	write-file src/oauth.go "package main

func oauthLogin(token string) bool  { return false }
func oauthLogout(token string)      {}"
	commit "Add OAuth login flow"
	git push origin feature-auth

	# ── bugfix-login: merge main, add the fix ────────────────────────────────
	git checkout bugfix-login
	git merge --no-ff main -m "Merge main into bugfix-login"
	write-file src/auth.go "package main

import \"time\"

const loginTimeout = 30 * time.Second

func login(user, pass string) bool {
	// fixed: respect timeout
	return false
}
func logout(token string) {}"
	commit "Fix login timeout (#42)"
	git push origin bugfix-login

	# ── refactor-db: starts from main, one commit ─────────────────────────────
	git checkout main
	git checkout -b refactor-db
	write-file src/db.go "package main

func openDB(dsn string) error  { return nil }
func closeDB()                 {}
func queryDB(sql string) error { return nil }"
	commit "Add database layer"
	git push origin refactor-db

	# ── chore-cleanup: exists on remote but worktree won't track it ───────────
	git checkout main
	git checkout -b chore-cleanup
	write-file src/cleanup.go "package main

func removeDeprecated() {}
func archiveLogs()      {}"
	commit "Start cleanup work"
	git push origin chore-cleanup

	git checkout main
	git push origin --tags # push all version tags to upstream
}

# ── worktrees repo ────────────────────────────────────────────────────────────

function create-worktrees-repo() {
	cd "$TEST_DIR"
	mkdir worktrees
	cd worktrees

	git init --bare
	git remote add origin "$TEST_DIR/upstream.git"
	git remote update       # fetches all branches into refs/remotes/origin/*
	git fetch origin --tags # fetch tags (not included in remote update by default)

	# main — synced
	git worktree add -b main main origin/main

	# feature-ui — will be 2 ahead after local commits
	git worktree add -b feature-ui feature-ui origin/feature-ui
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
	echo "package main

// TODO: bezier curve helpers" >curves.go
	cd ..

	# feature-auth — pinned to v1, 1 local commit ahead, 1 upstream commit
	# behind → diverged
	git worktree add -b feature-auth feature-auth feature-auth-v1
	cd feature-auth
	write-file src/session.go "package main

import \"time\"

type Session struct {
	Token     string
	ExpiresAt time.Time
}"
	commit "Add session management"
	# Unstaged modification
	printf "package main\n\n// BUG: needs constant-time comparison\nfunc login(user, pass string) bool { return false }\nfunc logout(token string)          {}\n" >src/auth.go
	cd ..

	# feature-api — pinned to v1 (2 commits behind origin/feature-api)
	git worktree add -b feature-api feature-api feature-api-v1

	# bugfix-login — synced
	git worktree add -b bugfix-login bugfix-login origin/bugfix-login

	# chore-cleanup — remote branch exists but local branch has no tracking set
	git branch --no-track chore-cleanup origin/chore-cleanup
	git worktree add chore-cleanup chore-cleanup

	# refactor-db — synced with origin, but has staged + untracked changes
	git worktree add -b refactor-db refactor-db origin/refactor-db
	cd refactor-db
	# Staged change (modified existing file)
	printf "package main\n\nimport \"database/sql\"\n\nfunc openDB(dsn string) (*sql.DB, error) { return sql.Open(\"sqlite3\", dsn) }\nfunc closeDB(db *sql.DB)                  { db.Close() }\nfunc queryDB(db *sql.DB, q string) error  { return nil }\n" >src/db.go
	git add src/db.go
	# Untracked new file
	printf "package main\n\n// TODO: connection pooling\nconst maxConns = 10\n" >src/db_pool.go
	cd ..
}

# ── helpers ───────────────────────────────────────────────────────────────────

function write-file() {
	local path="$1"
	local content="$2"
	mkdir -p "$(dirname "$path")"
	printf "%s\n" "$content" >"$path"
	git add "$path"
}

function commit() {
	git commit -m "$1"
}

main "$@"
