#!/usr/bin/env bash
# publish-help-site.sh — publish docs/help to the neca-help GitHub Pages repository.
#
# Default is dry-run. Use --yes to commit and push.
# The source tree must be committed first: the script publishes HEAD:docs/help,
# never uncommitted working-tree changes.
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "$0")/.." && pwd)"
TARGET="${NECA_HELP_REPO:-https://github.com/hyhdsl/neca-help.git}"
BRANCH="${NECA_HELP_BRANCH:-main}"
MODE="dry-run"

usage() {
  cat <<'EOF'
Usage: scripts/publish-help-site.sh [--dry-run|--yes]

  --dry-run   Show the staged diff only (default)
  --yes       Commit and push docs/help to the target repository

Environment:
  NECA_HELP_REPO     Target git URL (default: https://github.com/hyhdsl/neca-help.git)
  NECA_HELP_BRANCH   Target branch (default: main)
  NECA_SOURCE_COMMIT Commit label for the publish commit (default: current HEAD)
EOF
}

for arg in "$@"; do
  case "$arg" in
    --dry-run) MODE="dry-run" ;;
    --yes) MODE="apply" ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[ERR]  unknown argument: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

if ! git -C "$ROOT" diff --quiet -- docs/help \
  || ! git -C "$ROOT" diff --cached --quiet -- docs/help; then
  echo "[ERR]  docs/help has uncommitted changes; commit them first" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'cd "$ROOT" && rm -rf "$tmp"' EXIT

echo "[INFO] target: $TARGET ($BRANCH)"
git clone --quiet --branch "$BRANCH" "$TARGET" "$tmp/help"

# Mirror HEAD:docs/help into the target worktree. Remove tracked target files
# from inside the target checkout so paths cannot accidentally resolve to the
# source repository.
cd "$tmp/help"
git ls-files -z | xargs -0 rm -f
cd "$ROOT"
git archive --format=tar HEAD:docs/help | tar -x -C "$tmp/help"

cd "$tmp/help"
git add -A
if git diff --cached --quiet; then
  echo "[OK]   help site is up to date; nothing to publish"
  exit 0
fi

echo "[INFO] staged changes:"
git diff --cached --stat

if [ "$MODE" != "apply" ]; then
  echo "[INFO] dry-run only; rerun with --yes to publish"
  exit 0
fi

SOURCE_COMMIT="${NECA_SOURCE_COMMIT:-}"
if [ -z "$SOURCE_COMMIT" ]; then SOURCE_COMMIT="$(git -C "$ROOT" rev-parse --short HEAD)"; fi
git commit -m "docs: publish help site from $SOURCE_COMMIT"
git push origin "$BRANCH"
echo "[OK]   published to $TARGET"
echo "[INFO] site: https://neca.nekobiglazyer.work/"
