#!/usr/bin/env bash
set -euo pipefail

usage() {
	echo "Usage: scripts/bump.sh <major|minor|patch>"
	exit 1
}

[[ $# -ne 1 ]] && usage

BUMP="$1"
[[ "$BUMP" != "major" && "$BUMP" != "minor" && "$BUMP" != "patch" ]] && usage

# Get the latest tag (strip leading 'v')
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
VERSION="${LAST_TAG#v}"

IFS='.' read -r MAJOR MINOR PATCH <<<"$VERSION"

case "$BUMP" in
major)
	MAJOR=$((MAJOR + 1))
	MINOR=0
	PATCH=0
	;;
minor)
	MINOR=$((MINOR + 1))
	PATCH=0
	;;
patch) PATCH=$((PATCH + 1)) ;;
esac

NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"

echo "Bumping $LAST_TAG → $NEW_TAG"

git tag -a "$NEW_TAG" -m "Release $NEW_TAG"

echo "Created annotated tag $NEW_TAG"
echo ""
read -rp "Push commits and tag to origin? [y/N] " CONFIRM
if [[ "$CONFIRM" =~ ^[Yy]$ ]]; then
	git push origin
	git push origin "$NEW_TAG"
	echo "Pushed."
else
	echo "Skipped. To push manually:"
	echo "  git push origin"
	echo "  git push origin $NEW_TAG"
fi
