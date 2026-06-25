#!/usr/bin/env bash

# run.sh — launch gx against the deterministic demo seed for VHS recording.
#
# Keeps the .tape files free of shell-specific quoting: VHS may drive bash or
# fish, and inline `VAR=val cmd` assignments are bash-only. This wrapper sets
# XDG_CONFIG_HOME + EDITOR and cd's into the right worktree, then execs gx.
#
# Usage: run.sh <worktree|.> [subcommand]
#   run.sh .            worktrees   # worktrees tab (run from the .bare root)
#   run.sh feature-auth status      # status tab for one worktree
#   run.sh main         log         # log tab for one worktree
#
# gx must be on PATH (the `demos` make target prepends the repo root).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK="$SCRIPT_DIR/.work"

worktree="${1:-.}"
sub="${2:-}"

# VHS records in a plain ttyd terminal, but it inherits the recording host's
# env. Clear the kitty/tmux markers so gx treats this as a plain terminal:
# editors (Ask AI, reword) then open inline where VHS can capture them, instead
# of spawning an off-screen kitty/tmux split. (The kitty image-diff feature is
# demoed separately via a real screen recording.)
unset KITTY_WINDOW_ID KITTY_LISTEN_ON KITTY_PID TMUX TMUX_PANE

export XDG_CONFIG_HOME="$WORK/xdg"
# A config-less nvim keeps the editor steps (Ask AI, reword) fast and identical
# across machines. Override with DEMO_EDITOR if you want your own editor.
export EDITOR="${DEMO_EDITOR:-nvim -u NONE -i NONE}"

cd "$WORK/worktrees/$worktree"
exec gx ${sub:+"$sub"}
