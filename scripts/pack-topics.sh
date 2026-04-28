#!/usr/bin/env bash
# scripts/pack-topics.sh — zip each topic directory for use with repository.Manager
#
# USAGE
#   ./scripts/pack-topics.sh [topics...]
#
#   Without arguments, packs ALL topic directories under testdata/curriculum/.
#   Pass topic names to pack a subset, e.g.:
#     ./scripts/pack-topics.sh basics
#
# OUTPUT
#   One .zip file per topic is written to testdata/packed/<topic>.zip.
#
# ZIP STRUCTURE
#   The zip entries are relative to the topic directory, e.g.:
#     01-hello-world/exercise.yaml
#     01-hello-world/main.go
#     02-addition/exercise.yaml
#     ...
#
#   This matches the layout expected by repository.Manager.Fetch which
#   extracts to ~/.thoth-go/cache/topics/<topic>/ — the exercise sub-dirs
#   land directly under that path.
#
# DESIGN NOTES
#   Why a shell script instead of a Go command?
#
#   A shell script requires zero dependencies and can be run without
#   building the project first. This makes it useful for bootstrapping the
#   local curriculum cache for end-to-end testing without a remote server.
#
#   For production use the exercise server serves the same zip files.
#   This script lets developers generate equivalent archives locally.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CURRICULUM_DIR="${REPO_ROOT}/testdata/curriculum"
OUTPUT_DIR="${REPO_ROOT}/testdata/packed"

mkdir -p "${OUTPUT_DIR}"

if [[ $# -eq 0 ]]; then
    # No args — discover all topic directories automatically.
    readarray -t topics < <(find "${CURRICULUM_DIR}" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort)
else
    topics=("$@")
fi

if [[ ${#topics[@]} -eq 0 ]]; then
    echo "No topic directories found under ${CURRICULUM_DIR}" >&2
    exit 1
fi

for topic in "${topics[@]}"; do
    topic_dir="${CURRICULUM_DIR}/${topic}"

    if [[ ! -d "${topic_dir}" ]]; then
        echo "ERROR: topic directory '${topic_dir}' not found" >&2
        exit 1
    fi

    output="${OUTPUT_DIR}/${topic}.zip"

    echo "Packing topic '${topic}' → ${output}"

    # cd into the topic directory so that zip entries are relative to it
    # (e.g. 01-hello-world/main.go rather than basics/01-hello-world/main.go).
    # This matches the path structure repository.Manager expects when it
    # extracts the archive into ~/.thoth-go/cache/topics/<topic>/.
    (
        cd "${topic_dir}"
        zip -r "${output}" . --exclude "*.DS_Store" --exclude "__pycache__/*"
    )

    echo "  → $(du -h "${output}" | cut -f1) written"
done

echo ""
echo "Done. Packed topics: ${topics[*]}"
echo "Use 'thoth-go fetch <topic>' or point THOTH_GO_BASE_URL at a local server"
echo "to ingest these archives into the cache."
