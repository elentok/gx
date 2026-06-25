#!/usr/bin/env bash

# image-diff.sh — capture the kitty inline image-diff demo.
#
# VHS renders in a headless browser terminal that cannot speak the kitty
# graphics protocol, so this one demo is a *real* screen recording of a kitty
# window. The script seeds the fixture, opens gx in a dedicated kitty window
# already focused on the changed image, and (optionally) records a fixed screen
# region to docs/demo-image-diff.gif.
#
# Requirements: kitty, ffmpeg, and macOS `screencapture` (for --record).
#
# Usage:
#   web/demo/image-diff.sh              # open the demo window; record it yourself
#   web/demo/image-diff.sh --record     # also screen-record (needs CAPTURE_REGION)
#
# Recording a region needs its pixel geometry. Find it once with the macOS
# screenshot tool (Cmd-Shift-4, drag, read the WxH; Cmd-Shift-5 shows X,Y), then:
#   CAPTURE_REGION="x,y,w,h" web/demo/image-diff.sh --record
# Screen Recording permission must be granted to the terminal/kitty.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORK="$SCRIPT_DIR/.work"
SOCKET="unix:/tmp/gx-image-diff-demo"
DURATION="${DURATION:-9}"
OUT="$REPO_ROOT/docs/demo-image-diff.gif"

record=false
[[ "${1:-}" == "--record" ]] && record=true

command -v kitty >/dev/null || { echo "kitty not found"; exit 1; }

echo "==> seeding fixture"
bash "$SCRIPT_DIR/seed.sh" "$WORK" >/dev/null

echo "==> building gx"
( cd "$REPO_ROOT" && make build >/dev/null )

# Launch a dedicated kitty window with remote control. gx opens straight onto the
# changed image (gx status <path>); image diffs are enabled by the demo config.
echo "==> opening demo window"
XDG_CONFIG_HOME="$WORK/xdg" \
kitty \
  --directory "$WORK/worktrees/feature-ui" \
  --override allow_remote_control=yes \
  --override font_family="Agave Nerd Font" \
  --override font_size=16 \
  --override background="#1e1e2e" \
  --override remember_window_size=no \
  --override initial_window_width=1200 \
  --override initial_window_height=720 \
  --listen-on "$SOCKET" \
  --title "gx image-diff demo" \
  "$REPO_ROOT/gx" status assets/banner.png &
kitty_pid=$!

# Give gx time to start, then open the image diff (and toggle to side-by-side).
sleep 3
kitty @ --to "$SOCKET" send-text "l" 2>/dev/null || true
sleep 1
kitty @ --to "$SOCKET" send-text "s" 2>/dev/null || true

if $record; then
  if [[ -z "${CAPTURE_REGION:-}" ]]; then
    echo "CAPTURE_REGION is not set (x,y,w,h) — see the usage notes." >&2
    kill "$kitty_pid" 2>/dev/null || true
    exit 1
  fi
  mov="$(mktemp -t gx-image-diff).mov"
  echo "==> recording ${DURATION}s of region $CAPTURE_REGION"
  screencapture -v -V "$DURATION" -R "$CAPTURE_REGION" "$mov"
  echo "==> converting to $OUT"
  # Palette pass keeps the kitty-rendered image colours clean in the GIF.
  palette="$(mktemp -t gx-palette).png"
  ffmpeg -y -i "$mov" -vf "fps=12,scale=1000:-1:flags=lanczos,palettegen" "$palette" >/dev/null 2>&1
  ffmpeg -y -i "$mov" -i "$palette" \
    -lavfi "fps=12,scale=1000:-1:flags=lanczos[x];[x][1:v]paletteuse" "$OUT" >/dev/null 2>&1
  rm -f "$mov" "$palette"
  kill "$kitty_pid" 2>/dev/null || true
  echo "==> wrote $OUT"
else
  echo ""
  echo "Demo window is open and showing the image diff."
  echo "Record it (Cmd-Shift-5 / QuickTime), then convert with:"
  echo "  ffmpeg -i recording.mov -vf 'fps=12,scale=1000:-1:flags=lanczos' $OUT"
  echo "Close the window when done."
  wait "$kitty_pid"
fi
