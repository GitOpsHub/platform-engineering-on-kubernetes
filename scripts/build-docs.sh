#!/usr/bin/env bash
# build-docs.sh - build or preview the documentation site.
#
# MkDocs needs its source directory to be a child of the config file, so this script
# stages the published content into site-src/ first. The relative layout is kept, which
# keeps cross-links such as ../../docs/... valid. Code under code/ is not published.
#
# Usage:
#   ./scripts/build-docs.sh            # strict build into site/ (fails on any broken link)
#   ./scripts/build-docs.sh serve      # live preview at http://127.0.0.1:8000
#   ./scripts/build-docs.sh --help
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="${ROOT}/site-src"

usage() {
  sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

case "${1:-build}" in
  -h|--help) usage ;;
  build)     MKDOCS_CMD=(build --strict) ;;
  serve)     MKDOCS_CMD=(serve) ;;
  *)         echo "unknown argument: $1 (see --help)" >&2; exit 1 ;;
esac

command -v uv >/dev/null 2>&1 || { echo "uv is required (https://docs.astral.sh/uv/)" >&2; exit 1; }

cd "${ROOT}"
rm -rf "${STAGE}"
mkdir -p "${STAGE}"

cp README.md START-HERE.md CONTRIBUTING.md "${STAGE}/"
rsync -a --exclude 'bin/' --exclude '*.out' --exclude '.terraform/' docs labs "${STAGE}/"

uv run --group docs mkdocs "${MKDOCS_CMD[@]}"
